package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// Synthetic setup uses the supported storage API and fully readable saved
// Runs. The journey uses the CLI, optionally a separately built executable.
// No SQL, force removal or changes to an operator authority are involved.
func TestCLIClaimOwnershipAccumulatedAuthority(t *testing.T) {
	t.Parallel()
	root, authority := singleLaunchFixture(t)
	schemaPath := filepath.Join(root, ".prifly/workflows/single/schemas/object.yaml")
	schema, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(schemaPath, append(schema, []byte("description: "+strings.Repeat("p", 1<<20)+"\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	gitFixture(t, root, "add", ".")
	gitFixture(t, root, "commit", "-qm", "large valid input schema")
	sourceID := startSingle(t, root, authority)
	finishSingle(t, authority, sourceID)
	engine, err := prifly.Open(authority, false)
	if err != nil {
		t.Fatal(err)
	}
	view, err := engine.Store.Read(context.Background(), sourceID, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	var run prifly.Run
	if err := json.Unmarshal(view.Snapshot.Data, &run); err != nil {
		t.Fatal(err)
	}
	original := sha256.Sum256(view.Snapshot.Data)
	before := map[string][32]byte{sourceID: original}
	for i := range 65 {
		run.ID = fmt.Sprintf("run:journey-%04d", i)
		for _, inv := range run.Invocations {
			inv.RunID = run.ID
		}
		raw, err := json.Marshal(run)
		if err != nil {
			t.Fatal(err)
		}
		data, err := flow.Canonical(raw)
		if err != nil {
			t.Fatal(err)
		}
		zero := int64(0)
		result, err := engine.Store.Apply(context.Background(), local.Command{ID: fmt.Sprintf("command:journey-%04d", i), Actor: "fixture:population", RunID: run.ID, Payload: json.RawMessage(`{}`), Mode: local.CommandCAS, ExpectedVersion: &zero}, func(local.Snapshot) (local.Change, error) {
			return local.Change{Data: data, Events: []local.EventInput{{Type: "run.created", Data: json.RawMessage(`{}`)}}}, nil
		})
		if err != nil || result.Receipt.Rejection != nil {
			t.Fatalf("seed: %v %+v", err, result.Receipt.Rejection)
		}
		before[run.ID] = sha256.Sum256(data)
	}
	if _, _, err := engine.Store.ReadAll(context.Background(), 1000); err == nil {
		t.Fatal("fixture did not exceed bounded restored bytes")
	}
	engine.Close()
	cliCall := func(args ...string) (int, string, string) {
		if binary := os.Getenv("PRIFLY_CLAIM_FIXTURE_BINARY"); binary != "" && args[0] != "project" {
			commandArgs := append(append([]string{"--project", authority}, args...), "--json")
			command := exec.Command(binary, commandArgs...)
			measurement := ""
			if runtime.GOOS == "linux" {
				if _, err := os.Stat("/usr/bin/time"); err == nil {
					measurement = filepath.Join(t.TempDir(), "peak-rss-kib")
					measuredArgs := append([]string{"-o", measurement, "-f", "%M", "--", binary}, commandArgs...)
					command = exec.Command("/usr/bin/time", measuredArgs...)
				}
			}
			command.Env = append(os.Environ(), "XDG_CONFIG_HOME="+t.TempDir())
			var out, stderr bytes.Buffer
			command.Stdout = &out
			command.Stderr = &stderr
			err := command.Run()
			if measurement != "" {
				data, err := os.ReadFile(measurement)
				if err != nil {
					t.Fatal(err)
				}
				t.Logf("binary command=%s peak_rss_kib=%s", args[0], strings.TrimSpace(string(data)))
			} else if command.ProcessState != nil {
				if usage, ok := command.ProcessState.SysUsage().(*syscall.Rusage); ok {
					t.Logf("binary command=%s max_rss_platform_units=%d", args[0], usage.Maxrss)
				}
			}
			code := 0
			if err != nil {
				var exit *exec.ExitError
				if !errors.As(err, &exit) {
					t.Fatal(err)
				}
				code = exit.ExitCode()
			}
			return code, out.String(), stderr.String()
		}
		return runCLI(t, append([]string{"--project", authority}, args...)...)
	}
	code, out, stderr := cliCall("claim", "create", "--repository", root, "--owner", "session:synthetic")
	var claimed struct {
		Claim prifly.WorktreeClaim `json:"claim"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &claimed) != nil {
		t.Fatalf("create: %d %s", code, stderr)
	}
	if baseline := os.Getenv("PRIFLY_CLAIM_BASELINE_BINARY"); baseline != "" {
		cmd := exec.Command(baseline, "--project", authority, "claim", "release", "--id", claimed.Claim.ID, "--generation", fmt.Sprint(claimed.Claim.Generation), "--json")
		output, err := cmd.CombinedOutput()
		if err == nil || !bytes.Contains(output, []byte(`"code":"invalid_input"`)) {
			t.Fatalf("baseline did not reproduce scan failure: %v %s", err, output)
		}
		t.Logf("baseline valid public release refused: %s", output)
	}
	var memBefore, memAfter runtime.MemStats
	runtime.ReadMemStats(&memBefore)
	started := time.Now()
	code, out, stderr = cliCall("doctor", "--claim", claimed.Claim.ID)
	duration := time.Since(started)
	runtime.ReadMemStats(&memAfter)
	var diagnostic prifly.ClaimOwnershipDiagnostics
	if code != 0 || json.Unmarshal([]byte(out), &diagnostic) != nil || !diagnostic.Scan.Complete || diagnostic.Scan.RestoredBytes <= 64<<20 || diagnostic.Scan.Records != 66 || diagnostic.Storage == nil {
		t.Fatalf("doctor: %d %s %s", code, out, stderr)
	}
	t.Logf("scan duration=%s restored_bytes=%d records=%d parent_total_alloc_delta=%d (subprocess memory measured externally when binary selected)", duration, diagnostic.Scan.RestoredBytes, diagnostic.Scan.Records, memAfter.TotalAlloc-memBefore.TotalAlloc)
	code, _, stderr = cliCall("claim", "release", "--id", claimed.Claim.ID, "--generation", fmt.Sprint(claimed.Claim.Generation+1))
	if code == 0 || !strings.Contains(stderr, "claim_generation_conflict") {
		t.Fatalf("stale generation release accepted: %d %s", code, stderr)
	}
	code, _, stderr = cliCall("claim", "release", "--id", claimed.Claim.ID, "--generation", fmt.Sprint(claimed.Claim.Generation))
	if code != 0 {
		t.Fatalf("release: %d %s", code, stderr)
	}
	// A new workspace-binding Run must also get past the old aggregate ceiling.
	stepPath := filepath.Join(root, ".prifly/workflows/single/steps/work.yaml")
	step, err := os.ReadFile(stepPath)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/workflows/single/steps/work.yaml", strings.Replace(string(step), "class: none", "class: workspace_write", 1))
	gitFixture(t, root, "add", ".")
	gitFixture(t, root, "commit", "-qm", "workspace binding")
	code, out, stderr = cliCall("project", "start", "--repository", root, "--launch", "single", "--host", "codex-cli", "--workspace", "worktree", "--input", "task="+filepath.Join(root, "task.json"))
	if code != 0 {
		t.Fatalf("binding after population: %d %s", code, stderr)
	}
	var admitted projectStartResult
	if err := json.Unmarshal([]byte(out), &admitted); err != nil {
		t.Fatal(err)
	}
	if admitted.Workspace == nil || admitted.Workspace.RunID != admitted.RunID {
		t.Fatal("start did not bind its claim")
	}
	engine, err = prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	for id, digest := range before {
		current, err := engine.Store.Read(context.Background(), id, 0, 1)
		if err != nil || sha256.Sum256(current.Snapshot.Data) != digest {
			t.Fatalf("history changed: %s %v", id, err)
		}
	}
	code, out, stderr = cliCall("doctor", "--claim", admitted.Workspace.ID)
	if code == 0 || !strings.Contains(out, `"assessment":"blocked"`) || !strings.Contains(stderr, "claim_owner_unproven") {
		t.Fatalf("active holder accepted: %d %s %s", code, out, stderr)
	}
	// A legacy package identity conflict happens after the claim is taken.
	// It exercises the public failed-start response on this same large authority.
	finishSingle(t, authority, admitted.RunID)
	profilePath := filepath.Join(root, ".prifly/project.yaml")
	profile, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/project.yaml", strings.Replace(string(profile), "prifly-project-profile/3", "prifly-project-profile/2", 1))
	writeFixtureFile(t, root, "brief.json", `{"schema_version":"1","id":"test:brief/cleanup","subject":"Synthetic cleanup","desired_outcome":"Reach the handoff","in_scope":["handoff"],"out_of_scope":[],"completion_criteria":["handoff"],"source_refs":[],"assumptions":[],"confirmation":"explicit"}`)
	gitFixture(t, root, "add", ".")
	gitFixture(t, root, "commit", "-qm", "legacy cleanup scenario")
	legacyArgs := []string{"project", "start", "--repository", root, "--launch", "single", "--host", "codex-cli", "--workspace", "worktree", "--brief", filepath.Join(root, "brief.json"), "--input", "task=" + filepath.Join(root, "task.json")}
	code, out, stderr = cliCall(legacyArgs...)
	if code != 0 {
		t.Fatalf("legacy setup: %d %s", code, stderr)
	}
	var legacy projectStartResult
	if err := json.Unmarshal([]byte(out), &legacy); err != nil {
		t.Fatal(err)
	}
	contextPath := filepath.Join(root, ".prifly/workflows/single/contexts/work.yaml")
	contextBytes, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/workflows/single/contexts/work.yaml", strings.Replace(string(contextBytes), "Fixture work.", "Different fixture work.", 1))
	gitFixture(t, root, "add", ".")
	gitFixture(t, root, "commit", "-qm", "different bytes under legacy identity")
	code, out, stderr = cliCall(append(legacyArgs, "--command-id", "command:legacy-cleanup")...)
	if code == 0 || out != "" || !strings.Contains(stderr, "released claim") {
		t.Fatalf("failed start did not report rollback: %d stdout=%s stderr=%s", code, out, stderr)
	}
	var problem prifly.Problem
	if err := json.Unmarshal([]byte(stderr), &problem); err != nil {
		t.Fatal(err)
	}
	if problem.Code != "project_start_package_identity_conflict" {
		t.Fatalf("wrong original refusal: %+v", problem)
	}
	t.Logf("public failed-start cleanup: %s", stderr)

}

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func TestCLISchemaMigrationPublicPrepareApplyInspectAndExplicitStart(t *testing.T) {
	t.Parallel()
	project := t.TempDir()
	invoke := func(want int, args ...string) []byte {
		t.Helper()
		var out, errout bytes.Buffer
		argv := append([]string{"--project", project}, args...)
		argv = append(argv, "--json")
		code := execute(context.Background(), argv, &out, &errout)
		if code != want {
			t.Fatalf("%v: exit %d, want %d: %s", args, code, want, errout.String())
		}
		if want != 0 {
			return errout.Bytes()
		}
		return out.Bytes()
	}
	invoke(0, "init", "--profile", flow.CoreProfile, project)
	writeJSON := func(path string, value any) []byte {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return data
	}
	digest := func(data []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(data)) }
	files := map[string][]byte{"a.json": []byte(`{"type":"object"}`), "b.json": []byte(`{"additionalProperties":true,"type":"object"}`)}
	schemas := []flow.Ref{{ID: "test:schema/cli-a", Version: "1.0.0", Digest: digest(files["a.json"])}, {ID: "test:schema/cli-b", Version: "1.0.0", Digest: digest(files["b.json"])}}
	components := []map[string]any{{"kind": "schema", "ref": schemas[0], "path": "a.json"}, {"kind": "schema", "ref": schemas[1], "path": "b.json"}}
	importPackage := func(id string, payload map[string][]byte, items []map[string]any, deps []flow.Ref) flow.Ref {
		t.Helper()
		source := t.TempDir()
		declared := []map[string]any{}
		for name, data := range payload {
			if err := os.WriteFile(filepath.Join(source, name), data, 0600); err != nil {
				t.Fatal(err)
			}
			declared = append(declared, map[string]any{"path": name, "digest": digest(data), "size_bytes": len(data), "media_type": "application/json", "role": "data"})
		}
		manifest := writeJSON(filepath.Join(source, prifly.PackageManifestFile), map[string]any{"schema_version": "1", "id": id, "version": "1.0.0", "description": "Public CLI migration fixture", "requires_core_protocol": "1", "dependencies": deps, "components": items, "files": declared, "requested_capabilities": []string{}, "license": "MIT"})
		invoke(0, "package", "import", "--dir", source, "--reason", "reviewed fixture", "--command-id", "command:import-"+id)
		return flow.Ref{ID: id, Version: "1.0.0", Digest: digest(manifest)}
	}
	legacy := importPackage("test:package/cli-legacy", files, components, []flow.Ref{})
	workflow := emptyCLIWorkflow(t)
	workflow.Inputs = map[string]flow.InputPort{"a": {Port: flow.Port{Format: "json", SchemaRef: &schemas[0]}, Required: true}, "b": {Port: flow.Port{Format: "json", SchemaRef: &schemas[1]}, Required: true}}
	workflowBytes := writeJSON(filepath.Join(project, "workflow.json"), workflow)
	writeJSON(filepath.Join(project, "value.json"), map[string]any{})
	writeJSON(filepath.Join(project, "brief.json"), prifly.Brief{SchemaVersion: "1", ID: "test:brief/migration", Subject: "Shared schemas", DesiredOutcome: "No work", InScope: []string{"Read shared schema inputs"}, OutOfScope: []string{}, CompletionCriteria: []string{"Read inputs"}, SourceRefs: []prifly.ArtifactRef{}, Assumptions: []string{}, Confirmation: "explicit"})
	start := []string{"run", "start", "--workflow", "workflow.json", "--brief", "brief.json", "--input", "a=value.json", "--input", "b=value.json"}
	invoke(0, append(append([]string{}, start...), "--command-id", "command:old")...)
	invoke(0, "package", "revoke", "--id", legacy.ID, "--version", legacy.Version, "--reason", "mistaken upgrade retirement")
	supplier := importPackage("test:package/cli-supplier", files, components, []flow.Ref{})
	wfDigest, err := flow.Digest(workflowBytes)
	if err != nil {
		t.Fatal(err)
	}
	wfRef := flow.Ref{ID: workflow.ID, Version: workflow.Version, Digest: wfDigest}
	consumer := importPackage("test:package/cli-consumer", map[string][]byte{"workflow.json": workflowBytes}, []map[string]any{{"kind": "workflow", "ref": wfRef, "path": "workflow.json"}}, []flow.Ref{supplier})
	problem := invoke(3, append(append([]string{}, start...), "--command-id", "command:blocked")...)
	if !bytes.Contains(problem, []byte(`"code":"package_not_resolvable"`)) {
		t.Fatalf("original refusal: %s", problem)
	}
	request := prifly.SchemaMigrationRequest{SchemaVersion: "schema-migration-request/1", ConsumerRef: consumer, Selections: []prifly.SchemaMigrationSelection{}}
	requestPath := filepath.Join(t.TempDir(), "request.json")
	writeJSON(requestPath, request)
	var plan prifly.SchemaMigrationPlan
	if err := json.Unmarshal(invoke(0, "package", "schema-migration", "prepare", "--request", requestPath), &plan); err != nil {
		t.Fatal(err)
	}
	if plan.Applicable || len(plan.Schemas) != 2 || len(plan.Runs) != 1 {
		t.Fatalf("public diagnostic: %+v", plan)
	}
	for _, ref := range schemas {
		request.Selections = append(request.Selections, prifly.SchemaMigrationSelection{SchemaRef: ref, SupplierRef: supplier})
	}
	writeJSON(requestPath, request)
	if err := json.Unmarshal(invoke(0, "package", "schema-migration", "prepare", "--request", requestPath), &plan); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(t.TempDir(), "plan.json")
	writeJSON(planPath, plan)
	var result prifly.SchemaMigrationResult
	if err := json.Unmarshal(invoke(0, "package", "schema-migration", "apply", "--plan", planPath, "--expected-review-digest", plan.ReviewDigest, "--reason", "owner verified shared schemas", "--command-id", "command:cli-migrate"), &result); err != nil {
		t.Fatal(err)
	}
	var inspection prifly.SchemaMigrationInspection
	if err := json.Unmarshal(invoke(0, "package", "schema-migration", "inspect", "--id", result.Decision.ID), &inspection); err != nil || !inspection.Valid {
		t.Fatal("public inspect", err, inspection)
	}
	response := invoke(0, append(append([]string{}, start...), "--command-id", "command:migrated", "--schema-migration", result.Decision.ID, "--drive")...)
	if !bytes.Contains(response, []byte("core-read/45")) {
		t.Fatalf("public migration boundary missing: %s", response)
	}
	invoke(3, append(append([]string{}, start...), "--command-id", "command:still-blocked")...)
	// A lost-response retry reads its saved receipt before current payload bytes.
	engine, err := prifly.Open(project, true)
	if err != nil {
		t.Fatal(err)
	}
	packages, err := engine.Packages(context.Background())
	engine.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range packages.Packages {
		if pkg.Ref == supplier {
			path := filepath.Join(project, pkg.Components[0].Path)
			if err := os.Chmod(path, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(`{"type":"null"}`), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	repeated := invoke(0, "package", "schema-migration", "apply", "--plan", planPath, "--expected-review-digest", plan.ReviewDigest, "--reason", "owner verified shared schemas", "--command-id", "command:cli-migrate")
	var replay prifly.SchemaMigrationResult
	if err := json.Unmarshal(repeated, &replay); err != nil || !replay.Duplicate || replay.Receipt.Cut != result.Receipt.Cut {
		t.Fatal("public replay read changed bytes before receipt", err)
	}

}

func TestCLIProjectSchemaMigrationIsExplicitReviewedAndSerializedAtNine(t *testing.T) {
	t.Parallel()
	for _, naming := range []bool{false, true} {
		t.Run(fmt.Sprint("owned-naming=", naming), func(t *testing.T) {
			t.Parallel()
			var root, authority string
			packageName := "single"
			var launchArgs []string
			if naming {
				root, authority, launchArgs, _ = ownedNamingFixture(t)
				packageName = "inspect"
			} else {
				root, authority = singleLaunchFixture(t)
				launchArgs = []string{"--repository", root, "--launch", "single", "--host", "codex-cli", "--input", "task=" + filepath.Join(root, "task.json")}
			}
			output := filepath.Join(t.TempDir(), "compiled")
			raw := queueCLICommand(t, "--project", authority, "project", "compile", "--repository", root, "--package", packageName, "--host", "codex-cli", "--output", output)
			var compiled projectCompileResult
			if err := json.Unmarshal([]byte(raw), &compiled); err != nil {
				t.Fatal(err)
			}
			source := t.TempDir()
			files := []map[string]any{}
			components := []map[string]any{}
			schemas := []flow.Ref{}
			for _, component := range compiled.Components {
				if component.Kind == "schema" {
					data, err := os.ReadFile(filepath.Join(output, component.Path))
					if err != nil {
						t.Fatal(err)
					}
					writeFixtureFile(t, source, component.Path, string(data))
					files = append(files, map[string]any{"path": component.Path, "digest": fmt.Sprintf("sha256:%x", sha256.Sum256(data)), "size_bytes": len(data), "media_type": "application/json", "role": "data"})
					components = append(components, map[string]any{"kind": "schema", "ref": component.Ref, "path": component.Path})
					schemas = append(schemas, component.Ref)
				}
			}
			if len(schemas) == 0 {
				t.Fatal("Project fixture has no shared schema")
			}
			manifest, err := json.Marshal(map[string]any{"schema_version": "1", "id": "test:package/project-legacy-schemas", "version": "1.0.0", "description": "Legacy Project schemas", "requires_core_protocol": "1", "dependencies": []flow.Ref{}, "components": components, "files": files, "requested_capabilities": []string{}, "license": "MIT"})
			if err != nil {
				t.Fatal(err)
			}
			writeFixtureFile(t, source, prifly.PackageManifestFile, string(manifest))
			queueCLICommand(t, "--project", authority, "package", "import", "--dir", source, "--reason", "reviewed legacy schemas")
			queueCLICommand(t, "--project", authority, "package", "revoke", "--id", "test:package/project-legacy-schemas", "--version", "1.0.0", "--reason", "mistaken retirement")
			queueCLICommand(t, "--project", authority, "package", "import", "--dir", output, "--reason", "reviewed exact consumer")
			engine, err := prifly.Open(authority, false)
			if err != nil {
				t.Fatal(err)
			}
			request := prifly.SchemaMigrationRequest{SchemaVersion: "schema-migration-request/1", ConsumerRef: compiled.Package, Selections: []prifly.SchemaMigrationSelection{}}
			for _, schema := range schemas {
				request.Selections = append(request.Selections, prifly.SchemaMigrationSelection{SchemaRef: schema, SupplierRef: compiled.Package})
			}
			decisions := []string{}
			for _, command := range []string{"command:project-migration-one", "command:project-migration-two"} {
				plan, err := engine.PrepareSchemaMigration(context.Background(), request)
				if err != nil {
					t.Fatal(err)
				}
				result, err := engine.ApplySchemaMigration(context.Background(), prifly.SchemaMigrationApplyRequest{CommandID: command, Plan: plan, ExpectedReviewDigest: plan.ReviewDigest, Reason: "verified exact shared schema"})
				if err != nil || result.Receipt.Rejection != nil {
					t.Fatal(err, result.Receipt.Rejection)
				}
				decisions = append(decisions, result.Decision.ID)
			}
			engine.Close()
			base := append([]string{"--project", authority, "project", "questionnaire", "--prepare"}, launchArgs...)
			base = append(base, "--schema-migration", decisions[0])
			review := queueCLICommand(t, base...)
			var summary projectLaunchSummary
			if err := json.Unmarshal([]byte(review), &summary); err != nil {
				t.Fatal(err)
			}
			if summary.SchemaVersion != "project-launch-summary/9" || summary.SchemaMigrationID != decisions[0] {
				t.Fatalf("selected decision missing from review: %s", review)
			}
			validateQueueCLI(t, "ProjectLaunchSummaryV9", review)
			start := append([]string{"--project", authority, "project", "start"}, launchArgs...)
			start = append(start, "--expected-launch-digest", summary.ReviewDigest)
			code, _, stderr := runCLI(t, append(append([]string{}, start...), "--schema-migration", decisions[1])...)
			if code == 0 || !strings.Contains(stderr, "project_start_stale_launch") {
				t.Fatalf("changed decision escaped review digest: %d %s", code, stderr)
			}
			response := queueCLICommand(t, append(start, "--schema-migration", decisions[0])...)
			var started projectStartResult
			if err := json.Unmarshal([]byte(response), &started); err != nil {
				t.Fatal(err)
			}
			if started.SchemaVersion != "project-start/9" || started.SchemaMigrationID != decisions[0] || started.Run.Run.SchemaMigration == nil {
				t.Fatalf("Project Start lost selected decision: %s", response)
			}
			validateQueueCLI(t, "ProjectStartV9", response)
			if naming && (started.Workspace == nil || started.Workspace.NamingSnapshot == nil || started.LaunchSummary.WorktreeNaming == nil) {
				t.Fatal("migration lost owned naming snapshot")
			}

		})
	}
}

func TestCLIProjectSchemaMigrationRejectsLegacyProfileBeforeEffects(t *testing.T) {
	t.Parallel()
	root, authority := singleLaunchFixture(t)
	profilePath := filepath.Join(root, ".prifly/project.yaml")
	profile, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, ".prifly/project.yaml", strings.Replace(string(profile), "prifly-project-profile/3", "prifly-project-profile/2", 1))
	e, err := prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	_, cut, err := e.Store.ReadAll(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := e.Claims(context.Background())
	e.Close()
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, root, "brief.json", `{"schema_version":"1","id":"test:brief/legacy-migration","subject":"Legacy migration refusal","desired_outcome":"Refuse before launch","in_scope":["profile boundary"],"out_of_scope":[],"completion_criteria":["no authority changes"],"source_refs":[],"assumptions":[],"confirmation":"explicit"}`)
	code, out, stderr := runCLI(t, "--project", authority, "project", "start", "--repository", root, "--launch", "single", "--host", "codex-cli", "--brief", filepath.Join(root, "brief.json"), "--input", "task="+filepath.Join(root, "task.json"), "--schema-migration", "schema-migration:explicit-selection")
	var problem prifly.Problem
	if code != 2 || out != "" || json.Unmarshal([]byte(stderr), &problem) != nil || problem.Code != "project_questionnaire_prepare_requires_profile_3" {
		t.Fatalf("legacy migration did not refuse before launch: exit=%d out=%s stderr=%s", code, out, stderr)
	}
	e, err = prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	_, afterCut, err := e.Store.ReadAll(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	afterClaims, err := e.Claims(context.Background())
	claimsJSON, _ := json.Marshal(claims)
	afterClaimsJSON, _ := json.Marshal(afterClaims)
	if err != nil || cut != afterCut || !bytes.Equal(claimsJSON, afterClaimsJSON) {
		t.Fatal("legacy refusal changed authority or claims", err)
	}
	t.Log("[FIX:21] legacy profile refusal leaves authority cut and claims unchanged")
}

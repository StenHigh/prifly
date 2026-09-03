package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func TestLocalPolicyVersionsPreserveLegacyBytes(t *testing.T) {
	defs, registry, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	legacy := builtinRef(defs, "core:policy/local")
	want := flow.Ref{ID: "core:policy/local", Version: "1.0.0", Digest: "sha256:95b274a39a66d0da2b94c0a8233cae74b41a030eb0f67ee3d8b181cf617bcb4c"}
	if legacy != want || rawDigest(registry[legacy]) != want.Digest {
		t.Fatalf("historical policy bytes changed: %+v", legacy)
	}
	current := builtinVersionRef(defs, legacy.ID, "2.0.0")
	var document map[string]any
	if err := json.Unmarshal(registry[legacy], &document); err != nil {
		t.Fatal(err)
	}
	if document["limits"].(map[string]any)["max_child_depth"] != float64(0) {
		t.Fatal("legacy policy granted child execution")
	}
	document["version"] = "2.0.0"
	document["limits"].(map[string]any)["max_child_depth"] = 8
	wantCurrent, err := canonical(document)
	if err != nil || !bytes.Equal(wantCurrent, registry[current]) {
		t.Fatalf("policy2 changed more than its version and child depth: %v", err)
	}
	slices.Reverse(defs)
	if builtinRef(defs, legacy.ID) != legacy {
		t.Fatal("inventory ordering upgraded an implicit legacy reference")
	}
}

func TestLocalPolicyInitAndOpenSelectExactVersion(t *testing.T) {
	defs, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	legacy := builtinRef(defs, "core:policy/local")
	current := builtinVersionRef(defs, legacy.ID, "2.0.0")
	for _, profile := range []string{flow.Profile, flow.CoreProfile} {
		t.Run(profile, func(t *testing.T) {
			root := t.TempDir()
			if err := InitProfile(root, profile); err != nil {
				t.Fatal(err)
			}
			e, err := Open(root, false)
			if err != nil {
				t.Fatal(err)
			}
			config := e.Config
			want := legacy
			if profile == flow.CoreProfile {
				want = current
			}
			if config.DefaultPolicyRef != want {
				t.Fatalf("init policy: got %+v, want %+v", config.DefaultPolicyRef, want)
			}
			if err := e.Close(); err != nil {
				t.Fatal(err)
			}
			for _, tc := range []struct {
				name string
				ref  flow.Ref
				ok   bool
			}{
				{"legacy", legacy, true},
				{"current", current, profile == flow.CoreProfile},
				{"unknown_version", flow.Ref{ID: legacy.ID, Version: "3.0.0", Digest: legacy.Digest}, false},
				{"wrong_digest", flow.Ref{ID: legacy.ID, Version: current.Version, Digest: legacy.Digest}, false},
			} {
				t.Run(tc.name, func(t *testing.T) {
					config.DefaultPolicyRef = tc.ref
					writeRuntimeJSON(t, filepath.Join(root, "prifly.json"), config)
					opened, err := Open(root, true)
					if opened != nil {
						defer opened.Close()
					}
					if tc.ok && err != nil || !tc.ok && (err == nil || !strings.Contains(err.Error(), "unsupported_configuration")) {
						t.Fatalf("policy selection accepted=%v, error=%v", tc.ok, err)
					}
				})
			}
		})
	}
}

func TestCallPolicyRefusalBeforeAdmissionAndExplicitUpgrade(t *testing.T) {
	for _, version := range []string{"1.0.0", "2.0.0", "custom"} {
		t.Run(version, func(t *testing.T) {
			e, workflow, _, options := callFixture(t, "", "no_work", false)
			defs, registry, err := Builtins()
			if err != nil {
				t.Fatal(err)
			}
			policy := builtinVersionRef(defs, "core:policy/local", version)
			code := "resource_limit"
			if version == "custom" {
				// A valid, sealed policy is not automatically a qualified policy.
				var document map[string]any
				if err := json.Unmarshal(registry[builtinVersionRef(defs, "core:policy/local", "2.0.0")], &document); err != nil {
					t.Fatal(err)
				}
				document["id"], document["version"] = "test:policy/unqualified", "1.0.0"
				data, err := canonical(document)
				if err != nil {
					t.Fatal(err)
				}
				policy = flow.Ref{ID: document["id"].(string), Version: "1.0.0", Digest: rawDigest(data)}
				if err := os.WriteFile(filepath.Join(e.Root, "policy.json"), data, 0600); err != nil {
					t.Fatal(err)
				}
				registryData, err := os.ReadFile(filepath.Join(e.Root, "definitions.json"))
				var file RegistryFile
				if err != nil || json.Unmarshal(registryData, &file) != nil {
					t.Fatal("read registry", err)
				}
				file.Entries = append(file.Entries, Definition{Ref: policy, Kind: "policy", Path: "policy.json"})
				writeRuntimeJSON(t, filepath.Join(e.Root, "definitions.json"), file)
				code = "unsupported_policy"
			}
			workflow["policy_ref"] = policy
			// The project may keep its legacy default. Only the workflow's exact
			// selected policy authorizes its requested child depth.
			e.Config.DefaultPolicyRef = builtinRef(defs, "core:policy/local")
			writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
			writeRuntimeJSON(t, filepath.Join(e.Root, options.WorkflowFile), workflow)
			ctx := context.Background()
			_, before, err := e.Store.ReadAll(ctx, 100)
			if err != nil {
				t.Fatal(err)
			}
			_, previewErr := e.Preview(PreviewOptions{WorkflowFile: options.WorkflowFile, BriefFile: options.BriefFile})
			started, startErr := e.Start(ctx, options)
			if version != "2.0.0" {
				for _, err := range []error{previewErr, startErr} {
					if err == nil || !strings.Contains(err.Error(), code) {
						t.Fatalf("expected %s before admission, got %v", code, err)
					}
				}
				runs, cut, err := e.Store.ReadAll(ctx, 100)
				if err != nil || len(runs) != 0 || cut != before {
					t.Fatalf("policy refusal changed authority: runs=%d cut=%d error=%v", len(runs), cut, err)
				}
				files, err := os.ReadDir(filepath.Join(e.Root, e.Config.Configuration.WorkspaceRoot))
				if err != nil || len(files) != 0 {
					t.Fatalf("policy refusal allocated a workspace: %v %v", files, err)
				}
				return
			}
			if previewErr != nil || startErr != nil {
				t.Fatalf("explicit policy2 rejected: preview=%v start=%v", previewErr, startErr)
			}
			if err := e.Drive(ctx, started.Receipt.RunID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, e, started.Receipt.RunID)
			if r.Status != "completed" || len(r.Invocations) != 2 || r.ControlTransitions != 4 || len(r.Attempts) != 0 {
				t.Fatalf("policy2 call was not executed: status=%s invocations=%d controls=%d attempts=%d", r.Status, len(r.Invocations), r.ControlTransitions, len(r.Attempts))
			}
			found := false
			for _, definition := range r.Definitions {
				found = found || definition.Ref == policy
			}
			if !found {
				t.Fatal("selected policy2 was absent from the saved exact closure")
			}
		})
	}
}

func TestLocalPolicyEnforcesEveryDeclaredLimit(t *testing.T) {
	defs, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	limits := flow.Limits{MaxStepInstances: 256, MaxControlTransitions: 1024, MaxParallelism: 1, MaxChildDepth: 8}
	plan := &flow.Plan{Profile: flow.CoreProfile, Workflow: flow.WorkflowRevision{PolicyRef: builtinVersionRef(defs, "core:policy/local", "2.0.0"), Limits: limits}}
	e := &Engine{}
	if err := e.checkWorkflowCapabilities(plan); err != nil {
		t.Fatal(err)
	}
	for name, exceed := range map[string]func(*flow.Limits){
		"steps":       func(l *flow.Limits) { l.MaxStepInstances++ },
		"transitions": func(l *flow.Limits) { l.MaxControlTransitions++ },
		"parallelism": func(l *flow.Limits) { l.MaxParallelism++ },
		"child_depth": func(l *flow.Limits) { l.MaxChildDepth++ },
	} {
		t.Run(name, func(t *testing.T) {
			plan.Workflow.Limits = limits
			exceed(&plan.Workflow.Limits)
			if err := e.checkWorkflowCapabilities(plan); err == nil || !strings.Contains(err.Error(), "resource_limit") {
				t.Fatalf("selected policy limit was not enforced: %v", err)
			}
		})
	}
	plan.Profile, plan.Workflow.Limits = flow.Profile, flow.Limits{MaxStepInstances: 1, MaxControlTransitions: 4, MaxParallelism: 1}
	if err := e.checkWorkflowCapabilities(plan); err == nil || !strings.Contains(err.Error(), "unsupported_policy") {
		t.Fatalf("F1 accepted policy2: %v", err)
	}
}

func TestLocalPolicyLegacyStartRetryExcludesUnusedVersion(t *testing.T) {
	for _, profile := range []string{flow.Profile, flow.CoreProfile} {
		t.Run(profile, func(t *testing.T) {
			e, options := emptyRuntime(t)
			defs, _, err := Builtins()
			if err != nil {
				t.Fatal(err)
			}
			if profile == flow.CoreProfile {
				e.Config.Configuration.SemanticsProfile = profile
				e.Config.Configuration.SchemaVersion = CoreConfigVersion
				e.Config.ConfigurationSchemaRef = builtinRef(defs, "core:schema/core-configuration")
				writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
			}
			ctx := context.Background()
			first, err := e.Start(ctx, options)
			if err != nil {
				t.Fatal(err)
			}
			r, before, err := e.load(ctx, first.Receipt.RunID)
			if err != nil {
				t.Fatal(err)
			}
			legacy := builtinRef(defs, "core:policy/local")
			for _, definition := range r.Definitions {
				if definition.Ref.ID == legacy.ID && definition.Ref != legacy {
					t.Fatal("new inventory version leaked into a legacy Run's lock")
				}
			}
			second, err := e.Start(ctx, options)
			if err != nil || !second.Duplicate || first.Receipt.EventSeq != second.Receipt.EventSeq || !bytes.Equal(first.Receipt.Result, second.Receipt.Result) {
				t.Fatalf("legacy exact Start changed: %v", err)
			}
			again, after, err := e.load(ctx, first.Receipt.RunID)
			// Duplicate command telemetry may advance the authority cut, but
			// must not mutate the pinned Run or append workflow events.
			if err != nil || again.LockRef != r.LockRef || before.Snapshot.Version != after.Snapshot.Version || before.Snapshot.EventSeq != after.Snapshot.EventSeq || !bytes.Equal(before.Snapshot.Data, after.Snapshot.Data) {
				t.Fatalf("retry changed pinned lock or Run snapshot: %v", err)
			}
		})
	}
}

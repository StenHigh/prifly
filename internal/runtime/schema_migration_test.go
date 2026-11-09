package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

type migrationFixture struct {
	engine   *Engine
	options  StartOptions
	legacy   PackageEntry
	supplier PackageEntry
	consumer PackageEntry
	schemas  []flow.Ref
	oldRun   string
}

func sharedSchemaMigrationFixture(t *testing.T, status string, variants ...string) migrationFixture {
	t.Helper()
	e, workflow, options := choiceFixture(t, `{"flag":true}`, "pass")
	ctx := context.Background()
	extra := registerRuntimeSchema(t, e, "migration-extra", []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","additionalProperties":true}`))
	workflow["inputs"].(map[string]any)["extra"] = map[string]any{"format": "json", "schema_ref": extra, "required": true}
	options.Inputs["extra"] = "control.json"
	if slices.Contains(variants, "subset") {
		delete(workflow["inputs"].(map[string]any), "extra")
		delete(options.Inputs, "extra")
	}
	stages := choiceStages(workflow)
	stages["work"].(map[string]any)["on"] = map[string]any{"pass": "work_again"}
	stages["work_again"] = callClone(t, stages["other"].(map[string]any))
	workflow["definition"] = map[string]any{"entry": "work", "stages": map[string]any{"work": stages["work"], "work_again": stages["work_again"], "done": choiceFinish("succeeded")}}
	workflow["limits"].(map[string]any)["max_step_instances"] = 2
	if slices.Contains(variants, "nested") {
		child := callClone(t, workflow)
		child["id"] = "test:workflow/migration-child"
		childRef := callRegister(t, e, child, "workflows/migration-child.json")
		definitions, _, err := Builtins()
		if err != nil {
			t.Fatal(err)
		}
		workflow["policy_ref"] = builtinVersionRef(definitions, "core:policy/local", "2.0.0")
		workflow["limits"] = map[string]any{"max_step_instances": 4, "max_control_transitions": 32, "max_parallelism": 1, "max_child_depth": 4}
		workflow["definition"] = map[string]any{"entry": "call", "stages": map[string]any{"call": callStage(childRef, workflow["inputs"].(map[string]any), "succeeded", "done"), "done": choiceFinish("succeeded")}}
	}

	wb, err := canonical(workflow)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.Root, options.WorkflowFile), wb, 0600); err != nil {
		t.Fatal(err)
	}
	writeRuntimeJSON(t, filepath.Join(e.Root, "prifly.json"), e.Config)
	control := workflow["inputs"].(map[string]any)["control"].(map[string]any)["schema_ref"].(flow.Ref)
	refs := []flow.Ref{control, extra}
	files := map[string]string{}
	components := []map[string]any{}
	for i, path := range []string{"schemas/choice-control.json", "schemas/migration-extra.json"} {
		b, err := os.ReadFile(filepath.Join(e.Root, path))
		if err != nil {
			t.Fatal(err)
		}
		files[path] = string(b)
		components = append(components, map[string]any{"kind": "schema", "ref": refs[i], "path": path})
	}
	importSource := func(id string, files map[string]string, components []map[string]any, deps []flow.Ref) PackageEntry {
		t.Helper()
		var registry RegistryFile
		registryPath := filepath.Join(e.Root, e.Config.Configuration.RegistryFile)
		rb, err := os.ReadFile(registryPath)
		if err != nil || json.Unmarshal(rb, &registry) != nil {
			t.Fatal("registry", err)
		}
		kept := []Definition{}
		for _, entry := range registry.Entries {
			duplicate := false
			for _, component := range components {
				if entry.Ref == component["ref"].(flow.Ref) {
					duplicate = true
				}
			}
			if !duplicate {
				kept = append(kept, entry)
			}
		}
		registry.Entries = kept
		writeRuntimeJSON(t, registryPath, registry)
		source := packageSource(t, files, components, func(m map[string]any) { m["id"] = id; m["dependencies"] = deps })
		result, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: newID("command"), Directory: source, Reason: "reviewed synthetic shared schemas"})
		if err != nil || result.Receipt.Rejection != nil {
			t.Fatalf("import %s: %v %+v", id, err, result.Receipt.Rejection)
		}
		record, err := e.Packages(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range record.Packages {
			if p.Ref.ID == id {
				return p
			}
		}
		t.Fatal("missing imported package")
		return PackageEntry{}
	}
	legacy := PackageEntry{}
	oldRun := ""
	if status != "" {
		legacyFiles := map[string]string{}
		for path, data := range files {
			legacyFiles[path] = data
		}
		legacyComponents := append([]map[string]any{}, components...)
		if slices.Contains(variants, "revoked-root") {
			digest, err := flow.Digest(wb)
			if err != nil {
				t.Fatal(err)
			}
			legacyFiles["root-workflow.json"] = string(wb)
			legacyComponents = append(legacyComponents, map[string]any{"kind": "workflow", "ref": flow.Ref{ID: workflow["id"].(string), Version: workflow["version"].(string), Digest: digest}, "path": "root-workflow.json"})
		}
		legacy = importSource("test:package/legacy-schemas", legacyFiles, legacyComponents, []flow.Ref{})
		if status == PackageRevoked {
			started, err := e.Start(ctx, options)
			if err != nil {
				t.Fatal(err)
			}
			oldRun = started.Receipt.RunID
		}
		changed, err := e.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: newID("command"), ID: legacy.Ref.ID, Version: legacy.Ref.Version, Status: status, Reason: "ordinary upgrade used this lifecycle choice"})
		if err != nil || changed.Receipt.Rejection != nil {
			t.Fatalf("lifecycle: %v %+v", err, changed.Receipt.Rejection)
		}
	}
	legacy.Status = status
	legacy.StatusReason = "ordinary upgrade used this lifecycle choice"
	for index, variant := range variants {
		if variant != PackageRevoked && variant != PackageQuarantined {
			continue
		}
		other := importSource(fmt.Sprintf("test:package/other-legacy-%d", index), files, components, []flow.Ref{})
		changed, err := e.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: newID("command"), ID: other.Ref.ID, Version: other.Ref.Version, Status: variant, Reason: "additional legacy supplier"})
		if err != nil || changed.Receipt.Rejection != nil {
			t.Fatal(err, changed.Receipt.Rejection)
		}
	}
	supplier := importSource("test:package/new-schemas", files, components, []flow.Ref{})
	raw, err := canonical(workflow)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.Root, options.WorkflowFile), raw, 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := flow.Digest(raw)
	if err != nil {
		t.Fatal(err)
	}
	wfRef := flow.Ref{ID: workflow["id"].(string), Version: workflow["version"].(string), Digest: digest}
	consumer := importSource("test:package/consumer", map[string]string{"workflow.json": string(raw)}, []map[string]any{{"kind": "workflow", "ref": wfRef, "path": "workflow.json"}}, []flow.Ref{supplier.Ref})
	options.CommandID = newID("command")
	return migrationFixture{e, options, legacy, supplier, consumer, refs, oldRun}
}

func TestSharedSchemaUpgradeBaselineMultipleRefs(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"", PackageRemoved, PackageRevoked} {
		t.Run("status="+status, func(t *testing.T) {
			t.Parallel()
			f := sharedSchemaMigrationFixture(t, status)
			before, _, err := f.engine.Store.ReadAllAt(context.Background(), -1, maxPinScanRuns)
			if err != nil {
				t.Fatal(err)
			}
			result, err := f.engine.Start(context.Background(), f.options)
			if status == PackageRevoked {
				rejectionCode(t, err, "package_not_resolvable")
				after, _, err := f.engine.Store.ReadAllAt(context.Background(), -1, maxPinScanRuns)
				if err != nil || len(after) != len(before) {
					t.Fatal("refusal created a Run", err)
				}
				if f.oldRun == "" {
					t.Fatal("missing old Run")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := f.engine.Drive(context.Background(), result.Receipt.RunID); err != nil {
				t.Fatal(err)
			}
			r := driverRun(t, f.engine, result.Receipt.RunID)
			if r.Status != "completed" || len(r.Attempts) != 2 {
				b, _ := json.Marshal(r)
				t.Fatalf("two native steps did not complete: %s", b)
			}
		})
	}
}

func TestSchemaMigrationExplicitSelectionStartsTwoNativeSteps(t *testing.T) {
	t.Parallel()
	f := sharedSchemaMigrationFixture(t, PackageRevoked)
	ctx := context.Background()
	request := SchemaMigrationRequest{SchemaVersion: "schema-migration-request/1", ConsumerRef: f.consumer.Ref, Selections: []SchemaMigrationSelection{}}
	for _, ref := range f.schemas {
		request.Selections = append(request.Selections, SchemaMigrationSelection{ref, f.supplier.Ref})
	}
	plan, err := f.engine.PrepareSchemaMigration(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Applicable || len(plan.Schemas) != 2 || len(plan.Runs) != 1 {
		t.Fatalf("incomplete review: %+v", plan)
	}
	applied, err := f.engine.ApplySchemaMigration(ctx, SchemaMigrationApplyRequest{CommandID: "command:migrate", Plan: plan, ExpectedReviewDigest: plan.ReviewDigest, Reason: "reviewed mistaken retirement; only schemas"})
	if err != nil || applied.Receipt.Rejection != nil {
		t.Fatalf("apply: %v %+v", err, applied.Receipt.Rejection)
	}
	f.options.SchemaMigrationID = applied.Decision.ID
	started, err := f.engine.Start(ctx, f.options)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.engine.Drive(ctx, started.Receipt.RunID); err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, f.engine, started.Receipt.RunID)
	if r.Status != "completed" || len(r.Attempts) != 2 {
		t.Fatalf("migration did not admit two native steps: %+v", r)
	}
}

func (f migrationFixture) reviewedPlan(t *testing.T) SchemaMigrationPlan {
	t.Helper()
	request := SchemaMigrationRequest{SchemaVersion: "schema-migration-request/1", ConsumerRef: f.consumer.Ref, Selections: []SchemaMigrationSelection{}}
	for _, ref := range f.schemas {
		request.Selections = append(request.Selections, SchemaMigrationSelection{ref, f.supplier.Ref})
	}
	plan, err := f.engine.PrepareSchemaMigration(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Applicable {
		t.Fatalf("plan is not applicable: %+v", plan)
	}
	return plan
}

func (f migrationFixture) applyPlan(t *testing.T, plan SchemaMigrationPlan, id string) SchemaMigrationResult {
	t.Helper()
	result, err := f.engine.ApplySchemaMigration(context.Background(), SchemaMigrationApplyRequest{CommandID: id, Plan: plan, ExpectedReviewDigest: plan.ReviewDigest, Reason: "owner reviewed exact shared schema bytes"})
	if err != nil || result.Receipt.Rejection != nil {
		t.Fatalf("apply: %v %+v", err, result.Receipt.Rejection)
	}
	return result
}

func TestSchemaMigrationPlansAreReadOnlyCompleteAndOrderIndependent(t *testing.T) {
	t.Parallel()
	f := sharedSchemaMigrationFixture(t, PackageRevoked)
	ctx := context.Background()
	before, err := f.engine.Store.ReadAuthority(ctx, AuthorityPackagesKey)
	if err != nil {
		t.Fatal(err)
	}
	request := SchemaMigrationRequest{SchemaVersion: "schema-migration-request/1", ConsumerRef: f.consumer.Ref, Selections: []SchemaMigrationSelection{}}
	diagnostic, err := f.engine.PrepareSchemaMigration(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic.Applicable || !diagnostic.Complete || len(diagnostic.Schemas) != 2 || len(diagnostic.Runs) != 1 {
		t.Fatalf("incomplete diagnosis: %+v", diagnostic)
	}
	for _, schema := range diagnostic.Schemas {
		if len(schema.Candidates) != 1 || len(schema.Revocations) != 1 || schema.Revocations[0].TrustReason == schema.Revocations[0].StatusReason {
			t.Fatalf("lost supplier or lifecycle reasons: %+v", schema)
		}
	}
	plan := f.reviewedPlan(t)
	reversed := plan.Request
	reversed.Selections = append([]SchemaMigrationSelection{}, reversed.Selections...)
	reversed.Selections[0], reversed.Selections[1] = reversed.Selections[1], reversed.Selections[0]
	same, err := f.engine.PrepareSchemaMigration(ctx, reversed)
	if err != nil || same.ReviewDigest != plan.ReviewDigest {
		t.Fatal("selection order changed digest", err)
	}
	after, err := f.engine.Store.ReadAuthority(ctx, AuthorityPackagesKey)
	if err != nil || after.Version != before.Version || string(after.Data) != string(before.Data) {
		t.Fatal("prepare mutated authority", err)
	}
	for name, value := range map[string]any{"SchemaMigrationRequestV1": plan.Request, "SchemaMigrationPlanV1": plan} {
		if err := validatePublic(t, name, value); err != nil {
			t.Fatalf("actual %s: %v", name, err)
		}
	}
	wire, _ := json.Marshal(plan.Request)
	parsed, err := ParseSchemaMigrationRequest(wire)
	if err != nil || parsed.ConsumerRef != plan.Request.ConsumerRef {
		t.Fatal("request roundtrip", err)
	}
	var object map[string]any
	json.Unmarshal(wire, &object)
	object["unexpected"] = true
	wire, _ = json.Marshal(object)
	if _, err := ParseSchemaMigrationRequest(wire); err == nil {
		t.Fatal("closed request accepted an extension")
	}
	_, err = f.engine.ApplySchemaMigration(ctx, SchemaMigrationApplyRequest{CommandID: newID("command"), Plan: diagnostic, ExpectedReviewDigest: diagnostic.ReviewDigest, Reason: "partial"})
	migrationRefusalCode(t, err, "schema_migration_incomplete")
}

func TestSchemaMigrationDurableBoundaryRestartOldRunAndReceiptReplay(t *testing.T) {
	t.Parallel()
	f := sharedSchemaMigrationFixture(t, PackageRevoked)
	ctx := context.Background()
	plan := f.reviewedPlan(t)
	applied := f.applyPlan(t, plan, "command:reviewed-migration")
	record, err := f.engine.Packages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if record.SchemaVersion != AuthorityPackagesMigrationVersion || len(record.SchemaMigrations) != 1 {
		t.Fatal("first apply did not upgrade packages")
	}
	for _, pkg := range record.Packages {
		if pkg.LifecycleGeneration != 1 {
			t.Fatal("legacy generation not seeded")
		}
		if pkg.Ref == f.legacy.Ref && (pkg.Status != PackageRevoked || pkg.Trust != f.legacy.Trust) {
			t.Fatal("apply changed revoked trust/history")
		}
	}
	for name, value := range map[string]any{"AuthorityPackagesV2": record, "SchemaMigrationResultV1": applied, "SchemaMigrationDecisionV1": applied.Decision} {
		if err := validatePublic(t, name, value); err != nil {
			t.Fatalf("actual %s: %v", name, err)
		}
	}
	f.options.SchemaMigrationID = applied.Decision.ID
	started, err := f.engine.Start(ctx, f.options)
	if err != nil {
		t.Fatal(err)
	}
	r := driverRun(t, f.engine, started.Receipt.RunID)
	if r.SchemaVersion != CoreSchemaMigrationStateVersion || r.QueueTask != nil || r.SchemaMigration == nil || len(r.SchemaMigration.Selections) != 2 {
		t.Fatal("migration-only Run did not reach its boundary")
	}
	if err := validatePublic(t, "CoreRunStateV45", r); err != nil {
		t.Fatal("actual persisted migration Run", err)
	}
	if validateInBundle(t, taskQueuePublicContracts, "CoreRunStateV44", r) == nil {
		t.Fatal("old bundle accepted migration")
	}
	driverExecuteFirst(t, f.engine, started.Receipt.RunID)
	root := f.engine.Root
	if err := f.engine.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if err := reopened.Drive(ctx, started.Receipt.RunID); err != nil {
		t.Fatal(err)
	}
	done := driverRun(t, reopened, started.Receipt.RunID)
	if done.Status != "completed" || len(done.Attempts) != 2 {
		t.Fatal("restart lost the second admission")
	}
	view, err := reopened.View(ctx, started.Receipt.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePublic(t, "CoreRunViewV45", view); err != nil {
		t.Fatal("actual migration read", err)
	}
	old := driverRun(t, reopened, f.oldRun)
	if old.SchemaMigration != nil {
		t.Fatal("old Run acquired a binding")
	}
	_, blocked, err := reopened.revokedPin(ctx, old)
	if err != nil {
		t.Fatal(err)
	}
	rejectionCode(t, blocked, "package_revoked")
	// A retry after many later writes returns exactly the accepted receipt.
	f.engine = reopened
	retry := f.applyPlan(t, plan, "command:reviewed-migration")
	if !retry.Duplicate || retry.Receipt.Cut != applied.Receipt.Cut || retry.Decision.ID != applied.Decision.ID {
		t.Fatal("retry did not preserve original receipt")
	}
	_, err = reopened.ApplySchemaMigration(ctx, SchemaMigrationApplyRequest{CommandID: "command:reviewed-migration", Plan: plan, ExpectedReviewDigest: plan.ReviewDigest, Reason: "changed payload"})
	if !errors.Is(err, local.ErrCommandConflict) {
		t.Fatalf("changed retry payload: %v", err)
	}
}

func TestSchemaMigrationStalePlanSavesNamedRefusalWithoutPartialDecision(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"package", "run", "control", "closure"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := sharedSchemaMigrationFixture(t, PackageRevoked)
			ctx := context.Background()
			plan := f.reviewedPlan(t)
			switch kind {
			case "package":
				changed, err := f.engine.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: newID("command"), ID: f.legacy.Ref.ID, Version: f.legacy.Ref.Version, Status: PackageRevoked, Reason: "fresh security revoke"})
				if err != nil || changed.Receipt.Rejection != nil {
					t.Fatal(err, changed.Receipt.Rejection)
				}
			case "run":
				if _, err := f.engine.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: f.oldRun, Kind: "pause", Reason: "change affecting Run version"}); err != nil {
					t.Fatal(err)
				}
			case "closure":
				changed, err := f.engine.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: newID("command"), ID: f.supplier.Ref.ID, Version: f.supplier.Ref.Version, Status: PackageQuarantined, Reason: "closure no longer resolves"})
				if err != nil || changed.Receipt.Rejection != nil {
					t.Fatal(err, changed.Receipt.Rejection)
				}
			case "control":
				control, version, err := f.engine.Control(ctx)
				if err != nil {
					t.Fatal(err)
				}
				control.ControlEpoch++
				data, err := canonical(control)
				if err != nil {
					t.Fatal(err)
				}
				_, err = f.engine.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: newID("command"), Actor: f.engine.owner, Key: AuthorityControlKey, Payload: json.RawMessage(`{"test":"epoch"}`), ExpectedVersion: &version}, func(local.AuthoritySnapshot) (local.AuthorityChange, error) {
					return local.AuthorityChange{Data: data}, nil
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			result, err := f.engine.ApplySchemaMigration(ctx, SchemaMigrationApplyRequest{CommandID: "command:stale", Plan: plan, ExpectedReviewDigest: plan.ReviewDigest, Reason: "reviewed"})
			if err != nil || result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "schema_migration_plan_stale" {
				t.Fatalf("saved stale refusal: %v %+v", err, result.Receipt)
			}
			record, err := f.engine.Packages(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if record.SchemaVersion != AuthorityPackagesVersion || len(record.SchemaMigrations) != 0 {
				t.Fatal("stale apply partially upgraded state")
			}
		})
	}
}

func TestSchemaMigrationDecisionInvalidatedByNewLifecycleDecision(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"repeat-revoke", "supplier-quarantine"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			f := sharedSchemaMigrationFixture(t, PackageRevoked)
			ctx := context.Background()
			applied := f.applyPlan(t, f.reviewedPlan(t), newID("command"))
			pkg, status := f.legacy, PackageRevoked
			if kind == "supplier-quarantine" {
				pkg, status = f.supplier, PackageQuarantined
				pkg.StatusReason = "review supplier quarantine"
			}
			changed, err := f.engine.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: newID("command"), ID: pkg.Ref.ID, Version: pkg.Ref.Version, Status: status, Reason: pkg.StatusReason})
			if err != nil || changed.Receipt.Rejection != nil {
				t.Fatal(err, changed.Receipt.Rejection)
			}
			inspection, err := f.engine.InspectSchemaMigration(ctx, applied.Decision.ID)
			if err != nil {
				t.Fatal(err)
			}
			if inspection.Valid || len(inspection.Reasons) == 0 {
				t.Fatal("new lifecycle decision retained permission")
			}
			f.options.SchemaMigrationID = applied.Decision.ID
			_, err = f.engine.Start(ctx, f.options)
			migrationRefusalCode(t, err, "schema_migration_decision_invalid")
		})
	}
}

func migrationRefusalCode(t *testing.T, err error, code string) {
	t.Helper()
	var refusal *Fault
	if errors.As(err, &refusal) && refusal.Code == code {
		return
	}
	var rejected *local.Rejection
	if errors.As(err, &rejected) && rejected.Code == code {
		return
	}
	t.Fatalf("wanted %s, got %v", code, err)
}

func TestSchemaMigrationNestedWorkflowUsesTheSameScopedBinding(t *testing.T) {
	t.Parallel()
	f := sharedSchemaMigrationFixture(t, PackageRevoked, "nested")
	applied := f.applyPlan(t, f.reviewedPlan(t), newID("command"))
	f.options.SchemaMigrationID = applied.Decision.ID
	started, err := f.engine.Start(context.Background(), f.options)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.engine.Drive(context.Background(), started.Receipt.RunID); err != nil {
		t.Fatal(err)
	}
	run := driverRun(t, f.engine, started.Receipt.RunID)
	if run.Status != "completed" || len(run.Invocations) != 2 || len(run.Attempts) != 2 {
		t.Fatalf("nested admissions failed: %+v", run)
	}
}

func TestSchemaMigrationCoversEveryRevokedSupplierAndRejectsNewRevocations(t *testing.T) {
	t.Parallel()
	for _, state := range []string{PackageRevoked, PackageQuarantined} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			f := sharedSchemaMigrationFixture(t, PackageRevoked, state)
			plan := f.reviewedPlan(t)
			expected := 1
			if state == PackageRevoked {
				expected = 2
			}
			for _, schema := range plan.Schemas {
				if len(schema.Revocations) != expected {
					t.Fatalf("lost a revoked supplier: %+v", schema)
				}
			}
			applied := f.applyPlan(t, plan, newID("command"))
			if state == PackageRevoked {
				f.options.SchemaMigrationID = applied.Decision.ID
				if _, err := f.engine.Start(context.Background(), f.options); err != nil {
					t.Fatal(err)
				}
				return
			}
			record, err := f.engine.Packages(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			for _, pkg := range record.Packages {
				if pkg.Status == PackageQuarantined {
					result, err := f.engine.SetPackageStatus(context.Background(), PackageLifecycleRequest{CommandID: newID("command"), ID: pkg.Ref.ID, Version: pkg.Ref.Version, Status: PackageRevoked, Reason: "new security revocation"})
					if err != nil || result.Receipt.Rejection != nil {
						t.Fatal(err, result.Receipt.Rejection)
					}
				}
			}
			inspection, err := f.engine.InspectSchemaMigration(context.Background(), applied.Decision.ID)
			if err != nil || inspection.Valid {
				t.Fatal("new uncovered revoke retained permission", err)
			}
		})
	}
}

func TestSchemaMigrationScopeRefusalsNeverExpandTrust(t *testing.T) {
	t.Parallel()
	f := sharedSchemaMigrationFixture(t, PackageRevoked)
	plan := f.reviewedPlan(t)
	for _, kind := range []string{"different-digest", "executable", "outside-supplier", "partial"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			request := plan.Request
			request.Selections = append([]SchemaMigrationSelection{}, request.Selections...)
			switch kind {
			case "other-consumer":
				request.ConsumerRef = f.supplier.Ref
			case "different-digest":
				request.ConsumerRef.Digest = "sha256:" + strings.Repeat("0", 64)
			case "executable":
				request.Selections[0].SchemaRef = f.consumer.Components[0].Ref
			case "outside-supplier":
				request.Selections[0].SupplierRef = f.legacy.Ref
			case "partial":
				request.Selections = request.Selections[:1]
			}
			candidate, err := f.engine.PrepareSchemaMigration(context.Background(), request)
			if kind == "partial" {
				if err != nil || candidate.Applicable {
					t.Fatal("partial choice became applicable", err)
				}
				return
			}
			migrationRefusalCode(t, err, "schema_migration_scope_mismatch")
		})
	}
	applied := f.applyPlan(t, plan, newID("command"))
	f.options.SchemaMigrationID = applied.Decision.ID
	data, err := os.ReadFile(filepath.Join(f.engine.Root, f.consumer.Components[0].Path))
	if err != nil {
		t.Fatal(err)
	}
	var other map[string]any
	if err := json.Unmarshal(data, &other); err != nil {
		t.Fatal(err)
	}
	other["id"] = "test:workflow/another-consumer"
	otherBytes, err := canonical(other)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.engine.Root, "workflows/another-consumer.json"), otherBytes, 0600); err != nil {
		t.Fatal(err)
	}
	f.options.WorkflowFile = "workflows/another-consumer.json"
	_, err = f.engine.Start(context.Background(), f.options)
	migrationRefusalCode(t, err, "schema_migration_scope_mismatch")
	record, err := f.engine.Packages(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range record.Packages {
		if pkg.Ref == f.legacy.Ref && pkg.Status != PackageRevoked {
			t.Fatal("scope refusal restored legacy package")
		}
	}
}

func TestSchemaMigrationRevocationStopsSubsequentAndNestedAdmissions(t *testing.T) {
	t.Parallel()
	for _, nested := range []bool{false, true} {
		t.Run(fmt.Sprint("nested=", nested), func(t *testing.T) {
			t.Parallel()
			variants := []string{}
			if nested {
				variants = append(variants, "nested")
			}
			f := sharedSchemaMigrationFixture(t, PackageRevoked, variants...)
			ctx := context.Background()
			applied := f.applyPlan(t, f.reviewedPlan(t), newID("command"))
			f.options.SchemaMigrationID = applied.Decision.ID
			started, err := f.engine.Start(ctx, f.options)
			if err != nil {
				t.Fatal(err)
			}
			if !nested {
				driverExecuteFirst(t, f.engine, started.Receipt.RunID)
			}
			revoked, err := f.engine.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: newID("command"), ID: f.legacy.Ref.ID, Version: f.legacy.Ref.Version, Status: PackageRevoked, Reason: f.legacy.StatusReason})
			if err != nil || revoked.Receipt.Rejection != nil {
				t.Fatal(err, revoked.Receipt.Rejection)
			}
			_ = f.engine.Drive(ctx, started.Receipt.RunID)
			run := driverRun(t, f.engine, started.Receipt.RunID)
			expected := 1
			if nested {
				expected = 0
				if len(run.Invocations) != 1 {
					t.Fatal("revoke admitted a child invocation")
				}
			}
			if len(run.Attempts) != expected {
				t.Fatal("revoke admitted new executable work")
			}
		})
	}
}

func TestSchemaMigrationApplyRacesAreWholeScopeOrNoDecision(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"revoke", "run-change"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			f := sharedSchemaMigrationFixture(t, PackageRevoked)
			ctx := context.Background()
			plan := f.reviewedPlan(t)
			other, err := Open(f.engine.Root, false)
			if err != nil {
				t.Fatal(err)
			}
			defer other.Close()
			ready := make(chan struct{})
			changed := make(chan error, 1)
			go func() {
				<-ready
				if operation == "revoke" {
					result, err := other.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: "command:concurrent-revoke", ID: f.legacy.Ref.ID, Version: f.legacy.Ref.Version, Status: PackageRevoked, Reason: "concurrent security revoke"})
					if err == nil && result.Receipt.Rejection != nil {
						err = result.Receipt.Rejection
					}
					changed <- err
				} else {
					_, err := other.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: "command:concurrent-run-change", Scope: "run", ScopeID: f.oldRun, Kind: "pause", Reason: "change same Run population"})
					changed <- err
				}
			}()
			close(ready)
			result, applyErr := f.engine.ApplySchemaMigration(ctx, SchemaMigrationApplyRequest{CommandID: "command:concurrent-apply", Plan: plan, ExpectedReviewDigest: plan.ReviewDigest, Reason: "reviewed exact scope"})
			if err := <-changed; err != nil {
				t.Fatal(err)
			}
			if applyErr != nil {
				migrationRefusalCode(t, applyErr, "schema_migration_plan_stale")
			}
			record, err := f.engine.Packages(ctx)
			if err != nil {
				t.Fatal(err)
			}
			accepted := applyErr == nil && result.Receipt.Rejection == nil
			if accepted {
				if len(record.SchemaMigrations) != 1 || len(record.SchemaMigrations[0].Plan.Schemas) != 2 || record.SchemaVersion != AuthorityPackagesMigrationVersion {
					t.Fatal("partial decision committed")
				}
				if operation == "revoke" {
					inspection, err := f.engine.InspectSchemaMigration(ctx, result.Decision.ID)
					if err != nil || inspection.Valid {
						t.Fatal("concurrent revoke retained migration permission", err)
					}
				}
			} else if len(record.SchemaMigrations) != 0 || record.SchemaVersion != AuthorityPackagesVersion {
				t.Fatal("refused race committed partial migration")
			}
		})
	}
}

func TestSchemaMigrationStartAndRevokeSerializeBeforeAnyWorker(t *testing.T) {
	t.Parallel()
	f := sharedSchemaMigrationFixture(t, PackageRevoked)
	ctx := context.Background()
	applied := f.applyPlan(t, f.reviewedPlan(t), newID("command"))
	f.options.SchemaMigrationID = applied.Decision.ID
	other, err := Open(f.engine.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	ready := make(chan struct{})
	changed := make(chan error, 1)
	go func() {
		<-ready
		result, err := other.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: "command:start-race-revoke", ID: f.legacy.Ref.ID, Version: f.legacy.Ref.Version, Status: PackageRevoked, Reason: "concurrent security withdrawal"})
		if err == nil && result.Receipt.Rejection != nil {
			err = result.Receipt.Rejection
		}
		changed <- err
	}()
	close(ready)
	started, startErr := f.engine.Start(ctx, f.options)
	if err := <-changed; err != nil {
		t.Fatal(err)
	}
	if startErr == nil && started.Receipt.Rejection == nil {
		_ = f.engine.Drive(ctx, started.Receipt.RunID)
		run := driverRun(t, f.engine, started.Receipt.RunID)
		if len(run.Attempts) != 0 {
			t.Fatal("worker crossed a committed revoke")
		}
	} else {
		_, err := f.engine.Store.Read(ctx, f.engine.IntendedRunID(f.options.CommandID), 0, 1)
		if !errors.Is(err, local.ErrNotFound) {
			t.Fatal("refused Start left a Run", err)
		}
	}
}

func TestSchemaMigrationSupplierDriftRefusesApplyAndLaterAdmissions(t *testing.T) {
	t.Parallel()
	for _, afterApply := range []bool{false, true} {
		t.Run(fmt.Sprint("after-apply=", afterApply), func(t *testing.T) {
			t.Parallel()
			f := sharedSchemaMigrationFixture(t, PackageRevoked)
			ctx := context.Background()
			plan := f.reviewedPlan(t)
			var decision SchemaMigrationResult
			if afterApply {
				decision = f.applyPlan(t, plan, newID("command"))
			}
			path := filepath.Join(f.engine.Root, f.supplier.Components[0].Path)
			if err := os.Chmod(path, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(`{"type":"null"}`), 0600); err != nil {
				t.Fatal(err)
			}
			if afterApply {
				f.options.SchemaMigrationID = decision.Decision.ID
				_, err := f.engine.Start(ctx, f.options)
				if err == nil {
					t.Fatal("changed schema bytes admitted")
				}
			} else {
				_, err := f.engine.ApplySchemaMigration(ctx, SchemaMigrationApplyRequest{CommandID: newID("command"), Plan: plan, ExpectedReviewDigest: plan.ReviewDigest, Reason: "reviewed"})
				migrationRefusalCode(t, err, "schema_migration_integrity")
				record, err := f.engine.Packages(ctx)
				if err != nil || len(record.SchemaMigrations) != 0 {
					t.Fatal("drift partially applied", err)
				}
			}
		})
	}
}

func TestSchemaMigrationAndQueueBindingsReachOneNewBoundary(t *testing.T) {
	t.Parallel()
	f := sharedSchemaMigrationFixture(t, PackageRevoked)
	ctx := context.Background()
	applied := f.applyPlan(t, f.reviewedPlan(t), newID("command"))
	selection := queueClaimFixture(t, f.engine)
	binding := queueReserveFixture(t, f.engine, selection, "command:queue-and-migration")
	queueDispatchFixture(t, f.engine, binding)
	f.options.CommandID = "command:queue-and-migration"
	f.options.SchemaMigrationID = applied.Decision.ID
	f.options.QueueTask = &binding
	started, err := f.engine.Start(ctx, f.options)
	if err != nil {
		t.Fatal(err)
	}
	run := driverRun(t, f.engine, started.Receipt.RunID)
	if run.SchemaVersion != CoreSchemaMigrationStateVersion || run.QueueTask == nil || run.SchemaMigration == nil {
		t.Fatal("joint binding was not sealed")
	}
	if err := validatePublic(t, "CoreRunStateV45", run); err != nil {
		t.Fatal("joint contract", err)
	}
	if err := queueBindingInvariant(run); err != nil {
		t.Fatal(err)
	}
	wire, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(wire, &fields); err != nil {
		t.Fatal(err)
	}
	fields["schema_migration"] = json.RawMessage("null")
	bad, _ := json.Marshal(fields)
	var decoded Run
	if decodeState(bad, &decoded) == nil {
		t.Fatal("new boundary accepted null binding")
	}
	fields["schema_version"] = json.RawMessage(`"core-state/44"`)
	bad, _ = json.Marshal(fields)
	if decodeState(bad, &decoded) == nil {
		t.Fatal("old boundary accepted an explicit migration field")
	}
}

func TestSchemaMigrationBindingUsesOnlyTheWorkflowSchemaClosure(t *testing.T) {
	t.Parallel()
	f := sharedSchemaMigrationFixture(t, PackageRevoked, "subset")
	applied := f.applyPlan(t, f.reviewedPlan(t), newID("command"))
	f.options.SchemaMigrationID = applied.Decision.ID
	started, err := f.engine.Start(context.Background(), f.options)
	if err != nil {
		t.Fatal(err)
	}
	run := driverRun(t, f.engine, started.Receipt.RunID)
	if len(applied.Decision.Plan.Schemas) != 2 || len(run.SchemaMigration.Selections) != 1 {
		t.Fatal("package-level schemas expanded the workflow binding")
	}
	// The ordinary trusted-reuse gate continues to reject the revoked source.
	old := driverRun(t, f.engine, f.oldRun)
	_, err = f.engine.reuseTrustPin(context.Background(), old, []ArtifactRef{{ArtifactID: "artifact:old-result", Revision: 1, Digest: "sha256:old-result"}})
	rejectionCode(t, err, "reuse_trust_invalid")
}

func TestSchemaMigrationGenerationsSurviveExactReimportAndUnrelatedLifecycle(t *testing.T) {
	t.Parallel()
	f := sharedSchemaMigrationFixture(t, PackageRevoked)
	ctx := context.Background()
	applied := f.applyPlan(t, f.reviewedPlan(t), newID("command"))
	before, err := f.engine.Packages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.engine.ImportPackage(ctx, PackageImportRequest{CommandID: newID("command"), Directory: f.consumer.Origin.Location, Reason: "repeat exact trusted bytes"})
	if err != nil || result.Receipt.Rejection != nil {
		t.Fatal(err, result.Receipt.Rejection)
	}
	after, err := f.engine.Packages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range after.Packages {
		for _, old := range before.Packages {
			if pkg.Ref == old.Ref && (pkg.LifecycleGeneration != old.LifecycleGeneration || pkg.Trust != old.Trust) {
				t.Fatal("trusted reimport changed original trust/generation")
			}
		}
	}
	inspection, err := f.engine.InspectSchemaMigration(ctx, applied.Decision.ID)
	if err != nil || !inspection.Valid {
		t.Fatal("exact trusted reimport invalidated decision", err)
	}
	body := "# unrelated package\n"
	ref := flow.Ref{ID: "test:context/unrelated", Version: "1.0.0", Digest: rawDigest([]byte(body))}
	source := packageSource(t, map[string]string{"unrelated.md": body}, []map[string]any{{"kind": "context", "ref": ref, "path": "unrelated.md"}}, func(m map[string]any) { m["id"] = "test:package/unrelated" })
	result, err = f.engine.ImportPackage(ctx, PackageImportRequest{CommandID: newID("command"), Directory: source, Reason: "unrelated trusted package"})
	if err != nil || result.Receipt.Rejection != nil {
		t.Fatal(err, result.Receipt.Rejection)
	}
	for _, status := range []string{PackageQuarantined, PackageTrusted, PackageRemoved, PackageTrusted} {
		changed, err := f.engine.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: newID("command"), ID: "test:package/unrelated", Version: "1.0.0", Status: status, Reason: "unrelated lifecycle matrix"})
		if err != nil || changed.Receipt.Rejection != nil {
			t.Fatal(err, changed.Receipt.Rejection)
		}
	}
	inspection, err = f.engine.InspectSchemaMigration(ctx, applied.Decision.ID)
	if err != nil || !inspection.Valid {
		t.Fatal("unrelated lifecycle destroyed decision", err)
	}
	record, err := f.engine.Packages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range record.Packages {
		if pkg.Ref.ID == "test:package/unrelated" && pkg.LifecycleGeneration != 5 {
			t.Fatal("accepted lifecycle changes did not advance generation")
		}
	}
	// A dependent still protects its supplier from ordinary removal under /2.
	refused, err := f.engine.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: newID("command"), ID: f.supplier.Ref.ID, Version: f.supplier.Ref.Version, Status: PackageRemoved, Reason: "ordinary removal"})
	rejectionCode(t, err, "package_in_use")
	if refused.Receipt.Rejection != nil {
		t.Fatal("preflight refusal unexpectedly wrote a decision")
	}
	if _, err := f.engine.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: f.oldRun, Kind: "cancel", Reason: "release terminal holders"}); err != nil {
		t.Fatal(err)
	}
	if err := f.engine.Drive(ctx, f.oldRun); err != nil {
		t.Fatal(err)
	}
	_, err = f.engine.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: newID("command"), ID: f.supplier.Ref.ID, Version: f.supplier.Ref.Version, Status: PackageRemoved, Reason: "dependent consumer remains"})
	rejectionCode(t, err, "package_depended_on")
}

func TestSchemaMigrationScanLimitCannotProduceAnApplicablePlan(t *testing.T) {
	t.Parallel()
	f := sharedSchemaMigrationFixture(t, PackageRevoked)
	ctx := context.Background()
	// Exercise the production record bound with stored snapshots, without creating workers.
	for i := 0; i < maxPinScanRuns; i++ {
		id := fmt.Sprintf("run:scan-%04d", i)
		run := Run{SchemaVersion: StateVersion, ID: id, Ready: []string{}, Active: []string{}, Steps: map[string]*Step{}, Attempts: map[string]*Attempt{}}
		data, err := canonical(run)
		if err != nil {
			t.Fatal(err)
		}
		zero := int64(0)
		result, err := f.engine.Store.Apply(ctx, local.Command{ExpectedVersion: &zero, ID: fmt.Sprintf("command:scan-%04d", i), Actor: f.engine.owner, RunID: id, Payload: json.RawMessage(`{"seed":"bounded scan"}`), Mode: local.CommandCAS}, func(local.Snapshot) (local.Change, error) {
			return local.Change{Data: data, Events: []local.EventInput{{Type: "run.created", Version: 1, Data: json.RawMessage(`{"fixture":true}`)}}}, nil
		})
		if err != nil || result.Receipt.Rejection != nil {
			t.Fatal(err, result.Receipt.Rejection)
		}
	}
	request := SchemaMigrationRequest{SchemaVersion: "schema-migration-request/1", ConsumerRef: f.consumer.Ref, Selections: []SchemaMigrationSelection{}}
	for _, schema := range f.schemas {
		request.Selections = append(request.Selections, SchemaMigrationSelection{schema, f.supplier.Ref})
	}
	plan, err := f.engine.PrepareSchemaMigration(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Complete || plan.Applicable || len(plan.Reasons) == 0 {
		t.Fatal("scan limit claimed a complete applicable population")
	}
}

func TestSchemaMigrationApplyChecksOwnerAtTheAtomicBoundary(t *testing.T) {
	t.Parallel()
	f := sharedSchemaMigrationFixture(t, PackageRevoked)
	ctx := context.Background()
	plan := f.reviewedPlan(t)
	control, version, err := f.engine.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for i := range control.Access {
		control.Access[i].Operations = slices.DeleteFunc(control.Access[i].Operations, func(op string) bool { return op == ControlOperationTrust })
	}
	data, err := canonical(control)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.engine.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: newID("command"), Actor: f.engine.owner, Key: AuthorityControlKey, Payload: json.RawMessage(`{"test":"remove trust permission"}`), ExpectedVersion: &version}, func(local.AuthoritySnapshot) (local.AuthorityChange, error) {
		return local.AuthorityChange{Data: data}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := f.engine.ApplySchemaMigration(ctx, SchemaMigrationApplyRequest{CommandID: "command:denied-migration", Plan: plan, ExpectedReviewDigest: plan.ReviewDigest, Reason: "cannot grant own permission"})
	if err != nil || result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "object_access_denied" {
		t.Fatal("owner denial not saved atomically", err, result.Receipt.Rejection)
	}
	record, err := f.engine.Packages(ctx)
	if err != nil || len(record.SchemaMigrations) != 0 {
		t.Fatal("denied apply changed trust", err)
	}
}

func TestSchemaMigrationCannotAdmitTheRevokedRootWorkflow(t *testing.T) {
	t.Parallel()
	f := sharedSchemaMigrationFixture(t, PackageRevoked, "revoked-root")
	applied := f.applyPlan(t, f.reviewedPlan(t), newID("command"))
	f.options.SchemaMigrationID = applied.Decision.ID
	_, err := f.engine.Start(context.Background(), f.options)
	rejectionCode(t, err, "package_not_resolvable")
	old := driverRun(t, f.engine, f.oldRun)
	_, blocked, err := f.engine.revokedPin(context.Background(), old)
	if err != nil {
		t.Fatal(err)
	}
	rejectionCode(t, blocked, "package_revoked")
}

func TestSchemaMigrationFullAuthorityBudgetRefusesWithoutPruningHistory(t *testing.T) {
	t.Parallel()
	f := sharedSchemaMigrationFixture(t, PackageRevoked)
	ctx := context.Background()
	applied := f.applyPlan(t, f.reviewedPlan(t), newID("command"))
	record, err := f.engine.Packages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Synthetic retained decisions fill the real storage budget. Each has the
	// actual decision shape; this fixture does not attest historical receipts.
	prototype := applied.Decision
	prototype.ID = "schema-migration:historic-00000000"
	prototype.CommandID = "command:historic-00000000"
	unit, err := json.Marshal(prototype)
	if err != nil {
		t.Fatal(err)
	}
	record.SchemaMigrations = nil
	base, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	count := (local.MaxSnapshotBytes - len(base) - 1024) / (len(unit) + 1)
	record.SchemaMigrations = make([]SchemaMigrationDecision, count)
	for i := range record.SchemaMigrations {
		decision := prototype
		decision.ID = fmt.Sprintf("schema-migration:historic-%08d", i)
		decision.CommandID = fmt.Sprintf("command:historic-%08d", i)
		record.SchemaMigrations[i] = decision
	}
	data, err := canonicalState(record)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := f.engine.Store.ReadAuthority(ctx, AuthorityPackagesKey)
	if err != nil {
		t.Fatal(err)
	}
	seeded, err := f.engine.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: "command:retained-history-fixture", Actor: f.engine.owner, Key: AuthorityPackagesKey, Payload: json.RawMessage(`{"fixture":"retained migration history"}`), ExpectedVersion: &snapshot.Version}, func(local.AuthoritySnapshot) (local.AuthorityChange, error) {
		return local.AuthorityChange{Data: data}, nil
	})
	if err != nil || seeded.Receipt.Rejection != nil {
		t.Fatal(err, seeded.Receipt.Rejection)
	}
	current, err := f.engine.Packages(ctx)
	if err != nil || len(current.SchemaMigrations) != count {
		t.Fatal("bounded authority above wire size is unreadable", err)
	}
	plan := f.reviewedPlan(t)
	_, err = f.engine.ApplySchemaMigration(ctx, SchemaMigrationApplyRequest{CommandID: "command:budget-overflow", Plan: plan, ExpectedReviewDigest: plan.ReviewDigest, Reason: strings.Repeat("reviewed ", 400)})
	migrationRefusalCode(t, err, "schema_migration_incomplete")
	after, err := f.engine.Store.ReadAuthority(ctx, AuthorityPackagesKey)
	if err != nil || !bytes.Equal(after.Data, data) {
		t.Fatal("overflow pruned or partly wrote history", err)
	}
}

func TestSchemaMigrationDoesNotUpgradeAnOrdinaryUnrelatedRun(t *testing.T) {
	t.Parallel()
	f := sharedSchemaMigrationFixture(t, PackageRevoked)
	f.applyPlan(t, f.reviewedPlan(t), newID("command"))
	started, err := f.engine.Start(context.Background(), StartOptions{CommandID: newID("command"), WorkflowFile: "workflows/driver.json", BriefFile: "brief.json", Inputs: map[string]string{"source": "source.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	run := driverRun(t, f.engine, started.Receipt.RunID)
	if run.SchemaVersion != CoreStateVersion || run.SchemaMigration != nil {
		t.Fatal("authority decision changed an ordinary Run contract")
	}
	data, err := json.Marshal(run)
	if err != nil || bytes.Contains(data, []byte(`"schema_migration"`)) {
		t.Fatal("ordinary Run emitted migration field", err)
	}
	view, err := f.engine.View(context.Background(), started.Receipt.RunID)
	if err != nil || view.SchemaVersion != CoreReadVersion {
		t.Fatal("ordinary read boundary changed", err)
	}
}

func TestSchemaMigrationDigestMismatchNeverRecordsADecision(t *testing.T) {
	t.Parallel()
	f := sharedSchemaMigrationFixture(t, PackageRevoked)
	ctx := context.Background()
	plan := f.reviewedPlan(t)
	_, err := f.engine.ApplySchemaMigration(ctx, SchemaMigrationApplyRequest{CommandID: newID("command"), Plan: plan, ExpectedReviewDigest: "sha256:" + strings.Repeat("0", 64), Reason: "reviewed"})
	migrationRefusalCode(t, err, "schema_migration_invalid")
	record, err := f.engine.Packages(ctx)
	if err != nil || record.SchemaVersion != AuthorityPackagesVersion || len(record.SchemaMigrations) != 0 {
		t.Fatal("digest mismatch partly applied scope", err)
	}
}

func TestSchemaMigrationStaleProblemAlwaysUsesTheConflictExit(t *testing.T) {
	t.Parallel()
	for _, err := range []error{fault("schema_migration_plan_stale", "authority changed during prepare"), local.Reject("schema_migration_plan_stale", "authority changed before commit")} {
		problem, exit := ProblemFor(err)
		if problem.Code != "schema_migration_plan_stale" || exit != 3 || problem.Retryable {
			t.Fatalf("inconsistent stale problem: %+v exit %d", problem, exit)
		}
	}
}

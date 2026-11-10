package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/stenhigh/prifly/internal/local"
)

func launchMigrationFixture(t *testing.T) (migrationFixture, LaunchSchemaMigrationRequest) {
	t.Helper()
	f := sharedSchemaMigrationFixture(t, PackageRevoked)
	applied := f.applyPlan(t, f.reviewedPlan(t), newID("command"))
	build := LaunchSchemaMigrationBuild{BuildKey: rawDigest([]byte("exact launch build")), ConsumerRef: f.consumer.Ref}
	for _, component := range f.consumer.Components {
		if component.Kind == "workflow" {
			build.WorkflowRef = component.Ref
		}
	}
	choice, err := f.engine.CheckLaunchSchemaMigration(context.Background(), applied.Decision.ID, build)
	if err != nil {
		t.Fatal(err)
	}
	scope := LaunchSchemaMigrationScope{AuthorityID: f.engine.Installation.ID, ProjectID: f.engine.Config.ID, Repository: filepath.Join(t.TempDir(), "repository"), Launch: "sample"}
	return f, LaunchSchemaMigrationRequest{Scope: scope, Choice: &choice, CommandID: newID("command"), Reason: "reviewed launch"}
}

func applyLaunchMigration(t *testing.T, e *Engine, request LaunchSchemaMigrationRequest) LaunchSchemaMigrationResult {
	t.Helper()
	r, err := e.SetLaunchSchemaMigration(context.Background(), request)
	if err != nil || r.Receipt.Rejection != nil {
		t.Fatalf("choice: %v %+v", err, r.Receipt.Rejection)
	}
	return r
}

func TestLaunchSchemaMigrationChoiceReadOnlyCASReplayAndRestart(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f, request := launchMigrationFixture(t)
	before, _ := f.engine.Store.ReadAuthority(ctx, AuthorityPackagesKey)
	view, err := f.engine.InspectLaunchSchemaMigration(ctx, request.Scope)
	if err != nil || view.State.Choice != nil || view.State.Generation != 0 {
		t.Fatal(view, err)
	}
	if _, err := f.engine.Store.ReadAuthority(ctx, launchSchemaMigrationKey(request.Scope)); !errors.Is(err, local.ErrNotFound) {
		t.Fatal("inspection persisted state", err)
	}
	first := applyLaunchMigration(t, f.engine, request)
	if first.State.Generation != 1 {
		t.Fatal(first)
	}
	for _, name := range []string{"LaunchSchemaMigrationStateV1", "LaunchSchemaMigrationResultV1"} {
		value := any(first)
		if name == "LaunchSchemaMigrationStateV1" {
			value = first.State
		}
		if err := validatePublic(t, name, value); err != nil {
			t.Fatal(name, err)
		}
	}
	replay, err := f.engine.SetLaunchSchemaMigration(ctx, request)
	if err != nil || !replay.Duplicate || replay.Receipt.Cut != first.Receipt.Cut {
		t.Fatal("replay", replay, err)
	}
	changed := request
	changed.Reason = "different payload"
	if _, err := f.engine.SetLaunchSchemaMigration(ctx, changed); err == nil {
		t.Fatal("changed payload replayed")
	}
	stale := request
	stale.CommandID = newID("command")
	refused, err := f.engine.SetLaunchSchemaMigration(ctx, stale)
	if err != nil || refused.Receipt.Rejection == nil || refused.Receipt.Rejection.Code != "schema_migration_choice_stale" {
		t.Fatal(refused, err)
	}
	reopened, err := Open(f.engine.Root, true)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	view, err = reopened.InspectLaunchSchemaMigration(ctx, request.Scope)
	if err != nil || !view.Valid || view.State.Generation != 1 {
		t.Fatal(view, err)
	}
	other := request.Scope
	other.Launch = "other"
	view, err = reopened.InspectLaunchSchemaMigration(ctx, other)
	if err != nil || view.State.Choice != nil {
		t.Fatal("launch permission leaked", view, err)
	}
	other = request.Scope
	other.Repository = filepath.Join(t.TempDir(), "other")
	view, err = reopened.InspectLaunchSchemaMigration(ctx, other)
	if err != nil || view.State.Choice != nil {
		t.Fatal("repository permission leaked", view, err)
	}
	other = request.Scope
	for _, field := range []string{"authority", "project"} {
		other = request.Scope
		if field == "authority" {
			other.AuthorityID = "other"
		} else {
			other.ProjectID = "other"
		}
		if _, err := reopened.InspectLaunchSchemaMigration(ctx, other); err == nil {
			t.Fatal("foreign scope accepted", field)
		}
	}
	after, _ := f.engine.Store.ReadAuthority(ctx, AuthorityPackagesKey)
	if before.Version != after.Version || string(before.Data) != string(after.Data) {
		t.Fatal("choice changed package trust")
	}
	clear := request
	clear.Choice = nil
	clear.ExpectedGeneration = 1
	clear.CommandID = newID("command")
	removed := applyLaunchMigration(t, f.engine, clear)
	if removed.State.Generation != 2 || removed.State.Choice != nil {
		t.Fatal(removed)
	}
	request.ExpectedGeneration = 2
	request.CommandID = newID("command")
	restored := applyLaunchMigration(t, f.engine, request)
	if restored.State.Generation != 3 {
		t.Fatal(restored)
	}
}

func TestLaunchSchemaMigrationChoiceConcurrentSetAndOwnerGuard(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f, request := launchMigrationFixture(t)
	other, err := Open(f.engine.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	var wg sync.WaitGroup
	results := make(chan LaunchSchemaMigrationResult, 2)
	errs := make(chan error, 2)
	for _, e := range []*Engine{f.engine, other} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := request
			req.CommandID = newID("command")
			r, err := e.SetLaunchSchemaMigration(ctx, req)
			results <- r
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	accepted, rejected := 0, 0
	for r := range results {
		if r.Receipt.Rejection == nil {
			accepted++
		} else if r.Receipt.Rejection.Code == "schema_migration_choice_stale" {
			rejected++
		} else {
			t.Fatal(r)
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatal(accepted, rejected)
	}
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
	request.ExpectedGeneration = 1
	request.CommandID = newID("command")
	request.Choice = nil
	denied, err := f.engine.SetLaunchSchemaMigration(ctx, request)
	if err != nil || denied.Receipt.Rejection == nil || denied.Receipt.Rejection.Code != "object_access_denied" {
		t.Fatal(denied, err)
	}
}

func TestLaunchSchemaMigrationChoiceAtomicStartWithAndWithoutQueue(t *testing.T) {
	t.Parallel()
	for _, queued := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain", true: "queue"}[queued], func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			f, request := launchMigrationFixture(t)
			first := applyLaunchMigration(t, f.engine, request)
			f.options.CommandID = newID("command")
			f.options.SchemaMigrationID = request.Choice.DecisionID
			f.options.LaunchSchemaMigration = &first.State
			if queued {
				selection := queueClaimFixture(t, f.engine)
				binding := queueReserveFixture(t, f.engine, selection, f.options.CommandID)
				queueDispatchFixture(t, f.engine, binding)
				f.options.QueueTask = &binding
			}
			// Review generation 1, then clear and restore the identical decision.
			clear := request
			clear.Choice = nil
			clear.ExpectedGeneration = 1
			clear.CommandID = newID("command")
			applyLaunchMigration(t, f.engine, clear)
			request.ExpectedGeneration = 2
			request.CommandID = newID("command")
			current := applyLaunchMigration(t, f.engine, request)
			rejected, err := f.engine.Start(ctx, f.options)
			if err != nil {
				rejectionCode(t, err, "project_start_stale_launch")
			} else if rejected.Receipt.Rejection == nil || rejected.Receipt.Rejection.Code != "project_start_stale_launch" {
				t.Fatal("stale choice crossed atomic guard", rejected)
			}
			if _, err := f.engine.Store.Read(ctx, f.engine.IntendedRunID(f.options.CommandID), 0, 1); !errors.Is(err, local.ErrNotFound) {
				t.Fatal("refused Start persisted Run", err)
			}
			if queued {
				return
			}
			// A new command uses the current choice; the unrelated launch has no effect.
			f.options.CommandID = newID("command")
			unrelated := request
			unrelated.Scope.Launch = "unrelated"
			unrelated.ExpectedGeneration = 0
			unrelated.CommandID = newID("command")
			applyLaunchMigration(t, f.engine, unrelated)
			f.options.LaunchSchemaMigration = &current.State
			accepted, err := f.engine.Start(ctx, f.options)
			if err != nil || accepted.Receipt.Rejection != nil {
				t.Fatal(accepted, err)
			}
			run := driverRun(t, f.engine, accepted.Receipt.RunID)
			if run.SchemaVersion != CoreSchemaMigrationStateVersion || run.SchemaMigration == nil {
				t.Fatal("changed migration boundary")
			}
		})
	}
}

func TestLaunchSchemaMigrationChoiceLifecycleAndClosedState(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f, request := launchMigrationFixture(t)
	first := applyLaunchMigration(t, f.engine, request)
	for _, field := range []string{"schema_version", "unknown"} {
		data, _ := json.Marshal(first.State)
		var object map[string]any
		_ = json.Unmarshal(data, &object)
		object[field] = "unknown"
		bad, _ := json.Marshal(object)
		if _, err := decodeLaunchSchemaMigration(bad, request.Scope); err == nil {
			t.Fatal("accepted unknown state", field)
		}
	}
	revoke, err := f.engine.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: newID("command"), ID: f.legacy.Ref.ID, Version: f.legacy.Ref.Version, Status: PackageRevoked, Reason: "new withdrawal"})
	if err != nil || revoke.Receipt.Rejection != nil {
		t.Fatal(revoke, err)
	}
	inspection, err := f.engine.InspectLaunchSchemaMigration(ctx, request.Scope)
	if err != nil || inspection.Valid || len(inspection.Reasons) == 0 {
		t.Fatal(inspection, err)
	}
	clear := request
	clear.Choice = nil
	clear.ExpectedGeneration = 1
	clear.CommandID = newID("command")
	applyLaunchMigration(t, f.engine, clear)
	// An accepted set returns its original receipt even after lifecycle changed.
	replay, err := f.engine.SetLaunchSchemaMigration(ctx, request)
	if err != nil || !replay.Duplicate || replay.State.Generation != 1 {
		t.Fatal(replay, err)
	}
}

func TestLaunchSchemaMigrationQueueSavedChoiceStarts(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f, request := launchMigrationFixture(t)
	current := applyLaunchMigration(t, f.engine, request)
	f.options.CommandID = newID("command")
	selection := queueClaimFixture(t, f.engine)
	binding := queueReserveFixture(t, f.engine, selection, f.options.CommandID)
	queueDispatchFixture(t, f.engine, binding)
	f.options.QueueTask = &binding
	f.options.SchemaMigrationID = request.Choice.DecisionID
	f.options.LaunchSchemaMigration = &current.State
	result, err := f.engine.Start(ctx, f.options)
	if err != nil || result.Receipt.Rejection != nil {
		t.Fatal(result, err)
	}
	run := driverRun(t, f.engine, result.Receipt.RunID)
	if run.QueueTask == nil || run.SchemaMigration == nil || run.SchemaVersion != CoreSchemaMigrationStateVersion {
		t.Fatal("joint saved choice binding lost")
	}
	if err := validatePublic(t, "CoreRunStateV45", run); err != nil {
		t.Fatal(err)
	}
	clear := request
	clear.Choice = nil
	clear.ExpectedGeneration = 1
	clear.CommandID = newID("command")
	applyLaunchMigration(t, f.engine, clear)
	unchanged := driverRun(t, f.engine, result.Receipt.RunID)
	if unchanged.SchemaMigration.DecisionID != request.Choice.DecisionID {
		t.Fatal("clear changed accepted Run")
	}
}

func TestLaunchSchemaMigrationChoiceKeepsLifecycleIntegrityAndScopeRefusals(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"supplier-quarantine", "consumer-quarantine", "tampered-supplier", "revoked-code", "foreign-consumer", "foreign-workflow"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			f, request := launchMigrationFixture(t)
			current := applyLaunchMigration(t, f.engine, request)
			build := request.Choice.LaunchSchemaMigrationBuild
			want := "schema_migration_decision_invalid"
			switch kind {
			case "supplier-quarantine", "consumer-quarantine", "revoked-code":
				pkg, status := f.supplier, PackageQuarantined
				if kind == "consumer-quarantine" || kind == "revoked-code" {
					pkg = f.consumer
				}
				if kind == "revoked-code" {
					status = PackageRevoked
				}
				r, err := f.engine.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: newID("command"), ID: pkg.Ref.ID, Version: pkg.Ref.Version, Status: status, Reason: "changed trust"})
				if err != nil || r.Receipt.Rejection != nil {
					t.Fatal(r, err)
				}
			case "tampered-supplier":
				path := filepath.Join(f.engine.Root, f.supplier.Components[0].Path)
				if err := os.Chmod(path, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(`{"type":"null"}`), 0600); err != nil {
					t.Fatal(err)
				}
				want = "schema_migration_integrity"
			case "foreign-consumer":
				build.ConsumerRef = f.supplier.Ref
				want = "schema_migration_scope_mismatch"
			case "foreign-workflow":
				build.WorkflowRef.Digest = rawDigest([]byte("different workflow"))
				want = "schema_migration_scope_mismatch"
			}
			_, err := f.engine.CheckLaunchSchemaMigration(ctx, request.Choice.DecisionID, build)
			migrationRefusalCode(t, err, want)
			if kind == "foreign-consumer" || kind == "foreign-workflow" {
				return
			}
			inspection, err := f.engine.InspectLaunchSchemaMigration(ctx, request.Scope)
			if err != nil || inspection.Valid || len(inspection.Reasons) == 0 || inspection.State.Generation != 1 {
				t.Fatal(inspection, err)
			}
			f.options.CommandID = newID("command")
			f.options.SchemaMigrationID = request.Choice.DecisionID
			f.options.LaunchSchemaMigration = &current.State
			if _, err := f.engine.Start(ctx, f.options); err == nil {
				t.Fatal("saved choice bypassed admission")
			}
			if _, err := f.engine.Store.Read(ctx, f.engine.IntendedRunID(f.options.CommandID), 0, 1); !errors.Is(err, local.ErrNotFound) {
				t.Fatal("refused start created a Run", err)
			}
			clear := request
			clear.Choice = nil
			clear.ExpectedGeneration = 1
			clear.CommandID = newID("command")
			applyLaunchMigration(t, f.engine, clear)
		})
	}
}

func TestLaunchSchemaMigrationChoiceFreshAuthorityClearIsIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "authority")
	if err := InitProjectProfile(root); err != nil {
		t.Fatal(err)
	}
	e, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	scope := LaunchSchemaMigrationScope{AuthorityID: e.Installation.ID, ProjectID: e.Config.ID, Repository: filepath.Join(t.TempDir(), "repository"), Launch: "absent"}
	inspection, err := e.InspectLaunchSchemaMigration(ctx, scope)
	if err != nil || inspection.State.Choice != nil || inspection.State.Generation != 0 {
		t.Fatal(inspection, err)
	}
	if _, version, err := e.Control(ctx); err != nil || version != 0 {
		t.Fatal("read-only inspection enrolled the control plane", version, err)
	}
	if _, err := e.Store.ReadAuthority(ctx, launchSchemaMigrationKey(scope)); !errors.Is(err, local.ErrNotFound) {
		t.Fatal("read-only inspection materialized a choice", err)
	}
	request := LaunchSchemaMigrationRequest{Scope: scope, ExpectedGeneration: 0, CommandID: newID("command"), Reason: "clear absent choice"}
	first := applyLaunchMigration(t, e, request)
	if first.State.Choice != nil || first.State.Generation != 0 {
		t.Fatal("no-op clear advanced the choice", first)
	}
	if _, version, err := e.Control(ctx); err != nil || version == 0 {
		t.Fatal("owner command did not enroll control", version, err)
	}
	replayed := applyLaunchMigration(t, e, request)
	if !replayed.Duplicate || replayed.Receipt.Cut != first.Receipt.Cut {
		t.Fatal("no-op clear retry changed its receipt", replayed)
	}
	changed := request
	changed.Reason = "different clear intent"
	if _, err := e.SetLaunchSchemaMigration(ctx, changed); !errors.Is(err, local.ErrCommandConflict) {
		t.Fatal("changed no-op clear intent reused the receipt", err)
	}
	t.Log("[FIX:24] fresh clear records one no-op receipt after read-only inspection; retry returns the same receipt")
}

func TestLaunchSchemaMigrationReplayRequiresAtomicAcceptance(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f, request := launchMigrationFixture(t)
	selected := applyLaunchMigration(t, f.engine, request)
	fingerprint := rawDigest([]byte("original external request"))
	response := json.RawMessage(`{"original":"reviewed metadata"}`)
	f.options.CommandID = newID("command")
	f.options.SchemaMigrationID = request.Choice.DecisionID
	f.options.LaunchSchemaMigration = &selected.State
	f.options.SavedLaunchReplay = &SavedLaunchSchemaMigrationReplay{SchemaVersion: "saved-launch-replay/1", Scope: request.Scope, Fingerprint: fingerprint, Response: response}
	data, runID, err := f.engine.AcceptedLaunchSchemaMigrationReplay(ctx, request.Scope, f.options.CommandID, fingerprint)
	if err != nil || len(data) != 0 || runID != "" {
		t.Fatal("unaccepted command recovered", string(data), runID, err)
	}
	accepted, err := f.engine.Start(ctx, f.options)
	if err != nil || accepted.Receipt.Rejection != nil {
		t.Fatal(accepted, err)
	}
	if err := validateInBundle(t, publicContracts, "local_Receipt", accepted.Receipt); err != nil {
		t.Fatal(err)
	}
	// Change the saved scope after admission. Receipt recovery needs no new grant.
	request.Choice = nil
	request.ExpectedGeneration = 1
	request.CommandID = newID("command")
	applyLaunchMigration(t, f.engine, request)
	data, runID, err = f.engine.AcceptedLaunchSchemaMigrationReplay(ctx, request.Scope, f.options.CommandID, fingerprint)
	if err != nil || runID != accepted.Receipt.RunID || string(data) != string(response) {
		t.Fatal("accepted metadata lost", string(data), runID, err)
	}
	for _, changedScope := range []LaunchSchemaMigrationScope{request.Scope, {AuthorityID: request.Scope.AuthorityID, ProjectID: request.Scope.ProjectID, Repository: request.Scope.Repository, Launch: "other"}} {
		changedFingerprint := fingerprint
		if changedScope == request.Scope {
			changedFingerprint = rawDigest([]byte("different request"))
		}
		if _, _, err := f.engine.AcceptedLaunchSchemaMigrationReplay(ctx, changedScope, f.options.CommandID, changedFingerprint); !errors.Is(err, local.ErrCommandConflict) {
			t.Fatal("changed request replayed", err)
		}
	}
	// The same stale precondition under a new identity records no replay context.
	f.options.CommandID = newID("command")
	refused, err := f.engine.Start(ctx, f.options)
	if err != nil {
		rejectionCode(t, err, "project_start_stale_launch")
	} else if refused.Receipt.Rejection == nil {
		t.Fatal("stale start accepted", refused)
	}
	data, runID, err = f.engine.AcceptedLaunchSchemaMigrationReplay(ctx, request.Scope, f.options.CommandID, fingerprint)
	if len(data) != 0 || runID != "" {
		t.Fatal("rejected start recovered", string(data), runID, err)
	}
	t.Log("[FIX:24] only an atomically accepted receipt supplies replay metadata")
}

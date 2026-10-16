package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// JSON Schema permits arbitrary property names. The literal $pinned property
// selects the store's supported unpacked representation without corrupting a
// row or inventing an unknown Run field.
func populateClaimScan(t *testing.T, e *Engine, sourceID string, packed bool, count int) int64 {
	t.Helper()
	base, _, err := e.load(context.Background(), sourceID)
	if err != nil {
		t.Fatal(err)
	}
	schema := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "description": strings.Repeat("x", 1<<20)}
	if !packed {
		schema["properties"] = map[string]any{"$pinned": map[string]any{"type": "string"}}
	}
	data, err := canonical(schema)
	if err != nil {
		t.Fatal(err)
	}
	ref := flow.Ref{ID: "test:schema/scan-padding", Version: "1.0.0", Digest: rawDigest(data)}
	base.Definitions = append(base.Definitions, PinnedDefinition{Ref: ref, Kind: "schema", RawDigest: ref.Digest, Bytes: data})
	closure := []flow.Ref{}
	for _, d := range base.Definitions {
		if d.Ref != base.LockRef {
			closure = append(closure, d.Ref)
		}
	}
	for _, resource := range base.ContextResources {
		closure = append(closure, resource.Ref)
	}
	sort.Slice(closure, func(i, j int) bool { return closure[i].String() < closure[j].String() })
	for i := range base.Definitions {
		if base.Definitions[i].Ref == base.LockRef {
			var lock map[string]any
			if err := json.Unmarshal(base.Definitions[i].Bytes, &lock); err != nil {
				t.Fatal(err)
			}
			lock["closure"] = closure
			lockData, err := canonical(lock)
			if err != nil {
				t.Fatal(err)
			}
			base.LockRef.Digest = rawDigest(lockData)
			base.Definitions[i] = PinnedDefinition{Ref: base.LockRef, Kind: "resource", RawDigest: base.LockRef.Digest, Bytes: lockData}
			break
		}
	}
	var total int64
	for i := range count {
		base.ID = fmt.Sprintf("run:!scan-%04d", i)
		for _, inv := range base.Invocations {
			inv.RunID = base.ID
		}
		snapshot, err := canonicalState(base)
		if err != nil {
			t.Fatal(err)
		}
		var decoded Run
		if decoded, err = e.decodeSnapshot(local.Snapshot{RunID: base.ID, Data: snapshot}); err != nil || !supportedRun(decoded) {
			t.Fatalf("invalid fixture: %v", err)
		}
		zero := int64(0)
		result, err := e.Store.Apply(context.Background(), local.Command{ID: fmt.Sprintf("command:scan-%04d", i), Actor: e.owner, RunID: base.ID, Payload: json.RawMessage(`{"fixture":true}`), ExpectedVersion: &zero, Mode: local.CommandCAS}, func(local.Snapshot) (local.Change, error) {
			return local.Change{Data: snapshot, Events: []local.EventInput{{Type: "run.created", Data: json.RawMessage(`{"fixture":true}`)}}}, nil
		})
		if err != nil || result.Receipt.Rejection != nil {
			t.Fatalf("seed: %+v %v", result.Receipt.Rejection, err)
		}
		total += int64(len(snapshot))
	}
	return total
}

func TestUnboundClaimReleaseBeyondRestoredScanBudget(t *testing.T) {
	t.Parallel()
	for _, packed := range []bool{true, false} {
		t.Run(fmt.Sprintf("packed-%t", packed), func(t *testing.T) {
			t.Parallel()
			e, runID, claim := claimRunFixture(t)
			total := populateClaimScan(t, e, runID, packed, 65)
			if total <= 64<<20 {
				t.Fatalf("fixture restored only %d bytes", total)
			}
			released, err := e.ReleaseWorktree(context.Background(), ClaimReleaseRequest{CommandID: "command:large-release", ClaimID: claim.ID, Generation: claim.Generation})
			if err != nil || released.Status != "released" {
				t.Fatalf("free unbound claim blocked by %d restored bytes: status=%s error=%v", total, released.Status, err)
			}
		})
	}
}

func TestClaimOwnershipScanFindsLateLegacyHolderAndUnknownState(t *testing.T) {
	t.Parallel()
	for _, unknown := range []bool{false, true} {
		t.Run(fmt.Sprintf("unknown-%t", unknown), func(t *testing.T) {
			t.Parallel()
			e, runID, claim := claimRunFixture(t)
			ctx := context.Background()
			// The actual holder sorts after every tiny synthetic Run; count is a
			// separate boundary from the restored-byte fixture above.
			base, _, err := e.load(ctx, runID)
			if err != nil {
				t.Fatal(err)
			}
			prefix := "run:!scan-"
			for i := range 1001 {
				base.ID = fmt.Sprintf("%s%04d", prefix, i)
				for _, inv := range base.Invocations {
					inv.RunID = base.ID
				}
				data, err := canonicalState(base)
				if err != nil {
					t.Fatal(err)
				}
				_, err = e.Store.Apply(ctx, local.Command{ID: fmt.Sprintf("command:tiny-%d", i), Actor: e.owner, RunID: base.ID, Payload: json.RawMessage(`{}`), Mode: local.CommandCAS, ExpectedVersion: new(int64)}, func(local.Snapshot) (local.Change, error) {
					return local.Change{Data: data, Events: []local.EventInput{{Type: "run.created", Data: json.RawMessage(`{}`)}}}, nil
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			if unknown {
				view, err := e.Store.Read(ctx, runID, 0, 1)
				if err != nil {
					t.Fatal(err)
				}
				var doc map[string]any
				if err := json.Unmarshal(view.Snapshot.Data, &doc); err != nil {
					t.Fatal(err)
				}
				doc["schema_version"] = "core-state/999"
				data, _ := json.Marshal(doc)
				_, err = e.Store.Apply(ctx, local.Command{ID: "command:unknown", Actor: e.owner, RunID: runID, Payload: json.RawMessage(`{}`), Mode: local.CommandCAS, ExpectedVersion: &view.Snapshot.Version}, func(local.Snapshot) (local.Change, error) {
					return local.Change{Data: data, Events: []local.EventInput{{Type: "diagnostic.recorded", Data: json.RawMessage(`{}`)}}}, nil
				})
				if err != nil {
					t.Fatal(err)
				}
			} else {
				_ = handOver(t, e, runID)
				_, err = e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: "command:legacy", Actor: e.owner, Key: AuthorityClaimsKey, Payload: json.RawMessage(`{}`)}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
					record, err := e.decodeClaims(s)
					if err != nil {
						return local.AuthorityChange{}, err
					}
					record.SchemaVersion = authorityClaimsModeVersion
					for i := range record.Claims {
						record.Claims[i].RunID = ""
					}
					data, err := canonicalState(record)
					return local.AuthorityChange{Data: data}, err
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := e.prepareClaimRunBinding(ctx, "run:next", claim.ID, claim.Generation); refusalCode(err) != "claim_owner_unproven" {
				t.Fatalf("binding bypassed tail: %v", err)
			}
			if _, err := e.ReleaseWorktree(ctx, ClaimReleaseRequest{CommandID: "command:tail-release", ClaimID: claim.ID, Generation: claim.Generation}); refusalCode(err) != "claim_owner_unproven" {
				t.Fatalf("release bypassed tail: %v", err)
			}
			view, err := e.DiagnoseClaim(ctx, claim.ID)
			if err == nil || view.Scan.Complete || view.Scan.Records < 1002 {
				t.Fatalf("diagnostic claimed completeness: %+v %v", view, err)
			}
			expected := "blocked"
			if unknown {
				expected = "unknown"
			}
			if view.Assessment != expected {
				t.Fatalf("assessment=%s", view.Assessment)
			}
		})
	}
}

func TestClaimOwnershipDiagnosticsAndNamedScanProblem(t *testing.T) {
	t.Parallel()
	e, _, claim := claimRunFixture(t)
	before, beforeCut, err := e.Store.ReadAll(context.Background(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	view, err := e.DiagnoseClaim(context.Background(), claim.ID)
	if err != nil || !view.Scan.Complete || view.Assessment != "releasable" || view.Storage == nil || view.RunID != "" {
		t.Fatalf("diagnostics: %+v %v", view, err)
	}
	after, afterCut, err := e.Store.ReadAll(context.Background(), 1000)
	if err != nil || len(before) != len(after) || beforeCut != afterCut {
		t.Fatal("diagnostics mutated runs", err)
	}
	digest, _ := flow.Digest(ClaimOwnershipSchema)
	ref := flow.Ref{ID: "test:schema/claim-ownership", Version: "1.0.0", Digest: digest}
	data, _ := json.Marshal(view)
	if err := flow.ValidateSchema(flow.Registry{ref: ClaimOwnershipSchema}, ref, data); err != nil {
		t.Fatal(err)
	}
	var extra map[string]any
	_ = json.Unmarshal(data, &extra)
	extra["snapshot"] = map[string]any{}
	bad, _ := json.Marshal(extra)
	if err := flow.ValidateSchema(flow.Registry{ref: ClaimOwnershipSchema}, ref, bad); err == nil {
		t.Fatal("diagnostic schema accepted a foreign field")
	}
	problem, exit := ProblemFor(&local.ScanLimitError{Records: 1001, RestoredBytes: 8000, RecordLimit: 1000, ByteLimit: local.MaxScanBytes})
	if problem.Code != "authority_scan_budget_exhausted" || exit != 5 || !strings.Contains(problem.Message, "1001") {
		t.Fatalf("wrong scan Problem: %+v exit %d", problem, exit)
	}
}

func TestClaimBindingProofConflictsAfterExistingRunChanges(t *testing.T) {
	t.Parallel()
	e, runID, claim := claimRunFixture(t)
	ctx := context.Background()
	proof, err := e.prepareClaimRunBinding(ctx, runID, claim.ID, claim.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if proof.ReadCut == nil {
		t.Fatal("unbound binding has no ownership cut")
	}
	original, err := e.Store.Read(ctx, runID, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	// A writer changes the existing Run without changing population or claims.
	_, err = e.Store.Apply(ctx, local.Command{ID: "command:proof-race", Actor: e.owner, RunID: runID, Payload: json.RawMessage(`{}`), Mode: local.CommandCAS, ExpectedVersion: &original.Snapshot.Version}, func(s local.Snapshot) (local.Change, error) {
		return local.Change{Data: s.Data, Events: []local.EventInput{{Type: "diagnostic.recorded", Data: json.RawMessage(`{}`)}}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err := e.Store.Read(ctx, runID, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	_, err = e.applyWithReadCut(ctx, proof.ReadCut, &proof.Pin, nil, nil, proof.mutate, nil, e.owner, "command:stale-binding", runID, "diagnostic.recorded", map[string]any{}, &current.Snapshot.Version, local.CommandCAS, func(*Run, local.Snapshot, Observation) (local.Change, error) {
		called = true
		return local.Change{}, nil
	})
	if refusalCode(err) != "claim_ownership_conflict" || called {
		t.Fatalf("stale binding committed: %v called=%t", err, called)
	}
	untouched, err := e.claim(ctx, claim.ID)
	if err != nil || untouched.RunID != "" || untouched.Generation != claim.Generation {
		t.Fatalf("claim changed: %+v %v", untouched, err)
	}
}

func TestStartOutcomeUsesDurableRunAndReceipt(t *testing.T) {
	t.Parallel()
	e, runID, _ := claimRunFixture(t)
	ctx := context.Background()
	view, err := e.Store.Read(ctx, runID, 0, 1)
	if err != nil || len(view.Events) != 1 {
		t.Fatal("read creation", err)
	}
	outcome, err := e.StartOutcome(ctx, view.Events[0].CommandID)
	if err != nil || outcome != runID {
		t.Fatalf("lost response not reconciled: %s %v", outcome, err)
	}
	if outcome, err := e.StartOutcome(ctx, "command:never-started"); err != nil || outcome != "" {
		t.Fatalf("absence not proved: %s %v", outcome, err)
	}
	if err := e.Store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := e.StartOutcome(ctx, "command:unknown"); err == nil {
		t.Fatal("unavailable authority guessed absence")
	}
}

func TestRollbackPreservesAnAdmittedTerminalRun(t *testing.T) {
	t.Parallel()
	e, runID, claim := claimRunFixture(t)
	ctx := context.Background()
	binding, err := e.prepareClaimRunBinding(ctx, runID, claim.ID, claim.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if err := commitClaimBinding(t, e, runID, binding, false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "cancel", Reason: "fixture terminal Run"}); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	if !claimRunFinished(driverRun(t, e, runID)) {
		t.Fatal("Run is not terminal")
	}
	request := ClaimReleaseRequest{CommandID: "command:terminal-rollback", ClaimID: claim.ID, Generation: claim.Generation}
	if _, err := e.RollbackWorktreeClaim(ctx, request); refusalCode(err) != "claim_run_conflict" {
		t.Fatalf("rollback released admitted Run: %v", err)
	}
	request.CommandID = "command:operator-release"
	if released, err := e.ReleaseWorktree(ctx, request); err != nil || released.Status != "released" {
		t.Fatalf("normal release lost its terminal predicate: %+v %v", released, err)
	}
}

func TestClaimOwnershipScanFindsHolderBeyondRestoredByteLimit(t *testing.T) {
	t.Parallel()
	e, runID, claim := claimRunFixture(t)
	ctx := context.Background()
	total := populateClaimScan(t, e, runID, true, 65)
	_ = handOver(t, e, runID)
	_, err := e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: "command:large-legacy", Actor: e.owner, Key: AuthorityClaimsKey, Payload: json.RawMessage(`{}`)}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
		record, err := e.decodeClaims(s)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		record.SchemaVersion = authorityClaimsModeVersion
		for i := range record.Claims {
			record.Claims[i].RunID = ""
		}
		data, err := canonicalState(record)
		return local.AuthorityChange{Data: data}, err
	})
	if err != nil {
		t.Fatal(err)
	}
	diagnostic, err := e.DiagnoseClaim(ctx, claim.ID)
	if err == nil || diagnostic.Assessment != "blocked" || diagnostic.HolderRunID != runID || diagnostic.RunID != "" || diagnostic.Scan.RestoredBytes < total || diagnostic.Scan.Records != 66 {
		t.Fatalf("late holder was not checked: %+v %v", diagnostic, err)
	}
	if _, err := e.prepareClaimRunBinding(ctx, "run:new-owner", claim.ID, claim.Generation); refusalCode(err) != "claim_owner_unproven" {
		t.Fatalf("binding bypassed byte tail: %v", err)
	}
	if _, err := e.ReleaseWorktree(ctx, ClaimReleaseRequest{CommandID: "command:large-held-release", ClaimID: claim.ID, Generation: claim.Generation}); refusalCode(err) != "claim_owner_unproven" {
		t.Fatalf("release bypassed byte tail: %v", err)
	}
}

func TestClaimDiagnosisRejectsClaimsChangedDuringObservation(t *testing.T) {
	t.Parallel()
	e, _, claim := claimRunFixture(t)
	ctx := context.Background()
	_, version, err := e.readClaims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := e.DiagnoseClaim(ctx, claim.ID)
	if err != nil || observed.Assessment != "releasable" {
		t.Fatal(err)
	}
	// Deterministic interleaving between the observation and its final ledger
	// check, without timing or a process-wide test hook.
	if _, err := e.HeartbeatClaim(ctx, ClaimHeartbeatRequest{CommandID: "command:diagnostic-race", ClaimID: claim.ID, Generation: claim.Generation}); err != nil {
		t.Fatal(err)
	}
	if err := e.confirmClaimDiagnosis(ctx, version, &observed); refusalCode(err) != "claim_ownership_conflict" || observed.Assessment != "unknown" || observed.Reason != "claims_changed" {
		t.Fatalf("mixed observation accepted: %+v %v", observed, err)
	}
	fresh, err := e.DiagnoseClaim(ctx, claim.ID)
	if err != nil || fresh.Assessment != "releasable" || !fresh.Scan.Complete {
		t.Fatalf("fresh observation failed: %+v %v", fresh, err)
	}
}

func TestConcurrentDuplicateClaimHasOnlyOneCreatingCaller(t *testing.T) {
	t.Parallel()
	e := contextRegistryRuntime(t)
	request := ClaimRequest{CommandID: "command:concurrent-claim", Repository: gitRepository(t), OwnerID: "project-launch:concurrent", WorkspaceMode: "checkout"}
	type outcome struct {
		claim   WorktreeClaim
		created bool
		err     error
	}
	results := make(chan outcome, 2)
	for range 2 {
		go func() {
			claim, created, err := e.ClaimWorktreeWithOutcome(context.Background(), request)
			results <- outcome{claim, created, err}
		}()
	}
	createdCount := 0
	ids := map[string]bool{}
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.created {
			createdCount++
		}
		ids[result.claim.ID] = true
	}
	if createdCount != 1 || len(ids) != 1 {
		t.Fatalf("duplicate acquired cleanup rights: created=%d ids=%v", createdCount, ids)
	}
	duplicate, created, err := e.ClaimWorktreeWithOutcome(context.Background(), request)
	if err != nil || created || duplicate.Status != "active" {
		t.Fatalf("retry claimed new creation: %+v created=%t %v", duplicate, created, err)
	}
}

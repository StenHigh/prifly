package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stenhigh/prifly/internal/local"
)

func claimRunFixture(t *testing.T) (*Engine, string, WorktreeClaim) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	return assistedWorkspaceFixture(t, "checkout")
}

// Exercise the same coupled authority/Run transaction as admission, without
// inventing a worker outcome. The caller selects whether its Run reducer fails.
func commitClaimBinding(t *testing.T, e *Engine, runID string, binding *claimRunBinding, reject bool) error {
	t.Helper()
	_, view, err := e.load(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.applyControlledWithControlMutation(context.Background(), &binding.Pin, nil, binding.mutate, e.owner, newID("command"), runID, "diagnostic.recorded", map[string]any{"claim_binding_test": true}, &view.Snapshot.Version, local.CommandCAS, func(*Run, local.Snapshot, Observation) (local.Change, error) {
		if reject {
			return local.Change{}, fault("admission_blocked", "test rejects the Run transition")
		}
		return local.Change{}, nil
	})
	return err
}

func TestClaimBindingCommitIsAtomic(t *testing.T) {
	e, runID, claim := claimRunFixture(t)
	ctx := context.Background()
	binding, err := e.prepareClaimRunBinding(ctx, runID, claim.ID, claim.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if err := commitClaimBinding(t, e, runID, binding, true); refusalCode(err) != "admission_blocked" {
		t.Fatalf("rejected Run transition returned %v", err)
	}
	unbound, err := e.claim(ctx, claim.ID)
	if err != nil || unbound.RunID != "" {
		t.Fatalf("rejected Run mutation still bound its claim: %+v %v", unbound, err)
	}
	if err := commitClaimBinding(t, e, runID, binding, false); err != nil {
		t.Fatal(err)
	}
	if err := commitClaimBinding(t, e, runID, binding, false); refusalCode(err) != "control_conflict" {
		t.Fatalf("stale claim pin committed: %v", err)
	}
	bound, err := e.claim(ctx, claim.ID)
	if err != nil || bound.RunID != runID || bound.Generation != claim.Generation {
		t.Fatalf("coupled mutation did not bind the exact claim: %+v %v", bound, err)
	}
	if _, err := e.prepareClaimRunBinding(ctx, runID, claim.ID, claim.Generation+1); refusalCode(err) != "claim_generation_conflict" {
		t.Fatalf("another generation became admissible: %v", err)
	}
	if _, err := e.ReleaseWorktree(ctx, ClaimReleaseRequest{CommandID: newID("command"), ClaimID: claim.ID, Generation: claim.Generation}); refusalCode(err) != "claim_run_active" {
		t.Fatalf("release removed an unfinished Run's checkout: %v", err)
	}
	if _, err := os.Stat(filepath.Join(claim.Repository.Toplevel, "README.md")); err != nil {
		t.Fatal("refused release changed checkout files", err)
	}
}

// An expired lease is waived for exactly one triple: this claim, bound to this
// Run, asked for by the actor recorded on it. Every other case still stands on it.
func TestExpiredLeaseAdmitsOnlyTheReturningOwner(t *testing.T) {
	lapsed := WorktreeClaim{
		Status: "active", RunID: "run:one", Actor: "local:uid:501",
		Claimed: Observation{UTC: "2026-09-01T00:00:00Z"}, LeaseUntil: "2026-09-01T00:30:00Z",
	}
	observed := Observation{UTC: "2026-09-01T02:00:00Z"}
	for _, c := range []struct {
		name     string
		recorded func(*WorktreeClaim)
		runID    string
		actor    string
		code     string
	}{
		{"returning owner", func(*WorktreeClaim) {}, "run:one", "local:uid:501", ""},
		{"unbound claim", func(c *WorktreeClaim) { c.RunID = "" }, "run:one", "local:uid:501", "claim_owner_unproven"},
		{"another actor", func(*WorktreeClaim) {}, "run:one", "local:uid:999999", "claim_owner_unproven"},
		{"another Run", func(*WorktreeClaim) {}, "run:two", "local:uid:501", "claim_run_conflict"},
		{"no recorded actor", func(c *WorktreeClaim) { c.Actor = "" }, "run:one", "", "claim_owner_unproven"},
	} {
		t.Run(c.name, func(t *testing.T) {
			claim := lapsed
			c.recorded(&claim)
			err := claimAdmissibleForRun(claim, c.runID, c.actor, observed)
			if c.code == "" {
				if err != nil {
					t.Fatalf("the returning owner was refused its own claim: %v", err)
				}
				return
			}
			if code := refusalCode(err); code != c.code {
				t.Fatalf("wanted %s, got %s (%v)", c.code, code, err)
			}
		})
	}
}

// The pilot's live run: three steps admitted, and between them the owner's own
// pause outlived the lease. Nothing is observable in that pause by construction,
// so the fourth step must still be admissible without cancelling the Run.
func TestOwnerPauseBetweenStepsKeepsWorkspaceOwnership(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	// The pilot measured ninety minutes between two of its steps; nights are longer.
	const ownerPause = 90 * time.Minute
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		e, runID, claim := assistedWorkspaceFixture(t, "checkout")
		for step := 1; step <= 3; step++ {
			binding, err := e.prepareClaimRunBinding(ctx, runID, claim.ID, claim.Generation)
			if err != nil {
				t.Fatalf("step %d was refused admission after an owner's pause: %v", step, err)
			}
			if err := commitClaimBinding(t, e, runID, binding, false); err != nil {
				t.Fatalf("step %d could not commit its admission: %v", step, err)
			}
			admitted, err := e.claim(ctx, claim.ID)
			if err != nil || admitted.RunID != runID || claimPresence(admitted, e.clock.now()) != "present" {
				t.Fatalf("admission did not renew the lease of its own claim: %+v %v", admitted, err)
			}
			time.Sleep(ownerPause)
			lapsed, err := e.claim(ctx, claim.ID)
			if err != nil || claimPresence(lapsed, e.clock.now()) != "suspected" {
				t.Fatalf("the pause did not outlive the lease, so nothing was reproduced: %+v %v", lapsed, err)
			}
		}
		// The fourth admission is the real one: the Run hands its work over and
		// finishes on the same claim, with no cancellation in between.
		task := handOver(t, e, runID)
		if _, err := e.SubmitSession(ctx, hostResult(t, e, task, "fourth step after three pauses")); err != nil {
			t.Fatal(err)
		}
		if err := e.Drive(ctx, runID); err != nil {
			t.Fatal(err)
		}
		final := driverRun(t, e, runID)
		bound, err := e.claim(ctx, claim.ID)
		if err != nil || final.Status != "completed" || bound.RunID != runID {
			t.Fatalf("the paused Run lost its workspace or its progress: %s %+v %v", final.Status, bound, err)
		}
	})
}

func TestClaimReleaseFencesPreparedAdmission(t *testing.T) {
	e, runID, claim := claimRunFixture(t)
	ctx := context.Background()
	binding, err := e.prepareClaimRunBinding(ctx, runID, claim.ID, claim.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.ReleaseWorktree(ctx, ClaimReleaseRequest{CommandID: newID("command"), ClaimID: claim.ID, Generation: claim.Generation}); err != nil {
		t.Fatal(err)
	}
	if err := commitClaimBinding(t, e, runID, binding, false); refusalCode(err) != "control_conflict" {
		t.Fatalf("prepared admission crossed the release fence: %v", err)
	}
	if run := driverRun(t, e, runID); len(run.Attempts) != 0 {
		t.Fatal("released checkout still acquired an Attempt")
	}
}

func TestClaimReleaseCannotOverlapDriverPreparation(t *testing.T) {
	e, runID, claim := claimRunFixture(t)
	ctx := context.Background()
	before, version, err := e.readClaims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	claimBytes, err := canonical(before)
	if err != nil {
		t.Fatal(err)
	}
	checkoutFile := filepath.Join(claim.Repository.Toplevel, "README.md")
	checkoutBytes, err := os.ReadFile(checkoutFile)
	if err != nil {
		t.Fatal(err)
	}
	lock, err := e.driverLock(runID)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	request := ClaimReleaseRequest{CommandID: newID("command"), ClaimID: claim.ID, Generation: claim.Generation}
	if _, err := e.ReleaseWorktree(ctx, request); refusalCode(err) != "driver_already_active" {
		t.Fatalf("release crossed workspace preparation: %v", err)
	}
	after, afterVersion, err := e.readClaims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	afterBytes, err := canonical(after)
	if err != nil || version != afterVersion || !bytes.Equal(claimBytes, afterBytes) || !e.driverLiveFor(runID) {
		t.Fatalf("refused release changed the claim or driver ownership: %v", err)
	}
	currentBytes, err := os.ReadFile(checkoutFile)
	if err != nil || !bytes.Equal(checkoutBytes, currentBytes) {
		t.Fatalf("refused release changed checkout bytes: %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	released, err := e.ReleaseWorktree(ctx, request)
	if err != nil || released.Status != "released" {
		t.Fatalf("release could not continue after preparation unlocked: %+v %v", released, err)
	}
	currentBytes, err = os.ReadFile(checkoutFile)
	if err != nil || !bytes.Equal(checkoutBytes, currentBytes) {
		t.Fatalf("checkout release changed repository bytes: %v", err)
	}
}

func TestClaimBindingSurvivesRestartAndTerminalRelease(t *testing.T) {
	e, runID, claim := claimRunFixture(t)
	ctx := context.Background()
	task := handOver(t, e, runID)
	root := e.Root
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	e, err := Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	binding, err := e.prepareClaimRunBinding(ctx, runID, claim.ID, claim.Generation)
	if err != nil || binding.Claim.RunID != runID {
		t.Fatalf("same Run lost its binding after reopen: %+v %v", binding, err)
	}
	if _, err := e.prepareClaimRunBinding(ctx, "run:other", claim.ID, claim.Generation); refusalCode(err) != "claim_run_conflict" {
		t.Fatalf("reopen transferred the checkout to another Run: %v", err)
	}
	if _, err := e.SubmitSession(ctx, hostResult(t, e, task, "completed before explicit release")); err != nil {
		t.Fatal(err)
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(claim.Repository.Toplevel, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	released, err := e.ReleaseWorktree(ctx, ClaimReleaseRequest{CommandID: newID("command"), ClaimID: claim.ID, Generation: claim.Generation})
	if err != nil || released.Status != "released" || released.RunID != runID {
		t.Fatalf("terminal Run could not explicitly release its claim: %+v %v", released, err)
	}
	after, err := os.ReadFile(filepath.Join(claim.Repository.Toplevel, "README.md"))
	if err != nil || string(after) != string(before) {
		t.Fatal("checkout release changed repository files", err)
	}
}

func TestLegacyHandedClaimCannotBeAutomaticallyBound(t *testing.T) {
	e, runID, claim := claimRunFixture(t)
	ctx := context.Background()
	_ = handOver(t, e, runID)
	// Model an authority written before bindings existed; Run bytes, its
	// admitted Attempt and the handed filesystem identity remain untouched.
	_, err := e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: newID("command"), Actor: e.owner, Key: AuthorityClaimsKey, Payload: json.RawMessage(`{"legacy_claim_fixture":true}`)}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
		record, err := e.decodeClaims(s)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		record.SchemaVersion = authorityClaimsModeVersion
		for index := range record.Claims {
			record.Claims[index].RunID = ""
		}
		data, err := canonicalState(record)
		return local.AuthorityChange{Data: data}, err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{runID, "run:other"} {
		if _, err := e.prepareClaimRunBinding(ctx, owner, claim.ID, claim.Generation); refusalCode(err) != "claim_owner_unproven" {
			t.Fatalf("legacy claim silently assigned to %s: %v", owner, err)
		}
	}
	if _, err := e.ReleaseWorktree(ctx, ClaimReleaseRequest{CommandID: newID("command"), ClaimID: claim.ID, Generation: claim.Generation}); refusalCode(err) != "claim_owner_unproven" {
		t.Fatalf("unsettled legacy holder lost its checkout: %v", err)
	}
	for _, version := range []string{authorityClaimsLegacyVersion, authorityClaimsModeVersion} {
		data := json.RawMessage(`{"schema_version":"` + version + `","authority_id":"` + e.Installation.ID + `","claims":[{"run_id":null}]}`)
		if _, err := e.decodeClaims(local.AuthoritySnapshot{Version: 1, Data: data}); refusalCode(err) != "unsupported_claim_contract" {
			t.Fatalf("old claim contract accepted even a null Run binding: %v", err)
		}
	}
}

func TestClaimReleaseCleanupFailureStaysFencedAcrossRestart(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	claim, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: newID("command"), Repository: gitRepository(t), OwnerID: "session:cleanup"})
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(e.Root, filepath.FromSlash(claim.Path))
	original := target + ".preserved"
	if err := os.Rename(target, original); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	request := ClaimReleaseRequest{CommandID: newID("command"), ClaimID: claim.ID, Generation: claim.Generation}
	if _, err := e.ReleaseWorktree(ctx, request); refusalCode(err) != "claim_identity_conflict" {
		t.Fatalf("cleanup removed a substituted directory: %v", err)
	}
	fenced, err := e.claim(ctx, claim.ID)
	if err != nil || fenced.Status != "releasing" || fenced.Released != nil || !fenced.active() {
		t.Fatalf("failed cleanup became a free resource: %+v %v", fenced, err)
	}
	if _, err := e.prepareClaimRunBinding(ctx, "run:other", claim.ID, claim.Generation); refusalCode(err) != "claim_state_conflict" {
		t.Fatalf("releasing claim admitted new work: %v", err)
	}
	if _, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: newID("command"), Repository: claim.Repository.Toplevel, OwnerID: "session:other"}); refusalCode(err) != "claim_conflict" {
		t.Fatalf("failed cleanup admitted another physical owner: %v", err)
	}
	if err := os.Remove(target); err != nil { // only the empty test replacement
		t.Fatal(err)
	}
	if err := os.Rename(original, target); err != nil {
		t.Fatal(err)
	}
	root := e.Root
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	e, err = Open(root, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close() })
	released, err := e.ReleaseWorktree(ctx, request)
	if err != nil || released.Status != "released" {
		t.Fatalf("the same release could not finish after restart: %+v %v", released, err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("successful cleanup left the disposable worktree: %v", err)
	}
}

package runtime

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/local"
)

// errorsAs keeps the race assertion readable beside the other claim tests.
func errorsAs(err error, target **local.Rejection) bool { return errors.As(err, target) }

// The fixture is a real repository: a claim that never ran git would prove
// nothing about the boundary the pilot depends on.
func gitRepository(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = dir
		command.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
		if out, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "--initial-branch=main")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run("add", "README.md")
	run("commit", "-m", "initial")
	return dir
}

func TestWorktreeClaimIsExclusiveAndConfined(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	repository := gitRepository(t)
	claim, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:claim", Repository: repository, OwnerID: "run:pilot"})
	if err != nil {
		t.Fatal(err)
	}
	if claim.Status != "active" || claim.Generation != 1 || claim.OwnerID != "run:pilot" || claim.Actor != e.owner {
		t.Fatalf("unexpected claim: %+v", claim)
	}
	if len(claim.BaseCommit) != 40 {
		t.Fatalf("base commit was not resolved: %q", claim.BaseCommit)
	}
	if !strings.HasPrefix(claim.Path, ClaimRoot+"/") {
		t.Fatalf("claim path escaped the confined root: %s", claim.Path)
	}
	worktree := filepath.Join(e.Root, filepath.FromSlash(claim.Path))
	if _, err := os.Stat(filepath.Join(worktree, "README.md")); err != nil {
		t.Fatalf("the claimed worktree does not hold the base commit content: %v", err)
	}
	// A second claim on the same repository is refused; this slice admits one.
	if _, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:second", Repository: repository, OwnerID: "run:other"}); err == nil {
		t.Fatal("a second exclusive claim was granted for the same repository")
	} else {
		rejectionCode(t, err, "claim_conflict")
	}
}

func TestWorkspaceCheckoutClaimIsCleanExclusiveAndNeverMutatesGitTopology(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	repository := gitRepository(t)
	git := func(args ...string) string {
		t.Helper()
		command := exec.Command("git", args...)
		command.Dir = repository
		out, err := command.Output()
		if err != nil {
			t.Fatalf("git %s: %v", strings.Join(args, " "), err)
		}
		return strings.TrimSpace(string(out))
	}
	head, worktrees := git("rev-parse", "HEAD"), git("worktree", "list", "--porcelain")
	claim, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:checkout", Repository: repository, OwnerID: "run:checkout", WorkspaceMode: "checkout"})
	if err != nil {
		t.Fatal(err)
	}
	if claim.Mode != "checkout" || claim.Path != claim.Repository.Toplevel || claim.BaseCommit != head || git("worktree", "list", "--porcelain") != worktrees || git("rev-parse", "HEAD") != head {
		t.Fatalf("checkout claim changed Git topology or recorded another workspace: %+v", claim)
	}
	if _, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:conflict", Repository: repository, OwnerID: "run:other"}); err == nil {
		t.Fatal("checkout did not exclude a worktree claim for the same repository")
	} else {
		rejectionCode(t, err, "claim_conflict")
	}
	if _, err := e.ReleaseWorktree(ctx, ClaimReleaseRequest{CommandID: "command:release", ClaimID: claim.ID, Generation: claim.Generation}); err != nil {
		t.Fatal(err)
	}
	if git("worktree", "list", "--porcelain") != worktrees || git("rev-parse", "HEAD") != head {
		t.Fatal("checkout release changed Git topology")
	}
	if err := os.WriteFile(filepath.Join(repository, "dirty.txt"), []byte("dirty\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:dirty", Repository: repository, OwnerID: "run:dirty", WorkspaceMode: "checkout"}); err == nil {
		t.Fatal("dirty checkout was claimed")
	} else {
		rejectionCode(t, err, "checkout_dirty")
	}
}

func TestWorktreeClaimRefusesTheAuthorityRootAndNonRepositories(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	if _, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:root", Repository: e.Root, OwnerID: "run:pilot"}); err == nil {
		t.Fatal("the authority project root was claimed")
	}
	if _, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:plain", Repository: t.TempDir(), OwnerID: "run:pilot"}); err == nil {
		t.Fatal("a directory that is not a repository was claimed")
	}
	repository := gitRepository(t)
	if _, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:badbase", Repository: repository, BaseRef: "--upload-pack=touch", OwnerID: "run:pilot"}); err == nil {
		t.Fatal("an option-shaped base reference was accepted")
	}
	if _, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:missing", Repository: repository, BaseRef: "no-such-ref", OwnerID: "run:pilot"}); err == nil {
		t.Fatal("an unresolvable base reference was accepted")
	}
	if record, err := e.Claims(ctx); err != nil || len(record.Claims) != 0 {
		t.Fatalf("a refused claim was recorded: %v %+v", err, record.Claims)
	}
}

func TestWorktreeReleaseRemovesOnlyItsOwnGeneration(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	repository := gitRepository(t)
	claim, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:claim", Repository: repository, OwnerID: "run:pilot"})
	if err != nil {
		t.Fatal(err)
	}
	stale, err := e.ReleaseWorktree(ctx, ClaimReleaseRequest{CommandID: "command:stale", ClaimID: claim.ID, Generation: claim.Generation + 1})
	if err == nil {
		t.Fatalf("a stale generation released the claim: %+v", stale)
	}
	rejectionCode(t, err, "claim_generation_conflict")

	released, err := e.ReleaseWorktree(ctx, ClaimReleaseRequest{CommandID: "command:release", ClaimID: claim.ID, Generation: claim.Generation})
	if err != nil {
		t.Fatal(err)
	}
	if released.Status != "released" || released.Released == nil {
		t.Fatalf("release was not recorded: %+v", released)
	}
	if _, err := os.Stat(filepath.Join(e.Root, filepath.FromSlash(claim.Path))); !os.IsNotExist(err) {
		t.Fatalf("cleanup left the worktree in place: %v", err)
	}
	if _, err := e.ReleaseWorktree(ctx, ClaimReleaseRequest{CommandID: "command:again", ClaimID: claim.ID, Generation: claim.Generation}); err == nil {
		t.Fatal("an already released claim was released twice")
	}
	// The repository is free again, and the new claim owns a later generation.
	next, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:next", Repository: repository, OwnerID: "run:pilot"})
	if err != nil {
		t.Fatal(err)
	}
	if next.Generation != 1 || next.ID == claim.ID {
		t.Fatalf("a released repository did not admit an independent claim: %+v", next)
	}
}

// Cleanup must delete what this claim created, not whatever now sits at the
// path: a replaced directory blocks removal instead of being destroyed.
func TestWorktreeCleanupRefusesAReplacedDirectory(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	repository := gitRepository(t)
	claim, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:claim", Repository: repository, OwnerID: "run:pilot"})
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(e.Root, filepath.FromSlash(claim.Path))
	replacement := target + ".replacement"
	if err := os.Mkdir(replacement, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(target); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, target); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ReleaseWorktree(ctx, ClaimReleaseRequest{CommandID: "command:release", ClaimID: claim.ID, Generation: claim.Generation}); err == nil {
		t.Fatal("cleanup removed a directory this claim never created")
	} else if !strings.Contains(err.Error(), "claim_identity_conflict") {
		t.Fatalf("unexpected cleanup failure: %v", err)
	}
}

func TestWorktreeClaimIsRefusedUnderAControlStop(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	repository := gitRepository(t)
	if _, err := e.RestrictControl(ctx, ControlRestrictRequest{CommandID: "command:stop", Scope: "project", Reason: "pilot halted"}); err != nil {
		t.Fatal(err)
	}
	_, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:claim", Repository: repository, OwnerID: "run:pilot"})
	rejectionCode(t, err, "control_stop_active")
}

// Stage acceptance: physical aliases are one resource. A path that reaches the
// same repository through a symlink must not become a second owner.
func TestPhysicalAliasesAreOneResource(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	repository := gitRepository(t)
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(repository, alias); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	if _, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:claim", Repository: repository, OwnerID: "session:pilot"}); err != nil {
		t.Fatal(err)
	}
	_, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:alias", Repository: alias, OwnerID: "session:other"})
	if err == nil {
		t.Fatal("an alias of a claimed repository became a second owner")
	}
	rejectionCode(t, err, "claim_conflict")
}

// An expired lease is not proof the old owner stopped. Until that is settled a
// conflicting claim stays blocked rather than creating a second owner.
func TestExpiredLeaseBlocksInsteadOfHandingOverOwnership(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	repository := gitRepository(t)
	claim, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:claim", Repository: repository, OwnerID: "session:pilot"})
	if err != nil {
		t.Fatal(err)
	}
	if claim.LeaseUntil == "" || claim.Process.Session == "" || claim.Process.PID == 0 {
		t.Fatalf("the claim did not record a lease and an owning process: %+v", claim)
	}
	if presence := claimPresence(claim, e.clock.now()); presence != "present" {
		t.Fatalf("a fresh claim was not present: %s", presence)
	}
	// A lapsed lease is the state a vanished owner leaves behind.
	lapsed := claim
	lapsed.LeaseUntil = "2000-01-01T00:00:00Z"
	if presence := claimPresence(lapsed, e.clock.now()); presence != "suspected" {
		t.Fatalf("a lapsed lease did not become suspected: %s", presence)
	}
	record, version, err := e.readClaims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	record.Claims[0].LeaseUntil = "2000-01-01T00:00:00Z"
	payload, err := canonical(map[string]any{"operation": "test.lapse"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: "command:lapse", Actor: e.owner, Key: AuthorityClaimsKey, Payload: payload, ExpectedVersion: &version}, func(local.AuthoritySnapshot) (local.AuthorityChange, error) {
		data, err := canonicalState(record)
		return local.AuthorityChange{Data: data}, err
	}); err != nil {
		t.Fatal(err)
	}
	_, err = e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:takeover", Repository: repository, OwnerID: "session:other"})
	if err == nil {
		t.Fatal("an expired lease handed the resource to a second owner")
	}
	rejectionCode(t, err, "claim_owner_unproven")

	// The original owner can still extend its own lease; nobody else can.
	beaten, err := e.HeartbeatClaim(ctx, ClaimHeartbeatRequest{CommandID: "command:beat", ClaimID: claim.ID, Generation: claim.Generation})
	if err != nil {
		t.Fatal(err)
	}
	if beaten.Heartbeat == nil || claimPresence(beaten, e.clock.now()) != "present" {
		t.Fatalf("the owner could not extend its own lease: %+v", beaten)
	}
	stranger := *e
	stranger.clock = newClock()
	if _, err := stranger.HeartbeatClaim(ctx, ClaimHeartbeatRequest{CommandID: "command:stranger", ClaimID: claim.ID, Generation: claim.Generation}); err == nil {
		t.Fatal("a different process extended someone else's lease")
	} else {
		rejectionCode(t, err, "claim_owner_conflict")
	}
}

// RUN-014: the whole set is taken together or not at all. Reading the free
// resources and then locking them separately is not that.
func TestAtomicMultiClaimTakesAllOrNothing(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	first, second := gitRepository(t), gitRepository(t)
	claims, err := e.ClaimWorktrees(ctx, "command:pair", []ClaimRequest{
		{Repository: first, OwnerID: "session:pilot"},
		{Repository: second, OwnerID: "session:pilot"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(claims) != 2 || claims[0].Status != "active" || claims[1].Status != "active" || claims[0].ID == claims[1].ID {
		t.Fatalf("an atomic pair did not produce two distinct active claims: %+v", claims)
	}

	// One conflict refuses the whole set: nothing partial is recorded.
	third := gitRepository(t)
	_, err = e.ClaimWorktrees(ctx, "command:overlap", []ClaimRequest{
		{Repository: third, OwnerID: "session:other"},
		{Repository: first, OwnerID: "session:other"},
	})
	if err == nil {
		t.Fatal("a set containing a taken resource was granted")
	}
	rejectionCode(t, err, "claim_conflict")
	record, err := e.Claims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Claims) != 2 {
		t.Fatalf("a refused set recorded part of itself: %+v", record.Claims)
	}
	// The same repository named twice in one set would be two owners of one
	// resource, so the set is checked against itself first.
	if _, err := e.ClaimWorktrees(ctx, "command:self", []ClaimRequest{
		{Repository: third, OwnerID: "session:other"},
		{Repository: third, OwnerID: "session:other"},
	}); err == nil {
		t.Fatal("one resource was claimed twice inside one set")
	} else {
		rejectionCode(t, err, "claim_conflict")
	}
}

// Stage acceptance: racing claims never produce two owners of one resource.
func TestConcurrentClaimsProduceOneOwner(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	repository := gitRepository(t)
	if _, _, err := e.ensureControl(ctx); err != nil {
		t.Fatal(err)
	}
	const racers = 4
	results := make(chan error, racers)
	for i := 0; i < racers; i++ {
		go func(i int) {
			_, err := e.ClaimWorktree(context.Background(), ClaimRequest{
				CommandID: "command:race-" + strconv.Itoa(i), Repository: repository, OwnerID: "session:racer",
			})
			results <- err
		}(i)
	}
	granted, refused := 0, 0
	for i := 0; i < racers; i++ {
		if err := <-results; err == nil {
			granted++
		} else {
			refused++
			var rejection *local.Rejection
			if !errorsAs(err, &rejection) || rejection.Code != "claim_conflict" {
				t.Fatalf("a losing racer failed without an explainable reason: %v", err)
			}
		}
	}
	if granted != 1 || refused != racers-1 {
		t.Fatalf("racing claims produced %d owners", granted)
	}
	record, err := e.Claims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	active := 0
	for _, claim := range record.Claims {
		if claim.active() {
			active++
		}
	}
	if active != 1 {
		t.Fatalf("the record holds %d active owners of one resource", active)
	}
}

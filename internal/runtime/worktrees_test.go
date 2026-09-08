package runtime

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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
	// One branch, many tasks, one Run each: a second worktree of the same
	// repository is its own resource and is granted. This is the parallel work
	// the product was built for, and the old rule refused it.
	second, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:second", Repository: repository, OwnerID: "run:other"})
	if err != nil {
		t.Fatalf("a second worktree of the same repository was refused: %v", err)
	}
	if second.Path == claim.Path || second.Branch == claim.Branch {
		t.Fatalf("two claims took the same tree or branch: %+v %+v", claim, second)
	}
	if _, err := os.Stat(filepath.Join(e.Root, filepath.FromSlash(second.Path), "README.md")); err != nil {
		t.Fatalf("the second worktree does not hold the base commit content: %v", err)
	}
	if _, err := os.Stat(filepath.Join(worktree, "README.md")); err != nil {
		t.Fatalf("the first worktree did not survive the second claim: %v", err)
	}
	// Exclusivity is not traded away, only narrowed to what it protects. A
	// checkout works in the repository's own tree, where nothing bounds what it
	// touches, so it is still refused while any worktree claim is active. Two
	// worktree claims cannot collide on one tree at all: the path derives from
	// the command id, so asking twice is the same command, not a second claim.
	if _, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:checkout", Repository: repository, OwnerID: "run:third", WorkspaceMode: "checkout"}); err == nil {
		t.Fatal("a checkout was granted while worktrees of that repository were held")
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
// same tree through a symlink must not become a second owner. The resource is
// now the working tree rather than the repository, so the property is stated
// where a tree is actually shared: two checkouts of one repository, named two
// ways, are one checkout.
func TestPhysicalAliasesAreOneResource(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	repository := gitRepository(t)
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(repository, alias); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	if _, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:claim", Repository: repository, OwnerID: "session:pilot", WorkspaceMode: "checkout"}); err != nil {
		t.Fatal(err)
	}
	_, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:alias", Repository: alias, OwnerID: "session:other", WorkspaceMode: "checkout"})
	if err == nil {
		t.Fatal("an alias of a claimed checkout became a second owner of it")
	}
	rejectionCode(t, err, "claim_conflict")
}

// An expired lease is not proof the old owner stopped. Until that is settled a
// conflicting claim stays blocked rather than creating a second owner. Stated
// on a checkout, because that is where two claims contend for one tree: two
// worktree claims take two trees and never contend at all.
func TestExpiredLeaseBlocksInsteadOfHandingOverOwnership(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	repository := gitRepository(t)
	claim, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:claim", Repository: repository, OwnerID: "session:pilot", WorkspaceMode: "checkout"})
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
	// The owner's next command is a new process with a new clock session. That is
	// the ordinary way a lease is extended from the CLI, not a stranger.
	next := *e
	next.clock = newClock()
	extended, err := next.HeartbeatClaim(ctx, ClaimHeartbeatRequest{CommandID: "command:next-call", ClaimID: claim.ID, Generation: claim.Generation})
	if err != nil || claimPresence(extended, next.clock.now()) != "present" {
		t.Fatalf("a new call of the same owner could not extend its lease: %+v %v", extended, err)
	}
	stranger := *e
	stranger.owner = "local:uid:999999"
	if _, err := stranger.HeartbeatClaim(ctx, ClaimHeartbeatRequest{CommandID: "command:stranger", ClaimID: claim.ID, Generation: claim.Generation}); err == nil {
		t.Fatal("a different actor extended someone else's lease")
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

	// One refusal refuses the whole set: nothing partial is recorded. A taken
	// repository is no longer a refusal — the set would take its own trees — so
	// the property is exercised on the check that does still refuse: one
	// repository named twice inside one set.
	third := gitRepository(t)
	_, err = e.ClaimWorktrees(ctx, "command:overlap", []ClaimRequest{
		{Repository: third, OwnerID: "session:other"},
		{Repository: first, OwnerID: "session:other"},
		{Repository: first, OwnerID: "session:other"},
	})
	if err == nil {
		t.Fatal("one repository was named twice inside one set")
	}
	rejectionCode(t, err, "claim_conflict")
	record, err := e.Claims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(record.Claims) != 2 {
		t.Fatalf("a refused set recorded part of itself: %+v", record.Claims)
	}
	// A set naming only repositories nobody holds is granted, including one
	// already worked in by another Run: those are separate trees.
	beside, err := e.ClaimWorktrees(ctx, "command:beside", []ClaimRequest{
		{Repository: third, OwnerID: "session:other"},
		{Repository: first, OwnerID: "session:other"},
	})
	if err != nil {
		t.Fatalf("a set naming a repository another Run works in was refused: %v", err)
	}
	if len(beside) != 2 || beside[0].Path == claims[0].Path || beside[1].Path == claims[0].Path {
		t.Fatalf("an atomic set reused a tree another claim holds: %+v", beside)
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
				// Racing for one resource means racing for one tree. Four
				// worktree claims would take four trees and all four would win,
				// which proves nothing about the transaction.
				WorkspaceMode: "checkout",
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

// The refusal is read by someone standing in their own worktree, so a refusal
// that says only "this repository" tells them it is about someone else. It now
// names the relation — the tree a claim occupies — and, because the rule
// changed under them, says explicitly what no longer conflicts.
func TestClaimConflictNamesTheRelationNotJustTheObstacle(t *testing.T) {
	for _, expected := range []string{"this working tree", "another worktree of the same repository does not conflict", "same checkout", "claim list", "claim release"} {
		if !strings.Contains(claimConflictMessage, expected) {
			t.Fatalf("the conflict refusal does not name %q: %s", expected, claimConflictMessage)
		}
	}
	// A claim outlives the command that hit it and is ended explicitly, so a
	// state diagnostic never shows the way past one. Asserted here because the
	// CLI can no longer produce this refusal: claim create takes a fresh tree.
	problem, _ := ProblemFor(local.Reject("claim_conflict", claimConflictMessage))
	if !slices.Contains(problem.SafeNextActions, "claim.release") || slices.Contains(problem.SafeNextActions, "doctor") {
		t.Fatalf("a claim refusal was sent to a state diagnostic: %+v", problem.SafeNextActions)
	}
}

// A volume is renumbered across boots on APFS, so a claim that outlives a
// restart carried a device number nothing on the machine had any more. The
// guard then condemned the directory it had itself created, the repository was
// locked for good, and claim release — the documented way out — ran the same
// check and was refused by it. The inode is what a replacement changes; the
// device is what the operating system changes for its own reasons.
func TestAClaimSurvivesTheVolumeBeingRenumbered(t *testing.T) {
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	repository := gitRepository(t)
	claim, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:claim", Repository: repository, OwnerID: "run:pilot"})
	if err != nil {
		t.Fatal(err)
	}
	// What a reboot leaves behind: the same directory, the same inode, a device
	// number that belongs to nothing.
	record, _, err := e.readClaims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	stale := claim
	for _, held := range record.Claims {
		if held.ID == claim.ID {
			stale = held
		}
	}
	if stale.Device == 0 || stale.Inode == 0 {
		t.Fatalf("the claim recorded no directory identity: %+v", stale)
	}
	stale.Device = stale.Device + 4
	if err := e.removeWorktree(ctx, stale); err != nil {
		t.Fatalf("a renumbered volume condemned the claim's own directory: %v", err)
	}
	// Replacement is what the guard is for, and it still catches it.
	replaced, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:second", Repository: repository, OwnerID: "run:other"})
	if err != nil {
		t.Fatal(err)
	}
	wrong := replaced
	wrong.Inode = wrong.Inode + 1
	err = e.removeWorktree(ctx, wrong)
	if err == nil {
		t.Fatal("a directory this claim never created was removed")
	}
	if !strings.Contains(err.Error(), "claim_identity_conflict") || !strings.Contains(err.Error(), "claim release") {
		t.Fatalf("the refusal does not name itself or the way out: %v", err)
	}
}

// 0.13.0 opened parallel Runs and an authority still admits one attempt at a
// time by default, so the upgrade that enables the work refuses it until the
// number is raised. The refusal used to say only that no slot was free, and
// offered doctor and run.status — neither reaches capacity set. A reader could
// not tell "not supported" from "not configured".
func TestCapacityRefusalPointsAtTheNumberThatRefused(t *testing.T) {
	problem, _ := ProblemFor(local.Reject("capacity_conflict", "x"))
	for _, expected := range []string{"capacity.show", "capacity.set"} {
		if !slices.Contains(problem.SafeNextActions, expected) {
			t.Fatalf("a capacity refusal does not offer %s: %+v", expected, problem.SafeNextActions)
		}
	}
	if slices.Contains(problem.SafeNextActions, "doctor") {
		t.Fatalf("a capacity refusal was sent to a state diagnostic: %+v", problem.SafeNextActions)
	}
}

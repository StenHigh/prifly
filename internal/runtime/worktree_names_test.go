package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/stenhigh/prifly/internal/local"
)

func TestNamedWorktreeAllocationLifecycleAndDuplicate(t *testing.T) {
	t.Parallel()
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	repository := gitRepository(t)
	request := ClaimRequest{CommandID: "command:named", Repository: repository, OwnerID: "owner:named", Naming: &WorktreeNamingRequest{Base: "smsplace-trucker-default-123", Policy: WorktreeNamingPolicy}}
	preview, err := e.PreviewWorktreeName(ctx, repository, *request.Naming)
	if err != nil || preview.Reserved || !strings.HasSuffix(preview.Path, request.Naming.Base) {
		t.Fatalf("preview: %+v %v", preview, err)
	}
	record, err := e.Claims(ctx)
	if err != nil || len(record.Claims) != 0 {
		t.Fatalf("preview mutated claims: %+v %v", record, err)
	}
	claim, created, err := e.ClaimWorktreeWithOutcome(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if !created || claim.Path != preview.Path || claim.Branch != preview.Branch || claim.Naming == nil || claim.Status != "active" {
		t.Fatalf("claim: %+v", claim)
	}
	duplicate, created, err := e.ClaimWorktreeWithOutcome(ctx, request)
	if err != nil || created || duplicate.ID != claim.ID || duplicate.Path != claim.Path {
		t.Fatalf("duplicate: %+v %t %v", duplicate, created, err)
	}
	changed := request
	changed.Naming = &WorktreeNamingRequest{Base: "different", Policy: WorktreeNamingPolicy}
	if _, _, err := e.ClaimWorktreeWithOutcome(ctx, changed); !errors.Is(err, local.ErrCommandConflict) {
		t.Fatalf("changed payload: %v", err)
	}
	request.CommandID = "command:named-second"
	second, err := e.ClaimWorktree(ctx, request)
	if err != nil || !strings.HasSuffix(second.Path, "-02") {
		t.Fatalf("second: %+v %v", second, err)
	}
	if _, err := e.ReleaseWorktree(ctx, ClaimReleaseRequest{CommandID: "command:release-named", ClaimID: claim.ID, Generation: claim.Generation}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(e.Root, filepath.FromSlash(claim.Path))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("tree remains: %v", err)
	}
	if _, err := e.git(ctx, repository, "show-ref", "--verify", "refs/heads/"+claim.Branch); err == nil {
		t.Fatal("branch remains")
	}
	request.CommandID = "command:named-reuse"
	reused, err := e.ClaimWorktree(ctx, request)
	if err != nil || reused.Path != claim.Path || reused.Generation <= claim.Generation {
		t.Fatalf("reuse: %+v %v", reused, err)
	}
	duplicate, created, err = e.ClaimWorktreeWithOutcome(ctx, ClaimRequest{CommandID: "command:named", Repository: repository, OwnerID: "owner:named", Naming: request.Naming})
	if err != nil || created || duplicate.Status != "released" || duplicate.ID != claim.ID {
		t.Fatalf("released duplicate: %+v %t %v", duplicate, created, err)
	}
}

func TestNamedWorktreeInterruptedPreparationPreservesIdentity(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"reserved", "directory", "creating_branch", "unrecorded-branch", "branch", "unrecorded-tree", "tree"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			e := contextRegistryRuntime(t)
			ctx := context.Background()
			repository := gitRepository(t)
			request := ClaimRequest{CommandID: "command:interrupted", Repository: repository, OwnerID: "owner:interrupted", Naming: &WorktreeNamingRequest{Base: "crash", Policy: WorktreeNamingPolicy}}
			claim, err := e.ClaimWorktree(ctx, request)
			if err != nil {
				t.Fatal(err)
			}
			if phase != "tree" && phase != "unrecorded-tree" {
				if err := e.removeNamedWorktree(ctx, claim); err != nil {
					t.Fatal(err)
				}
				claim.Device, claim.Inode = 0, 0
			}
			target := filepath.Join(e.Root, filepath.FromSlash(claim.Path))
			if phase != "reserved" && phase != "tree" && phase != "unrecorded-tree" {
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
				info, err := os.Stat(target)
				if err != nil {
					t.Fatal(err)
				}
				stat := info.Sys().(*syscall.Stat_t)
				claim.Device, claim.Inode = uint64(stat.Dev), stat.Ino
			}
			if phase == "branch" || phase == "unrecorded-branch" {
				if _, err := e.git(ctx, repository, "update-ref", "--create-reflog", "-m", claim.ID, "refs/heads/"+claim.Branch, claim.BaseCommit, strings.Repeat("0", 40)); err != nil {
					t.Fatal(err)
				}
			}
			claim.Status = "preparing"
			naming := *claim.Naming
			naming.Phase = phase
			if phase == "unrecorded-branch" {
				naming.Phase = "creating_branch"
			}
			if phase == "unrecorded-tree" {
				naming.Phase = "branch"
			}
			claim.Naming = &naming
			_, err = e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: "command:interrupt-boundary", Actor: e.owner, Key: AuthorityClaimsKey, Payload: json.RawMessage(`{}`)}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
				record, err := e.decodeClaims(s)
				if err != nil {
					return local.AuthorityChange{}, err
				}
				record.Claims[0] = claim
				data, err := canonicalState(record)
				return local.AuthorityChange{Data: data, Result: json.RawMessage(`{}`)}, err
			})
			if err != nil {
				t.Fatal(err)
			}
			repeated, created, err := e.ClaimWorktreeWithOutcome(ctx, request)
			if refusalCode(err) != "claim_preparation_incomplete" || created || repeated.ID != claim.ID || repeated.Path != claim.Path {
				t.Fatalf("duplicate after %s: %+v %t %v", phase, repeated, created, err)
			}
			released, err := e.ReleaseWorktree(ctx, ClaimReleaseRequest{CommandID: "command:interrupt-release", ClaimID: claim.ID, Generation: claim.Generation})
			if err != nil || released.Status != "released" {
				t.Fatalf("release after %s: %+v %v", phase, released, err)
			}
		})
	}
}

func TestNamedWorktreeCleanupRejectsForeignBranchAndSymlinkRoot(t *testing.T) {
	t.Parallel()
	t.Run("foreign-ref", func(t *testing.T) {
		t.Parallel()
		e := contextRegistryRuntime(t)
		ctx := context.Background()
		repository := gitRepository(t)
		claim, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:foreign-ref", Repository: repository, OwnerID: "owner:foreign", Naming: &WorktreeNamingRequest{Base: "foreign", Policy: WorktreeNamingPolicy}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.git(ctx, repository, "update-ref", "-d", "refs/heads/"+claim.Branch); err != nil {
			t.Fatal(err)
		}
		if _, err := e.git(ctx, repository, "update-ref", "--create-reflog", "-m", "external owner", "refs/heads/"+claim.Branch, claim.BaseCommit); err != nil {
			t.Fatal(err)
		}
		if _, err := e.ReleaseWorktree(ctx, ClaimReleaseRequest{CommandID: "command:foreign-release", ClaimID: claim.ID, Generation: claim.Generation}); refusalCode(err) != "claim_identity_conflict" {
			t.Fatalf("foreign ref cleanup: %v", err)
		}
		if _, err := os.Stat(filepath.Join(e.Root, claim.Path)); err != nil {
			t.Fatal("foreign ref refusal changed tree", err)
		}
	})
	t.Run("symlink-root", func(t *testing.T) {
		t.Parallel()
		e := contextRegistryRuntime(t)
		repository := gitRepository(t)
		external := t.TempDir()
		parent := filepath.Join(e.Root, ".prifly/work")
		if err := os.MkdirAll(parent, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(external, filepath.Join(parent, "claims")); err != nil {
			t.Fatal(err)
		}
		if _, err := e.ClaimWorktree(context.Background(), ClaimRequest{CommandID: "command:symlink", Repository: repository, OwnerID: "owner:symlink", Naming: &WorktreeNamingRequest{Base: "safe", Policy: WorktreeNamingPolicy}}); !errors.Is(err, local.ErrUnsafePath) {
			t.Fatalf("symlink root: %v", err)
		}
		entries, err := os.ReadDir(external)
		if err != nil || len(entries) != 0 {
			t.Fatal("wrote outside claim root", err)
		}
	})
}

func TestNamedWorktreeConcurrentReservationsAndForeignObjects(t *testing.T) {
	t.Parallel()
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	repository := gitRepository(t)
	if err := os.MkdirAll(filepath.Join(e.Root, ClaimRoot, "demo"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := e.git(ctx, repository, "branch", "prifly/demo-02"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	start := make(chan struct{})
	results := make(chan WorktreeClaim, 4)
	errors := make(chan error, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			claim, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: fmt.Sprintf("command:named-%d", i), Repository: repository, OwnerID: "owner:parallel", Naming: &WorktreeNamingRequest{Base: "demo", Policy: WorktreeNamingPolicy}})
			if err != nil {
				errors <- err
				return
			}
			results <- claim
		}(i)
	}
	close(start)
	wg.Wait()
	close(errors)
	close(results)
	for err := range errors {
		t.Error(err)
	}
	seen := map[string]bool{}
	for claim := range results {
		if seen[claim.Path] {
			t.Fatalf("duplicate name: %s", claim.Path)
		}
		seen[claim.Path] = true
	}
	if len(seen) != 4 {
		t.Fatalf("got %d claims", len(seen))
	}
	for i := 3; i <= 6; i++ {
		if !seen[fmt.Sprintf("%s/demo-%02d", ClaimRoot, i)] {
			t.Fatalf("missing suffix %d: %+v", i, seen)
		}
	}
	if _, err := os.Stat(filepath.Join(e.Root, ClaimRoot, "demo")); err != nil {
		t.Fatal("foreign directory changed", err)
	}
}

func TestWorktreeNameBoundsAndExhaustion(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"", "../outside", "UPPER", "a_b", "a/b", "a\\b", "a..b", "a--b", "nul", "com1", strings.Repeat("a", 97)} {
		if ValidateWorktreeName(name) == nil {
			t.Errorf("accepted %q", name)
		}
	}
	busy := map[string]bool{}
	for i := 1; i <= MaxWorktreeNames; i++ {
		path, _ := worktreeCandidate("demo", i)
		busy[path] = true
	}
	_, err := allocateWorktreeName(ClaimRecord{}, WorktreeClaim{}, WorktreeNamingRequest{Base: "demo", Policy: WorktreeNamingPolicy}, busy, 0)
	if refusalCode(err) != "claim_name_exhausted" {
		t.Fatalf("exhaustion: %v", err)
	}
	for _, i := range []int{99, 100, 9999} {
		path, branch := worktreeCandidate("demo", i)
		if path != fmt.Sprintf("%s/demo-%02d", ClaimRoot, i) || branch != fmt.Sprintf("prifly/demo-%02d", i) {
			t.Fatalf("candidate %d: %s %s", i, path, branch)
		}
	}
}

func TestNamedWorktreeExternalCollisionRetriesSameClaim(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"directory", "ref", "symlink", "descendant-ref"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			e := contextRegistryRuntime(t)
			ctx := context.Background()
			repository := gitRepository(t)
			request := ClaimRequest{CommandID: "command:collision", Repository: repository, OwnerID: "owner:collision", Naming: &WorktreeNamingRequest{Base: "collision", Policy: WorktreeNamingPolicy}}
			seed := request
			seed.CommandID = "command:collision-seed"
			claim, err := e.ClaimWorktree(ctx, seed)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.ReleaseWorktree(ctx, ClaimReleaseRequest{CommandID: "command:seed-release", ClaimID: claim.ID, Generation: claim.Generation}); err != nil {
				t.Fatal(err)
			}
			claim.ID = derivedID("claim", e.Installation.ID, request.CommandID)
			claim.Generation++
			claim.Released = nil
			claim.Status = "preparing"
			claim.Device, claim.Inode = 0, 0
			naming := *claim.Naming
			naming.Phase = "reserved"
			claim.Naming = &naming
			payload, err := canonical(map[string]any{"operation": "worktree.claim_named", "repository": claim.Repository, "base_ref": request.BaseRef, "owner_id": request.OwnerID, "naming": request.Naming})
			if err != nil {
				t.Fatal(err)
			}
			_, err = e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: request.CommandID, Actor: e.owner, Key: AuthorityClaimsKey, Payload: payload}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
				record, err := e.decodeClaims(s)
				if err != nil {
					return local.AuthorityChange{}, err
				}
				record.Claims = append(record.Claims, claim)
				data, err := canonicalState(record)
				return local.AuthorityChange{Data: data, Result: json.RawMessage(`{}`)}, err
			})
			if err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(e.Root, claim.Path)
			switch kind {
			case "directory":
				err = os.Mkdir(target, 0700)
			case "symlink":
				err = os.Symlink(t.TempDir(), target)
			case "ref":
				_, err = e.git(ctx, repository, "branch", claim.Branch)
			case "descendant-ref":
				_, err = e.git(ctx, repository, "branch", claim.Branch+"/child")
			}
			if err != nil {
				t.Fatal(err)
			}
			chosen, err := e.prepareNamedWorktree(ctx, claim)
			if err != nil || chosen.ID != claim.ID || chosen.Naming.Ordinal != 2 {
				t.Fatalf("collision %s: %+v %v", kind, chosen, err)
			}
			duplicate, created, err := e.ClaimWorktreeWithOutcome(ctx, request)
			if err != nil || created || duplicate.Path != chosen.Path {
				t.Fatalf("duplicate: %+v %t %v", duplicate, created, err)
			}
			if _, err := e.ReleaseWorktree(ctx, ClaimReleaseRequest{CommandID: "command:collision-release", ClaimID: chosen.ID, Generation: chosen.Generation}); err != nil {
				t.Fatal(err)
			}
			if kind == "ref" || kind == "descendant-ref" {
				ref := "refs/heads/" + claim.Branch
				if kind == "descendant-ref" {
					ref += "/child"
				}
				if _, err := e.git(ctx, repository, "show-ref", "--verify", ref); err != nil {
					t.Fatal("foreign branch deleted", err)
				}
			} else {
				if _, err := os.Lstat(target); err != nil {
					t.Fatal("foreign path deleted", err)
				}
			}
		})
	}
}

func TestNamedWorktreePersistedContractsAndExhaustionPreview(t *testing.T) {
	t.Parallel()
	e := contextRegistryRuntime(t)
	ctx := context.Background()
	repository := gitRepository(t)
	claim, err := e.ClaimWorktree(ctx, ClaimRequest{CommandID: "command:contracts", Repository: repository, OwnerID: "owner:contracts", Naming: &WorktreeNamingRequest{Base: "contracts", Policy: WorktreeNamingPolicy}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := e.Store.ReadAuthority(ctx, AuthorityClaimsKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePublic(t, "AuthorityClaimsV4", json.RawMessage(snapshot.Data)); err != nil {
		t.Fatal(err)
	}
	record, err := e.Claims(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePublic(t, "WorktreeClaimsV3", ClaimView(record)); err != nil {
		t.Fatal(err)
	}
	legacy := strings.Replace(string(snapshot.Data), NamedAuthorityClaimsVersion, AuthorityClaimsVersion, 1)
	snapshot.Data = json.RawMessage(legacy)
	if _, err := e.decodeClaims(snapshot); refusalCode(err) != "unsupported_claim_contract" {
		t.Fatalf("old contract accepted naming: %v", err)
	}
	if _, err := e.ReleaseWorktree(ctx, ClaimReleaseRequest{CommandID: "command:contracts-release", ClaimID: claim.ID, Generation: claim.Generation}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.git(ctx, repository, "branch", "prifly"); err != nil {
		t.Fatal(err)
	}
	preview, err := e.PreviewWorktreeName(ctx, repository, WorktreeNamingRequest{Base: "exhausted", Policy: WorktreeNamingPolicy})
	if err != nil || preview.WouldRefuse != "claim_name_exhausted" || preview.Reserved {
		t.Fatalf("exhausted preview: %+v %v", preview, err)
	}
}

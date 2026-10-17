package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/stenhigh/prifly/internal/local"
	"github.com/stenhigh/prifly/internal/purity"
)

const (
	NamedAuthorityClaimsVersion = "authority-claims/4"
	WorktreeNamingPolicy        = "first-free/1"
	MaxWorktreeNames            = 9999
)

var worktreeNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

type WorktreeNamingRequest struct {
	Base   string `json:"base"`
	Policy string `json:"policy"`
}

// Allocation evidence is authority metadata, never the identity of a Run.
// The reflog creation marker proves ownership of the ref even if Git returned
// without the caller receiving its response. Directory identity is independent.
type WorktreeNameAllocation struct {
	Base    string `json:"base"`
	Policy  string `json:"policy"`
	Ordinal int    `json:"ordinal"`
	Phase   string `json:"phase"`
}

type WorktreeNamePreview struct {
	Path        string `json:"path,omitempty"`
	Branch      string `json:"branch,omitempty"`
	Reserved    bool   `json:"reserved"`
	WouldRefuse string `json:"would_refuse,omitempty"`
}

func ValidateWorktreeName(base string) error {
	if len(base) == 0 || len(base) > 96 || !worktreeNamePattern.MatchString(base) {
		return fault("project_worktree_value_invalid", "name requires 1..96 lowercase ASCII letters, digits and single hyphen separators")
	}
	if base == "con" || base == "prn" || base == "aux" || base == "nul" || len(base) == 4 && (strings.HasPrefix(base, "com") || strings.HasPrefix(base, "lpt")) && base[3] >= '1' && base[3] <= '9' {
		return fault("project_worktree_value_invalid", "reserved portable filename")
	}
	return nil
}

func worktreeCandidate(base string, ordinal int) (string, string) {
	name := base
	if ordinal > 1 {
		name += fmt.Sprintf("-%02d", ordinal)
	}
	return ClaimRoot + "/" + name, "prifly/" + name
}

func (e *Engine) checkClaimRoot() error {
	purity.Guard("check claim root")
	path := e.Root
	for _, part := range strings.Split(ClaimRoot, "/") {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return local.ErrUnsafePath
		}
	}
	return nil
}

// External observations are computed before the authority chooses a name.
// A stale observation cannot defeat a reservation in the current snapshot.
func (e *Engine) worktreeNameInventory(ctx context.Context, repository RepositoryIdentity, base string) (map[string]bool, error) {
	purity.Guard("worktree naming inventory")
	if err := e.checkClaimRoot(); err != nil {
		return nil, err
	}
	busy := map[string]bool{}
	entries, err := os.ReadDir(filepath.Join(e.Root, filepath.FromSlash(ClaimRoot)))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for _, entry := range entries {
		busy[ClaimRoot+"/"+entry.Name()] = true
	}
	refs, err := e.git(ctx, repository.Toplevel, "for-each-ref", "--format=%(refname)", "refs/heads/prifly")
	if err != nil {
		return nil, err
	}
	branches := strings.Fields(refs)
	for ordinal := 1; ordinal <= MaxWorktreeNames; ordinal++ {
		path, branch := worktreeCandidate(base, ordinal)
		ref := "refs/heads/" + branch
		for _, existing := range branches {
			if ref == existing || strings.HasPrefix(ref, existing+"/") || strings.HasPrefix(existing, ref+"/") {
				busy[path] = true
				break
			}
		}
	}
	return busy, nil
}

func allocateWorktreeName(record ClaimRecord, claim WorktreeClaim, naming WorktreeNamingRequest, busy map[string]bool, after int) (WorktreeClaim, error) {
	for ordinal := after + 1; ordinal <= MaxWorktreeNames; ordinal++ {
		claim.Path, claim.Branch = worktreeCandidate(naming.Base, ordinal)
		if busy[claim.Path] {
			continue
		}
		conflict := false
		for _, existing := range record.Claims {
			if existing.ID == claim.ID || !existing.active() {
				continue
			}
			if existing.Repository.CommonDir == claim.Repository.CommonDir && claimMode(existing) == "checkout" {
				return WorktreeClaim{}, fault("claim_conflict", claimConflictMessage)
			}
			if existing.Path == claim.Path || existing.Repository.CommonDir == claim.Repository.CommonDir && existing.Branch == claim.Branch {
				conflict = true
				break
			}
		}
		if conflict {
			continue
		}
		claim.Naming = &WorktreeNameAllocation{Base: naming.Base, Policy: naming.Policy, Ordinal: ordinal, Phase: "reserved"}
		claim.Device, claim.Inode = 0, 0
		return claim, nil
	}
	return WorktreeClaim{}, fault("claim_name_exhausted", "all 9999 worktree names are occupied")
}

func (e *Engine) PreviewWorktreeName(ctx context.Context, repository string, naming WorktreeNamingRequest) (WorktreeNamePreview, error) {
	if err := validateWorktreeNamingRequest(naming); err != nil {
		return WorktreeNamePreview{}, err
	}
	identity, err := e.repositoryIdentity(ctx, repository)
	if err != nil {
		return WorktreeNamePreview{}, err
	}
	busy, err := e.worktreeNameInventory(ctx, identity, naming.Base)
	if err != nil {
		return WorktreeNamePreview{}, err
	}
	record, _, err := e.readClaims(ctx)
	if err != nil {
		return WorktreeNamePreview{}, err
	}
	claim, err := allocateWorktreeName(record, WorktreeClaim{Repository: identity, Mode: "worktree"}, naming, busy, 0)
	if err != nil {
		var f *Fault
		if errors.As(err, &f) {
			return WorktreeNamePreview{WouldRefuse: f.Code}, nil
		}
		return WorktreeNamePreview{}, err
	}
	return WorktreeNamePreview{Path: claim.Path, Branch: claim.Branch}, nil
}

func validateWorktreeNamingRequest(naming WorktreeNamingRequest) error {
	if err := ValidateWorktreeName(naming.Base); err != nil {
		return err
	}
	if naming.Policy != WorktreeNamingPolicy {
		return fault("project_worktree_template_invalid", "unsupported worktree naming policy")
	}
	return nil
}

func (e *Engine) claimEffectLock(id string) (*os.File, error) {
	purity.Guard("claim effect lock")
	if !claimIDPattern.MatchString(id) {
		return nil, local.ErrUnsafePath
	}
	path := filepath.Join(e.Root, ".prifly/state/claim-"+strings.TrimPrefix(id, "claim:")+".lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	if err = flockWithin(file, syscall.LOCK_EX, gitTimeout); err != nil {
		file.Close()
		return nil, err
	}
	return file, nil
}

func (e *Engine) claimNamedWorktree(ctx context.Context, request ClaimRequest) (WorktreeClaim, bool, error) {
	if e.ReadOnly {
		return WorktreeClaim{}, false, local.ErrReadOnly
	}
	if request.CommandID == "" || request.OwnerID == "" || request.Repository == "" || request.WorkspaceMode != "" && request.WorkspaceMode != "worktree" {
		return WorktreeClaim{}, false, fault("project_worktree_template_invalid", "named allocation requires a command, owner, repository and worktree mode")
	}
	if err := validateWorktreeNamingRequest(*request.Naming); err != nil {
		return WorktreeClaim{}, false, err
	}
	identity, err := e.repositoryIdentity(ctx, request.Repository)
	if err != nil {
		return WorktreeClaim{}, false, err
	}
	id := derivedID("claim", e.Installation.ID, request.CommandID)
	lock, err := e.claimEffectLock(id)
	if err != nil {
		return WorktreeClaim{}, false, err
	}
	defer lock.Close()
	payload, err := canonical(map[string]any{"operation": "worktree.claim_named", "repository": identity, "base_ref": request.BaseRef, "owner_id": request.OwnerID, "naming": request.Naming})
	if err != nil {
		return WorktreeClaim{}, false, err
	}
	command := local.AuthorityCommand{ID: request.CommandID, Actor: e.owner, Key: AuthorityClaimsKey, Payload: payload}
	if receipt, err := e.Store.LookupAuthorityReceipt(ctx, command); err == nil {
		if receipt.Rejection != nil {
			return WorktreeClaim{}, false, receipt.Rejection
		}
		claim, err := e.claim(ctx, id)
		if err == nil && claim.Status == "preparing" {
			err = claimPreparationIncomplete(claim)
		}
		return claim, false, err
	} else if !errors.Is(err, local.ErrNotFound) {
		return WorktreeClaim{}, false, err
	}
	control, _, err := e.ensureControl(ctx)
	if err != nil {
		return WorktreeClaim{}, false, err
	}
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationAdmit) {
		return WorktreeClaim{}, false, fault("object_access_denied", "principal cannot claim resources")
	}
	if control.blockingStop(e.Installation.ID, e.Config.ID) != nil {
		return WorktreeClaim{}, false, fault("control_stop_active", "active stop forbids new claims")
	}
	if err := e.releaseSettledClaim(ctx, identity.CommonDir); err != nil {
		return WorktreeClaim{}, false, err
	}
	base := request.BaseRef
	if base == "" {
		base = "HEAD"
	}
	commit, err := e.resolveCommit(ctx, identity.Toplevel, base)
	if err != nil {
		return WorktreeClaim{}, false, err
	}
	busy, err := e.worktreeNameInventory(ctx, identity, request.Naming.Base)
	if err != nil {
		return WorktreeClaim{}, false, err
	}
	observed := e.clock.now()
	until, err := time.Parse(time.RFC3339Nano, observed.UTC)
	if err != nil {
		return WorktreeClaim{}, false, err
	}
	prepared := WorktreeClaim{ID: id, Repository: identity, Mode: "worktree", BaseCommit: commit, Generation: 1, OwnerID: request.OwnerID, Actor: e.owner, Status: "preparing", Claimed: observed, LeaseUntil: until.Add(claimLease).Format(time.RFC3339Nano), Process: ClaimProcess{PID: os.Getpid(), Session: e.clock.session, Started: e.clock.start.UTC().Format(time.RFC3339Nano)}}
	result, err := e.Store.ApplyAuthority(ctx, command, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
		record, err := e.decodeClaims(s)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		if len(record.Claims) >= MaxWorktreeClaims {
			return local.AuthorityChange{}, fault("claim_capacity", "release recorded claims before taking another")
		}
		selected, err := allocateWorktreeName(record, prepared, *request.Naming, busy, 0)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		for _, old := range record.Claims {
			if old.Path == selected.Path && old.Generation >= selected.Generation {
				selected.Generation = old.Generation + 1
			}
		}
		record.SchemaVersion = NamedAuthorityClaimsVersion
		record.Claims = append(record.Claims, selected)
		data, err := canonicalState(record)
		return local.AuthorityChange{Data: data, Result: json.RawMessage(`{"status":"preparing"}`)}, err
	})
	if err != nil {
		return WorktreeClaim{}, false, err
	}
	if result.Receipt.Rejection != nil {
		return WorktreeClaim{}, false, result.Receipt.Rejection
	}
	claim, err := e.claim(ctx, id)
	if err != nil {
		return claim, false, err
	}
	if result.Duplicate {
		if claim.Status == "preparing" {
			err = claimPreparationIncomplete(claim)
		}
		return claim, false, err
	}
	claim, err = e.prepareNamedWorktree(ctx, claim)
	return claim, true, err
}

func claimPreparationIncomplete(claim WorktreeClaim) error {
	return faultf("claim_preparation_incomplete", "claim %s generation %d at %s is still preparing; inspect claim list and doctor --claim %s before claim release", claim.ID, claim.Generation, claim.Path, claim.ID)
}

// Each phase change is a CAS over the recorded preparation. The supplied
// observation is computed outside this transform, including collision reads.
func (e *Engine) updateNamedClaim(ctx context.Context, before, after WorktreeClaim) (WorktreeClaim, error) {
	payload, err := canonical(map[string]any{"before": before, "after": after})
	if err != nil {
		return before, err
	}
	result, err := e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: derivedID("command", before.ID, strconv.Itoa(before.Naming.Ordinal), after.Naming.Phase, strconv.Itoa(after.Naming.Ordinal)), Actor: e.owner, Key: AuthorityClaimsKey, Payload: payload}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
		record, err := e.decodeClaims(s)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		for i, current := range record.Claims {
			if current.ID != before.ID {
				continue
			}
			if current.Status != "preparing" || current.Generation != before.Generation || current.Naming == nil || *current.Naming != *before.Naming {
				return local.AuthorityChange{}, fault("claim_generation_conflict", "preparation changed before phase commit")
			}
			for _, other := range record.Claims {
				if other.ID != current.ID && other.active() && (other.Path == after.Path || other.Repository.CommonDir == after.Repository.CommonDir && other.Branch == after.Branch) {
					return local.AuthorityChange{}, fault("claim_conflict", "candidate was reserved by another claim")
				}
			}
			if after.Path != before.Path {
				for _, previous := range record.Claims {
					if previous.ID != before.ID && previous.Path == after.Path && previous.Generation >= after.Generation {
						after.Generation = previous.Generation + 1
					}
				}
			}
			record.Claims[i] = after
			data, err := canonicalState(record)
			return local.AuthorityChange{Data: data, Result: json.RawMessage(`{}`)}, err
		}
		return local.AuthorityChange{}, local.ErrNotFound
	})
	if err != nil {
		return before, err
	}
	if result.Receipt.Rejection != nil {
		return before, result.Receipt.Rejection
	}
	return e.claim(ctx, before.ID)
}

func (e *Engine) namedPhase(ctx context.Context, claim WorktreeClaim, phase string, device, inode uint64) (WorktreeClaim, error) {
	after := claim
	naming := *claim.Naming
	naming.Phase = phase
	after.Naming = &naming
	if inode != 0 {
		after.Device, after.Inode = device, inode
	}
	return e.updateNamedClaim(ctx, claim, after)
}

func (e *Engine) prepareNamedWorktree(ctx context.Context, claim WorktreeClaim) (WorktreeClaim, error) {
	purity.Guard("prepare named worktree")
	if err := e.checkClaimRoot(); err != nil {
		return claim, err
	}
	if err := os.MkdirAll(filepath.Join(e.Root, filepath.FromSlash(ClaimRoot)), 0700); err != nil {
		return claim, err
	}
	for claim.Naming.Ordinal <= MaxWorktreeNames {
		target := filepath.Join(e.Root, filepath.FromSlash(claim.Path))
		err := os.Mkdir(target, 0700)
		collision := errors.Is(err, os.ErrExist)
		if err != nil && !collision {
			return claim, err
		}
		if !collision {
			info, err := os.Lstat(target)
			if err != nil {
				return claim, err
			}
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok || !info.IsDir() {
				return claim, local.ErrUnsafePath
			}
			claim, err = e.namedPhase(ctx, claim, "directory", uint64(stat.Dev), stat.Ino)
			if err != nil {
				return claim, err
			}
			claim, err = e.namedPhase(ctx, claim, "creating_branch", 0, 0)
			if err != nil {
				return claim, err
			}
			_, err = e.git(ctx, claim.Repository.Toplevel, "update-ref", "--create-reflog", "-m", claim.ID, "refs/heads/"+claim.Branch, claim.BaseCommit, strings.Repeat("0", 40))
			if err != nil {
				// A failed command is not proof of no effect. Only an observed
				// foreign ref and our still-empty directory allow a retry.
				if e.namedBranchOwned(ctx, claim) == nil {
					return claim, claimPreparationIncomplete(claim)
				}
				refs, probeErr := e.git(ctx, claim.Repository.Toplevel, "for-each-ref", "--format=%(refname)", "refs/heads/prifly")
				occupied := false
				ref := "refs/heads/" + claim.Branch
				for _, existing := range strings.Fields(refs) {
					if existing == ref || strings.HasPrefix(existing, ref+"/") || strings.HasPrefix(ref, existing+"/") {
						occupied = true
						break
					}
				}
				if probeErr != nil || !occupied {
					return claim, claimPreparationIncomplete(claim)
				}
				if err := removeOwnedEmptyClaimDirectory(target, claim); err != nil {
					return claim, claimPreparationIncomplete(claim)
				}
				collision = true
			} else {
				claim, err = e.namedPhase(ctx, claim, "branch", 0, 0)
				if err != nil {
					return claim, err
				}
				if _, err = e.git(ctx, claim.Repository.Toplevel, "worktree", "add", "--end-of-options", target, claim.Branch); err != nil {
					return claim, claimPreparationIncomplete(claim)
				}
				claim, err = e.namedPhase(ctx, claim, "tree", 0, 0)
				if err != nil {
					return claim, err
				}
				return e.activateClaim(ctx, claim.ID, claim.Device, claim.Inode)
			}
		}
		if collision {
			busy, err := e.worktreeNameInventory(ctx, claim.Repository, claim.Naming.Base)
			if err != nil {
				return claim, err
			}
			record, _, err := e.readClaims(ctx)
			if err != nil {
				return claim, err
			}
			after, err := allocateWorktreeName(record, claim, WorktreeNamingRequest{Base: claim.Naming.Base, Policy: claim.Naming.Policy}, busy, claim.Naming.Ordinal)
			if err != nil {
				return claim, err
			}
			claim, err = e.updateNamedClaim(ctx, claim, after)
			if err != nil {
				return claim, err
			}
		}
	}
	return claim, fault("claim_name_exhausted", "no available worktree name")
}

func (e *Engine) namedBranchOwned(ctx context.Context, claim WorktreeClaim) error {
	log, err := e.git(ctx, claim.Repository.Toplevel, "reflog", "show", "--format=%gs", "refs/heads/"+claim.Branch)
	if err != nil {
		return claimPreparationIncomplete(claim)
	}
	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) == 0 || lines[len(lines)-1] != claim.ID {
		return fault("claim_identity_conflict", "branch creation evidence does not belong to claim "+claim.ID)
	}
	return nil
}

func removeOwnedEmptyClaimDirectory(target string, claim WorktreeClaim) error {
	info, err := os.Lstat(target)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || claim.Inode == 0 || stat.Ino != claim.Inode {
		return local.ErrUnsafePath
	}
	return os.Remove(target)
}

func (e *Engine) removeNamedWorktree(ctx context.Context, claim WorktreeClaim) error {
	purity.Guard("remove named worktree")
	if err := e.checkClaimRoot(); err != nil {
		return err
	}
	path, branch := worktreeCandidate(claim.Naming.Base, claim.Naming.Ordinal)
	if err := ValidateWorktreeName(claim.Naming.Base); err != nil {
		return err
	}
	if path != claim.Path || branch != claim.Branch || claim.Naming.Ordinal < 1 || claim.Naming.Ordinal > MaxWorktreeNames {
		return local.ErrUnsafePath
	}
	target := filepath.Join(e.Root, filepath.FromSlash(path))
	info, err := os.Lstat(target)
	present := err == nil
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if present {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || !info.IsDir() || claim.Inode == 0 || claim.Inode != stat.Ino {
			return fault("claim_identity_conflict", "directory identity differs from claim "+claim.ID)
		}
	}
	ref, refErr := e.git(ctx, claim.Repository.Toplevel, "show-ref", "--verify", "--hash", "refs/heads/"+branch)
	branchExists := refErr == nil && ref != ""
	if branchExists && claim.Naming.Phase != "reserved" && claim.Naming.Phase != "directory" {
		if err := e.namedBranchOwned(ctx, claim); err != nil {
			return err
		}
	} else if refErr != nil {
		// rev-parse failure alone does not prove absence (permissions, a
		// missing repository or corrupt refs must leave the fence in place).
		refs, err := e.git(ctx, claim.Repository.Toplevel, "for-each-ref", "--format=%(refname)", "refs/heads/"+branch)
		if err != nil {
			return err
		}
		if strings.Contains(refs, "refs/heads/"+branch+"\n") || strings.TrimSpace(refs) == "refs/heads/"+branch {
			return claimPreparationIncomplete(claim)
		}
	}
	if present {
		registered := claim.Naming.Phase == "tree"
		if !registered && branchExists && e.namedBranchOwned(ctx, claim) == nil {
			top, topErr := e.git(ctx, target, "rev-parse", "--show-toplevel")
			common, commonErr := e.git(ctx, target, "rev-parse", "--path-format=absolute", "--git-common-dir")
			checked, branchErr := e.git(ctx, target, "symbolic-ref", "HEAD")
			registered = topErr == nil && commonErr == nil && branchErr == nil && filepath.Clean(top) == target && filepath.Clean(common) == claim.Repository.CommonDir && checked == "refs/heads/"+branch
		}
		if registered {
			if _, err := e.git(ctx, claim.Repository.Toplevel, "worktree", "remove", "--force", "--force", "--end-of-options", target); err != nil {
				return faultf("claim_worktree_removal_failed", "claim %s: %v", claim.ID, err)
			}
		} else {
			// An empty pre-Git directory is ours; a partly materialised tree
			// requires its registered Git identity before it can be removed.
			if err := removeOwnedEmptyClaimDirectory(target, claim); err != nil {
				return claimPreparationIncomplete(claim)
			}
		}
	}
	if branchExists && claim.Naming.Phase != "reserved" && claim.Naming.Phase != "directory" {
		if _, err := e.git(ctx, claim.Repository.Toplevel, "update-ref", "-d", "refs/heads/"+branch, ref); err != nil {
			return faultf("claim_worktree_removal_failed", "claim %s branch changed during cleanup: %v", claim.ID, err)
		}
	}
	if !present {
		_, _ = e.git(ctx, claim.Repository.Toplevel, "worktree", "prune")
	}
	return nil
}

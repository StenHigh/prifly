package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/stenhigh/prifly/internal/local"
)

// A claimed Git worktree is the only place an assisted session may change
// files. The claim is authority state, not a lock file inside the repository:
// a crashed session leaves a recorded claim that still names its owner and
// generation instead of an anonymous directory nobody may remove.
const (
	AuthorityClaimsKey           = "claims"
	AuthorityClaimsVersion       = "authority-claims/2"
	authorityClaimsLegacyVersion = "authority-claims/1"

	ClaimRoot         = ".prifly/work/claims"
	MaxWorktreeClaims = 128
	gitTimeout        = 30 * time.Second
	// ponytail: one fixed lease until a policy contract can state its own; a
	// heartbeat extends it, and its expiry never frees the resource by itself.
	claimLease = 30 * time.Minute
)

var commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Branch names are our own, derived from the claim identity: a caller never
// supplies raw ref text that git could read as an option or a revision.
var claimIDPattern = regexp.MustCompile(`^claim:[0-9a-f]{64}$`)

type ClaimRecord struct {
	SchemaVersion string          `json:"schema_version"`
	AuthorityID   string          `json:"authority_id"`
	Claims        []WorktreeClaim `json:"claims"`
}

// Repository identity is the resolved common directory and top level, not the
// path the caller typed: a moved or symlinked source is the same repository.
type RepositoryIdentity struct {
	CommonDir string `json:"common_dir"`
	Toplevel  string `json:"toplevel"`
}

type WorktreeClaim struct {
	ID         string             `json:"id"`
	Repository RepositoryIdentity `json:"repository"`
	Path       string             `json:"path"`
	// Mode distinguishes a disposable authority-owned worktree from the
	// repository checkout selected explicitly by the developer. A missing mode
	// in a historical record is always the old worktree behavior.
	Mode       string       `json:"mode,omitempty"`
	Branch     string       `json:"branch"`
	BaseCommit string       `json:"base_commit"`
	Generation int64        `json:"generation"`
	OwnerID    string       `json:"owner_id"`
	Actor      string       `json:"actor_id"`
	Status     string       `json:"status"`
	Device     uint64       `json:"device"`
	Inode      uint64       `json:"inode"`
	Claimed    Observation  `json:"claimed"`
	Released   *Observation `json:"released,omitempty"`
	// A lease bounds how long an owner is presumed present. Its expiry moves
	// ownership to suspected, never to free: an absent heartbeat is not proof
	// that the old owner stopped, and a conflicting claim stays blocked until
	// that is settled.
	LeaseUntil string       `json:"lease_until,omitempty"`
	Heartbeat  *Observation `json:"heartbeat,omitempty"`
	Process    ClaimProcess `json:"process"`
}

// ClaimProcess identifies the owner beyond a PID: a reused number must not let
// a later process inherit an earlier owner's claim.
type ClaimProcess struct {
	PID     int    `json:"pid"`
	Session string `json:"clock_session"`
	Started string `json:"started"`
}

func (c WorktreeClaim) active() bool { return c.Status == "preparing" || c.Status == "active" }

type ClaimRequest struct {
	CommandID     string
	Repository    string
	BaseRef       string
	OwnerID       string
	WorkspaceMode string
}

type ClaimReleaseRequest struct {
	CommandID  string
	ClaimID    string
	Generation int64
}

func (e *Engine) readClaims(ctx context.Context) (ClaimRecord, int64, error) {
	snapshot, err := e.Store.ReadAuthority(ctx, AuthorityClaimsKey)
	if errors.Is(err, local.ErrNotFound) {
		return ClaimRecord{SchemaVersion: AuthorityClaimsVersion, AuthorityID: e.Installation.ID, Claims: []WorktreeClaim{}}, 0, nil
	}
	if err != nil {
		return ClaimRecord{}, 0, err
	}
	var record ClaimRecord
	if err := decode(snapshot.Data, &record); err != nil {
		return ClaimRecord{}, 0, err
	}
	if (record.SchemaVersion != AuthorityClaimsVersion && record.SchemaVersion != authorityClaimsLegacyVersion) || record.AuthorityID != e.Installation.ID {
		return ClaimRecord{}, 0, errors.New("unsupported or foreign claim record")
	}
	record.SchemaVersion = AuthorityClaimsVersion
	return record, snapshot.Version, nil
}

// Claims lists the recorded worktree claims of this installation.
func (e *Engine) Claims(ctx context.Context) (ClaimRecord, error) {
	record, _, err := e.readClaims(ctx)
	return record, err
}

// ClaimWorktrees takes several resources in one decision. Taking them one at a
// time would let two callers each hold part of what they need, so the whole set
// is recorded together or not at all. RUN-014 forbids "read the free ones, then
// lock them" as a substitute for this.
func (e *Engine) ClaimWorktrees(ctx context.Context, commandID string, requests []ClaimRequest) ([]WorktreeClaim, error) {
	if e.ReadOnly {
		return nil, local.ErrReadOnly
	}
	if commandID == "" || len(requests) == 0 || len(requests) > MaxWorktreeClaims {
		return nil, errors.New("an atomic claim names one command and 1..128 resources")
	}
	prepared := []WorktreeClaim{}
	seen := map[string]bool{}
	for index, request := range requests {
		if request.Repository == "" || request.OwnerID == "" {
			return nil, errors.New("every claimed resource names its repository and owner")
		}
		identity, err := e.repositoryIdentity(ctx, request.Repository)
		if err != nil {
			return nil, err
		}
		// Two aliases of one repository inside a single request would be two
		// owners of one resource, so the set is checked against itself first.
		if seen[identity.CommonDir] {
			return nil, local.Reject("claim_conflict", "the same repository was named twice in one atomic claim")
		}
		seen[identity.CommonDir] = true
		base := request.BaseRef
		if base == "" {
			base = "HEAD"
		}
		commit, err := e.resolveCommit(ctx, identity.Toplevel, base)
		if err != nil {
			return nil, err
		}
		claimID := derivedID("claim", e.Installation.ID, commandID, strconv.Itoa(index))
		if !claimIDPattern.MatchString(claimID) {
			return nil, local.ErrIntegrity
		}
		suffix := strings.TrimPrefix(claimID, "claim:")
		if request.WorkspaceMode != "" && request.WorkspaceMode != "worktree" {
			return nil, errors.New("atomic claims support worktree mode only")
		}
		prepared = append(prepared, WorktreeClaim{
			ID: claimID, Repository: identity, Path: ClaimRoot + "/" + suffix, Branch: "prifly/" + suffix,
			Mode:       "worktree",
			BaseCommit: commit, OwnerID: request.OwnerID, Actor: e.owner, Status: "preparing",
			Process: ClaimProcess{PID: os.Getpid(), Session: e.clock.session, Started: e.clock.start.UTC().Format(time.RFC3339Nano)},
		})
	}
	control, _, err := e.ensureControl(ctx)
	if err != nil {
		return nil, err
	}
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationAdmit) {
		return nil, local.Reject("object_access_denied", "the session principal cannot claim work resources for this project")
	}
	if stop := control.blockingStop(e.Installation.ID, e.Config.ID); stop != nil {
		return nil, local.Reject("control_stop_active", "an active "+stop.Scope+" stop forbids new claims")
	}
	payload, err := canonical(map[string]any{"operation": "worktree.claim_set", "command_id": commandID, "claims": prepared})
	if err != nil {
		return nil, err
	}
	result, err := e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: commandID, Actor: e.owner, Key: AuthorityClaimsKey, Payload: payload}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
		record, err := e.decodeClaims(s)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		if len(record.Claims)+len(prepared) > MaxWorktreeClaims {
			return local.AuthorityChange{}, local.Reject("claim_capacity", "release recorded claims before taking another set")
		}
		obs := e.clock.now()
		now, err := time.Parse(time.RFC3339Nano, obs.UTC)
		if err != nil {
			return local.AuthorityChange{}, local.ErrIntegrity
		}
		for i := range prepared {
			claim := &prepared[i]
			var generation int64
			for _, existing := range record.Claims {
				if existing.Repository.CommonDir != claim.Repository.CommonDir {
					continue
				}
				if existing.active() {
					if claimPresence(existing, obs) == "suspected" {
						return local.AuthorityChange{}, local.Reject("claim_owner_unproven", "an existing claim's lease expired without proof its owner stopped; resolve it explicitly")
					}
					return local.AuthorityChange{}, local.Reject("claim_conflict", "this repository already has an active worktree claim")
				}
				if existing.Path == claim.Path && existing.Generation > generation {
					generation = existing.Generation
				}
			}
			claim.Generation, claim.Claimed = generation+1, obs
			claim.LeaseUntil = now.Add(claimLease).Format(time.RFC3339Nano)
		}
		record.Claims = append(record.Claims, prepared...)
		data, err := canonicalState(record)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		return local.AuthorityChange{Data: data, Result: json.RawMessage(`{"status":"preparing"}`)}, nil
	})
	if err != nil {
		return nil, err
	}
	if result.Receipt.Rejection != nil {
		return nil, result.Receipt.Rejection
	}
	claims := []WorktreeClaim{}
	for _, claim := range prepared {
		if result.Duplicate {
			existing, err := e.claim(ctx, claim.ID)
			if err != nil {
				return nil, err
			}
			claims = append(claims, existing)
			continue
		}
		device, inode, err := e.createWorktree(ctx, claim)
		if err != nil {
			return claims, err
		}
		activated, err := e.activateClaim(ctx, claim.ID, device, inode)
		if err != nil {
			return claims, err
		}
		claims = append(claims, activated)
	}
	return claims, nil
}

// ClaimWorktree records an exclusive claim and then creates exactly the Git
// worktree it describes. The claim is durable before the directory exists, so
// an interrupted preparation leaves an owned record to clean up rather than an
// unattributable directory.
func (e *Engine) ClaimWorktree(ctx context.Context, request ClaimRequest) (WorktreeClaim, error) {
	if e.ReadOnly {
		return WorktreeClaim{}, local.ErrReadOnly
	}
	if request.CommandID == "" || request.Repository == "" || request.OwnerID == "" {
		return WorktreeClaim{}, errors.New("explicit command, repository and owner required")
	}
	identity, err := e.repositoryIdentity(ctx, request.Repository)
	if err != nil {
		return WorktreeClaim{}, err
	}
	base := request.BaseRef
	if base == "" {
		base = "HEAD"
	}
	commit, err := e.resolveCommit(ctx, identity.Toplevel, base)
	if err != nil {
		return WorktreeClaim{}, err
	}
	mode := request.WorkspaceMode
	if mode == "" {
		mode = "worktree"
	}
	if mode != "worktree" && mode != "checkout" {
		return WorktreeClaim{}, errors.New("workspace mode must be worktree or checkout")
	}
	var checkoutDevice, checkoutInode uint64
	if mode == "checkout" {
		checkoutDevice, checkoutInode, err = e.checkCleanCheckout(ctx, identity)
		if err != nil {
			return WorktreeClaim{}, err
		}
	}
	control, _, err := e.ensureControl(ctx)
	if err != nil {
		return WorktreeClaim{}, err
	}
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationAdmit) {
		return WorktreeClaim{}, local.Reject("object_access_denied", "the session principal cannot claim work resources for this project")
	}
	if stop := control.blockingStop(e.Installation.ID, e.Config.ID); stop != nil {
		return WorktreeClaim{}, local.Reject("control_stop_active", "an active "+stop.Scope+" stop forbids new claims")
	}
	claimID := derivedID("claim", e.Installation.ID, request.CommandID)
	if !claimIDPattern.MatchString(claimID) {
		return WorktreeClaim{}, local.ErrIntegrity
	}
	path := ClaimRoot + "/" + strings.TrimPrefix(claimID, "claim:")
	branch := "prifly/" + strings.TrimPrefix(claimID, "claim:")
	if mode == "checkout" {
		path, branch = identity.Toplevel, ""
	}
	claim := WorktreeClaim{ID: claimID, Repository: identity, Path: path, Mode: mode, Branch: branch, BaseCommit: commit, OwnerID: request.OwnerID, Actor: e.owner, Status: "preparing",
		Process: ClaimProcess{PID: os.Getpid(), Session: e.clock.session, Started: e.clock.start.UTC().Format(time.RFC3339Nano)}}
	payload, err := canonical(map[string]any{"operation": "worktree.claim", "command_id": request.CommandID, "repository": identity, "base_commit": commit, "owner_id": request.OwnerID, "workspace_mode": mode})
	if err != nil {
		return WorktreeClaim{}, err
	}
	result, err := e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: request.CommandID, Actor: e.owner, Key: AuthorityClaimsKey, Payload: payload}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
		record, err := e.decodeClaims(s)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		if len(record.Claims) >= MaxWorktreeClaims {
			return local.AuthorityChange{}, local.Reject("claim_capacity", "release recorded claims before taking another")
		}
		var generation int64
		for _, existing := range record.Claims {
			if existing.Repository.CommonDir != identity.CommonDir {
				continue
			}
			// The conflict relation is the physical repository, not the path a
			// caller typed: two aliases of one repository are one resource.
			if existing.active() {
				state := claimPresence(existing, e.clock.now())
				if state == "suspected" {
					// An expired lease is not proof the old owner stopped. Until
					// that is settled the conflicting claim stays blocked rather
					// than creating a second owner of one resource.
					return local.AuthorityChange{}, local.Reject("claim_owner_unproven", "the existing claim's lease expired without proof its owner stopped; resolve it explicitly")
				}
				return local.AuthorityChange{}, local.Reject("claim_conflict", "this repository already has an active worktree claim")
			}
			if existing.Path == path && existing.Generation > generation {
				generation = existing.Generation
			}
		}
		claim.Generation = generation + 1
		claim.Claimed = e.clock.now()
		leaseEnd, err := time.Parse(time.RFC3339Nano, claim.Claimed.UTC)
		if err != nil {
			return local.AuthorityChange{}, local.ErrIntegrity
		}
		claim.LeaseUntil = leaseEnd.Add(claimLease).Format(time.RFC3339Nano)
		record.Claims = append(record.Claims, claim)
		data, err := canonicalState(record)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		return local.AuthorityChange{Data: data, Result: json.RawMessage(`{"status":"preparing"}`)}, nil
	})
	if err != nil {
		return WorktreeClaim{}, err
	}
	if result.Receipt.Rejection != nil {
		return WorktreeClaim{}, result.Receipt.Rejection
	}
	if result.Duplicate {
		return e.claim(ctx, claimID)
	}
	device, inode := checkoutDevice, checkoutInode
	if mode == "worktree" {
		device, inode, err = e.createWorktree(ctx, claim)
		if err != nil {
			return WorktreeClaim{}, err
		}
	}
	return e.activateClaim(ctx, claimID, device, inode)
}

func (e *Engine) decodeClaims(s local.AuthoritySnapshot) (ClaimRecord, error) {
	record := ClaimRecord{SchemaVersion: AuthorityClaimsVersion, AuthorityID: e.Installation.ID, Claims: []WorktreeClaim{}}
	if s.Version == 0 {
		return record, nil
	}
	if err := decode(s.Data, &record); err != nil {
		return ClaimRecord{}, err
	}
	if (record.SchemaVersion != AuthorityClaimsVersion && record.SchemaVersion != authorityClaimsLegacyVersion) || record.AuthorityID != e.Installation.ID {
		return ClaimRecord{}, errors.New("unsupported or foreign claim record")
	}
	record.SchemaVersion = AuthorityClaimsVersion
	return record, nil
}

func (e *Engine) claim(ctx context.Context, id string) (WorktreeClaim, error) {
	record, _, err := e.readClaims(ctx)
	if err != nil {
		return WorktreeClaim{}, err
	}
	for _, claim := range record.Claims {
		if claim.ID == id {
			return claim, nil
		}
	}
	return WorktreeClaim{}, local.ErrNotFound
}

func (e *Engine) activateClaim(ctx context.Context, id string, device, inode uint64) (WorktreeClaim, error) {
	var activated WorktreeClaim
	payload, err := canonical(map[string]any{"operation": "worktree.activate", "claim_id": id, "device": device, "inode": inode})
	if err != nil {
		return WorktreeClaim{}, err
	}
	result, err := e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: derivedID("command", id, "activate"), Actor: e.owner, Key: AuthorityClaimsKey, Payload: payload}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
		record, err := e.decodeClaims(s)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		found := false
		for i := range record.Claims {
			claim := &record.Claims[i]
			if claim.ID != id {
				continue
			}
			if claim.Status != "preparing" {
				return local.AuthorityChange{}, local.Reject("claim_state_conflict", "only a preparing claim becomes active")
			}
			claim.Status, claim.Device, claim.Inode = "active", device, inode
			activated, found = *claim, true
		}
		if !found {
			return local.AuthorityChange{}, local.Reject("not_found", "claim not found")
		}
		data, err := canonicalState(record)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		return local.AuthorityChange{Data: data, Result: json.RawMessage(`{"status":"active"}`)}, nil
	})
	if err != nil {
		return WorktreeClaim{}, err
	}
	if result.Receipt.Rejection != nil {
		return WorktreeClaim{}, result.Receipt.Rejection
	}
	if activated.ID == "" {
		return e.claim(ctx, id)
	}
	return activated, nil
}

// ReleaseWorktree ends the claim and removes only the directory this exact
// owner and generation created, identified by the device and inode recorded
// when it was made. A replaced or foreign directory blocks removal instead of
// being deleted, and the release is recorded either way.
func (e *Engine) ReleaseWorktree(ctx context.Context, request ClaimReleaseRequest) (WorktreeClaim, error) {
	if e.ReadOnly {
		return WorktreeClaim{}, local.ErrReadOnly
	}
	if request.CommandID == "" || request.ClaimID == "" || request.Generation < 1 {
		return WorktreeClaim{}, errors.New("explicit command, claim and generation required")
	}
	claim, err := e.claim(ctx, request.ClaimID)
	if err != nil {
		return WorktreeClaim{}, err
	}
	if claim.Generation != request.Generation {
		return WorktreeClaim{}, local.Reject("claim_generation_conflict", "a newer generation owns this path")
	}
	if !claim.active() {
		return WorktreeClaim{}, local.Reject("claim_state_conflict", "claim is already released")
	}
	if err := e.removeWorktree(ctx, claim); err != nil {
		return WorktreeClaim{}, err
	}
	payload, err := canonical(map[string]any{"operation": "worktree.release", "command_id": request.CommandID, "claim_id": claim.ID, "generation": claim.Generation})
	if err != nil {
		return WorktreeClaim{}, err
	}
	var released WorktreeClaim
	result, err := e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: request.CommandID, Actor: e.owner, Key: AuthorityClaimsKey, Payload: payload}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
		record, err := e.decodeClaims(s)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		found := false
		for i := range record.Claims {
			current := &record.Claims[i]
			if current.ID != claim.ID {
				continue
			}
			if current.Generation != claim.Generation || !current.active() {
				return local.AuthorityChange{}, local.Reject("claim_generation_conflict", "claim changed before the release committed")
			}
			obs := e.clock.now()
			current.Status, current.Released = "released", &obs
			released, found = *current, true
		}
		if !found {
			return local.AuthorityChange{}, local.Reject("not_found", "claim not found")
		}
		data, err := canonicalState(record)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		return local.AuthorityChange{Data: data, Result: json.RawMessage(`{"status":"released"}`)}, nil
	})
	if err != nil {
		return WorktreeClaim{}, err
	}
	if result.Receipt.Rejection != nil {
		return WorktreeClaim{}, result.Receipt.Rejection
	}
	return released, nil
}

// repositoryIdentity resolves what git itself considers the repository, and
// refuses one that overlaps the authority project root: an assisted session
// must not be able to change the state it is being controlled by.
func (e *Engine) repositoryIdentity(ctx context.Context, path string) (RepositoryIdentity, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return RepositoryIdentity{}, err
	}
	absolute, err = filepath.EvalSymlinks(absolute)
	if err != nil {
		return RepositoryIdentity{}, err
	}
	toplevel, err := e.git(ctx, absolute, "rev-parse", "--show-toplevel")
	if err != nil {
		return RepositoryIdentity{}, fmt.Errorf("not a git repository: %w", err)
	}
	common, err := e.git(ctx, absolute, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return RepositoryIdentity{}, err
	}
	toplevel, err = filepath.EvalSymlinks(toplevel)
	if err != nil {
		return RepositoryIdentity{}, err
	}
	if overlaps(filepath.ToSlash(toplevel), filepath.ToSlash(e.Root)) {
		return RepositoryIdentity{}, fault("unsafe_repository", "a claimed repository cannot overlap the authority project root")
	}
	return RepositoryIdentity{CommonDir: common, Toplevel: toplevel}, nil
}

func (e *Engine) resolveCommit(ctx context.Context, toplevel, ref string) (string, error) {
	if strings.HasPrefix(ref, "-") || strings.ContainsAny(ref, " \t\r\n\x00") {
		return "", errors.New("invalid base reference")
	}
	commit, err := e.git(ctx, toplevel, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("base reference does not resolve to a commit: %w", err)
	}
	if !commitPattern.MatchString(commit) {
		return "", errors.New("git returned an unexpected commit identity")
	}
	return commit, nil
}

func (e *Engine) createWorktree(ctx context.Context, claim WorktreeClaim) (uint64, uint64, error) {
	target := filepath.Join(e.Root, filepath.FromSlash(claim.Path))
	if err := os.MkdirAll(filepath.Join(e.Root, filepath.FromSlash(ClaimRoot)), 0700); err != nil {
		return 0, 0, err
	}
	if _, err := os.Lstat(target); err == nil {
		return 0, 0, errors.New("claim path already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, 0, err
	}
	if _, err := e.git(ctx, claim.Repository.Toplevel, "worktree", "add", "--detach", "--end-of-options", target, claim.BaseCommit); err != nil {
		return 0, 0, err
	}
	// The branch is created from the checked-out base commit, so the claim owns
	// a named ref instead of leaving the session on a detached HEAD.
	if _, err := e.git(ctx, target, "switch", "--create", claim.Branch); err != nil {
		return 0, 0, err
	}
	info, err := os.Lstat(target)
	if err != nil {
		return 0, 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() {
		return 0, 0, local.ErrUnsafePath
	}
	return uint64(stat.Dev), stat.Ino, nil
}

func (e *Engine) checkCleanCheckout(ctx context.Context, repository RepositoryIdentity) (uint64, uint64, error) {
	status, err := e.git(ctx, repository.Toplevel, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return 0, 0, err
	}
	if status != "" {
		return 0, 0, local.Reject("checkout_dirty", "checkout mode requires a clean repository")
	}
	info, err := os.Lstat(repository.Toplevel)
	if err != nil {
		return 0, 0, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() {
		return 0, 0, local.ErrUnsafePath
	}
	return uint64(stat.Dev), stat.Ino, nil
}

// removeWorktree deletes only the recorded directory identity. An absent one is
// already clean; a replaced one is refused, because removing it would delete
// something this claim never created.
func (e *Engine) removeWorktree(ctx context.Context, claim WorktreeClaim) error {
	if claimMode(claim) == "checkout" {
		return nil
	}
	target := filepath.Join(e.Root, filepath.FromSlash(claim.Path))
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		_, _ = e.git(ctx, claim.Repository.Toplevel, "worktree", "prune")
		return nil
	}
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() {
		return local.ErrUnsafePath
	}
	if claim.Status == "active" && (uint64(stat.Dev) != claim.Device || stat.Ino != claim.Inode) {
		return fault("claim_identity_conflict", "the claimed directory is not the one this claim created")
	}
	if _, err := e.git(ctx, claim.Repository.Toplevel, "worktree", "remove", "--force", "--end-of-options", target); err != nil {
		return err
	}
	if _, err := os.Lstat(target); err == nil {
		return errors.New("claim cleanup left the worktree in place")
	}
	_, _ = e.git(ctx, claim.Repository.Toplevel, "branch", "--delete", "--force", "--end-of-options", claim.Branch)
	return nil
}

func claimMode(claim WorktreeClaim) string {
	if claim.Mode == "" {
		return "worktree"
	}
	return claim.Mode
}

func (e *Engine) claimWorkspacePath(claim WorktreeClaim) (string, error) {
	if claimMode(claim) == "checkout" {
		if !filepath.IsAbs(claim.Path) || claim.Path != claim.Repository.Toplevel {
			return "", local.ErrIntegrity
		}
		return claim.Path, nil
	}
	if filepath.IsAbs(claim.Path) || !strings.HasPrefix(claim.Path, ClaimRoot+"/") {
		return "", local.ErrIntegrity
	}
	return filepath.Join(e.Root, filepath.FromSlash(claim.Path)), nil
}

// git runs one typed argv with no shell, no inherited environment beyond what
// git needs, and a bounded deadline. Task text never reaches this call.
func (e *Engine) git(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "git", args...)
	command.Dir = dir
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0"}
	output, err := command.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return "", fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(string(exit.Stderr)))
		}
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// activeClaim returns the single active worktree claim of this installation. An
// assisted step must have somewhere confined to write before it is handed to a
// host. The claim is taken before the Run exists, so it is not owned by a run
// id; ambiguity is refused rather than resolved by guessing which one was meant.
func (e *Engine) activeClaim(ctx context.Context) (WorktreeClaim, error) {
	record, _, err := e.readClaims(ctx)
	if err != nil {
		return WorktreeClaim{}, err
	}
	var active []WorktreeClaim
	for _, claim := range record.Claims {
		if claim.Status == "active" {
			active = append(active, claim)
		}
	}
	if len(active) == 0 {
		return WorktreeClaim{}, local.Reject("claim_missing", "an assisted step requires an active worktree claim")
	}
	if len(active) > 1 {
		return WorktreeClaim{}, local.Reject("claim_ambiguous", "more than one active claim; this slice admits exactly one")
	}
	return active[0], nil
}

// claimPresence reports whether the owner is still presumed present. An expired
// lease yields "suspected", never "free": ownership is settled by evidence, not
// by a clock running out.
func claimPresence(claim WorktreeClaim, obs Observation) string {
	if !claim.active() {
		return "released"
	}
	if claim.LeaseUntil == "" {
		return "present"
	}
	due, err := time.Parse(time.RFC3339Nano, claim.LeaseUntil)
	now, nowErr := time.Parse(time.RFC3339Nano, obs.UTC)
	if err != nil || nowErr != nil || now.Before(due) {
		return "present"
	}
	return "suspected"
}

type ClaimHeartbeatRequest struct {
	CommandID  string
	ClaimID    string
	Generation int64
}

// HeartbeatClaim extends a lease from the owning process. A different process
// cannot extend it: that would let a stranger keep a resource alive.
func (e *Engine) HeartbeatClaim(ctx context.Context, c ClaimHeartbeatRequest) (WorktreeClaim, error) {
	if e.ReadOnly {
		return WorktreeClaim{}, local.ErrReadOnly
	}
	if c.CommandID == "" || c.ClaimID == "" || c.Generation < 1 {
		return WorktreeClaim{}, errors.New("explicit command, claim and generation required")
	}
	payload, err := canonical(map[string]any{"operation": "worktree.heartbeat", "claim_id": c.ClaimID, "generation": c.Generation})
	if err != nil {
		return WorktreeClaim{}, err
	}
	var beaten WorktreeClaim
	result, err := e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: c.CommandID, Actor: e.owner, Key: AuthorityClaimsKey, Payload: payload}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
		record, err := e.decodeClaims(s)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		for i := range record.Claims {
			claim := &record.Claims[i]
			if claim.ID != c.ClaimID {
				continue
			}
			if claim.Generation != c.Generation || !claim.active() {
				return local.AuthorityChange{}, local.Reject("claim_generation_conflict", "a newer generation owns this path")
			}
			if claim.Process.Session != e.clock.session {
				return local.AuthorityChange{}, local.Reject("claim_owner_conflict", "only the owning process extends its own lease")
			}
			obs := e.clock.now()
			now, err := time.Parse(time.RFC3339Nano, obs.UTC)
			if err != nil {
				return local.AuthorityChange{}, local.ErrIntegrity
			}
			claim.Heartbeat, claim.LeaseUntil = &obs, now.Add(claimLease).Format(time.RFC3339Nano)
			beaten = *claim
			data, err := canonicalState(record)
			if err != nil {
				return local.AuthorityChange{}, err
			}
			return local.AuthorityChange{Data: data, Result: json.RawMessage(`{"lease":"extended"}`)}, nil
		}
		return local.AuthorityChange{}, local.Reject("not_found", "claim not found")
	})
	if err != nil {
		return WorktreeClaim{}, err
	}
	if result.Receipt.Rejection != nil {
		return WorktreeClaim{}, result.Receipt.Rejection
	}
	return beaten, nil
}

// ClaimView is the reference CLI projection of the recorded claims.
func ClaimView(record ClaimRecord) map[string]any {
	claims := make([]WorktreeClaim, len(record.Claims))
	copy(claims, record.Claims)
	sort.Slice(claims, func(i, j int) bool { return claims[i].ID < claims[j].ID })
	return map[string]any{"schema_version": "foundation-claims/1", "claims": claims}
}

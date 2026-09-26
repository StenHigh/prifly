package runtime

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/stenhigh/prifly/internal/local"
)

// The claims pin is the primary pin of the coupled Run/authority commit.
// A slot becoming free must never make an already bound checkout available.
type claimRunBinding struct {
	Claim       WorktreeClaim
	Pin         local.ControlPin
	runID       string
	actor       string
	authorityID string
	// handedFrom is the finished Run this claim is handed over from; empty for
	// an ordinary binding.
	handedFrom string
}

func (e *Engine) prepareClaimRunBinding(ctx context.Context, runID, claimID string, generation int64) (*claimRunBinding, error) {
	if runID == "" || claimID == "" && generation != 0 {
		return nil, fault("claim_identity_conflict", "a claim binding requires a Run and an exact claim generation")
	}
	record, version, err := e.readClaims(ctx)
	if err != nil {
		return nil, err
	}
	var selected *WorktreeClaim
	var soleActive *WorktreeClaim
	var legacyProjectClaim *WorktreeClaim
	activeCount := 0
	for _, claim := range record.Claims {
		copy := claim
		if claimID != "" {
			if claim.ID != claimID {
				continue
			}
			if selected != nil {
				return nil, fault("claim_ambiguous", "more than one claim has the selected ID")
			}
			selected = &copy
			continue
		}
		if claim.Status != "active" {
			continue
		}
		activeCount++
		soleActive = &copy
		if claim.RunID == "" && claim.Actor == e.owner && strings.HasPrefix(claim.OwnerID, "project-launch:") {
			commandID := strings.TrimPrefix(claim.OwnerID, "project-launch:")
			if commandID != "" && claim.ID == derivedID("claim", e.Installation.ID, commandID+":workspace") && startRunID(e.owner, commandID) == runID {
				if legacyProjectClaim != nil {
					return nil, fault("claim_ambiguous", "more than one legacy project claim matches this Run")
				}
				legacyProjectClaim = &copy
			}
		}
		if claim.RunID == runID {
			if selected != nil {
				return nil, fault("claim_ambiguous", "this Run holds more than one active claim; select an exact claim")
			}
			selected = &copy
		}
	}
	if claimID == "" && selected == nil {
		if legacyProjectClaim != nil {
			selected = legacyProjectClaim
		} else if activeCount > 1 {
			return nil, fault("claim_ambiguous", "more than one active claim; select an exact claim")
		} else {
			selected = soleActive
		}
	}
	if selected == nil {
		return nil, fault("claim_missing", "an assisted workspace write requires an active claim")
	}
	if claimID != "" && selected.Generation != generation {
		return nil, fault("claim_generation_conflict", "the requested claim generation is no longer current")
	}
	if err := claimAdmissibleForRun(*selected, runID, e.owner, e.clock.now()); err != nil {
		return nil, err
	}
	if selected.RunID == "" {
		if err := e.checkLegacyClaimHolders(ctx, selected.ID, false); err != nil {
			return nil, err
		}
	}
	if err := e.checkClaimDirectory(*selected); err != nil {
		return nil, err
	}
	return &claimRunBinding{Claim: *selected, Pin: local.ControlPin{Key: AuthorityClaimsKey, Version: version}, runID: runID, actor: e.owner, authorityID: e.Installation.ID}, nil
}

func (e *Engine) checkClaimDirectory(claim WorktreeClaim) error {
	path, err := e.claimWorkspacePath(claim)
	if err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	// The device number is not identity: a volume is renumbered across boots on
	// APFS, and comparing it refused admission to the very directory the claim
	// created. The inode is what a replacement changes.
	if !ok || !info.IsDir() || claim.Inode == 0 || stat.Ino != claim.Inode {
		return fault("claim_identity_conflict", "the claimed directory identity changed before admission: the inode at "+claim.Path+" differs from the recorded one, so this is no longer the directory the claim created; claim list names it and claim release --id "+claim.ID+" --generation N ends the claim")
	}
	return nil
}

// prepareClaimHandover hands the tree of a finished Run to the Run created from
// it. The directory is the state: whatever the source's steps left in it,
// committed or not, is what the new Run starts with, so the authority needs no
// knowledge of how that state is kept. The generation moves so that a release
// prepared against the source's binding can no longer remove the tree.
func (e *Engine) prepareClaimHandover(ctx context.Context, runID, sourceRunID, claimID string, generation int64) (*claimRunBinding, error) {
	record, version, err := e.readClaims(ctx)
	if err != nil {
		return nil, err
	}
	var selected *WorktreeClaim
	for _, claim := range record.Claims {
		if claim.ID == claimID {
			copy := claim
			selected = &copy
		}
	}
	if selected == nil || selected.Status != "active" {
		return nil, fault("claim_state_conflict", "the source Run's claim is no longer active; it was released or is being released")
	}
	if selected.RunID != sourceRunID {
		return nil, fault("claim_run_conflict", "the claim is not bound to the source Run it is handed over from")
	}
	if selected.Generation != generation {
		return nil, fault("claim_generation_conflict", "the requested claim generation is no longer current")
	}
	if selected.Actor != e.owner {
		return nil, fault("claim_owner_unproven", "the claim was taken by another actor; only its own actor hands it over")
	}
	source, _, err := e.load(ctx, sourceRunID)
	if err != nil {
		return nil, err
	}
	if !claimRunFinished(source) {
		return nil, fault("claim_run_active", "the source Run is unfinished or uncertain; its tree is handed over only once it is over")
	}
	if err := e.checkClaimDirectory(*selected); err != nil {
		return nil, err
	}
	return &claimRunBinding{Claim: *selected, Pin: local.ControlPin{Key: AuthorityClaimsKey, Version: version}, runID: runID, actor: e.owner, authorityID: e.Installation.ID, handedFrom: sourceRunID}, nil
}

// A lease bounds presence, not ownership. The same actor coming back to the Run
// this claim is already bound to is the owner returning from a pause between its
// steps, where there is nothing to observe: only that Run's progress is at stake,
// and every exclusivity check above still stands.
func claimOwnerReturned(claim WorktreeClaim, runID, actor string) bool {
	return claim.RunID != "" && claim.RunID == runID && claim.Actor != "" && claim.Actor == actor
}

func claimAdmissibleForRun(claim WorktreeClaim, runID, actor string, observed Observation) error {
	if claim.Status != "active" {
		return fault("claim_state_conflict", "the claim is not active or is fenced for release")
	}
	if claim.RunID != "" && claim.RunID != runID {
		return fault("claim_run_conflict", "the claimed workspace remains bound to another Run")
	}
	now, nowErr := time.Parse(time.RFC3339Nano, observed.UTC)
	claimed, claimedErr := time.Parse(time.RFC3339Nano, claim.Claimed.UTC)
	if nowErr != nil || claimedErr != nil {
		return local.ErrIntegrity
	}
	if now.Before(claimed) {
		return fault("claim_owner_unproven", "the current clock predates the recorded claim")
	}
	if claim.LeaseUntil != "" {
		due, dueErr := time.Parse(time.RFC3339Nano, claim.LeaseUntil)
		if dueErr != nil {
			return local.ErrIntegrity
		}
		if !now.Before(due) && !claimOwnerReturned(claim, runID, actor) {
			return fault("claim_owner_unproven", "the claim lease expired and this admission does not prove its owner; extend it with claim heartbeat as the actor claim list names for it")
		}
	}
	return nil
}

func (binding *claimRunBinding) mutate(snapshot local.AuthoritySnapshot, observed Observation) (json.RawMessage, error) {
	record, err := decodeClaimRecord(snapshot, binding.authorityID)
	if err != nil {
		return nil, err
	}
	for index := range record.Claims {
		claim := &record.Claims[index]
		if claim.ID != binding.Claim.ID {
			continue
		}
		if claim.Generation != binding.Claim.Generation || claim.RunID != binding.Claim.RunID {
			return nil, fault("claim_generation_conflict", "claim ownership changed before admission")
		}
		if binding.handedFrom != "" {
			if claim.Status != "active" || claim.RunID != binding.handedFrom || claim.Actor != binding.actor {
				return nil, fault("claim_state_conflict", "the source Run's claim changed before the handover")
			}
			claim.Generation++
		} else if err := claimAdmissibleForRun(*claim, binding.runID, binding.actor, observed); err != nil {
			return nil, err
		}
		// Handing work to this claim is the authority observing its owner, so it
		// renews the lease here rather than leaving a live step to outlast it.
		now, err := time.Parse(time.RFC3339Nano, observed.UTC)
		if err != nil {
			return nil, local.ErrIntegrity
		}
		claim.RunID, claim.LeaseUntil = binding.runID, now.Add(claimLease).Format(time.RFC3339Nano)
		return canonicalState(record)
	}
	return nil, fault("claim_missing", "the selected claim no longer exists")
}

// Only unbound legacy claims need this bounded scan. Every new admission also
// changes/pins the claims record, so a competing modern admission invalidates
// the scan's prepared pin rather than silently creating another holder.
func (e *Engine) checkLegacyClaimHolders(ctx context.Context, claimID string, releasing bool) error {
	snapshots, _, err := e.Store.ReadAll(ctx, maxPinScanRuns)
	if err != nil {
		return err
	}
	for _, snapshot := range snapshots {
		var run Run
		if err := decodeState(snapshot.Data, &run); err != nil || !supportedRun(run) {
			return fault("claim_owner_unproven", "a saved Run cannot be checked for legacy workspace ownership")
		}
		for _, attempt := range run.Attempts {
			if attempt != nil && attempt.Session != nil && attempt.Session.ClaimID == claimID && (!releasing || !claimRunFinished(run)) {
				return fault("claim_owner_unproven", "an unbound legacy claim was already handed to a Run; resolve ownership before reuse")
			}
		}
	}
	return nil
}

func claimRunFinished(run Run) bool {
	return run.terminal() && len(run.Active) == 0 && run.ActiveCheckID == "" && run.PendingAcceptance == nil && run.PendingDecision == nil && !run.HasUnresolvedEffects
}

// runFinishedItsWork names the outcomes after which a worktree holds nothing a
// continuation could still need. Removing a worktree deletes its directory and
// branch, so a partial, rejected, failed or cancelled Run keeps its tree -- with
// whatever its step left uncommitted -- until a linked Run takes it over or the
// operator releases it. A checkout is never removed, so it is not asked this.
func runFinishedItsWork(run Run) bool {
	return run.Outcome != nil && (*run.Outcome == "succeeded" || *run.Outcome == "completed_with_waivers" || *run.Outcome == "no_work")
}

// releaseSettledClaim frees a repository still held by a Run that is over. That
// holder was never the operator's mistake, so the two commands the refusal used
// to demand before every launch happen here instead, on exactly the predicate
// claimReleaseAllowed already uses: bound to a Run that is terminal with nothing
// outstanding. Exclusivity is not traded for convenience -- an unfinished Run
// still refuses, and a claim no Run is bound to is left alone because nothing
// proves its owner stopped. The claiming transaction re-checks the conflict
// afterwards, so a racing holder is still refused rather than overwritten.
func (e *Engine) releaseSettledClaim(ctx context.Context, commonDir string) error {
	record, _, err := e.readClaims(ctx)
	if err != nil {
		return err
	}
	for _, claim := range record.Claims {
		if claim.Repository.CommonDir != commonDir || !claim.active() || claim.RunID == "" {
			continue
		}
		run, _, err := e.load(ctx, claim.RunID)
		if err != nil {
			return err
		}
		if !claimRunFinished(run) || claimMode(claim) == "worktree" && !runFinishedItsWork(run) {
			continue
		}
		command := derivedID("command", claim.ID, "settled-release", strconv.FormatInt(claim.Generation, 10))
		if _, err := e.ReleaseWorktree(ctx, ClaimReleaseRequest{CommandID: command, ClaimID: claim.ID, Generation: claim.Generation}); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) claimReleaseAllowed(ctx context.Context, claim WorktreeClaim) error {
	if claim.RunID == "" {
		return e.checkLegacyClaimHolders(ctx, claim.ID, true)
	}
	run, _, err := e.load(ctx, claim.RunID)
	if err != nil {
		return err
	}
	if !claimRunFinished(run) {
		return fault("claim_run_active", "the bound Run is unfinished or uncertain; finish it with run drive or end it with run cancel, then release its workspace; the lease is not read here")
	}
	return nil
}

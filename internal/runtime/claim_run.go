package runtime

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
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
	for _, claim := range record.Claims {
		if claimID != "" && claim.ID != claimID || claimID == "" && claim.Status != "active" {
			continue
		}
		if selected != nil {
			return nil, fault("claim_ambiguous", "more than one active claim; select an exact claim")
		}
		copy := claim
		selected = &copy
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
	path, err := e.claimWorkspacePath(*selected)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || selected.Device == 0 || selected.Inode == 0 || uint64(stat.Dev) != selected.Device || stat.Ino != selected.Inode {
		return nil, fault("claim_identity_conflict", "the claimed directory identity changed before admission")
	}
	return &claimRunBinding{Claim: *selected, Pin: local.ControlPin{Key: AuthorityClaimsKey, Version: version}, runID: runID, actor: e.owner, authorityID: e.Installation.ID}, nil
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
		if err := claimAdmissibleForRun(*claim, binding.runID, binding.actor, observed); err != nil {
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
		if !claimRunFinished(run) {
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

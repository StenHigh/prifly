package runtime

import (
	"context"
	"encoding/json"
	"os"
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
	if err := claimAdmissibleForRun(*selected, runID, e.clock.now()); err != nil {
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
	return &claimRunBinding{Claim: *selected, Pin: local.ControlPin{Key: AuthorityClaimsKey, Version: version}, runID: runID, authorityID: e.Installation.ID}, nil
}

func claimAdmissibleForRun(claim WorktreeClaim, runID string, observed Observation) error {
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
		if !now.Before(due) {
			return fault("claim_owner_unproven", "the claim lease expired; an answer or free slot does not renew ownership")
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
		if err := claimAdmissibleForRun(*claim, binding.runID, observed); err != nil {
			return nil, err
		}
		claim.RunID = binding.runID
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

func (e *Engine) claimReleaseAllowed(ctx context.Context, claim WorktreeClaim) error {
	if claim.RunID == "" {
		return e.checkLegacyClaimHolders(ctx, claim.ID, true)
	}
	run, _, err := e.load(ctx, claim.RunID)
	if err != nil {
		return err
	}
	if !claimRunFinished(run) {
		return fault("claim_run_active", "the bound Run is unfinished or uncertain; settle it before releasing its workspace")
	}
	return nil
}

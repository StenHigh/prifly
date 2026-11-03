package runtime

import "context"

// SourceWorktreeNaming finds retained naming evidence without transferring the
// claim. Handover changes its holder, but leaves session and fork provenance.
func (e *Engine) SourceWorktreeNaming(ctx context.Context, sourceRunID string) (*WorktreeClaim, error) {
	record, err := e.Claims(ctx)
	if err != nil {
		return nil, err
	}
	return sourceWorktreeNaming(record.Claims, sourceRunID, func(id string) (Run, error) {
		run, _, err := e.load(ctx, id)
		return run, err
	})
}

func sourceWorktreeNaming(claims []WorktreeClaim, sourceRunID string, load func(string) (Run, error)) (*WorktreeClaim, error) {
	source, err := load(sourceRunID)
	if err != nil {
		return nil, err
	}
	// A recorded session names the exact original claim even after handover.
	ids := map[string]bool{}
	for _, attempt := range source.Attempts {
		if attempt != nil && attempt.Session != nil && attempt.Session.ClaimID != "" {
			ids[attempt.Session.ClaimID] = true
		}
	}
	var found *WorktreeClaim
	retain := func(claim WorktreeClaim) error {
		if found != nil {
			if (found.Naming == nil) != (claim.Naming == nil) || found.Naming != nil && (found.Naming.Base != claim.Naming.Base || found.Naming.Policy != claim.Naming.Policy) || !EqualWorktreeNamingSnapshot(found.NamingSnapshot, claim.NamingSnapshot) {
				return fault("project_worktree_binding_invalid", "source Run has conflicting retained naming evidence")
			}
			return nil
		}
		copy := claim
		copy.NamingSnapshot = CloneWorktreeNamingSnapshot(claim.NamingSnapshot)
		found = &copy
		return nil
	}
	for _, claim := range claims {
		if claim.RunID == sourceRunID || ids[claim.ID] {
			if err := retain(claim); err != nil {
				return nil, err
			}
		}
	}
	if found != nil {
		return found, nil
	}
	// A cancelled source may have no assisted session. Follow only native
	// continuation, resume and recovery ancestry, including released claims.
	cache := map[string]Run{sourceRunID: source}
	for _, claim := range claims {
		if claim.NamingSnapshot == nil {
			continue
		}
		id := claim.RunID
		visited := map[string]bool{}
		for hop := 0; ; hop++ {
			if id == sourceRunID {
				if err := retain(claim); err != nil {
					return nil, err
				}
				break
			}
			if visited[id] || hop >= continuationChainLimit {
				return nil, fault("project_worktree_binding_invalid", "source naming ancestry is cyclic or exceeds its depth limit")
			}
			visited[id] = true
			run, ok := cache[id]
			if !ok {
				run, err = load(id)
				if err != nil {
					return nil, err
				}
				cache[id] = run
			}
			if run.Fork == nil || run.Fork.SourceRunID == "" || (run.Fork.Reason != ContinuationReason && run.Fork.Reason != ResumeReason && run.Fork.Reason != RecoveryReason) {
				break
			}
			id = run.Fork.SourceRunID
		}
	}
	return found, nil
}

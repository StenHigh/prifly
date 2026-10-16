package main

import (
	"context"
	"fmt"

	"github.com/stenhigh/prifly/internal/local"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// projectCleanupError adds truthful cleanup context to the original Problem/1
// without changing its code, exit class, cause or public envelope.
type projectCleanupError struct {
	Cause   error
	Detail  string
	Actions []string
}

func (e *projectCleanupError) Error() string { return e.Cause.Error() + "; " + e.Detail }
func (e *projectCleanupError) Unwrap() error { return e.Cause }

type projectCleanupEngine interface {
	StartOutcome(context.Context, string) (string, error)
	RollbackWorktreeClaim(context.Context, prifly.ClaimReleaseRequest) (prifly.WorktreeClaim, error)
	Claims(context.Context) (prifly.ClaimRecord, error)
	SetPackageStatus(context.Context, prifly.PackageLifecycleRequest) (local.AuthorityApplyResult, error)
}

// Reconciliation and fencing precede package rollback. In particular an error
// returned after COMMIT is not proof that the admitted Run failed to exist.
func projectStartCleanup(ctx context.Context, engine *prifly.Engine, command string, claim *prifly.WorktreeClaim, created, imported bool, packageID, packageVersion string, original error) error {
	return projectStartCleanupWith(ctx, engine, command, claim, created, imported, packageID, packageVersion, original)
}

func projectStartCleanupWith(ctx context.Context, engine projectCleanupEngine, command string, claim *prifly.WorktreeClaim, created, imported bool, packageID, packageVersion string, original error) error {
	detail := "start failed before Run admission"
	actions := []string{"doctor"}
	canRollback := true
	outcomeKnownAbsent := true
	{
		run, err := engine.StartOutcome(ctx, command)
		if err != nil {
			p, _ := makeProblem(err)
			detail = "start outcome unknown (" + p.Code + "); preserve claim and package and reconcile command " + command
			canRollback = false
			outcomeKnownAbsent = false
		} else if run != "" {
			detail = "Run " + run + " was created; preserve its claim and package; inspect with run status " + run
			actions = []string{"run status " + run}
			canRollback = false
		}
	}
	if claim != nil {
		actions = append(actions, "doctor --claim "+claim.ID, "claim list")
		if canRollback && created {
			released, err := engine.RollbackWorktreeClaim(ctx, prifly.ClaimReleaseRequest{CommandID: command + ":rollback", ClaimID: claim.ID, Generation: claim.Generation})
			if err == nil && released.Status == "released" {
				detail += fmt.Sprintf("; released claim %s generation %d", claim.ID, claim.Generation)
			} else {
				canRollback = false
				reason := "release returned no confirmed released state"
				if err != nil {
					p, _ := makeProblem(err)
					reason = p.Code + ": " + p.Message
				}
				detail += fmt.Sprintf("; cleanup failed for claim %s requested generation %d: %s", claim.ID, claim.Generation, reason)
			}
		} else if !created {
			detail += "; claim not proven newly created by this call; preserved"
		}
		if !canRollback || !created {
			record, err := engine.Claims(ctx)
			if err != nil {
				detail += "; remaining claim status unknown (claim inspection failed)"
			} else {
				found := false
				for _, current := range record.Claims {
					if current.ID == claim.ID {
						found = true
						detail += fmt.Sprintf("; remaining claim %s generation %d observed status %s", current.ID, current.Generation, current.Status)
						if outcomeKnownAbsent && current.Generation == claim.Generation && current.RunID == "" && current.Status != "released" {
							actions = append(actions, fmt.Sprintf("claim release --id %s --generation %d", current.ID, current.Generation))
						}
					}
				}
				if !found {
					detail += "; requested claim not present in the observed ledger"
				}
			}
		}
	}
	if canRollback && imported {
		result, err := engine.SetPackageStatus(ctx, prifly.PackageLifecycleRequest{CommandID: command + ":rollback-package", ID: packageID, Version: packageVersion, Status: prifly.PackageRemoved, Reason: "project start did not create a Run"})
		if err == nil && result.Receipt.Rejection != nil {
			err = result.Receipt.Rejection
		}
		if err != nil {
			p, _ := makeProblem(err)
			detail += "; package cleanup failed: " + p.Code + ": " + p.Message
		}
	}
	return &projectCleanupError{Cause: original, Detail: detail, Actions: actions}
}

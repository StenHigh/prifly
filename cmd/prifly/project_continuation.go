package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

var projectCommitID = regexp.MustCompile(`^[0-9a-f]{40}$`)

// projectContinuationReview is what a continuation takes, as its workflow
// declared it, and the tree it starts in: the source Run's own, handed over,
// unless the operator names a commit to claim instead.
type projectContinuationReview struct {
	Source          prifly.ContinuationSource `json:"source"`
	WorkspaceCommit string                    `json:"workspace_commit,omitempty"`
}

func projectCheckActiveContinuation(ctx context.Context, engine *prifly.Engine, sourceRunID string) error {
	runs, err := engine.Runs(ctx)
	if err != nil {
		return err
	}
	if run := projectActiveContinuation(runs, sourceRunID); run != nil {
		return refusal("project_continue_active_child", fmt.Sprintf("source Run already has active continuation %s (%s); inspect that Run before another start, or explicitly use --allow-duplicate-continuation for independent work", run.ID, run.Status))
	}
	return nil
}

func projectActiveContinuation(runs []prifly.RunSummary, sourceRunID string) *prifly.RunSummary {
	for _, run := range runs {
		if run.ForkSourceRunID == sourceRunID && run.ForkReason == prifly.ContinuationReason && run.Status != "completed" && run.Status != "failed" && run.Status != "cancelled" {
			return &run
		}
	}
	return nil
}

func projectPrepareContinuation(ctx context.Context, engine *prifly.Engine, repository, runID, workspaceMode, workspaceCommit string, target *flow.Plan) (projectContinuationReview, map[string]json.RawMessage, error) {
	var review projectContinuationReview
	if workspaceCommit != "" && workspaceMode != "worktree" {
		return review, nil, refusal("project_continue_invalid_workspace", "--workspace-commit requires worktree mode so the claimed tree can start at that commit")
	}
	source, err := engine.ContinuationSource(ctx, runID, target)
	if err != nil {
		return review, nil, err
	}
	inputs := map[string]json.RawMessage{}
	for name, carried := range source.Inputs {
		_, value, err := engine.Artifact(carried.Ref)
		if err != nil {
			return review, nil, err
		}
		inputs[name] = value
	}
	if workspaceCommit != "" {
		if workspaceCommit, err = projectContinuationHead(ctx, repository, workspaceCommit); err != nil {
			return review, nil, err
		}
	}
	if workspaceMode == "checkout" && source.Claim == nil {
		status, err := projectGit(ctx, repository, projectGitListTimeout, "status", "--porcelain=v1", "--untracked-files=all")
		if err != nil || status != "" {
			return review, nil, refusal("project_continue_dirty_checkout", "checkout mode requires a clean repository")
		}
	}
	return projectContinuationReview{Source: source, WorkspaceCommit: workspaceCommit}, inputs, nil
}

func projectContinuationHead(ctx context.Context, repository, requested string) (string, error) {
	if !projectCommitID.MatchString(requested) {
		return "", refusal("project_continue_invalid_head", "--workspace-commit requires a full 40-character commit ID")
	}
	head, err := projectGit(ctx, repository, projectGitListTimeout, "rev-parse", "--verify", requested+"^{commit}")
	if err != nil || head != requested {
		return "", refusal("project_continue_invalid_head", "the selected workspace commit is not an available commit")
	}
	return head, nil
}

// projectContinuationPrepare reads what the launch's compiled workflow declares
// it continues from, then reads exactly that from the source Run.
func projectContinuationPrepare(ctx context.Context, engine *prifly.Engine, root string, plan *flow.Plan, sourceRun, workspace, standingWorkspace, workspaceCommit string, allowDuplicate bool) (projectContinuationReview, map[string]json.RawMessage, error) {
	if plan.Workflow.Continuation == nil {
		return projectContinuationReview{}, nil, refusal("project_continue_undeclared", "workflow "+plan.Workflow.ID+" declares no continuation; choose a launch whose workflow declares one")
	}
	if !allowDuplicate {
		if err := projectCheckActiveContinuation(ctx, engine, sourceRun); err != nil {
			return projectContinuationReview{}, nil, err
		}
	}
	mode := workspace
	if mode == "" {
		mode = standingWorkspace
	}
	if mode == "" {
		mode = "worktree"
	}
	return projectPrepareContinuation(ctx, engine, root, sourceRun, mode, workspaceCommit, plan)
}

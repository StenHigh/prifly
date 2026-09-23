package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

var projectCommitID = regexp.MustCompile(`^[0-9a-f]{40}$`)

type projectImplementation struct {
	BaseCommit   string   `json:"base_commit"`
	HeadCommit   string   `json:"head_commit"`
	ChangedFiles []string `json:"changed_files"`
}

type projectContinuationReview struct {
	Source         prifly.ContinuationSource `json:"source"`
	Implementation projectImplementation     `json:"implementation"`
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

func projectPrepareContinuation(ctx context.Context, engine *prifly.Engine, repository, runID, workspaceMode, implementationHead string) (projectContinuationReview, map[string]json.RawMessage, error) {
	var review projectContinuationReview
	if implementationHead != "" && workspaceMode != "worktree" {
		return review, nil, refusal("project_continue_invalid_workspace", "--implementation-head requires worktree mode so the claimed tree can start at that commit")
	}
	source, err := engine.ContinuationSource(ctx, runID)
	if err != nil {
		return review, nil, err
	}
	inputs := map[string]json.RawMessage{}
	for port, ref := range map[string]prifly.ArtifactRef{"task": source.Task, "handoff": source.Handoff, "plan": source.Plan} {
		_, value, err := engine.Artifact(ref)
		if err != nil {
			return review, nil, err
		}
		inputs[port] = value
	}
	_, data, err := engine.Artifact(source.Implementation)
	if err != nil {
		return review, nil, err
	}
	var previous projectImplementation
	if err := json.Unmarshal(data, &previous); err != nil || !projectCommitID.MatchString(previous.BaseCommit) || !projectCommitID.MatchString(previous.HeadCommit) {
		return review, nil, refusal("project_continue_invalid_implementation", "source Implementation has no valid Git base and head")
	}
	head, err := projectContinuationHead(ctx, repository, implementationHead)
	if err != nil {
		return review, nil, err
	}
	for _, ancestor := range []string{previous.BaseCommit, previous.HeadCommit} {
		if _, err := projectGit(ctx, repository, projectGitListTimeout, "merge-base", "--is-ancestor", ancestor, head); err != nil {
			return review, nil, refusal("project_continue_unrelated_head", "selected implementation head must contain the source implementation base and head; when implementation is on an unmerged branch, run project continue --repository DIR --launch ID --host HOST --source-run RUN_ID --implementation-head FULL_40_CHARACTER_COMMIT --workspace worktree --allow-execution --prepare from the primary Project checkout, then use the same SHA for start")
		}
	}
	if workspaceMode == "checkout" {
		status, err := projectGit(ctx, repository, projectGitListTimeout, "status", "--porcelain=v1", "--untracked-files=all")
		if err != nil || status != "" {
			return review, nil, refusal("project_continue_dirty_checkout", "checkout mode requires a clean repository")
		}
	}
	changed, err := projectGit(ctx, repository, projectGitListTimeout, "diff", "--name-only", "-z", "--no-renames", previous.BaseCommit, head)
	if err != nil {
		return review, nil, err
	}
	files := []string{}
	for _, file := range strings.Split(strings.TrimSuffix(changed, "\x00"), "\x00") {
		if file == "" {
			continue
		}
		if !utf8.ValidString(file) {
			return review, nil, refusal("project_continue_invalid_path", "Git diff contains a non-UTF-8 path")
		}
		files = append(files, file)
	}
	if len(files) > 1000 {
		return review, nil, refusal("project_continue_diff_too_large", "the continuation Implementation schema supports at most 1000 changed files")
	}
	review = projectContinuationReview{Source: source, Implementation: projectImplementation{BaseCommit: previous.BaseCommit, HeadCommit: head, ChangedFiles: files}}
	encoded, err := json.Marshal(review.Implementation)
	inputs["implementation"] = encoded
	return review, inputs, err
}

func projectContinuationHead(ctx context.Context, repository, requested string) (string, error) {
	ref := "HEAD"
	if requested != "" {
		if !projectCommitID.MatchString(requested) {
			return "", refusal("project_continue_invalid_head", "--implementation-head requires a full 40-character commit ID")
		}
		ref = requested
	}
	head, err := projectGit(ctx, repository, projectGitListTimeout, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil || !projectCommitID.MatchString(head) || requested != "" && head != requested {
		return "", refusal("project_continue_invalid_head", "selected implementation head is not an available commit")
	}
	return head, nil
}

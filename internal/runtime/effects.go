package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// workspaceMark fingerprints a claimed workspace as it stands: the commit it is
// on and everything git reports as changed against it, untracked files
// included, ignored ones not. A step permitted only its output slot must hand
// the workspace back with the same mark. The value is what the executor can
// reproduce with `git status --porcelain --untracked-files=all` and compare.
func (e *Engine) workspaceMark(ctx context.Context, path string) (string, string, error) {
	head, err := e.git(ctx, path, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return "", "", err
	}
	status, err := e.git(ctx, path, "status", "--porcelain=v1", "--untracked-files=all", "--no-renames")
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256([]byte(head + "\n" + status))
	return "sha256:" + hex.EncodeToString(sum[:]), status, nil
}

// workspaceChanges names the paths whose status differs between two porcelain
// listings, so a refusal can say what moved rather than that something did.
func workspaceChanges(before, after string) []string {
	lines := func(s string) map[string]bool {
		set := map[string]bool{}
		for _, line := range strings.Split(s, "\n") {
			if line != "" {
				set[line] = true
			}
		}
		return set
	}
	was, now := lines(before), lines(after)
	changed := map[string]bool{}
	for line := range now {
		if !was[line] {
			changed[strings.TrimSpace(line[3:])] = true
		}
	}
	for line := range was {
		if !now[line] {
			changed[strings.TrimSpace(line[3:])] = true
		}
	}
	paths := make([]string, 0, len(changed))
	for path := range changed {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

// effectsBoundaryPaths lists the workspaces a Run holds that a step without a
// workspace effect must leave as it found them: every active claim of the Run.
// A step handed its own claim is a writer and is not measured here.
func (e *Engine) effectsBoundaryPaths(ctx context.Context, runID string) (map[string]string, error) {
	record, _, err := e.readClaims(ctx)
	if err != nil {
		return nil, err
	}
	paths := map[string]string{}
	for _, claim := range record.Claims {
		if claim.Status != "active" || claim.RunID != runID {
			continue
		}
		path, err := e.claimWorkspacePath(claim)
		if err != nil {
			return nil, err
		}
		paths[claim.ID] = path
	}
	return paths, nil
}

// checkEffectsBoundary refuses a report from a step that was permitted only
// its output slot and left a workspace of the Run changed. The refusal keeps
// the handoff awaiting, like a malformed report: the executor can put the
// workspace back and report again, or the author can declare the effect.
func (e *Engine) checkEffectsBoundary(ctx context.Context, runID string, attempt *Attempt, step flow.StepDefinition) error {
	if attempt.Session == nil || len(attempt.Session.WorkspaceMarks) == 0 || step.Effects.Class == "workspace_write" {
		return nil
	}
	paths, err := e.effectsBoundaryPaths(ctx, runID)
	if err != nil {
		return err
	}
	for claimID, expected := range attempt.Session.WorkspaceMarks {
		path, held := paths[claimID]
		if !held {
			// The claim ended while the step ran; that is the claim's own
			// story and is refused or accepted elsewhere.
			continue
		}
		mark, status, err := e.workspaceMark(ctx, path)
		if err != nil {
			return err
		}
		if mark == expected {
			continue
		}
		changed := workspaceChanges(attempt.Session.WorkspaceStatus[claimID], status)
		named := strings.Join(changed, ", ")
		if len(changed) > 12 {
			named = strings.Join(changed[:12], ", ") + " and " + strconv.Itoa(len(changed)-12) + " more"
		}
		if named == "" {
			named = "HEAD moved"
		}
		return local.Reject("effect_not_permitted", "this step may write only inside its declared output slot, and the workspace at "+path+" changed while it ran: "+named+". Put the workspace back as it was handed over (git status --porcelain --untracked-files=all must match) and report again. If the named paths are byproducts of a build or a test run, list them in .gitignore -- the mark respects it, and a gate must not be given workspace_write to make room for its own cache. Declare effects.class: workspace_write only for a step that is meant to change the tree")
	}
	return nil
}

// processWorkspaceBoundary is what a program step is handed and held to: the
// Run's claimed repository workspace, when the Run holds exactly one, and --
// for a step permitted no workspace effect -- the mark of that workspace as
// it stood before the program started.
type processWorkspaceBoundary struct {
	claimID, path string
	mark, status  string
	measured      bool
}

func (e *Engine) processWorkspaceBoundary(ctx context.Context, r Run, step flow.StepDefinition) (processWorkspaceBoundary, error) {
	paths, err := e.effectsBoundaryPaths(ctx, r.ID)
	if err != nil {
		return processWorkspaceBoundary{}, err
	}
	if len(paths) != 1 {
		return processWorkspaceBoundary{}, nil
	}
	boundary := processWorkspaceBoundary{}
	for claimID, path := range paths {
		boundary.claimID, boundary.path = claimID, path
	}
	if step.Effects.Class == "workspace_write" || !isEffectsState(r.SchemaVersion) {
		return boundary, nil
	}
	mark, status, err := e.workspaceMark(ctx, boundary.path)
	if err != nil {
		// Not a repository, or no commit yet: nothing to hold the program to.
		return boundary, nil
	}
	boundary.mark, boundary.status, boundary.measured = mark, status, true
	return boundary, nil
}

// changes names what the program changed in the workspace it was shown, or
// returns "" when it was not measured or left the tree as it found it.
func (b processWorkspaceBoundary) changes(ctx context.Context, e *Engine) string {
	if !b.measured {
		return ""
	}
	mark, status, err := e.workspaceMark(ctx, b.path)
	if err != nil || mark == b.mark {
		return ""
	}
	named := strings.Join(workspaceChanges(b.status, status), ", ")
	if named == "" {
		named = "HEAD moved"
	}
	return "this step may write only inside its declared output slot, and the workspace at " + b.path + " changed while its program ran: " + named + ". Byproducts of a build or a test run belong in .gitignore, which the mark respects; declare effects.class: workspace_write only for a step that is meant to change the tree"
}

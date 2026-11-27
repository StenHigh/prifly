package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/stenhigh/prifly/internal/flow"
)

const gitDiffSchemaID = "core:schema/git-diff"

const gitDiffSchema = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "type": "object",
  "required": ["schema_version", "repository", "base_commit", "head_commit", "profile", "scope", "complete", "files"],
  "additionalProperties": false,
  "properties": {
    "schema_version": {"const": "git-diff/1"},
    "repository": {
      "type": "object", "required": ["common_dir", "toplevel"], "additionalProperties": false,
      "properties": {"common_dir": {"type": "string"}, "toplevel": {"type": "string"}}
    },
    "base_commit": {"type": "string", "pattern": "^[0-9a-f]{40}$"},
    "head_commit": {"type": "string", "pattern": "^[0-9a-f]{40}$"},
    "profile": {"const": "myers-context3-renames50"},
    "scope": {"const": "text-and-file-list"},
    "complete": {"const": true},
    "files": {"type": "array", "items": {
      "type": "object", "required": ["status", "old_mode", "new_mode", "kind"],
      "additionalProperties": false,
      "properties": {
        "old_path": {"type": "string"}, "new_path": {"type": "string"},
        "status": {"enum": ["A", "D", "M", "R", "T"]},
        "old_mode": {"type": "string", "pattern": "^[0-7]{6}$"},
        "new_mode": {"type": "string", "pattern": "^[0-7]{6}$"},
        "kind": {"enum": ["text", "binary", "submodule", "metadata"]},
        "patch": {"type": "string"}
      }
    }}
  }
}`

type gitCommitRange struct {
	Base string `json:"base_commit"`
	Head string `json:"head_commit"`
}

type savedGitDiff struct {
	SchemaVersion string             `json:"schema_version"`
	Repository    RepositoryIdentity `json:"repository"`
	gitCommitRange
	Profile  string        `json:"profile"`
	Scope    string        `json:"scope"`
	Complete bool          `json:"complete"`
	Files    []gitDiffFile `json:"files"`
}

type gitDiffFile struct {
	OldPath string `json:"old_path,omitempty"`
	NewPath string `json:"new_path,omitempty"`
	Status  string `json:"status"`
	OldMode string `json:"old_mode"`
	NewMode string `json:"new_mode"`
	Kind    string `json:"kind"`
	Patch   string `json:"patch,omitempty"`
}

func gitDiffOutput(step flow.StepDefinition, port string) bool {
	ref := step.Outputs[port].SchemaRef
	return ref != nil && ref.ID == gitDiffSchemaID && ref.Version == "1.0.0"
}

func validateGitDiffOutputs(step flow.StepDefinition, assisted bool) error {
	for port := range step.Outputs {
		if !gitDiffOutput(step, port) {
			continue
		}
		output := step.Outputs[port]
		implementation, hasImplementation := step.Outputs["implementation"]
		_, hasInput := step.Inputs["implementation"]
		if !assisted || step.Effects.Class != "workspace_write" || output.Format != "json" || workspaceTreeBinding(step, port) != nil ||
			!hasImplementation || implementation.Format != "json" || !slices.Contains(implementation.RequiredFor, "pass") ||
			!gitDiffOutput(step, "diff") || (port == "diff" && !slices.Equal(output.RequiredFor, []string{"pass"})) ||
			(port != "diff" && (port != "step_diff" || !hasInput || len(output.RequiredFor) != 0)) ||
			(hasInput && !gitDiffOutput(step, "step_diff")) {
			return outputProblem("git_diff_binding_invalid", port, "declare a required diff for pass beside the assisted implementation output and an optional step_diff when implementation is also an input")
		}
	}
	return nil
}

var gitCommitID = regexp.MustCompile(`^[0-9a-f]{40}$`)
var gitDiffDigest = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func readGitCommitRange(data []byte) (gitCommitRange, error) {
	var pair gitCommitRange
	if err := json.Unmarshal(data, &pair); err != nil || !gitCommitID.MatchString(pair.Base) || !gitCommitID.MatchString(pair.Head) {
		return gitCommitRange{}, fault("git_diff_range_invalid", "implementation must name two full Git commit identifiers")
	}
	return pair, nil
}

// Use the same Git invocation for the inventory and the patch. The NUL
// delimiters keep paths with spaces, tabs, or newlines unambiguous.
func captureGitDiff(ctx context.Context, workspace string, repository RepositoryIdentity, pair gitCommitRange) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "--no-pager", "--no-replace-objects", "-c", "protocol.allow=never",
		"-c", "diff.mnemonicPrefix=false", "-c", "core.quotePath=true", "-c", "diff.renames=true", "-c", "diff.relative=false",
		"-c", "diff.suppressBlankEmpty=false", "-c", "diff.orderFile=/dev/null",
		"diff", "--raw", "-z", "--patch", "--no-abbrev", "--no-color", "--no-ext-diff", "--no-textconv",
		"--diff-algorithm=myers", "--no-indent-heuristic", "--find-renames=50%", "-l0", "-U3", "--inter-hunk-context=0",
		"--src-prefix=a/", "--dst-prefix=b/", "--submodule=short", "--ignore-submodules=none",
		pair.Base+"^{commit}", pair.Head+"^{commit}", "--")
	cmd.Dir = workspace
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "LC_ALL=C", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_NO_LAZY_FETCH=1", "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0"}
	var output boundedDiffBuffer
	cmd.Stdout = &output
	if err := cmd.Run(); err != nil {
		if output.exceeded {
			return nil, fault("git_diff_too_large", "the complete comparison exceeds the artifact limit")
		}
		return nil, fault("git_diff_capture_failed", fmt.Sprintf("cannot read comparison %s to %s: %v", pair.Base, pair.Head, err))
	}
	files, err := parseGitDiff(output.data.Bytes())
	if err != nil {
		return nil, err
	}
	data, err := canonical(savedGitDiff{
		SchemaVersion: "git-diff/1", Repository: repository, gitCommitRange: pair,
		Profile: "myers-context3-renames50", Scope: "text-and-file-list", Complete: true, Files: files,
	})
	if len(data) > MaxArtifactBytes {
		return nil, fault("git_diff_too_large", "the complete comparison exceeds the artifact limit after JSON encoding")
	}
	return data, err
}

type boundedDiffBuffer struct {
	data     bytes.Buffer
	exceeded bool
}

func (b *boundedDiffBuffer) Write(p []byte) (int, error) {
	if len(p) > MaxArtifactBytes-b.data.Len() {
		b.exceeded = true
		return 0, errors.New("git comparison exceeds the artifact limit")
	}
	return b.data.Write(p)
}

func parseGitDiff(data []byte) ([]gitDiffFile, error) {
	files := []gitDiffFile{}
	invalid := fault("git_diff_capture_invalid", "Git did not return a complete UTF-8 file inventory and patch")
	if !utf8.Valid(data) {
		return nil, invalid
	}
	for len(data) > 0 && data[0] == ':' {
		header, rest, ok := bytes.Cut(data[1:], []byte{0})
		fields := strings.Fields(string(header))
		if !ok || len(fields) != 5 || !strings.Contains("ADMRT", fields[4][:1]) {
			return nil, invalid
		}
		path, rest, ok := bytes.Cut(rest, []byte{0})
		if !ok || len(path) == 0 {
			return nil, invalid
		}
		f := gitDiffFile{Status: fields[4][:1], OldMode: fields[0], NewMode: fields[1], OldPath: string(path), NewPath: string(path)}
		switch f.Status {
		case "A":
			f.OldPath = ""
		case "D":
			f.NewPath = ""
		case "R":
			path, rest, ok = bytes.Cut(rest, []byte{0})
			if !ok || len(path) == 0 {
				return nil, invalid
			}
			f.NewPath = string(path)
		}
		files = append(files, f)
		data = rest
	}
	if len(files) == 0 && len(data) == 0 {
		return files, nil
	}
	prefix := []byte("diff --git ")
	patch, ok := bytes.CutPrefix(data, []byte("\x00diff --git "))
	if !ok {
		return nil, invalid
	}
	patches := bytes.Split(patch, []byte("\ndiff --git "))
	// Git can emit two adjacent patches for a file type change.
	for i := range files {
		f := &files[i]
		count := 1
		if f.Status == "T" {
			count = 2
		}
		if len(patches) < count {
			return nil, invalid
		}
		for _, part := range patches[:count] {
			f.Patch += string(prefix) + strings.TrimSuffix(string(part), "\n") + "\n"
		}
		patches = patches[count:]
		switch {
		case f.OldMode == "160000" || f.NewMode == "160000":
			f.Kind = "submodule"
		case strings.Contains(f.Patch, "\nBinary files "):
			f.Kind = "binary"
		case strings.Contains(f.Patch, "\n@@ "):
			f.Kind = "text"
		default:
			f.Kind = "metadata"
		}
	}
	if len(patches) != 0 {
		return nil, invalid
	}
	return files, nil
}

func gitDiffCapturePath(ref ArtifactRef) string {
	return "tmp/git-diff-" + strings.TrimPrefix(ref.Digest, "sha256:") + ".json"
}

func (e *Engine) gitDiffPairs(attempt *Attempt, implementation []byte) (map[string]gitCommitRange, error) {
	pair, err := readGitCommitRange(implementation)
	if err != nil {
		return nil, err
	}
	pairs := map[string]gitCommitRange{"diff": pair}
	if input, ok := attempt.Context.Inputs["implementation"]; ok {
		_, data, err := e.Artifact(input.Ref)
		if err != nil {
			return nil, err
		}
		before, err := readGitCommitRange(data)
		if err != nil {
			return nil, err
		}
		if before.Head != pair.Head && before.Head != pair.Base {
			pairs["step_diff"] = gitCommitRange{Base: before.Head, Head: pair.Head}
		}
	}
	return pairs, nil
}

func (e *Engine) prepareGitDiffs(ctx context.Context, run Run, attempt *Attempt, step flow.StepDefinition, reported Result) error {
	if !gitDiffOutput(step, "diff") {
		return nil
	}
	for _, port := range []string{"diff", "step_diff"} {
		if _, ok := reported.Outputs[port]; ok {
			return outputProblem("git_diff_host_supplied", port, "the authority captures this comparison")
		}
	}
	if reported.Verdict != "pass" {
		return nil
	}
	slot := attempt.Context.Outputs["implementation"]
	ref, ok := reported.Outputs["implementation"]
	if !ok || ref.ArtifactID != slot.ArtifactID || ref.Revision != slot.Revision {
		return outputProblem("git_diff_range_invalid", "implementation", "report the admitted implementation output")
	}
	data, err := readLocal(attempt.Workspace, slot.Path, MaxArtifactBytes)
	if err != nil || rawDigest(data) != ref.Digest {
		return outputProblem("git_diff_range_invalid", "implementation", "implementation bytes must match the reported digest")
	}
	pairs, err := e.gitDiffPairs(attempt, data)
	if err != nil {
		return err
	}
	// On exact retry the candidate already carries the captured references.
	// A later Git state must not change the meaning of the first report.
	recorded := attempt.Accepted
	if recorded == nil && len(attempt.Candidate) != 0 {
		var previous Result
		if err := decode(attempt.Candidate, &previous); err != nil {
			return err
		}
		recorded = &previous
	}
	if recorded != nil && recorded.Outputs["implementation"] == ref && recorded.Verdict == reported.Verdict {
		for port := range pairs {
			saved, ok := recorded.Outputs[port]
			if !ok {
				return outputProblem("git_diff_archive_missing", port, "recorded result is missing its comparison")
			}
			reported.Outputs[port] = saved
		}
		return nil
	}
	if attempt.Settled != nil || attempt.Session.ClaimID == "" {
		return fault("git_diff_claim_required", "capture requires an active assisted handoff with a repository claim")
	}
	claim, err := e.claim(ctx, attempt.Session.ClaimID)
	if err != nil {
		return err
	}
	if claim.Status != "active" || claim.RunID != run.ID ||
		(attempt.Session.ClaimGeneration != 0 && claim.Generation != attempt.Session.ClaimGeneration) {
		return fault("git_diff_claim_mismatch", "the run no longer owns the claim generation from the handoff")
	}
	workspace, err := e.claimWorkspacePath(claim)
	if err != nil {
		return err
	}
	workspace, err = filepath.EvalSymlinks(workspace)
	if err != nil {
		return err
	}
	common, err := e.git(ctx, workspace, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil || filepath.Clean(common) != filepath.Clean(claim.Repository.CommonDir) {
		return fault("git_diff_claim_mismatch", "the claimed repository identity changed")
	}
	top, err := e.git(ctx, workspace, "rev-parse", "--show-toplevel")
	if err != nil || filepath.Clean(top) != filepath.Clean(workspace) {
		return fault("git_diff_claim_mismatch", "the claim path no longer names its worktree")
	}
	head, err := e.git(ctx, workspace, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || head != pairs["diff"].Head {
		return fault("git_diff_head_mismatch", "the implementation head must match the claimed worktree")
	}
	for port, pair := range pairs {
		var data []byte
		if port == "diff" {
			data = e.reuseGitDiff(run, attempt, claim.Repository, pair)
		}
		if data == nil {
			data, err = captureGitDiff(ctx, workspace, claim.Repository, pair)
		}
		if err != nil {
			return err
		}
		ref := ArtifactRef{ArtifactID: outputArtifactID(attempt.ID, port), Revision: 1, Digest: rawDigest(data)}
		path := gitDiffCapturePath(ref)
		if err := writeCapturedTreeFile(attempt.Workspace, path, data); err != nil {
			existing, readErr := readLocal(attempt.Workspace, path, MaxArtifactBytes)
			if readErr != nil || !bytes.Equal(existing, data) {
				return err
			}
		}
		reported.Outputs[port] = ref
	}
	return nil
}

// The same comparison in an earlier accepted attempt needs a new output
// identity, but can reuse the bytes. Match the actual bound implementation
// ref because a stage name can occur many times in the same Run.
func (e *Engine) reuseGitDiff(run Run, attempt *Attempt, repository RepositoryIdentity, pair gitCommitRange) []byte {
	input, ok := attempt.Context.Inputs["implementation"]
	if !ok {
		return nil
	}
	implementation, _, err := e.Artifact(input.Ref)
	if err != nil {
		return nil
	}
	producer, _ := implementation.Producer["attempt_id"].(string)
	previous := run.Attempts[producer]
	if previous == nil || previous.Accepted == nil || previous.Accepted.Outputs["implementation"] != input.Ref {
		return nil
	}
	artifact, data, err := e.Artifact(previous.Accepted.Outputs["diff"])
	if err != nil || artifact.SchemaRef == nil || artifact.SchemaRef.ID != gitDiffSchemaID || artifact.SchemaRef.Version != "1.0.0" {
		return nil
	}
	var saved savedGitDiff
	if json.Unmarshal(data, &saved) != nil || saved.gitCommitRange != pair || saved.Repository != repository || !saved.Complete {
		return nil
	}
	return data
}

func (e *Engine) verifyClaimGitDiffs(run Run, claim WorktreeClaim) error {
	definitions := run.registry()
	for _, attempt := range run.Attempts {
		if attempt.Session == nil || attempt.Session.ClaimID != claim.ID || attempt.Accepted == nil || attempt.Accepted.Verdict != "pass" {
			continue
		}
		instance := run.Steps[attempt.StepID]
		if instance == nil {
			return fault("git_diff_archive_missing", "accepted attempt has no step instance")
		}
		var step flow.StepDefinition
		if err := decode(definitions[instance.Ref], &step); err != nil {
			return err
		}
		if !gitDiffOutput(step, "diff") {
			continue
		}
		_, implementation, err := e.Artifact(attempt.Accepted.Outputs["implementation"])
		if err != nil {
			return fault("git_diff_archive_missing", "cannot verify the archived implementation range: "+err.Error())
		}
		pairs, err := e.gitDiffPairs(attempt, implementation)
		if err != nil {
			return err
		}
		for port, pair := range pairs {
			ref := attempt.Accepted.Outputs[port]
			artifact, data, err := e.Artifact(ref)
			var saved savedGitDiff
			if err != nil || artifact.SchemaRef == nil || *artifact.SchemaRef != *step.Outputs[port].SchemaRef ||
				json.Unmarshal(data, &saved) != nil || !saved.Complete || saved.gitCommitRange != pair || saved.Repository != claim.Repository {
				return outputProblem("git_diff_archive_missing", port, "the accepted comparison is missing or does not match the claim and commit range")
			}
		}
	}
	return nil
}

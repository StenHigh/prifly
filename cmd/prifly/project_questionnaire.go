package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

type projectDecisionState struct {
	DecisionID    string `json:"decision_id"`
	Phase         string `json:"phase"`
	Applicability string `json:"applicability"`
	Answered      bool   `json:"answered"`
	WaitReason    string `json:"wait_reason,omitempty"`
}

func projectDecisionStates(selection projectPreflight) []projectDecisionState {
	answers := map[string]json.RawMessage{}
	for _, record := range selection.Sheet.Records {
		answers[record.DefinitionID] = record.Value
	}
	// Ask the runtime's existing policy rule for the reason, independently of
	// visibility: an unanswered predecessor must be shown as conditional, not
	// silently treated as false while the questionnaire is still incomplete.
	catalog := selection.Catalog
	catalog.Decisions = append([]prifly.DecisionDefinition(nil), catalog.Decisions...)
	for i := range catalog.Decisions {
		catalog.Decisions[i].When = nil
	}
	blocked := map[string]string{}
	for _, item := range prifly.DecisionsAutonomyCannotTake(&catalog, &selection.Sheet) {
		blocked[item.DecisionID] = item.Reason
	}
	states := make([]projectDecisionState, 0, len(catalog.Decisions))
	for _, definition := range selection.Catalog.Decisions {
		state := projectDecisionState{DecisionID: definition.ID, Phase: definition.Phase, Applicability: "applicable"}
		_, state.Answered = answers[definition.ID]
		if !projectDecisionApplies(definition, selection.PackageProfile, answers) {
			state.Applicability = "inactive"
			if definition.When != nil {
				possible := map[string]json.RawMessage{}
				for id, expected := range definition.When.Answers {
					possible[id] = expected
				}
				for id, actual := range answers {
					possible[id] = actual
				}
				if projectDecisionApplies(definition, selection.PackageProfile, possible) {
					state.Applicability = "conditional"
				}
			}
		}
		if !state.Answered && state.Applicability != "inactive" {
			if definition.Phase == "preflight" && definition.Required {
				state.WaitReason = "required_before_start"
			} else if definition.Phase == "runtime" {
				if selection.Sheet.DecisionPolicy == "attended" {
					state.WaitReason = "owner_answer_if_requested"
				} else {
					state.WaitReason = blocked[definition.ID]
				}
			}
		}
		states = append(states, state)
	}
	return states
}

// The digest is an optimistic review check, not a permission or evidence that
// a separate human answered. No input-file, supporting-file or environment
// bytes are printed. Declared decision values and shared argv are owner-visible
// like the existing final Decision Sheet; they are not secret storage.
type projectLaunchSummary struct {
	SchemaVersion       string                        `json:"schema_version"`
	Repository          string                        `json:"repository"`
	Authority           string                        `json:"authority"`
	Launch              string                        `json:"launch"`
	Host                string                        `json:"host,omitempty"`
	WorkspaceMode       string                        `json:"workspace_mode,omitempty"`
	Package             flow.Ref                      `json:"package"`
	AuthorPackage       *projectBuildIdentity         `json:"author_package,omitempty"`
	BuildKey            string                        `json:"build_key,omitempty"`
	Workflow            flow.Ref                      `json:"workflow"`
	Requirements        *projectLaunchRequirements    `json:"requirements,omitempty"`
	InputDigests        map[string]string             `json:"input_digests"`
	InputRefs           map[string]prifly.ArtifactRef `json:"input_refs"`
	BriefDigest         string                        `json:"brief_digest,omitempty"`
	ConfigurationDigest string                        `json:"configuration_digest"`
	Execution           []projectExecutionReview      `json:"execution"`
	SessionLimits       []prifly.SessionLimitPreview  `json:"session_limits,omitempty"`
	DecisionSheet       prifly.DecisionSheet          `json:"decision_sheet"`
	DecisionStates      []projectDecisionState        `json:"decision_states"`
	KnownQuestionsOnly  bool                          `json:"known_questions_only"`
	ReviewDigest        string                        `json:"review_digest,omitempty"`
}

type projectExecutionReview struct {
	DefinitionRef       flow.Ref          `json:"definition_ref"`
	Executable          string            `json:"executable"`
	ExecutableDigest    string            `json:"executable_digest"`
	Args                []string          `json:"args"`
	FileDigests         map[string]string `json:"file_digests"`
	ConfigurationDigest string            `json:"configuration_digest"`
}

func projectReviewDigest(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	data, err = flow.Canonical(data)
	if err != nil {
		return "", err
	}
	return projectBytesDigest(data), nil
}

func projectBytesDigest(data []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(data)) }

func projectReviewConfiguration(config prifly.ProjectConfig) (string, error) {
	// Installing another exact package must not invalidate an otherwise equal
	// launch review or an exact retry of the launch that installed this package.
	config.InstalledPackages = nil
	return projectReviewDigest(config)
}

func projectReviewExecutors(bindings *prifly.ExecutionBindings) ([]projectExecutionReview, error) {
	result := []projectExecutionReview{}
	if bindings == nil {
		return result, nil
	}
	for _, binding := range bindings.Bindings {
		resolved, err := filepath.EvalSymlinks(binding.Config.Executable)
		if err != nil {
			return nil, err
		}
		binding.Config.Executable = resolved
		digest, err := local.ProcessExecutableDigest(binding.Config.Executable)
		if err != nil {
			return nil, err
		}
		configDigest, err := projectReviewDigest(binding.Config)
		if err != nil {
			return nil, err
		}
		item := projectExecutionReview{DefinitionRef: binding.DefinitionRef, Executable: binding.Config.Executable, ExecutableDigest: digest, Args: binding.Config.Args, FileDigests: map[string]string{}, ConfigurationDigest: configDigest}
		for target, source := range binding.Config.Files {
			item.FileDigests[target] = projectBytesDigest(binding.Files[source])
		}
		result = append(result, item)
	}
	return result, nil
}

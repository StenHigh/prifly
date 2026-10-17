// Package projectwire holds the Project CLI response shapes shared with schema generation.
package projectwire

import (
	"encoding/json"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

type DecisionState struct {
	ID            string `json:"id"`
	Phase         string `json:"phase"`
	Applicability string `json:"applicability"`
	Answered      bool   `json:"answered"`
	WaitReason    string `json:"wait_reason,omitempty"`
}

type LaunchSummary struct {
	WorktreeNaming      *WorktreeNamingReview         `json:"worktree_naming,omitempty"`
	WorktreeNamePreview *prifly.WorktreeNamePreview   `json:"worktree_name_preview,omitempty"`
	WorktreeClaim       *prifly.WorktreeClaim         `json:"worktree_claim,omitempty"`
	QueueTask           *prifly.QueueTaskSelection    `json:"queue_task,omitempty"`
	SchemaVersion       string                        `json:"schema_version"`
	Repository          string                        `json:"repository"`
	Authority           string                        `json:"authority"`
	Launch              string                        `json:"launch"`
	Host                string                        `json:"host,omitempty"`
	WorkspaceMode       string                        `json:"workspace_mode,omitempty"`
	Package             flow.Ref                      `json:"package"`
	AuthorPackage       *BuildIdentity                `json:"author_package,omitempty"`
	BuildKey            string                        `json:"build_key,omitempty"`
	Workflow            flow.Ref                      `json:"workflow"`
	Requirements        *LaunchRequirements           `json:"requirements,omitempty"`
	InputDigests        map[string]string             `json:"input_digests"`
	InputRefs           map[string]prifly.ArtifactRef `json:"input_refs"`
	BriefDigest         string                        `json:"brief_digest,omitempty"`
	ConfigurationDigest string                        `json:"configuration_digest"`
	Execution           []ExecutionReview             `json:"execution"`
	// RegistryBudget is how full the authority's definition budget is after
	// this launch's edition: a project that follows every release fills it by
	// being diligent, and the refusal came without warning.
	RegistryBudget *prifly.RegistryBudget `json:"registry_budget,omitempty"`
	// RegistryRetirement is the older editions of this package the launch
	// withdraws so its edition fits the registry, and the ones it keeps and
	// why. Present only when the edition does not fit as things are. The
	// review digest covers it: a Run or package changed since the review makes
	// the plan, and the reviewed launch, stale.
	RegistryRetirement *prifly.RegistryRetirement   `json:"registry_retirement,omitempty"`
	SessionLimits      []prifly.SessionLimitPreview `json:"session_limits,omitempty"`
	// ModelProfiles is what this launch would seal as the meaning of each
	// declared profile name for the selected host. It belongs in the summary
	// because the Run seals it: without it the digest this summary is checked
	// by says nothing about a machine-local table edited between the review
	// and the start, and the staleness check exists for exactly that class.
	ModelProfiles              map[string]prifly.ModelProfileTranslation `json:"model_profiles,omitempty"`
	DecisionSheet              prifly.DecisionSheet                      `json:"decision_sheet"`
	DecisionStates             []DecisionState                           `json:"decision_states"`
	KnownQuestionsOnly         bool                                      `json:"known_questions_only"`
	ReviewDigest               string                                    `json:"review_digest,omitempty"`
	Continuation               *ContinuationReview                       `json:"continuation,omitempty"`
	AllowDuplicateContinuation bool                                      `json:"allow_duplicate_continuation,omitempty"`
	Recovery                   *prifly.RecoveryPlan                      `json:"recovery,omitempty"`
	// Admission is filled only by --prepare, and deliberately after the review
	// digest is computed: it reads a quantity other Runs move, and folding it in
	// would make the digest go stale on its own.
	Admission *AdmissionPreview `json:"admission,omitempty"`
}

type WorktreeNamingReview struct {
	Template      string                     `json:"template"`
	Bindings      map[string]json.RawMessage `json:"bindings"`
	Values        map[string]string          `json:"values"`
	Base          string                     `json:"base"`
	Root          string                     `json:"root"`
	BranchPrefix  string                     `json:"branch_prefix"`
	Policy        string                     `json:"policy"`
	MaxCandidates int                        `json:"max_candidates"`
}

type AdmissionPreview struct {
	Capacity    int64  `json:"capacity"`
	Held        int64  `json:"held"`
	Waiting     int64  `json:"waiting"`
	Available   int64  `json:"available"`
	WouldRefuse string `json:"would_refuse,omitempty"`
}

type ExecutionReview struct {
	DefinitionRef       flow.Ref          `json:"definition_ref"`
	Executable          string            `json:"executable"`
	ExecutableDigest    string            `json:"executable_digest"`
	Args                []string          `json:"args"`
	FileDigests         map[string]string `json:"file_digests"`
	ConfigurationDigest string            `json:"configuration_digest"`
	// EnvironmentNames are the machine-local variables this program runs with,
	// by name only. The values are what a project puts its database password
	// into, and this document goes to a terminal and a host's log; README
	// promised they were not printed, and until 0.13.33 they were. The values
	// still enter the configuration digest, so a changed one still invalidates
	// a reviewed launch.
	EnvironmentNames []string `json:"environment_names,omitempty"`
	// EnvironmentSources says where a value that is not written down comes
	// from: the reviewer sees the place, never the secret.
	EnvironmentSources map[string]string `json:"environment_sources,omitempty"`
	// LiveOutput says a reader of the Run will see this program's newest
	// output while it works; OutputLimitBytes is what it may print per stream.
	LiveOutput       bool  `json:"live_output,omitempty"`
	OutputLimitBytes int64 `json:"output_limit_bytes,omitempty"`
}

type StartResult struct {
	QueueTask     *prifly.QueueTaskBinding `json:"queue_task,omitempty"`
	SchemaVersion string                   `json:"schema_version"`
	// RunID, NextCommand and RetiredEditions lead the document: the full Run
	// view below runs to tens of kilobytes, and a model reading a started
	// launch could not find the new Run's id in it and fell back to run list.
	RunID string `json:"run_id,omitempty"`
	// NextCommand is what to run next for this Run: the command that takes a
	// task waiting for its host, otherwise run next.
	NextCommand string `json:"next_command,omitempty"`
	// RetiredEditions are the older editions this launch withdrew to fit its
	// own into the registry; package restore undoes each.
	RetiredEditions []prifly.RetiredEdition `json:"retired_editions,omitempty"`
	Repository      string                  `json:"repository"`
	Launch          string                  `json:"launch"`
	Package         flow.Ref                `json:"package"`
	AuthorPackage   *BuildIdentity          `json:"author_package,omitempty"`
	BuildKey        string                  `json:"build_key,omitempty"`
	PackageProfile  string                  `json:"package_profile,omitempty"`
	DecisionSheet   *prifly.DecisionSheet   `json:"decision_sheet,omitempty"`
	// A pointer so an autonomous launch with nothing blocked still reports an
	// empty list: an absent field would read as "nothing to say" rather than
	// "the policy can take every declared runtime decision".
	AutonomyUnanswered *[]prifly.UnansweredDecision `json:"autonomy_unanswered,omitempty"`
	Run                prifly.RunView               `json:"run"`
	Workspace          *prifly.WorktreeClaim        `json:"workspace,omitempty"`
	// WorkspacePath is where that claim actually is. The record's own path is
	// relative to the authority that holds it, and beside repository.toplevel
	// it reads as relative to the repository: a cold start looked for the tree
	// under the repository, found nothing, and fell back to git worktree list.
	WorkspacePath string               `json:"workspace_path,omitempty"`
	LaunchSummary *LaunchSummary       `json:"launch_summary,omitempty"`
	Recovery      *prifly.RecoveryPlan `json:"recovery,omitempty"`
}

type ContinuationReview struct {
	Source          prifly.ContinuationSource `json:"source"`
	WorkspaceCommit string                    `json:"workspace_commit,omitempty"`
}

type BuildIdentity struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type LaunchRequirements struct {
	Assisted      bool              `json:"assisted"`
	GitWorkspace  bool              `json:"git_workspace"`
	EffectClasses map[string]string `json:"effect_classes"`
}

type QueueCommand struct {
	SchemaVersion string          `json:"schema_version"`
	AuthorityID   string          `json:"authority_id"`
	ProjectID     string          `json:"project_id"`
	CommandID     string          `json:"command_id"`
	Operation     string          `json:"operation"`
	Duplicate     bool            `json:"duplicate"`
	Result        json.RawMessage `json:"result"`
}

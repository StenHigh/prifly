package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// An assisted host is a session that already exists: this authority does not
// start it, cannot signal it and holds no pipe to it. So the handoff is a
// recorded fact rather than a process. The host proves which work it holds by
// returning the exact attempt identity and envelope digest it was handed, and
// it acts as the authenticated session principal, never as a token bearer.
const (
	AssistedSessionVersion          = "assisted-session/1"
	AssistedSessionCostVersion      = "assisted-session/2"
	AssistedSessionWorkspaceVersion = "assisted-session/3"
	AssistedSessionTreeVersion      = "assisted-session/4"
	AssistedSessionDecisionVersion  = "assisted-session/5"
	AssistedSessionTimingVersion    = "assisted-session/6"
	ReportedCostVersion             = "reported-cost/1"
	MaxReportedCosts                = 8

	SessionAwaiting         = "awaiting_host"
	SessionReported         = "reported"
	SessionDisconnected     = "disconnected"
	SessionWaitingAdmission = "waiting_admission"
)

// ReportedCost is the exact non-negative decimal amount one source claimed for
// this Attempt. Pri-Fly preserves separate sources and never derives this value
// from usage, reconciles reports or converts their currencies.
type ReportedCost struct {
	SchemaVersion string `json:"schema_version"`
	Source        string `json:"source"`
	Amount        string `json:"amount"`
	Currency      string `json:"currency"`
}

var (
	reportedCostAmount   = regexp.MustCompile(`^(?:0|[1-9][0-9]{0,29})(?:\.[0-9]{1,18})?$`)
	reportedCostCurrency = regexp.MustCompile(`^[A-Z]{3}$`)
	reportedCostSource   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._:/-]{0,127}$`)
)

func validateReportedCosts(costs []ReportedCost) error {
	if len(costs) > MaxReportedCosts {
		return fmt.Errorf("a submission carries at most %d named cost reports", MaxReportedCosts)
	}
	seen := map[string]bool{}
	for _, cost := range costs {
		if cost.SchemaVersion != ReportedCostVersion || !reportedCostSource.MatchString(cost.Source) {
			return errors.New("each reported cost names its schema version and source")
		}
		if !reportedCostAmount.MatchString(cost.Amount) || !reportedCostCurrency.MatchString(cost.Currency) {
			return errors.New("each reported cost carries an exact non-negative decimal amount and uppercase three-letter currency")
		}
		key := cost.Source + "\x00" + cost.Currency
		if seen[key] {
			return errors.New("one source reports at most one amount per currency for an attempt")
		}
		seen[key] = true
	}
	return nil
}

func hasReportedCostStateFields(r Run) bool {
	for _, attempt := range r.Attempts {
		if attempt != nil && (len(attempt.ReportedCosts) != 0 || attempt.Session != nil && (attempt.Session.SchemaVersion == AssistedSessionCostVersion || attempt.Session.SchemaVersion == AssistedSessionWorkspaceVersion || attempt.Session.SchemaVersion == AssistedSessionTreeVersion || attempt.Session.SchemaVersion == AssistedSessionDecisionVersion || attempt.Session.SchemaVersion == AssistedSessionTimingVersion)) {
			return true
		}
	}
	return false
}

func isReportedCostState(version string) bool { return atLeast(version, CoreReportedCostStateVersion) }

// SessionHandoff is what the Run knows about work given to a host. Absence of a
// report is never success: it becomes a disconnected fact that recovery reads.
type SessionHandoff struct {
	SchemaVersion      string                     `json:"schema_version"`
	PrincipalID        string                     `json:"principal_id"`
	SkillRefs          []flow.Ref                 `json:"skill_refs"`
	ClaimID            string                     `json:"claim_id,omitempty"`
	ClaimGeneration    int64                      `json:"claim_generation,omitempty"`
	WorkspaceMode      string                     `json:"workspace_mode,omitempty"`
	WorkspaceTrees     []WorkspaceTreeHandoff     `json:"workspace_trees,omitempty"`
	DecisionContext    map[string]json.RawMessage `json:"decision_context,omitempty"`
	DeliveryGeneration int64                      `json:"delivery_generation,omitempty"`
	Timing             *SessionTiming             `json:"timing,omitempty"`
	HostState          string                     `json:"host_state"`
	// DeadlineTrust names the clock the deadline is enforced on. The local
	// profile reports an unqualified wall clock; nothing here upgrades it.
	DeadlineTrust string       `json:"deadline_trust"`
	Handed        Observation  `json:"handed"`
	Reported      *Observation `json:"reported,omitempty"`
}

func hasSessionStateFields(r Run) bool {
	for _, a := range r.Attempts {
		if a != nil && a.Session != nil {
			return true
		}
	}
	return false
}

func hasWorkspaceStateFields(r Run) bool {
	for _, attempt := range r.Attempts {
		if attempt != nil && attempt.Session != nil && (attempt.Session.SchemaVersion == AssistedSessionWorkspaceVersion || attempt.Session.SchemaVersion == AssistedSessionTreeVersion || attempt.Session.WorkspaceMode != "") {
			return true
		}
	}
	return false
}

// assistedAdapter is the only adapter this core executes without starting a
// process. It is selected by the StepDefinition, never by configuration, so no
// pinned configuration contract changes to admit it.
func assistedAdapter(definitions []PinnedDefinition) flow.Ref {
	return builtinRef(definitions, "core:adapter/assisted-session")
}

func isAssistedExecutor(definitions []PinnedDefinition, executor flow.Executor) bool {
	return executor.AdapterRef == assistedAdapter(definitions) && executor.Operation == "session"
}

// requiresSessionState reports whether any admitted step of this plan is
// executed by a host session rather than by a process this authority starts.
func requiresSessionState(definitions []PinnedDefinition, p *flow.Plan) bool {
	for ref, executor := range executorDefinitions(p) {
		_ = ref
		if isAssistedExecutor(definitions, executor) {
			return true
		}
	}
	return false
}

// Legacy definitions retain their original absolute deadline. New definitions
// pin the separate active and human-wait allowances in SessionLimits.
const assistedAttemptTimeoutMS = 60 * 60 * 1000

// validateAssistedStep keeps the assisted surface narrow: the host may change
// files in its claimed worktree and nothing else, and its skill bytes must be
// pinned context, not text the step invents at run time.
func validateAssistedStep(plan *flow.Plan, step flow.StepDefinition) error {
	if !requiresContextState(plan) {
		return fault("unsupported_executor", "an assisted session step requires the full context contract")
	}
	// A step that only produces a proposal has no business holding write
	// permission on a shared worktree: two such steps would be kept apart by
	// convention alone. Both declarations are admitted, and the declaration is
	// what decides whether a worktree is claimed at all.
	if step.Effects.Class != "workspace_write" && step.Effects.Class != "none" {
		// Two boundaries refuse this, and neither is visible in the step's own
		// declaration: the profile does not qualify the class at all, and the
		// assisted contract narrows it further.
		return fault("unsupported_effect", "an assisted session step declares workspace_write or none; this profile qualifies neither external_write nor destructive, and the assisted contract narrows the rest to workspace_write or none")
	}
	if len(step.RequiredCapabilities) > 0 {
		return fault("unsupported_capability", "the assisted contract supplies no extra capabilities")
	}
	if step.InstructionsRef == nil && len(step.ContextRefs) == 0 {
		return fault("unsupported_context", "an assisted session step names the pinned skill it hands over")
	}
	for _, output := range step.Outputs {
		if output.Format == "blob" && len(output.MediaTypes) > 1 {
			return fault("unsupported_output_media", "a fixed output slot requires one declared media type")
		}
	}
	return nil
}

// assistedSkillRefs is the exact pinned context the host is handed, in a stable
// order. Instructions come first because they are the skill itself.
func assistedSkillRefs(step flow.StepDefinition) []flow.Ref {
	refs := []flow.Ref{}
	if step.InstructionsRef != nil {
		refs = append(refs, *step.InstructionsRef)
	}
	refs = append(refs, step.ContextRefs...)
	return refs
}

// skillFileName names a pinned context file after its reference, so a host
// reads which entry it holds instead of inferring it from the listing order.
func skillFileName(ref flow.Ref) string {
	name := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '.' {
			return r
		}
		return '-'
	}, ref.ID+"@"+ref.Version)
	return strings.Trim(name, "-")
}

// materializeSkills writes the pinned skill bytes into the attempt workspace.
// The host reads files; it never reaches into authority storage. A reference
// the Run did not pin is a preparation failure, not a silent omission.
func (e *Engine) materializeSkills(r Run, workspace string, refs []flow.Ref) ([]LocalPort, error) {
	ports := []LocalPort{}
	if len(refs) == 0 {
		return ports, nil
	}
	if err := os.MkdirAll(filepath.Join(workspace, "context", "skills"), 0700); err != nil {
		return nil, err
	}
	for _, ref := range refs {
		var pinned *PinnedResource
		for j := range r.ContextResources {
			if r.ContextResources[j].Ref == ref {
				pinned = &r.ContextResources[j]
				break
			}
		}
		if pinned == nil {
			return nil, faultf("missing_context", "%s is not pinned by this run", ref.ID)
		}
		if rawDigest(pinned.Bytes) != ref.Digest {
			return nil, local.ErrIntegrity
		}
		// The file is named after the reference it holds. Numbered files made
		// the mapping to skill_refs a matter of order, which is not a contract:
		// a host could only tell a skill from its bridge by reading both.
		path := "context/skills/" + skillFileName(ref)
		if err := writeExclusive(filepath.Join(workspace, filepath.FromSlash(path)), pinned.Bytes); err != nil {
			if errors.Is(err, os.ErrExist) {
				return nil, local.ErrIntegrity
			}
			return nil, err
		}
		ports = append(ports, LocalPort{Ref: ArtifactRef{}, Path: path})
	}
	return ports, nil
}

// assistedReportAdmissible answers a different question from remainingBudget.
// remainingBudget grants execution time and therefore refuses an unqualified
// clock; an assisted host's work is not bounded by this authority at all, and
// its report always arrives from another process, so no monotonic domain is
// ever shared. What the deadline governs here is only whether a report is still
// admitted, decided on recorded UTC.
//
// The bound is therefore exactly as good as the local wall clock, whose trust
// level is recorded in the handoff. A clock change moves it. This is a stated
// limitation of assisted execution, not a qualified guarantee.
func assistedReportAdmissible(admitted, deadline, now Observation) error {
	start, startErr := time.Parse(time.RFC3339Nano, admitted.UTC)
	due, dueErr := time.Parse(time.RFC3339Nano, deadline.UTC)
	current, nowErr := time.Parse(time.RFC3339Nano, now.UTC)
	if startErr != nil || dueErr != nil || nowErr != nil {
		return local.ErrIntegrity
	}
	if current.Before(start) {
		return local.Reject("deadline_clock_unqualified", "the local clock moved behind the handoff; this report cannot be placed in time")
	}
	if !current.Before(due) {
		return local.Reject("attempt_deadline_expired", "the handoff deadline passed before this report")
	}
	return nil
}

// SessionTask is what a host is handed. It carries the exact pinned skill
// references and digests, the sealed context manifest, the claim it may change
// files in, its deadline and the result contract. It carries no credentials and
// no authority handle: the host cannot write state through it.
type SessionTask struct {
	SchemaVersion       string                     `json:"schema_version"`
	RunID               string                     `json:"run_id"`
	AttemptID           string                     `json:"attempt_id"`
	StepInstanceID      string                     `json:"step_instance_id"`
	EnvelopeDigest      string                     `json:"envelope_digest"`
	RunVersion          int64                      `json:"run_version,omitempty"`
	PrincipalID         string                     `json:"principal_id"`
	SkillRefs           []flow.Ref                 `json:"skill_refs"`
	Context             ContextManifest            `json:"context"`
	Workspace           string                     `json:"workspace"`
	ClaimID             string                     `json:"claim_id,omitempty"`
	ClaimGeneration     int64                      `json:"claim_generation,omitempty"`
	ClaimPath           string                     `json:"claim_path,omitempty"`
	WorkspaceMode       string                     `json:"workspace_mode,omitempty"`
	RepositoryWorkspace string                     `json:"repository_workspace,omitempty"`
	WorkspaceTrees      []WorkspaceTreeHandoff     `json:"workspace_trees,omitempty"`
	DecisionSheet       *DecisionSheet             `json:"decision_sheet,omitempty"`
	DecisionContext     map[string]json.RawMessage `json:"decision_context,omitempty"`
	DecisionBridge      bool                       `json:"decision_bridge,omitempty"`
	Delivery            *SessionDelivery           `json:"delivery,omitempty"`
	ResultSchemaRef     flow.Ref                   `json:"result_schema_ref"`
	Deadline            string                     `json:"deadline"`
	PermittedEffects    []string                   `json:"permitted_effects"`
}

// SessionSubmission is the host's terminal report. It names the attempt and the
// envelope digest it was handed, so a report about other work is refused rather
// than accepted under the wrong identity.
type SessionSubmission struct {
	SchemaVersion   string                  `json:"schema_version"`
	RunID           string                  `json:"run_id"`
	AttemptID       string                  `json:"attempt_id"`
	EnvelopeDigest  string                  `json:"envelope_digest"`
	Result          json.RawMessage         `json:"result"`
	ReportedCosts   []ReportedCost          `json:"reported_costs,omitempty"`
	WorkspaceTrees  []WorkspaceTreeLocation `json:"workspace_trees,omitempty"`
	DecisionRequest *DecisionRequest        `json:"decision_request,omitempty"`
}

// SessionTask returns one outstanding handoff for a Run, or a not-found error
// when no attempt is awaiting a host. A Run may hold several at once, so a host
// names the attempt it came for; without a name the first in identity order is
// returned, which is the whole set only when there is one.
func (e *Engine) SessionTask(ctx context.Context, runID, attemptID string) (SessionTask, error) {
	r, view, err := e.load(ctx, runID)
	if err != nil {
		return SessionTask{}, err
	}
	for _, id := range r.Active {
		a := r.Attempts[id]
		if a == nil || a.Session == nil || a.Session.HostState != SessionAwaiting {
			continue
		}
		if attemptID != "" && id != attemptID {
			continue
		}
		if timedSession(a) {
			if err := sessionReportAdmissible(r, a, e.clock.now()); err != nil {
				return SessionTask{}, err
			}
			if r.terminal() || r.HasUnresolvedEffects || r.cancelRequestedFor(r.Activations[a.ActivationID].InvocationID) {
				return SessionTask{}, local.Reject("dispatch_blocked", "this delivery was cancelled; drive the Run to close it")
			}
		}
		activation := r.Activations[a.ActivationID]
		if activation == nil {
			return SessionTask{}, local.ErrIntegrity
		}
		p, err := r.planFor(activation.InvocationID)
		if err != nil {
			return SessionTask{}, err
		}
		step := p.Steps[activation.StageID]
		task := SessionTask{
			SchemaVersion: a.Session.SchemaVersion, RunID: r.ID, AttemptID: a.ID, StepInstanceID: a.StepID,
			EnvelopeDigest: a.EnvelopeDigest, PrincipalID: a.Session.PrincipalID, SkillRefs: a.Session.SkillRefs,
			Context: a.Context, Workspace: a.Workspace, ClaimID: a.Session.ClaimID, ClaimGeneration: a.Session.ClaimGeneration,
			WorkspaceTrees: slices.Clone(a.Session.WorkspaceTrees), DecisionContext: cloneDecisionContext(a.Session.DecisionContext),
			ResultSchemaRef: step.ResultSchemaRef, Deadline: a.Deadline.UTC,
			PermittedEffects: []string{"write_inside_declared_output_slot"},
		}
		if a.Session.SchemaVersion == AssistedSessionDecisionVersion || timedSession(a) {
			task.RunVersion = view.Snapshot.Version
			task.DecisionBridge, task.DecisionSheet = decisionRuntimeAvailable(r.DecisionCatalog, r.DecisionSheet), r.DecisionSheet
		}
		if timedSession(a) {
			delivery := sessionDelivery(a)
			task.Delivery = &delivery
		}
		if step.Effects.Class == "workspace_write" {
			if a.Session.SchemaVersion == AssistedSessionWorkspaceVersion || a.Session.SchemaVersion == AssistedSessionTreeVersion || a.Session.SchemaVersion == AssistedSessionDecisionVersion || a.Session.SchemaVersion == AssistedSessionTimingVersion {
				task.PermittedEffects = []string{"write_inside_claimed_workspace", "local_git_commit_on_claimed_workspace"}
			} else {
				task.PermittedEffects = []string{"write_inside_claimed_worktree", "local_git_commit_on_claimed_base"}
			}
		}
		if a.Session.ClaimID != "" {
			claim, err := e.claim(ctx, a.Session.ClaimID)
			if err != nil {
				return SessionTask{}, err
			}
			task.ClaimPath = claim.Path
			if a.Session.SchemaVersion == AssistedSessionWorkspaceVersion || a.Session.SchemaVersion == AssistedSessionTreeVersion || a.Session.SchemaVersion == AssistedSessionDecisionVersion || a.Session.SchemaVersion == AssistedSessionTimingVersion {
				if a.Session.WorkspaceMode != claimMode(claim) {
					return SessionTask{}, local.ErrIntegrity
				}
				workspace, err := e.claimWorkspacePath(claim)
				if err != nil {
					return SessionTask{}, err
				}
				task.WorkspaceMode, task.RepositoryWorkspace = a.Session.WorkspaceMode, workspace
			}
		}
		return task, nil
	}
	// A Run that holds no handoff is not a Run that does not exist: reporting
	// both as not_found sends the reader looking for the wrong thing.
	return SessionTask{}, &flow.Problem{Code: "no_active_handoff", Message: "this run holds no handoff awaiting a host; read its next action or drive it"}
}

// SessionTasks lists every outstanding handoff, so a caller can see that a Run
// is holding more than one and which attempt each belongs to.
func (e *Engine) SessionTasks(ctx context.Context, runID string) ([]SessionTask, error) {
	r, _, err := e.load(ctx, runID)
	if err != nil {
		return nil, err
	}
	tasks := []SessionTask{}
	for _, id := range r.Active {
		a := r.Attempts[id]
		if a == nil || a.Session == nil || a.Session.HostState != SessionAwaiting {
			continue
		}
		task, err := e.SessionTask(ctx, runID, id)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

func sessionPublisherAttempt(r Run, principal string, command PublishCommand) (*Attempt, *Activation, error) {
	attempt, activation, err := publicationAttempt(r, command)
	if err != nil || attempt.Session == nil || attempt.Session.PrincipalID != principal {
		return nil, nil, local.Reject("publisher_forbidden", "session principal has no current access to this publication namespace")
	}
	return attempt, activation, nil
}

func currentSessionPublisher(r *Run, principal string, command PublishCommand, definitionRef flow.Ref, invocationID, stageID string, obs Observation) (*Attempt, *Activation, error) {
	attempt, activation, err := sessionPublisherAttempt(*r, principal, command)
	if err != nil {
		return nil, nil, err
	}
	if r.terminal() || r.HasUnresolvedEffects || r.Status == "uncertain" || attempt.Settled != nil ||
		!slices.Contains(r.Active, attempt.ID) || attempt.Session.HostState != SessionAwaiting || attempt.Status != "pending" ||
		r.Steps[attempt.StepID].Ref != definitionRef || activation.InvocationID != invocationID || activation.StageID != stageID {
		return nil, nil, local.Reject("publisher_frozen", "terminal, inactive or fenced session publishers cannot create new publications")
	}
	if err := sessionReportAdmissible(*r, attempt, obs); err != nil {
		return nil, nil, err
	}
	return attempt, activation, nil
}

func (e *Engine) sessionPublisherReceipt(ctx context.Context, command PublishCommand, result local.ApplyResult, err error) (local.ApplyResult, error) {
	if err != nil {
		return result, err
	}
	current, _, readErr := e.load(ctx, command.RunID)
	if readErr == nil {
		_, _, readErr = sessionPublisherAttempt(current, e.owner, command)
	}
	if readErr != nil {
		return local.ApplyResult{}, local.Reject("publisher_forbidden", "session principal no longer has receipt read access")
	}
	return result, nil
}

// PublishSessionPublication is the assisted-host transport for publication
// variants that need the producer workspace or its live generation.
func (e *Engine) PublishSessionPublication(ctx context.Context, command PublishCommand) (local.ApplyResult, error) {
	if e.ReadOnly {
		return local.ApplyResult{}, local.ErrReadOnly
	}
	if err := validatePublishCommand(command); err != nil {
		return local.ApplyResult{}, err
	}
	if command.Kind != "artifact" && command.Kind != "close" {
		return local.ApplyResult{}, local.Reject("unsupported_publication", "assisted publication accepts artifact and close variants")
	}
	control, _, err := e.ensureControl(ctx)
	if err != nil {
		return local.ApplyResult{}, err
	}
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationAdmit) {
		return local.ApplyResult{}, local.Reject("object_access_denied", "the session principal cannot publish for this project")
	}
	r, _, err := e.load(ctx, command.RunID)
	if err != nil {
		return local.ApplyResult{}, err
	}
	attempt, activation, err := sessionPublisherAttempt(r, e.owner, command)
	if err != nil {
		return local.ApplyResult{}, err
	}
	definitionRef := r.Steps[attempt.StepID].Ref
	current := func(r *Run, obs Observation) (*Attempt, *Activation, error) {
		return currentSessionPublisher(r, e.owner, command, definitionRef, activation.InvocationID, activation.StageID, obs)
	}
	receipt := func(result local.ApplyResult, err error) (local.ApplyResult, error) {
		return e.sessionPublisherReceipt(ctx, command, result, err)
	}
	if command.Kind == "close" {
		return e.publishArtifactClosureAs(ctx, command, r, attempt, activation, current, receipt)
	}
	return e.publishArtifactAs(ctx, command, r, attempt, activation, current, receipt)
}

// PublishSessionArtifact retains the narrow Go API used by v2 callers.
func (e *Engine) PublishSessionArtifact(ctx context.Context, command PublishCommand) (local.ApplyResult, error) {
	if command.Kind != "artifact" {
		return local.ApplyResult{}, local.Reject("unsupported_publication", "this transport accepts only the artifact variant")
	}
	return e.PublishSessionPublication(ctx, command)
}

// SubmitSession accepts a host report for exactly the attempt it was handed.
// submissionProblem names the field of a malformed report, so a host corrects
// that field instead of reading the runtime's own schemas to find it.
func submissionProblem(pointer, message string) error {
	return &flow.Problem{Code: "submission_shape_invalid", Path: pointer, Message: message}
}

// It is not a token exchange: the caller must be the enrolled session principal
// and must return the attempt identity and envelope digest of that handoff.
func (e *Engine) SubmitSession(ctx context.Context, submission SessionSubmission) (local.ApplyResult, error) {
	if e.ReadOnly {
		return local.ApplyResult{}, local.ErrReadOnly
	}
	if (submission.SchemaVersion != AssistedSessionVersion && submission.SchemaVersion != AssistedSessionCostVersion && submission.SchemaVersion != AssistedSessionWorkspaceVersion && submission.SchemaVersion != AssistedSessionTreeVersion && submission.SchemaVersion != AssistedSessionDecisionVersion && submission.SchemaVersion != AssistedSessionTimingVersion) || submission.RunID == "" || submission.AttemptID == "" || submission.EnvelopeDigest == "" {
		return local.ApplyResult{}, submissionProblem("/schema_version", "a submission names a supported schema version, run, attempt and envelope digest")
	}
	if submission.DecisionRequest != nil {
		request := submission.DecisionRequest
		if submission.SchemaVersion != AssistedSessionDecisionVersion && submission.SchemaVersion != AssistedSessionTimingVersion || len(submission.Result) != 0 || len(submission.ReportedCosts) != 0 || len(submission.WorkspaceTrees) != 0 || request.RunID != submission.RunID || request.AttemptID != submission.AttemptID || request.EnvelopeDigest != submission.EnvelopeDigest {
			return local.ApplyResult{}, submissionProblem("/decision_request", "a decision request is the only assisted-session/5 submission and must name its delivery")
		}
		return e.RequestDecision(ctx, *request)
	}
	if submission.SchemaVersion == AssistedSessionVersion && len(submission.ReportedCosts) != 0 {
		return local.ApplyResult{}, &flow.Problem{Code: "submission_cost_unsupported", Path: "/reported_costs", Message: "assisted-session/1 cannot report cost"}
	}
	if err := validateReportedCosts(submission.ReportedCosts); err != nil {
		return local.ApplyResult{}, err
	}
	if len(submission.Result) == 0 || !json.Valid(submission.Result) || len(submission.Result) > local.MaxCommandBytes {
		return local.ApplyResult{}, submissionProblem("/result", "a submission carries one bounded JSON result")
	}
	control, _, err := e.ensureControl(ctx)
	if err != nil {
		return local.ApplyResult{}, err
	}
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationAdmit) {
		return local.ApplyResult{}, local.Reject("object_access_denied", "the session principal cannot report for this project")
	}
	r, view, err := e.load(ctx, submission.RunID)
	if err != nil {
		return local.ApplyResult{}, err
	}
	attempt := r.Attempts[submission.AttemptID]
	if attempt == nil || attempt.Session == nil {
		return local.ApplyResult{}, local.Reject("not_found", "no assisted attempt with this identity")
	}
	if attempt.Session.SchemaVersion != submission.SchemaVersion {
		return local.ApplyResult{}, local.Reject("session_version_conflict", "the report does not use the contract this attempt was handed")
	}
	if timedSession(attempt) && (attempt.Session.HostState == sessionWaitingDecision || attempt.Session.HostState == SessionWaitingAdmission || attempt.Session.HostState == SessionDisconnected) {
		return local.ApplyResult{}, local.Reject("session_state_conflict", "a parked or closed delivery cannot publish a result")
	}
	if timedSession(attempt) && attempt.Session.HostState == SessionAwaiting {
		if err := sessionReportAdmissible(r, attempt, e.clock.now()); err != nil {
			return local.ApplyResult{}, err
		}
	}
	canonicalResult, err := flow.Canonical(submission.Result)
	if err != nil {
		return local.ApplyResult{}, local.Reject("invalid_result", "the report is not canonicalizable JSON")
	}
	if err := flow.ValidateProtocol("StepResult", canonicalResult); err != nil {
		return local.ApplyResult{}, err
	}
	var reported Result
	if err := decode(canonicalResult, &reported); err != nil {
		return local.ApplyResult{}, err
	}
	if reported.RunID != r.ID || reported.AttemptID != attempt.ID || reported.StepInstanceID != attempt.StepID || reported.EnvelopeDigest != attempt.EnvelopeDigest {
		return local.ApplyResult{}, local.Reject("result_identity_mismatch", "candidate differs from the assisted handoff")
	}
	activation := r.Activations[attempt.ActivationID]
	if activation == nil {
		return local.ApplyResult{}, local.ErrIntegrity
	}
	plan, err := r.planFor(activation.InvocationID)
	if err != nil {
		return local.ApplyResult{}, err
	}
	step, exists := plan.Steps[activation.StageID]
	if !exists {
		return local.ApplyResult{}, local.ErrIntegrity
	}
	if submission.SchemaVersion != AssistedSessionTreeVersion && submission.SchemaVersion != AssistedSessionDecisionVersion && submission.SchemaVersion != AssistedSessionTimingVersion && len(submission.WorkspaceTrees) != 0 {
		return local.ApplyResult{}, &flow.Problem{Code: "submission_trees_unsupported", Path: "/workspace_trees", Message: "this assisted-session version cannot report workspace trees"}
	}
	// Capture follows the step's declared bindings, not the wording of the
	// report: a step that declares trees has them captured by the runtime even
	// when the host names no location, which is the only form it may use where
	// the location has a single legal value.
	if len(step.WorkspaceTrees) != 0 || len(submission.WorkspaceTrees) != 0 {
		reported, err = e.captureWorkspaceTreeOutputs(attempt, step, reported, submission.WorkspaceTrees)
		if err != nil {
			return local.ApplyResult{}, err
		}
		canonicalResult, err = canonical(reported)
		if err != nil {
			return local.ApplyResult{}, err
		}
		if err := flow.ValidateProtocol("StepResult", canonicalResult); err != nil {
			return local.ApplyResult{}, err
		}
	}
	// Acceptance checks the same ports later, but it settles the attempt: a
	// report it rejects is a failed attempt, and a step that never retries has
	// no second chance. Reading them here keeps a malformed report a refusal
	// with its handoff still awaiting.
	if err := plan.ValidateJSON(step.ResultSchemaRef, canonicalResult); err != nil {
		return local.ApplyResult{}, err
	}
	if _, err := e.readResultOutputs(r, attempt, step, plan, reported); err != nil {
		return local.ApplyResult{}, err
	}
	// The identity covers the report itself, so an exact retry is idempotent
	// while a corrected report is a new command instead of a burnt identity.
	commandID := derivedID("command", submission.AttemptID, "session-report", submission.EnvelopeDigest, rawDigest(canonicalResult))
	payload := map[string]any{"attempt_id": submission.AttemptID, "envelope_digest": submission.EnvelopeDigest, "source": "assisted_session"}
	reportedCosts := append([]ReportedCost(nil), submission.ReportedCosts...)
	if submission.SchemaVersion != AssistedSessionVersion {
		costBytes, err := canonical(reportedCosts)
		if err != nil {
			return local.ApplyResult{}, err
		}
		costDigest := rawDigest(costBytes)
		commandID = derivedID("command", submission.AttemptID, "session-report-v2", submission.EnvelopeDigest, rawDigest(canonicalResult), costDigest)
		payload["reported_costs_digest"] = costDigest
	}
	result, err := e.apply(ctx, e.owner, commandID, r.ID, "attempt.result_candidate", payload, &view.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		current := r.Attempts[submission.AttemptID]
		if current == nil || current.Session == nil {
			return local.Change{}, local.Reject("not_found", "no assisted attempt with this identity")
		}
		if current.Session.PrincipalID != e.owner {
			return local.Change{}, local.Reject("session_identity_conflict", "this attempt was handed to a different session principal")
		}
		if current.Session.SchemaVersion != submission.SchemaVersion {
			return local.Change{}, local.Reject("session_version_conflict", "the report does not use the contract this attempt was handed")
		}
		if current.EnvelopeDigest != submission.EnvelopeDigest {
			return local.Change{}, local.Reject("envelope_conflict", "the report does not carry the envelope this attempt was handed")
		}
		if current.Session.HostState != SessionAwaiting {
			return local.Change{}, local.Reject("session_state_conflict", "this handoff is no longer awaiting a host report")
		}
		if err := sessionReportAdmissible(*r, current, obs); err != nil {
			return local.Change{}, err
		}
		if r.admissionsBlockedFor(r.Activations[current.ActivationID].InvocationID) || r.cancelRequestedFor(r.Activations[current.ActivationID].InvocationID) {
			return local.Change{}, local.Reject("dispatch_blocked", "a restriction forbids accepting new work for this attempt")
		}
		current.Session.HostState, current.Session.Reported = SessionReported, &obs
		current.Candidate = append(json.RawMessage(nil), canonicalResult...)
		current.CandidateAt = &obs
		current.ReportedCosts = reportedCosts
		data, err := canonical(map[string]any{"observation": obs, "attempt_id": current.ID, "envelope_digest": current.EnvelopeDigest, "source": "assisted_session", "candidate_digest": rawDigest(canonicalResult)})
		if err != nil {
			return local.Change{}, err
		}
		events := []local.EventInput{{Type: "attempt.result_candidate", Version: 1, Data: data}}
		if len(reportedCosts) != 0 {
			costData, err := canonical(map[string]any{"observation": obs, "attempt_id": current.ID, "step_instance_id": current.StepID, "reported_by": e.owner, "reported_costs": reportedCosts})
			if err != nil {
				return local.Change{}, err
			}
			events = append(events, local.EventInput{Type: "attempt.cost_reported", Version: 1, Data: costData})
		}
		return local.Change{Events: events}, nil
	})
	if err != nil {
		return result, err
	}
	// The report is recorded before it is judged, so a crash between the two
	// leaves a reported handoff the driver can still close, not a lost result.
	return result, e.settleAssisted(ctx, submission.RunID, submission.AttemptID)
}

// settleAssisted closes an attempt on the host's own report. It classifies the
// outcome from the report and the declared outputs only, and records no process
// facts, so the journal never claims an observation it did not make. It is
// idempotent: its command identity is derived from the attempt.
func (e *Engine) settleAssisted(ctx context.Context, runID, attemptID string) error {
	evidence := settlementEvidence{Kind: "host_report", Actor: e.owner, CommandID: derivedID("command", attemptID, "session-settle")}
	return e.settleWith(ctx, runID, attemptID, evidence, nil)
}

// MarkSessionDisconnected records that a handed attempt passed its deadline
// without a report. It never becomes a success and never retries by itself:
// the run keeps an honest non-terminal fact for recovery to resolve.
func (e *Engine) MarkSessionDisconnected(ctx context.Context, runID, attemptID string) (local.ApplyResult, error) {
	if e.ReadOnly {
		return local.ApplyResult{}, local.ErrReadOnly
	}
	run, view, err := e.load(ctx, runID)
	if err != nil {
		return local.ApplyResult{}, err
	}
	if timedSession(run.Attempts[attemptID]) {
		return local.ApplyResult{}, local.Reject("session_state_conflict", "timed deliveries use run drive to reconcile active/wait expiry; this legacy disconnect command cannot decide their outcome")
	}
	commandID := derivedID("command", attemptID, "session-disconnect", fmt.Sprint(view.Snapshot.Version))
	return e.apply(ctx, e.owner, commandID, runID, "diagnostic.recorded", map[string]any{"attempt_id": attemptID, "code": "assisted_host_disconnected"}, &view.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		current := r.Attempts[attemptID]
		if current == nil || current.Session == nil {
			return local.Change{}, local.Reject("not_found", "no assisted attempt with this identity")
		}
		if current.Session.HostState != SessionAwaiting {
			return local.Change{}, local.Reject("session_state_conflict", "this handoff is not awaiting a host report")
		}
		deadline, err := time.Parse(time.RFC3339Nano, current.Deadline.UTC)
		now, parseErr := time.Parse(time.RFC3339Nano, obs.UTC)
		if err != nil || parseErr != nil {
			return local.Change{}, local.ErrIntegrity
		}
		if now.Before(deadline) {
			return local.Change{}, local.Reject("deadline_not_reached", "the host may still report before its deadline")
		}
		current.Session.HostState = SessionDisconnected
		// The effect is unknown, not failed: the host may have changed files in
		// its claimed worktree before it went away.
		r.HasUnresolvedEffects = true
		r.Status = "uncertain"
		// The obligation itself carries the uncertainty, not only the Run: the
		// owner resolves an attempt, and a Run whose attempt still read as
		// dispatched offered nothing to resolve.
		current.Status = "uncertain"
		if step := r.Steps[current.StepID]; step != nil {
			step.Status = "uncertain"
		}
		if activation := r.Activations[current.ActivationID]; activation != nil {
			activation.Status = "uncertain"
			if err := r.setInvocationStatus(activation.InvocationID, "uncertain", nil); err != nil {
				return local.Change{}, err
			}
		}
		if err := diagnostic(r, attemptID, attemptID, "assisted_host_disconnected", "dispatch", "the assisted host did not report before its deadline", obs); err != nil {
			return local.Change{}, err
		}
		return local.Change{}, nil
	})
}

// Package runtime owns admission, lifecycle, and local protocol handling.
// Definitions and worker messages are data; they cannot change core code.
package runtime

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

var Version = "0.2.0-dev.5"

const (
	ReadVersion      = "foundation-read/1"
	StateVersion     = "foundation-state/1"
	CoreReadVersion  = "core-read/1"
	CoreStateVersion = "core-state/1"
	// Invocation state is a separate wire contract. Flat Runs keep the
	// delivered state/read versions, including after a newer core opens them.
	CoreInvocationReadVersion    = "core-read/2"
	CoreInvocationStateVersion   = "core-state/2"
	CoreInvocationNextVersion    = "core-next/2"
	CoreInvocationPreviewVersion = "core-preview/2"
	CoreRepeatReadVersion        = "core-read/3"
	CoreRepeatStateVersion       = "core-state/3"
	CoreRepeatNextVersion        = "core-next/3"
	CoreRepeatPreviewVersion     = "core-preview/3"
	CoreContextReadVersion       = "core-read/4"
	CoreContextStateVersion      = "core-state/4"
	CoreContextNextVersion       = "core-next/4"
	CoreContextPreviewVersion    = "core-preview/4"
	// Assisted execution records who holds an attempt and whether the host ever
	// came back. Those facts belong in the Run, so they need their own version.
	CoreSessionReadVersion    = "core-read/5"
	CoreSessionStateVersion   = "core-state/5"
	CoreSessionNextVersion    = "core-next/5"
	CoreSessionPreviewVersion = "core-preview/5"
	// A waiver is a recorded reduction in quality that must stay visible in the
	// outcome, so it belongs in the Run and needs its own version.
	CoreWaiverReadVersion    = "core-read/6"
	CoreWaiverStateVersion   = "core-state/6"
	CoreWaiverNextVersion    = "core-next/6"
	CoreWaiverPreviewVersion = "core-preview/6"
	// A parallel stage owns branch invocations and one durable join decision
	// per settled branch. Those facts belong in the Run, so they need their
	// own version rather than an unannounced widening of the waiver state.
	CoreParallelReadVersion    = "core-read/7"
	CoreParallelStateVersion   = "core-state/7"
	CoreParallelNextVersion    = "core-next/7"
	CoreParallelPreviewVersion = "core-preview/7"
	// A map seals its collection into item identities and derived artifacts
	// before it admits anything. That seal is a durable fact about the Run, so
	// it needs its own version rather than an unannounced widening of parallel.
	CoreMapReadVersion    = "core-read/8"
	CoreMapStateVersion   = "core-state/8"
	CoreMapNextVersion    = "core-next/8"
	CoreMapPreviewVersion = "core-preview/8"
	// A wait holds durable registrations and an inbox of what arrived. Those
	// are facts about the Run, so they need their own version rather than an
	// unannounced widening of the sealed-collection state.
	CoreWaitReadVersion    = "core-read/9"
	CoreWaitStateVersion   = "core-state/9"
	CoreWaitNextVersion    = "core-next/9"
	CoreWaitPreviewVersion = "core-preview/9"
	// A live guard is a durable rule about whether one scope of this Run may
	// start or must stop, together with the observations it was decided on.
	// Those are facts about the Run, so they need their own version rather than
	// an unannounced widening of the wait state.
	// There is no core-preview/10: a preview describes a workflow, and a guard
	// is declared when the Run is created rather than by the workflow, so no
	// preview this build can produce would ever carry that version.
	CoreGuardReadVersion  = "core-read/10"
	CoreGuardStateVersion = "core-state/10"
	CoreGuardNextVersion  = "core-next/10"
	// A reported cost is an exact amount claimed by a named source for one
	// Attempt. It is durable Run data, not a price computed from usage, so the
	// additive state/read contract gets its own version.
	CoreReportedCostReadVersion  = "core-read/11"
	CoreReportedCostStateVersion = "core-state/11"
	CoreReportedCostNextVersion  = "core-next/11"
	// An early artifact publication adds an immutable result record while its
	// producer may still be running. The workflow preview changes too because
	// StepDefinition v3 exposes the declared artifact hook.
	CoreArtifactPublicationReadVersion     = "core-read/12"
	CoreArtifactPublicationStateVersion    = "core-state/12"
	CoreArtifactPublicationNextVersion     = "core-next/12"
	CoreArtifactPublicationPreviewVersion  = "core-preview/12"
	CoreArtifactPublicationStepReadVersion = "core-step-read/12"
	// Closing a keyed-many artifact hook adds one sealed exact manifest and a
	// durable closure fact. Older publication Runs cannot silently acquire it.
	CoreArtifactClosureReadVersion        = "core-read/13"
	CoreArtifactClosureStateVersion       = "core-state/13"
	CoreArtifactClosureNextVersion        = "core-next/13"
	CoreArtifactClosurePreviewVersion     = "core-preview/13"
	CoreArtifactClosureStepReadVersion    = "core-step-read/13"
	CorePublicationChecksReadVersion      = "core-read/15"
	CorePublicationChecksStateVersion     = "core-state/15"
	CorePublicationChecksNextVersion      = "core-next/15"
	CorePublicationChecksPreviewVersion   = "core-preview/15"
	CorePublicationChecksStepReadVersion  = "core-step-read/15"
	CorePublicationNewOnlyReadVersion     = "core-read/16"
	CorePublicationNewOnlyStateVersion    = "core-state/16"
	CorePublicationNewOnlyNextVersion     = "core-next/16"
	CorePublicationNewOnlyPreviewVersion  = "core-preview/16"
	CorePublicationNewOnlyStepReadVersion = "core-step-read/16"
	CorePublicationFailureReadVersion     = "core-read/17"
	CorePublicationFailureStateVersion    = "core-state/17"
	CorePublicationFailureNextVersion     = "core-next/17"
	CorePublicationFailurePreviewVersion  = "core-preview/17"
	CorePublicationFailureStepReadVersion = "core-step-read/17"
	// An ActionIntent is a durable exact proposal. It is not an admission or
	// delivery, but its ownership and digest must survive a restart before any
	// later approval can refer to it.
	CoreActionIntentReadVersion     = "core-read/18"
	CoreActionIntentStateVersion    = "core-state/18"
	CoreActionIntentNextVersion     = "core-next/18"
	CoreActionIntentPreviewVersion  = "core-preview/18"
	CoreActionIntentStepReadVersion = "core-step-read/18"
	// ActionAdmission records the separately authorized decision for an exact
	// ActionIntent. It still does not create a delivery or external effect.
	CoreActionAdmissionReadVersion     = "core-read/19"
	CoreActionAdmissionStateVersion    = "core-state/19"
	CoreActionAdmissionNextVersion     = "core-next/19"
	CoreActionAdmissionPreviewVersion  = "core-preview/19"
	CoreActionAdmissionStepReadVersion = "core-step-read/19"
	// ActionGrantAdmission adds exact resource-scoped Grant consumption to the
	// separately authorized action decision. Version 19 remains approval-only.
	CoreActionGrantAdmissionReadVersion     = "core-read/20"
	CoreActionGrantAdmissionStateVersion    = "core-state/20"
	CoreActionGrantAdmissionNextVersion     = "core-next/20"
	CoreActionGrantAdmissionPreviewVersion  = "core-preview/20"
	CoreActionGrantAdmissionStepReadVersion = "core-step-read/20"
	// ActionDelivery records the durable pre-dispatch boundary. It does not call
	// an adapter; prepared means an effect has not started.
	CoreActionDeliveryReadVersion     = "core-read/21"
	CoreActionDeliveryStateVersion    = "core-state/21"
	CoreActionDeliveryNextVersion     = "core-next/21"
	CoreActionDeliveryPreviewVersion  = "core-preview/21"
	CoreActionDeliveryStepReadVersion = "core-step-read/21"
	// Fork provenance makes a new Run's causal source readable without
	// widening older Run records or pretending their history changed.
	CoreForkReadVersion     = "core-read/22"
	CoreForkStateVersion    = "core-state/22"
	CoreForkNextVersion     = "core-next/22"
	CoreForkPreviewVersion  = "core-preview/22"
	CoreForkStepReadVersion = "core-step-read/22"
	// Workspace state records the exact selected repository workspace for an
	// assisted handoff. Older sessions only know disposable worktrees, so they
	// must not read a direct checkout as one.
	CoreWorkspaceReadVersion     = "core-read/23"
	CoreWorkspaceStateVersion    = "core-state/23"
	CoreWorkspaceNextVersion     = "core-next/23"
	CoreWorkspacePreviewVersion  = "core-preview/23"
	CoreWorkspaceStepReadVersion = "core-step-read/23"
	// A workspace-tree handoff adds a declared native-file boundary to the
	// assisted session. Older Runs must not acquire that wider state silently.
	CoreWorkspaceTreeReadVersion     = "core-read/24"
	CoreWorkspaceTreeStateVersion    = "core-state/24"
	CoreWorkspaceTreeNextVersion     = "core-next/24"
	CoreWorkspaceTreePreviewVersion  = "core-preview/24"
	CoreWorkspaceTreeStepReadVersion = "core-step-read/24"
	// Decision state seals a package's declared human choices with one Run.
	// Older Runs retain their prior schemas and therefore no invented ledger.
	CoreDecisionReadVersion     = "core-read/25"
	CoreDecisionStateVersion    = "core-state/25"
	CoreDecisionNextVersion     = "core-next/25"
	CoreDecisionPreviewVersion  = "core-preview/25"
	CoreDecisionStepReadVersion = "core-step-read/25"
	// Neutral Start records its declared inputs without requiring task intake.
	CoreNeutralReadVersion     = "core-read/26"
	CoreNeutralStateVersion    = "core-state/26"
	CoreNeutralNextVersion     = "core-next/26"
	CoreNeutralPreviewVersion  = "core-preview/26"
	CoreNeutralStepReadVersion = "core-step-read/26"
	// Timed assisted deliveries distinguish active work from declared waiting.
	CoreTimingReadVersion     = "core-read/27"
	CoreTimingStateVersion    = "core-state/27"
	CoreTimingNextVersion     = "core-next/27"
	CoreTimingPreviewVersion  = "core-preview/27"
	CoreTimingStepReadVersion = "core-step-read/27"
	CoreConfigVersion         = "core-configuration/1"
	CoreContextConfigVersion  = "core-configuration/2"
	MaxDefinitionBytes        = 2 << 20
	MaxArtifactBytes          = 16 << 20
	MaxRunPublications        = 1024
)

// Clock observations are explicit inputs to state transitions. Persisted time
// never relies on time.Time's in-memory monotonic component surviving JSON.
type Observation struct {
	UTC          string `json:"utc"`
	Session      string `json:"session"`
	MonotonicMS  int64  `json:"monotonic_ms"`
	Source       string `json:"source"`
	SuspendBasis string `json:"suspend_basis"`
	UTCTrust     string `json:"utc_trust"`
}

type clock struct {
	start   time.Time
	session string
}

func newClock() clock { return clock{time.Now(), newID("clock")} }
func (c clock) now() Observation {
	now := time.Now()
	return Observation{now.UTC().Format(time.RFC3339Nano), c.session, now.Sub(c.start).Milliseconds(), "go.time.monotonic", "excludes_suspend_on_darwin", "local_wall_unqualified"}
}
func newID(kind string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return kind + ":" + hex.EncodeToString(b[:])
}
func rawDigest(data []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(data)) }
func derivedID(kind string, parts ...string) string {
	return fmt.Sprintf("%s:%x", kind, sha256.Sum256([]byte(strings.Join(parts, "\x00"))))
}

type ArtifactRef struct {
	ArtifactID string `json:"artifact_id"`
	Revision   int64  `json:"revision"`
	Digest     string `json:"digest"`
}
type Artifact struct {
	SchemaVersion        string         `json:"schema_version"`
	ID                   string         `json:"id"`
	Revision             int64          `json:"revision"`
	Digest               string         `json:"digest"`
	Producer             map[string]any `json:"producer"`
	Format               string         `json:"format"`
	SchemaRef            *flow.Ref      `json:"schema_ref,omitempty"`
	MediaType            string         `json:"media_type"`
	SizeBytes            int64          `json:"size_bytes"`
	Classification       string         `json:"classification"`
	ContentCheckEvidence []any          `json:"content_check_evidence"`
	Provenance           []ArtifactRef  `json:"provenance"`
	CreatedAt            string         `json:"created_at"`
}

func (a Artifact) Ref() ArtifactRef { return ArtifactRef{a.ID, a.Revision, a.Digest} }

type Definition struct {
	Ref  flow.Ref `json:"ref"`
	Kind string   `json:"kind"`
	Path string   `json:"path"`
	// Registry3 context resources declare both fields. Their absence keeps
	// the existing canonical JSON definition interpretation.
	ByteEncoding string `json:"byte_encoding,omitempty"`
	MediaType    string `json:"media_type,omitempty"`
}
type RegistryFile struct {
	SchemaVersion string       `json:"schema_version"`
	Entries       []Definition `json:"entries"`
	// Aliases are author-local workflow names, resolved to exact references
	// before the package lock is created. Version 1 registries omit this field.
	Aliases map[string]string `json:"aliases,omitempty"`
}
type PinnedDefinition struct {
	Ref       flow.Ref        `json:"ref"`
	Kind      string          `json:"kind"`
	RawDigest string          `json:"raw_digest"`
	Bytes     json.RawMessage `json:"bytes"`
}

// ExecutorConfig is the configuration schema's local process binding, not an
// extra field inserted into StepDefinition or ExecutionEnvelope v1.
type ExecutorConfig struct {
	Executable        string            `json:"executable"`
	Args              []string          `json:"args"`
	Files             map[string]string `json:"files"`
	Environment       map[string]string `json:"environment"`
	TimeoutMS         int64             `json:"timeout_ms"`
	GraceMS           int64             `json:"grace_ms"`
	MaxOutputBytes    int64             `json:"max_output_bytes"`
	ContextProfileRef *flow.Ref         `json:"context_profile_ref,omitempty"`
}
type PinnedExecutor struct {
	Config           ExecutorConfig           `json:"config"`
	ExecutableDigest string                   `json:"executable_digest"`
	Files            map[string]local.BlobRef `json:"files"`
	ContextProfile   *ContextProfile          `json:"context_profile,omitempty"`
}
type Configuration struct {
	SchemaVersion    string                                `json:"schema_version"`
	SemanticsProfile string                                `json:"semantics_profile"`
	TrustProfile     string                                `json:"trust_profile"`
	StateRoot        string                                `json:"state_root"`
	ArtifactRoot     string                                `json:"artifact_root"`
	WorkspaceRoot    string                                `json:"workspace_root"`
	RegistryFile     string                                `json:"registry_file"`
	Executors        map[string]ExecutorConfig             `json:"executors"`
	InputValues      map[string]map[string]json.RawMessage `json:"input_values,omitempty"`
}
type ProjectConfig struct {
	SchemaVersion          string              `json:"schema_version"`
	ID                     string              `json:"id"`
	InstalledPackages      []flow.Ref          `json:"installed_packages"`
	DefaultPolicyRef       flow.Ref            `json:"default_policy_ref"`
	AdapterBindings        map[string]flow.Ref `json:"adapter_bindings"`
	ConfigurationSchemaRef flow.Ref            `json:"configuration_schema_ref"`
	Configuration          Configuration       `json:"configuration"`
	SecretRefs             map[string]string   `json:"secret_refs"`
}
type Installation struct {
	SchemaVersion      string `json:"schema_version"`
	ID                 string `json:"id"`
	ProjectRoot        string `json:"project_root"`
	OwnerUID           int    `json:"owner_uid"`
	CreatedAt          string `json:"created_at"`
	TelemetryCursorKey string `json:"telemetry_cursor_key"`
}
type Brief struct {
	SchemaVersion      string        `json:"schema_version"`
	ID                 string        `json:"id"`
	Subject            string        `json:"subject"`
	DesiredOutcome     string        `json:"desired_outcome"`
	InScope            []string      `json:"in_scope"`
	OutOfScope         []string      `json:"out_of_scope"`
	CompletionCriteria []string      `json:"completion_criteria"`
	SourceRefs         []ArtifactRef `json:"source_refs"`
	Assumptions        []string      `json:"assumptions"`
	Confirmation       string        `json:"confirmation"`
}

type Stop struct {
	ID         string       `json:"id"`
	Generation int64        `json:"generation"`
	Epoch      int64        `json:"control_epoch"`
	Kind       string       `json:"kind"`
	Reason     string       `json:"reason"`
	Actor      string       `json:"actor_id"`
	Status     string       `json:"status"`
	Created    Observation  `json:"created"`
	Released   *Observation `json:"released,omitempty"`
	// Empty scope fields preserve a legacy Run-wide stop. Invocation-scoped
	// stops are admitted only by the invocation state contract.
	Scope   string `json:"scope,omitempty"`
	ScopeID string `json:"scope_id,omitempty"`
}
type Activation struct {
	ID           string       `json:"id"`
	StageID      string       `json:"stage_id"`
	InvocationID string       `json:"workflow_invocation_id"`
	Kind         string       `json:"kind"`
	Status       string       `json:"status"`
	StepID       string       `json:"step_instance_id,omitempty"`
	Created      Observation  `json:"created"`
	Settled      *Observation `json:"settled,omitempty"`
	// Repeat owns only the bounded loop's current progress. Previous body
	// invocations and decisions remain separately identified in the journal.
	Repeat *RepeatProgress `json:"repeat,omitempty"`
	// Parallel owns only the fan-out's current progress. Every branch remains
	// a separately identified invocation and every decision stays in the journal.
	Parallel *ParallelProgress `json:"parallel,omitempty"`
	// Wait owns the registration this activation holds and its one resolution.
	Wait *WaitProgress `json:"wait,omitempty"`
}
type Step struct {
	ID           string                 `json:"id"`
	ActivationID string                 `json:"stage_activation_id"`
	Ref          flow.Ref               `json:"definition_ref"`
	Status       string                 `json:"status"`
	Verdict      string                 `json:"verdict,omitempty"`
	AttemptIDs   []string               `json:"attempt_ids"`
	Outputs      map[string]ArtifactRef `json:"outputs"`
	Created      Observation            `json:"created"`
	Settled      *Observation           `json:"settled,omitempty"`
}
type Result struct {
	SchemaVersion     string                 `json:"schema_version"`
	RunID             string                 `json:"run_id"`
	StepInstanceID    string                 `json:"step_instance_id"`
	AttemptID         string                 `json:"attempt_id"`
	EnvelopeDigest    string                 `json:"envelope_digest"`
	Verdict           string                 `json:"verdict"`
	Outputs           map[string]ArtifactRef `json:"outputs"`
	EvidenceRefs      []any                  `json:"evidence_refs"`
	EffectReceiptRefs []any                  `json:"effect_receipt_refs"`
	Summary           string                 `json:"summary"`
}
type LocalPort struct {
	Ref  ArtifactRef `json:"ref"`
	Path string      `json:"path"`
}
type ContextManifest struct {
	SchemaVersion string                `json:"schema_version"`
	Inputs        map[string]LocalPort  `json:"inputs"`
	Outputs       map[string]OutputSlot `json:"outputs"`
	Dependencies  []flow.Ref            `json:"dependencies"`
	Manifest      *LocalPort            `json:"manifest,omitempty"`
	Rendering     *LocalPort            `json:"rendering,omitempty"`
	Sources       []LocalPort           `json:"sources,omitempty"`
}
type OutputSlot struct {
	ArtifactID string `json:"artifact_id"`
	Revision   int64  `json:"revision"`
	Path       string `json:"path"`
}

// ObligationResolution is the owner's statement about an obligation whose
// outcome the authority could not establish. Recovery preserves that
// uncertainty and keeps the slot it holds; nothing but this statement removes
// it, and it removes it by being told what happened, never by guessing.
type ObligationResolution struct {
	Outcome  string      `json:"outcome"`
	Reason   string      `json:"reason"`
	Actor    string      `json:"actor"`
	Observed Observation `json:"observed"`
}

type Attempt struct {
	ID                string                 `json:"id"`
	StepID            string                 `json:"step_instance_id"`
	ActivationID      string                 `json:"stage_activation_id"`
	Status            string                 `json:"status"`
	AdmissionID       string                 `json:"execution_admission_id"`
	ReservationID     string                 `json:"budget_reservation_id"`
	AdmittedVersion   int64                  `json:"admitted_run_version"`
	ControlEpoch      int64                  `json:"control_epoch"`
	Envelope          json.RawMessage        `json:"envelope"`
	EnvelopeDigest    string                 `json:"envelope_digest"`
	TokenHash         string                 `json:"token_hash"`
	Workspace         string                 `json:"workspace"`
	Context           ContextManifest        `json:"context"`
	Admitted          Observation            `json:"admitted"`
	Deadline          Observation            `json:"deadline"`
	DispatchDeadline  Observation            `json:"dispatch_deadline"`
	Dispatch          *Observation           `json:"dispatch,omitempty"`
	Started           *Observation           `json:"started,omitempty"`
	CandidateAt       *Observation           `json:"candidate_at,omitempty"`
	ExecutorEnd       *Observation           `json:"executor_end,omitempty"`
	Settled           *Observation           `json:"settled,omitempty"`
	Process           *local.ProcessIdentity `json:"process,omitempty"`
	ProcessOutcome    *local.ProcessOutcome  `json:"process_outcome,omitempty"`
	Candidate         json.RawMessage        `json:"candidate,omitempty"`
	CandidateConflict bool                   `json:"candidate_conflict"`
	Accepted          *Result                `json:"accepted,omitempty"`
	// ReportedCosts is absent when nobody named a cost. An explicit zero is a
	// ReportedCost with amount "0"; those two facts must not collapse.
	ReportedCosts []ReportedCost `json:"reported_costs,omitempty"`
	// Session is present only for an assisted attempt. It names the host that
	// holds the work, so a Run never claims a dispatch nobody is answering for.
	Session *SessionHandoff `json:"session,omitempty"`
}
type Diagnostic struct {
	ID            string      `json:"id"`
	RunID         string      `json:"run_id"`
	AttemptID     string      `json:"attempt_id,omitempty"`
	ActivationID  string      `json:"stage_activation_id,omitempty"`
	Origin        string      `json:"origin"`
	Severity      string      `json:"severity"`
	Code          string      `json:"code"`
	Category      string      `json:"category"`
	Phase         string      `json:"phase"`
	Message       string      `json:"message"`
	Observed      Observation `json:"observed"`
	PublicationID string      `json:"publication_id,omitempty"`
	CauseRefs     []string    `json:"cause_refs"`
}
type Publication struct {
	ID        string          `json:"id"`
	AttemptID string          `json:"attempt_id"`
	StepID    string          `json:"step_instance_id"`
	Hook      string          `json:"hook"`
	Kind      string          `json:"kind"`
	Version   int64           `json:"state_version"`
	EventKey  string          `json:"event_key,omitempty"`
	Value     json.RawMessage `json:"value"`
	Digest    string          `json:"digest"`
	Received  Observation     `json:"received"`
	Actor     string          `json:"actor_id"`
}

const ArtifactPublicationVersion = "artifact-publication/1"

const (
	ArtifactManifestVersion = "artifact-manifest/1"
	ArtifactClosureVersion  = "artifact-closure/1"
)

// ArtifactPublication is the accepted logical item, distinct from both the
// mutable candidate path and the producer's eventual final StepResult.
type ArtifactPublication struct {
	SchemaVersion        string        `json:"schema_version"`
	ID                   string        `json:"id"`
	AttemptID            string        `json:"attempt_id"`
	StepID               string        `json:"step_instance_id"`
	Hook                 string        `json:"hook"`
	ItemKey              string        `json:"item_key"`
	Artifact             ArtifactRef   `json:"artifact_ref"`
	Format               string        `json:"format"`
	SchemaRef            flow.Ref      `json:"schema_ref"`
	MediaType            string        `json:"media_type"`
	SizeBytes            int64         `json:"size_bytes"`
	ContentCheckEvidence []EvidenceRef `json:"content_check_evidence"`
	Classification       string        `json:"classification"`
	Consumption          string        `json:"consumption"`
	AcceptedSequence     int64         `json:"accepted_sequence"`
	Accepted             Observation   `json:"accepted"`
	Actor                string        `json:"actor_id"`
}

// PendingArtifactPublication holds authority-sealed bytes while the declared
// readiness checks decide whether they may become a visible publication.
// It is deliberately distinct from ArtifactPublication: subscribers never
// receive a candidate whose checks have not passed.
type PendingArtifactPublication struct {
	ID           string      `json:"id"`
	CommandID    string      `json:"command_id"`
	AttemptID    string      `json:"attempt_id"`
	StepID       string      `json:"step_instance_id"`
	ActivationID string      `json:"stage_activation_id"`
	Hook         string      `json:"hook"`
	ItemKey      string      `json:"item_key"`
	Artifact     ArtifactRef `json:"artifact_ref"`
	Format       string      `json:"format"`
	SchemaRef    flow.Ref    `json:"schema_ref"`
	MediaType    string      `json:"media_type"`
	SizeBytes    int64       `json:"size_bytes"`
	CheckIDs     []string    `json:"check_execution_ids"`
	Created      Observation `json:"created"`
}

// ArtifactManifest is the authority-sealed exact cut of one keyed-many hook.
// Items retain their full accepted PublicationRecords, not just content hashes.
type ArtifactManifest struct {
	SchemaVersion string                `json:"schema_version"`
	RunID         string                `json:"run_id"`
	StepID        string                `json:"step_instance_id"`
	Hook          string                `json:"hook"`
	ItemCount     int64                 `json:"item_count"`
	CutSequence   int64                 `json:"cut_sequence"`
	Items         []ArtifactPublication `json:"items"`
}

// ArtifactClosure is the durable acceptance of one exact manifest. The
// manifest bytes live in artifact storage; ItemKeys and the cut keep the state
// invariant checkable without filesystem I/O.
type ArtifactClosure struct {
	SchemaVersion    string      `json:"schema_version"`
	ID               string      `json:"id"`
	AttemptID        string      `json:"attempt_id"`
	StepID           string      `json:"step_instance_id"`
	Hook             string      `json:"hook"`
	ItemKeys         []string    `json:"item_keys"`
	Manifest         ArtifactRef `json:"manifest_ref"`
	ItemCount        int64       `json:"item_count"`
	CutSequence      int64       `json:"cut_sequence"`
	AcceptedSequence int64       `json:"accepted_sequence"`
	Accepted         Observation `json:"accepted"`
	Actor            string      `json:"actor_id"`
}
type PublishCommand struct {
	SchemaVersion        string          `json:"schema_version"`
	CommandID            string          `json:"command_id"`
	RunID                string          `json:"run_id"`
	StepID               string          `json:"step_instance_id"`
	AttemptID            string          `json:"attempt_id"`
	EnvelopeDigest       string          `json:"envelope_digest"`
	Hook                 string          `json:"hook"`
	Kind                 string          `json:"kind"`
	ExpectedStateVersion *int64          `json:"expected_state_version,omitempty"`
	EventKey             string          `json:"event_key,omitempty"`
	Value                json.RawMessage `json:"value,omitempty"`
	ItemKey              string          `json:"item_key,omitempty"`
	CandidatePath        string          `json:"candidate_path,omitempty"`
	ExpectedDigest       string          `json:"expected_digest,omitempty"`
	ExpectedSizeBytes    *int64          `json:"expected_size_bytes,omitempty"`
	ItemKeys             []string        `json:"item_keys,omitempty"`
}
type Run struct {
	SchemaVersion          string                    `json:"schema_version"`
	ID                     string                    `json:"id"`
	AuthorityID            string                    `json:"authority_id"`
	ProjectID              string                    `json:"project_id"`
	Profile                string                    `json:"semantics_profile"`
	TrustProfile           string                    `json:"trust_profile"`
	InteractionMode        string                    `json:"interaction_mode"`
	ExecutionMode          string                    `json:"execution_mode"`
	CapacityProfile        string                    `json:"capacity_profile"`
	Status                 string                    `json:"status"`
	Outcome                *string                   `json:"outcome"`
	RootInvocationID       string                    `json:"root_workflow_invocation_id"`
	WorkflowRef            flow.Ref                  `json:"workflow_ref"`
	Workflow               json.RawMessage           `json:"workflow"`
	Definitions            []PinnedDefinition        `json:"definitions"`
	ContextResources       []PinnedResource          `json:"context_resources,omitempty"`
	Executors              map[string]PinnedExecutor `json:"executors"`
	EffectiveConfiguration *EffectiveConfiguration   `json:"effective_configuration,omitempty"`
	// WorkflowConfigurations pins configuration for the transitive workflow
	// closure by exact workflow digest; call entry never reads live settings.
	WorkflowConfigurations map[string]*EffectiveConfiguration `json:"workflow_configurations,omitempty"`
	// Fork records the immutable source of a semantic rework. It names a
	// separate Run rather than changing the source Run's history in place.
	Fork            *ForkProvenance        `json:"fork,omitempty"`
	Brief           ArtifactRef            `json:"brief_ref"`
	LockRef         flow.Ref               `json:"package_lock_ref"`
	Inputs          map[string]ArtifactRef `json:"input_artifacts"`
	Outputs         map[string]ArtifactRef `json:"output_artifacts"`
	DecisionCatalog *DecisionCatalog       `json:"decision_catalog,omitempty"`
	DecisionSheet   *DecisionSheet         `json:"decision_sheet,omitempty"`
	DecisionLedger  []DecisionRecord       `json:"decision_ledger,omitempty"`
	PendingDecision *DecisionRequest       `json:"pending_decision,omitempty"`
	Ready           []string               `json:"ready_stages"`
	// Invocations owns the scoped frontiers in core-state/2 and core-state/3.
	// Ready is legacy state only and is omitted from their JSON by MarshalJSON.
	Invocations       map[string]*Invocation     `json:"invocations,omitempty"`
	Active            []string                   `json:"active_attempt_ids"`
	Activations       map[string]*Activation     `json:"activations"`
	Steps             map[string]*Step           `json:"steps"`
	Attempts          map[string]*Attempt        `json:"attempts"`
	CheckExecutions   map[string]*CheckExecution `json:"check_executions,omitempty"`
	ActiveCheckID     string                     `json:"active_check_execution_id,omitempty"`
	PendingAcceptance *PendingAcceptance         `json:"pending_acceptance,omitempty"`
	// A run written before recorded waivers carries neither field, which
	// honestly means nothing was waived rather than an unknown quality.
	Waivers       []Waiver `json:"waivers,omitempty"`
	WaiverApplied bool     `json:"waiver_applied,omitempty"`
	// Waits are the durable promises this Run has made about signals it will
	// accept. Inbox is what actually arrived, including what was refused: a
	// delivery nobody used is evidence, not litter.
	Waits map[string]*WaitRegistration `json:"wait_registrations,omitempty"`
	Inbox []InboxEvent                 `json:"inbox,omitempty"`
	// Guards are the live rules this Run was registered with: whether a scope
	// may start, and whether it must stop. They read only facts this Run
	// already holds, and each carries the observations it was decided on.
	Guards                     map[string]*GuardRegistration       `json:"guards,omitempty"`
	Stops                      []Stop                              `json:"stops"`
	ControlEpoch               int64                               `json:"control_epoch"`
	ControlTransitions         int64                               `json:"control_transitions"`
	HasUnresolvedEffects       bool                                `json:"has_unresolved_effects"`
	CancelRequested            bool                                `json:"cancel_requested"`
	ResumeRequired             bool                                `json:"resume_required"`
	Publications               []Publication                       `json:"publications"`
	ArtifactPublications       []ArtifactPublication               `json:"artifact_publications,omitempty"`
	PendingArtifactPublication *PendingArtifactPublication         `json:"pending_artifact_publication,omitempty"`
	ArtifactClosures           []ArtifactClosure                   `json:"artifact_closures,omitempty"`
	PublicationSubscriptions   map[string]*PublicationSubscription `json:"publication_subscriptions,omitempty"`
	PublicationAssignments     []PublicationAssignment             `json:"publication_assignments,omitempty"`
	ActionIntents              map[string]ActionIntentRecord       `json:"action_intents,omitempty"`
	ActionAdmissions           map[string]ActionAdmission          `json:"action_admissions,omitempty"`
	ActionDeliveries           map[string]ActionDelivery           `json:"action_deliveries,omitempty"`
	Diagnostics                []Diagnostic                        `json:"diagnostics"`
	Created                    Observation                         `json:"created"`
	LastObserved               Observation                         `json:"last_observed"`
	Settled                    *Observation                        `json:"settled,omitempty"`
	CoreBuild                  string                              `json:"core_build"`
	Gaps                       []TimingGap                         `json:"gaps"`
	Transitions                []StateChange                       `json:"transitions"`
}

// ForkProvenance names only the exact source cut and the artifacts explicitly
// carried into a new Run. It never transports approvals, worker state or an
// assertion that an earlier external effect is safe to repeat.
type ForkProvenance struct {
	SchemaVersion    string        `json:"schema_version"`
	SourceRunID      string        `json:"source_run_id"`
	SourceRunVersion int64         `json:"source_run_version"`
	CommandID        string        `json:"command_id"`
	Reason           string        `json:"reason"`
	ReuseRefs        []ArtifactRef `json:"reuse_refs"`
}
type TimingGap struct {
	From   Observation `json:"from"`
	To     Observation `json:"to"`
	Reason string      `json:"reason"`
}
type StateChange struct {
	Kind string      `json:"kind"`
	ID   string      `json:"id"`
	From string      `json:"from"`
	To   string      `json:"to"`
	At   Observation `json:"at"`
}

func (r Run) terminal() bool {
	return r.Status == "completed" || r.Status == "failed" || r.Status == "cancelled"
}
func (r Run) restricted() bool {
	for _, s := range r.Stops {
		if s.Status == "active" {
			return true
		}
	}
	return false
}
func (r Run) admissionsBlocked() bool { return r.restricted() || r.ResumeRequired }
func (r Run) registry() flow.Registry {
	reg := make(flow.Registry, len(r.Definitions))
	for _, d := range r.Definitions {
		reg[d.Ref] = d.Bytes
	}
	return reg
}

// planCache keeps compiled plans. Compiling parses the workflow, resolves every
// pinned definition and validates their schemas — work that a command must not
// repeat, least of all inside a transaction. Every input is immutable pinned
// state, so the same key names the same plan, and a compiled plan is read-only
// and shared between goroutines.
var planCache = struct {
	sync.Mutex
	entries map[string]*flow.Plan
}{entries: map[string]*flow.Plan{}}

// maxCachedPlans bounds the cache. Eviction is a deliberate reset: a plan is
// cheap to recompile and one process runs few distinct workflows.
const maxCachedPlans = 64

// planCompilations counts actual compilations. A test reads it to prove that
// driving a Run compiles its workflow once, not once per command.
var planCompilations atomic.Int64

func compiledPlan(key string, compile func() (*flow.Plan, error)) (*flow.Plan, error) {
	planCache.Lock()
	cached, found := planCache.entries[key]
	planCache.Unlock()
	if found {
		return cached, nil
	}
	planCompilations.Add(1)

	p, err := compile()
	if err != nil {
		return nil, err
	}
	planCache.Lock()
	if len(planCache.entries) >= maxCachedPlans {
		clear(planCache.entries)
	}
	planCache.entries[key] = p
	planCache.Unlock()
	return p, nil
}

// planKey names everything a compile reads: the workflow bytes, the semantics
// it is compiled under and the exact pinned definitions and context resources.
func (r Run) planKey() string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%s|", r.SchemaVersion, r.Profile, rawDigest(r.Workflow))
	// The same pinned set is the same plan whatever order it was recorded in.
	pins := make([]string, 0, len(r.Definitions)+len(r.ContextResources))
	for _, d := range r.Definitions {
		pins = append(pins, "d:"+d.Ref.ID+"@"+d.Ref.Version+":"+d.Ref.Digest)
	}
	for _, resource := range r.ContextResources {
		pins = append(pins, "r:"+resource.Ref.ID+"@"+resource.Ref.Version+":"+resource.Ref.Digest+":"+resource.RawDigest+":"+resource.ByteEncoding+":"+resource.MediaType)
	}
	slices.Sort(pins)
	for _, pin := range pins {
		fmt.Fprintf(h, "%s|", pin)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (r Run) plan() (*flow.Plan, error) {
	if !supportedRun(r) {
		return nil, faultf("incompatible_run", "unsupported state/semantics profile")
	}
	if isContextState(r.SchemaVersion) {
		if err := contextPinnedInvariant(r); err != nil {
			return nil, err
		}
	}
	p, err := compiledPlan(r.planKey(), func() (*flow.Plan, error) {
		if isContextState(r.SchemaVersion) {
			resources, resourceErr := resourcesFromPins(r.ContextResources)
			if resourceErr != nil {
				return nil, resourceErr
			}
			return flow.CompileCore(r.Workflow, "json", r.registry(), resources)
		}
		return flow.CompileProfile(r.Workflow, "json", r.registry(), r.Profile)
	})
	if err != nil {
		return nil, err
	}
	if requiresRepeatState(p) && !isRepeatState(r.SchemaVersion) {
		return nil, faultf("incompatible_run", "repeat closure requires core-state/3")
	}
	if requiresPublicationNewOnlyState(p) && !isPublicationNewOnlyState(r.SchemaVersion) {
		return nil, faultf("incompatible_run", "new-only publication source requires core-state/16")
	}
	if requiresPublicationFailureState(p) && !isPublicationFailureState(r.SchemaVersion) {
		return nil, faultf("incompatible_run", "terminal-failure publication source requires core-state/17")
	}
	return p, nil
}

type RunView struct {
	SchemaVersion string      `json:"schema_version"`
	RunVersion    int64       `json:"run_version"`
	EventSequence int64       `json:"event_sequence"`
	Cut           int64       `json:"cut"`
	AsOf          Observation `json:"as_of"`
	DriverLive    bool        `json:"driver_live"`
	Run           Run         `json:"run"`
	Timing        TimingTree  `json:"timing"`
}

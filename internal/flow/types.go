// Package flow contains the versioned deterministic workflow model and
// compiler. Author schema computation runs in a bounded child process; routing
// is pure and execution, storage and authority checks belong to runtime.
package flow

import (
	"encoding/json"
	"fmt"
	"strings"
)

const Profile = "foundation-sequence/1"
const CoreProfile = "core-workflow/1"

// Ref identifies exact immutable bytes; a matching name/version is insufficient.
type Ref struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

func (r Ref) String() string { return r.ID + "@" + r.Version + "#" + r.Digest }

// Registry is an explicitly supplied local dependency inventory. Compile never
// fetches a missing reference or resolves a moving version.
type Registry map[Ref][]byte

type Port struct {
	Format           string   `json:"format"`
	SchemaRef        *Ref     `json:"schema_ref,omitempty"`
	MediaTypes       []string `json:"media_types,omitempty"`
	ContentCheckRefs []Ref    `json:"content_check_refs,omitempty"`
	Description      string   `json:"description,omitempty"`
}

type InputPort struct {
	Port
	Required      bool                `json:"required"`
	Configuration *InputConfiguration `json:"configuration,omitempty"`
}

// InputConfiguration declares a workflow value, never executable behavior or
// permissions. A nil Default is absent; the JSON bytes "null" are a value.
type InputConfiguration struct {
	Scope   string          `json:"scope"`
	Default json.RawMessage `json:"default,omitempty"`
}

type OutputPort struct {
	Port
	RequiredFor []string `json:"required_for"`
}

type Binding struct {
	From               string          `json:"from"`
	Port               string          `json:"port,omitempty"`
	StageID            string          `json:"stage_id,omitempty"`
	SourceRef          *Ref            `json:"source_ref,omitempty"`
	Value              json.RawMessage `json:"value,omitempty"`
	SchemaRef          *Ref            `json:"schema_ref,omitempty"`
	Pointer            *string         `json:"pointer,omitempty"`
	ProjectedSchemaRef *Ref            `json:"projected_schema_ref,omitempty"`
}

// FieldRef addresses a value in pinned input/result data. It never names an
// ambient file, clock, environment variable or live state source.
type FieldRef struct {
	From    string `json:"from"`
	StageID string `json:"stage_id,omitempty"`
	Port    string `json:"port"`
	Pointer string `json:"pointer,omitempty"`
}

type Operand struct {
	Kind  string          `json:"kind"`
	Value json.RawMessage `json:"value,omitempty"`
	Ref   *FieldRef       `json:"ref,omitempty"`
}

type Predicate struct {
	Op    string      `json:"op"`
	Left  *Operand    `json:"left,omitempty"`
	Right *Operand    `json:"right,omitempty"`
	Ref   *FieldRef   `json:"ref,omitempty"`
	Args  []Predicate `json:"args,omitempty"`
}

type ChoiceBranch struct {
	ID        string    `json:"id"`
	Predicate Predicate `json:"predicate"`
	Next      string    `json:"next"`
}

type Stage struct {
	Kind            string             `json:"kind"`
	StepRef         Ref                `json:"step_ref,omitempty"`
	WorkflowRef     Ref                `json:"workflow_ref,omitempty"`
	BodyWorkflowRef Ref                `json:"body_workflow_ref,omitempty"`
	InputBindings   map[string]Binding `json:"input_bindings,omitempty"`
	InitialBindings map[string]Binding `json:"initial_bindings,omitempty"`
	NextBindings    map[string]Binding `json:"next_bindings,omitempty"`
	ContinueOn      []string           `json:"continue_on,omitempty"`
	Until           Predicate          `json:"until,omitempty"`
	MaxIterations   int64              `json:"max_iterations,omitempty"`
	// LimitConfiguration optionally names a project-scoped input on this
	// WorkflowRevision. Its positive integer can narrow MaxIterations for one
	// Run, but never raise the declared bound the compiler has costed.
	LimitConfiguration string            `json:"limit_configuration,omitempty"`
	OnComplete         map[string]string `json:"on_complete,omitempty"`
	OnLimit            string            `json:"on_limit,omitempty"`
	On                 map[string]string `json:"on,omitempty"`
	OnError            string            `json:"on_error,omitempty"`
	Selection          string            `json:"selection,omitempty"`
	Branches           []ChoiceBranch    `json:"branches,omitempty"`
	// The published contracts reuse the name "branches" for two different
	// shapes, discriminated by kind, so a parallel stage decodes its own.
	ParallelBranches []ParallelBranch `json:"-"`
	Join             *Join            `json:"join,omitempty"`
	MaxParallelism   int              `json:"max_parallelism,omitempty"`
	// A map stage names the collection it fans out over, the body port each
	// item is bound to, and where inside an item its stable identity is found.
	Items          *Binding `json:"items,omitempty"`
	ItemInput      string   `json:"item_input,omitempty"`
	ItemKeyPointer string   `json:"item_key_pointer,omitempty"`
	MaxItems       int      `json:"max_items,omitempty"`
	// A wait stage names the source that may resolve it, the event it accepts,
	// the business key that correlates one, and how long it is willing to wait.
	// A nil timeout is an explicitly indefinite wait, which is why the field is
	// a pointer: absent and "no deadline" are different statements.
	SourceRef        Ref                `json:"source_ref,omitempty"`
	EventType        string             `json:"event_type,omitempty"`
	EventSchemaRef   Ref                `json:"event_schema_ref,omitempty"`
	CorrelationInput *Binding           `json:"correlation_input,omitempty"`
	CursorInput      *Binding           `json:"cursor_input,omitempty"`
	TimeoutSeconds   *int64             `json:"timeout_seconds,omitempty"`
	OnEvent          string             `json:"on_event,omitempty"`
	OnTimeout        string             `json:"on_timeout,omitempty"`
	Default          string             `json:"default,omitempty"`
	OnUnknown        string             `json:"on_unknown,omitempty"`
	Outcome          string             `json:"outcome,omitempty"`
	OutputBindings   map[string]Binding `json:"output_bindings,omitempty"`
	Description      string             `json:"description,omitempty"`
}

// ParallelBranch is one member of a parallel stage. It names its own child
// workflow, so a branch is an ordinary invocation rather than a second kind of
// execution.
type ParallelBranch struct {
	ID            string             `json:"id"`
	WorkflowRef   Ref                `json:"workflow_ref"`
	InputBindings map[string]Binding `json:"input_bindings"`
}

// Join is the completion contract of a parallel stage. A satisfied join is not
// the same statement as a successful outcome, so both are recorded.
type Join struct {
	Mode              string   `json:"mode"`
	AcceptOutcomes    []string `json:"accept_outcomes"`
	RequiredSuccesses int      `json:"required_successes,omitempty"`
	Selection         string   `json:"selection"`
	Remainder         string   `json:"remainder"`
}

// UnmarshalJSON mirrors MarshalJSON's kind switch. Only "branches" needs it:
// its item shape depends on the stage kind, so decoding it blindly would either
// fail on a valid parallel stage or silently drop its members.
func (s *Stage) UnmarshalJSON(data []byte) error {
	type wireStage Stage
	var wire struct {
		wireStage
		Branches json.RawMessage `json:"branches,omitempty"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	*s = Stage(wire.wireStage)
	s.Branches, s.ParallelBranches = nil, nil
	if len(wire.Branches) == 0 {
		return nil
	}
	if wire.wireStage.Kind == "parallel" {
		return json.Unmarshal(wire.Branches, &s.ParallelBranches)
	}
	return json.Unmarshal(wire.Branches, &s.Branches)
}

// MarshalJSON preserves required collections while keeping stage variants
// closed. The original pinned bytes remain Plan.Canonical.
func (s Stage) MarshalJSON() ([]byte, error) {
	value := map[string]any{"kind": s.Kind}
	if s.Description != "" {
		value["description"] = s.Description
	}
	switch s.Kind {
	case "step":
		value["step_ref"], value["input_bindings"], value["on"] = s.StepRef, s.InputBindings, s.On
		if s.OnError != "" {
			value["on_error"] = s.OnError
		}
	case "call":
		value["workflow_ref"], value["input_bindings"], value["on"] = s.WorkflowRef, s.InputBindings, s.On
		if s.OnError != "" {
			value["on_error"] = s.OnError
		}
	case "repeat":
		value["body_workflow_ref"], value["initial_bindings"], value["next_bindings"] = s.BodyWorkflowRef, s.InitialBindings, s.NextBindings
		value["continue_on"], value["until"], value["max_iterations"] = s.ContinueOn, s.Until, s.MaxIterations
		if s.LimitConfiguration != "" {
			value["limit_configuration"] = s.LimitConfiguration
		}
		value["on_complete"], value["on_limit"] = s.OnComplete, s.OnLimit
		if s.OnUnknown != "" {
			value["on_unknown"] = s.OnUnknown
		}
		if s.OnError != "" {
			value["on_error"] = s.OnError
		}
	case "finish":
		value["outcome"], value["output_bindings"] = s.Outcome, s.OutputBindings
	case "parallel":
		value["branches"], value["join"], value["max_parallelism"], value["on"] = s.ParallelBranches, s.Join, s.MaxParallelism, s.On
		if s.OnError != "" {
			value["on_error"] = s.OnError
		}
	case "wait":
		value["source_ref"], value["event_type"], value["event_schema_ref"] = s.SourceRef, s.EventType, s.EventSchemaRef
		value["correlation_input"], value["on_event"] = s.CorrelationInput, s.OnEvent
		if s.CursorInput != nil {
			value["cursor_input"] = s.CursorInput
		}
		// Always emitted, including as null: an absent timeout and an
		// explicitly indefinite one are different declarations.
		value["timeout_seconds"] = s.TimeoutSeconds
		if s.OnTimeout != "" {
			value["on_timeout"] = s.OnTimeout
		}
		if s.OnError != "" {
			value["on_error"] = s.OnError
		}
	case "map":
		value["items"], value["body_workflow_ref"], value["item_input"] = s.Items, s.BodyWorkflowRef, s.ItemInput
		value["item_key_pointer"], value["input_bindings"] = s.ItemKeyPointer, s.InputBindings
		value["max_items"], value["max_parallelism"], value["join"], value["on"] = s.MaxItems, s.MaxParallelism, s.Join, s.On
		if s.OnError != "" {
			value["on_error"] = s.OnError
		}
	case "choice":
		value["selection"], value["branches"] = s.Selection, s.Branches
		if s.Default != "" {
			value["default"] = s.Default
		}
		if s.OnUnknown != "" {
			value["on_unknown"] = s.OnUnknown
		}
		if s.OnError != "" {
			value["on_error"] = s.OnError
		}
	default:
		return nil, problem("unsupported", "/kind", "cannot encode an unsupported stage")
	}
	return json.Marshal(value)
}

type Limits struct {
	MaxStepInstances      int64 `json:"max_step_instances"`
	MaxControlTransitions int64 `json:"max_control_transitions"`
	MaxParallelism        int   `json:"max_parallelism"`
	MaxChildDepth         int   `json:"max_child_depth"`
}

type WorkflowRevision struct {
	SchemaVersion   string                `json:"schema_version"`
	ID              string                `json:"id"`
	Version         string                `json:"version"`
	Title           string                `json:"title"`
	Inputs          map[string]InputPort  `json:"inputs"`
	Outputs         map[string]OutputPort `json:"outputs"`
	AllowedOutcomes []string              `json:"allowed_outcomes"`
	Definition      struct {
		Entry  string           `json:"entry"`
		Stages map[string]Stage `json:"stages"`
	} `json:"definition"`
	Limits    Limits `json:"limits"`
	PolicyRef Ref    `json:"policy_ref"`
}

type StepDefinition struct {
	SchemaVersion string                `json:"schema_version"`
	ID            string                `json:"id"`
	Version       string                `json:"version"`
	Title         string                `json:"title"`
	Kind          string                `json:"kind"`
	Inputs        map[string]InputPort  `json:"inputs"`
	Outputs       map[string]OutputPort `json:"outputs"`
	Executor      struct {
		AdapterRef Ref    `json:"adapter_ref"`
		Operation  string `json:"operation"`
	} `json:"executor"`
	InstructionsRef      *Ref     `json:"instructions_ref,omitempty"`
	ContextRefs          []Ref    `json:"context_refs"`
	RequiredCapabilities []string `json:"required_capabilities"`
	Effects              struct {
		Class      string `json:"class"`
		RetryClass string `json:"retry_class"`
	} `json:"effects"`
	ResultCheckRefs []Ref                  `json:"result_check_refs"`
	ResultSchemaRef Ref                    `json:"result_schema_ref"`
	Hooks           map[string]Hook        `json:"hooks,omitempty"`
	Telemetry       []Mapping              `json:"telemetry,omitempty"`
	WorkspaceTrees  []WorkspaceTreeBinding `json:"workspace_trees,omitempty"`
	SessionLimits   *SessionLimits         `json:"session_limits,omitempty"`
}

// SessionLimits separates an assisted delivery's finite work allowance from
// one declared decision wait. A nil wait limit means no calendar deadline.
type SessionLimits struct {
	ActiveTimeoutMS       int64  `json:"active_timeout_ms"`
	DecisionWaitTimeoutMS *int64 `json:"decision_wait_timeout_ms"`
}

const (
	DefaultSessionActiveTimeoutMS int64 = 3600000
	// Milliseconds must remain representable as a time.Duration in the runtime.
	MaxSessionTimeoutMS int64 = 9223372036854
)

// WorkspaceTreeBinding declares the bounded part of a claimed Workspace that
// carries one sealed WorkspaceTreeManifest between assisted steps. The runtime,
// not a host, materializes and captures the files described by that manifest.
type WorkspaceTreeBinding struct {
	InputPort  string                     `json:"input_port,omitempty"`
	OutputPort string                     `json:"output_port"`
	Capture    WorkspaceTreeCapturePolicy `json:"capture"`
}

// WorkspaceTreeCapturePolicy deliberately has only the shapes needed for one
// file and one shallow bundle. It is a confinement declaration, never a glob
// or a request to synchronize a directory.
type WorkspaceTreeCapturePolicy struct {
	Kind       string `json:"kind"`
	Path       string `json:"path"`
	Entrypoint string `json:"entrypoint,omitempty"`
}

// ToolDescriptor names one externally observable operation. Its presence in a
// registry is descriptive only: admission and delivery remain runtime work.
type ToolDescriptor struct {
	SchemaVersion        string   `json:"schema_version"`
	ID                   string   `json:"id"`
	Version              string   `json:"version"`
	AdapterRef           Ref      `json:"adapter_ref"`
	Operation            string   `json:"operation"`
	ArgumentsSchemaRef   Ref      `json:"arguments_schema_ref"`
	ResultSchemaRef      Ref      `json:"result_schema_ref"`
	EffectClass          string   `json:"effect_class"`
	RetryClass           string   `json:"retry_class"`
	RequiredCapabilities []string `json:"required_capabilities"`
}

// Hook belongs to the declaring StepDefinition, never to the authority's
// lifecycle namespace. Presence of a hook does not require its publication.
type Hook struct {
	Kind            string `json:"kind"`
	SchemaRef       Ref    `json:"schema_ref"`
	Description     string `json:"description"`
	Classification  string `json:"classification"`
	ReadPolicy      string `json:"read_policy"`
	MaxPayloadBytes int64  `json:"max_payload_bytes"`
	MaxCount        int64  `json:"max_count"`
	MaxPerMinute    int64  `json:"max_per_minute"`
	AllowDuringStop bool   `json:"allow_during_stop"`
	FreshnessMS     *int64 `json:"freshness_ms,omitempty"`
	// Artifact is present only for an artifact hook. Keeping its variant fields
	// nested leaves the delivered state/event hook encoding unchanged.
	Artifact *ArtifactHook `json:"artifact,omitempty"`
}

// ArtifactHook declares the bytes accepted by one early-result channel. The
// common Hook byte/count/rate limits apply to the artifacts themselves.
type ArtifactHook struct {
	Format           string   `json:"format"`
	MediaTypes       []string `json:"media_types,omitempty"`
	Cardinality      string   `json:"cardinality"`
	ContentCheckRefs []Ref    `json:"content_check_refs"`
	EarlyConsumption bool     `json:"early_consumption"`
}

// Mapping is declarative: it selects a typed JSON field, not executable code.
// Descriptor identity is local to this exact StepDefinition revision.
type Mapping struct {
	Name        string            `json:"name"`
	Revision    string            `json:"revision"`
	Description string            `json:"description"`
	Hook        string            `json:"hook"`
	Kind        string            `json:"kind"`
	Field       string            `json:"field,omitempty"`
	Unit        string            `json:"unit,omitempty"`
	Aggregation string            `json:"aggregation"`
	Reset       string            `json:"reset"`
	Minimum     *float64          `json:"minimum,omitempty"`
	Maximum     *float64          `json:"maximum,omitempty"`
	Dimensions  map[string]string `json:"dimensions"`
	Severity    string            `json:"severity,omitempty"`
	Code        string            `json:"code,omitempty"`
	Message     string            `json:"message,omitempty"`
}

// Problem provides a stable machine code and a JSON Pointer into the rejected
// definition. Error strings are explanations; callers should switch on Code.
type Problem struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (e *Problem) Error() string {
	return fmt.Sprintf("%s at %s: %s", e.Code, e.Path, e.Message)
}

func problem(code, path, message string) error {
	// A malicious property name must not turn a diagnostic into another copy
	// of the input document. Keep a valid ancestor pointer, never a cut token.
	if len(path) > 1024 {
		path = path[:strings.LastIndex(path[:1024], "/")+1]
		path = strings.TrimSuffix(path, "/")
	}
	return &Problem{Code: code, Path: path, Message: message}
}

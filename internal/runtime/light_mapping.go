package runtime

import (
	"bytes"
	"encoding/json"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
)

const LightProjectionVersion = "prifly-light-projection/1"

// LightProjection is local observer configuration, pinned for the listener's
// lifetime. Exact refs keep it from reinterpreting a later workflow revision.
type LightProjection struct {
	SchemaVersion string                 `json:"schema_version"`
	Workflows     []LightWorkflowMapping `json:"workflows"`
	Digest        string                 `json:"-"`
}
type LightWorkflowMapping struct {
	WorkflowRef flow.Ref             `json:"workflow_ref"`
	Phases      []LightPhaseMapping  `json:"phases"`
	Reviews     []LightReviewMapping `json:"reviews"`
}
type LightPhaseMapping struct {
	Stage string `json:"stage"`
	Key   string `json:"key"`
	Label string `json:"label"`
}
type LightCondition struct {
	Input   string            `json:"input,omitempty"`
	Pointer string            `json:"pointer,omitempty"`
	Equals  json.RawMessage   `json:"equals,omitempty"`
	AnyOf   []json.RawMessage `json:"in,omitempty"`
	All     []LightCondition  `json:"all,omitempty"`
	Any     []LightCondition  `json:"any,omitempty"`
}
type LightReviewMapping struct {
	Lens                string          `json:"lens"`
	ReportStage         string          `json:"report_stage"`
	ReportPort          string          `json:"report_port"`
	ReportSchema        flow.Ref        `json:"report_schema"`
	GateStage           string          `json:"gate_stage"`
	GateAlternatives    []string        `json:"gate_alternatives"`
	GatePort            string          `json:"gate_port"`
	GateSchema          flow.Ref        `json:"gate_schema"`
	GateScope           string          `json:"gate_scope"`
	GateAncestor        int             `json:"gate_ancestor"`
	RepairAncestor      int             `json:"repair_ancestor"`
	GateReportInputs    []string        `json:"gate_report_inputs"`
	RequestPort         string          `json:"request_port"`
	CandidatePort       string          `json:"candidate_port"`
	CandidateStage      string          `json:"candidate_stage"`
	CandidateOutput     string          `json:"candidate_output"`
	RepairStages        []string        `json:"repair_stages"`
	Enabled             *LightCondition `json:"enabled"`
	DisabledState       string          `json:"disabled_state"`
	RoundPointer        string          `json:"round_pointer"`
	CandidatePointer    string          `json:"candidate_pointer"`
	RequestPointer      string          `json:"request_pointer"`
	ChecksPointer       string          `json:"checks_pointer"`
	GateStatusPointer   string          `json:"gate_status_pointer"`
	GateBlockingPointer string          `json:"gate_blocking_pointer"`
	GateOwnerPointer    string          `json:"gate_owner_pointer"`
	FindingsPointer     string          `json:"findings_pointer"`
}

func ParseLightProjection(data []byte) (*LightProjection, error) {
	if len(data) > 256<<10 {
		return nil, fault("light_mapping_invalid", "mapping exceeds 256 KiB")
	}
	var p LightProjection
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return nil, fault("light_mapping_invalid", err.Error())
	}
	if d.Decode(new(any)) != io.EOF || p.SchemaVersion != LightProjectionVersion || len(p.Workflows) > 64 {
		return nil, fault("light_mapping_invalid", "unsupported mapping version or size")
	}
	workflows := map[flow.Ref]bool{}
	for _, w := range p.Workflows {
		if !lightRefValid(w.WorkflowRef) || workflows[w.WorkflowRef] || len(w.Phases) > 128 || len(w.Reviews) > 16 {
			return nil, fault("light_mapping_invalid", "invalid or duplicate workflow mapping")
		}
		workflows[w.WorkflowRef] = true
		stages := map[string]bool{}
		for _, phase := range w.Phases {
			if phase.Stage == "" || len(phase.Stage) > 128 || stages[phase.Stage] || !slices.Contains([]string{"plan", "implementation", "review", "work"}, phase.Key) || phase.Label == "" || len(phase.Label) > 160 {
				return nil, fault("light_mapping_invalid", "invalid or duplicate phase")
			}
			stages[phase.Stage] = true
		}
		lenses := map[string]bool{}
		for _, m := range w.Reviews {
			if m.Lens == "" || len(m.Lens) > 80 || lenses[m.Lens] || !lightRefValid(m.ReportSchema) || !lightRefValid(m.GateSchema) || m.ReportStage == "" || m.ReportPort == "" || m.GateStage == "" || m.GatePort == "" || m.GateAncestor < 0 || m.GateAncestor > 8 || m.RepairAncestor < 0 || m.RepairAncestor > 8 || len(m.GateAlternatives) > 8 || len(m.GateReportInputs) > 16 || len(m.RepairStages) > 128 || !slices.Contains([]string{"lens", "aggregate"}, m.GateScope) {
				return nil, fault("light_mapping_invalid", "invalid review selector")
			}
			lenses[m.Lens] = true
			if m.RequestPort == "" || m.CandidatePort == "" || len(m.GateReportInputs) == 0 || (m.CandidateStage == "") != (m.CandidateOutput == "") {
				return nil, fault("light_mapping_invalid", "missing request/candidate/gate correlation selector")
			}
			for _, stage := range append(append([]string{}, m.GateAlternatives...), m.RepairStages...) {
				if stage == "" || len(stage) > 128 {
					return nil, fault("light_mapping_invalid", "invalid stage selector")
				}
			}

			for _, p := range []string{m.RoundPointer, m.CandidatePointer, m.RequestPointer, m.ChecksPointer, m.GateStatusPointer, m.GateBlockingPointer, m.GateOwnerPointer, m.FindingsPointer} {
				if !lightPointerValid(p) {
					return nil, fault("light_mapping_invalid", "invalid JSON pointer")
				}
			}
			if m.DisabledState != "disabled" && m.DisabledState != "not_applicable" {
				return nil, fault("light_mapping_invalid", "disabled_state must declare its scope")
			}
			if m.Enabled != nil {
				nodes := 0
				if !lightConditionValid(*m.Enabled, 0, &nodes) {
					return nil, fault("light_mapping_invalid", "invalid applicability predicate")
				}
			}
		}
	}
	p.Digest = rawDigest(data)
	return &p, nil
}
func lightRefValid(ref flow.Ref) bool {
	return ref.ID != "" && len(ref.ID) <= 128 && ref.Version != "" && len(ref.Version) <= 64 && len(ref.Digest) == 71 && strings.HasPrefix(ref.Digest, "sha256:") && strings.IndexFunc(ref.Digest[7:], func(r rune) bool { return !strings.ContainsRune("0123456789abcdef", r) }) < 0
}
func lightPointerValid(p string) bool {
	if len(p) > 2048 || p != "" && !strings.HasPrefix(p, "/") {
		return false
	}
	for i := 0; i < len(p); i++ {
		if p[i] == '~' {
			i++
			if i >= len(p) || (p[i] != '0' && p[i] != '1') {
				return false
			}
		}
	}
	return true
}
func lightPointer(v any, p string) (any, bool) {
	if p == "" {
		return v, true
	}
	for _, part := range strings.Split(strings.TrimPrefix(p, "/"), "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch object := v.(type) {
		case map[string]any:
			var ok bool
			v, ok = object[part]
			if !ok {
				return nil, false
			}
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(object) || strconv.Itoa(i) != part {
				return nil, false
			}
			v = object[i]
		default:
			return nil, false
		}
	}
	return v, true
}

func lightConditionValid(c LightCondition, depth int, nodes *int) bool {
	*nodes++
	if depth > 4 || *nodes > 64 || len(c.All) > 16 || len(c.Any) > 16 || len(c.AnyOf) > 16 {
		return false
	}
	groups := 0
	if len(c.All) > 0 {
		groups++
	}
	if len(c.Any) > 0 {
		groups++
	}
	if groups > 0 {
		if groups != 1 || c.Input != "" || c.Pointer != "" || len(c.Equals) > 0 || len(c.AnyOf) > 0 {
			return false
		}
		for _, child := range append(append([]LightCondition{}, c.All...), c.Any...) {
			if !lightConditionValid(child, depth+1, nodes) {
				return false
			}
		}
		return true
	}
	if c.Input == "" || !lightPointerValid(c.Pointer) || (len(c.Equals) > 0) == (len(c.AnyOf) > 0) {
		return false
	}
	values := c.AnyOf
	if len(c.Equals) > 0 {
		values = []json.RawMessage{c.Equals}
	}
	for _, raw := range values {
		if len(raw) > 1024 {
			return false
		}
		var value any
		if json.Unmarshal(raw, &value) != nil {
			return false
		}
		switch value.(type) {
		case nil, string, bool, float64:
		default:
			return false
		}
	}
	return true
}

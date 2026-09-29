package runtime

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
)

// DeclaredEffect is what a step's sealed definition says it may change: its
// effect class, the retry class that decides what an interrupted attempt
// does, and for an external write the boundary its author declared. It is the
// declaration the Run executed under, not an observation of what the step
// did.
type DeclaredEffect struct {
	Class         string                      `json:"class"`
	RetryClass    string                      `json:"retry_class"`
	ExternalWrite *flow.ExternalWriteBoundary `json:"external_write,omitempty"`
}

// StepEffect is one step instance's declared effect, with the stage it ran as.
type StepEffect struct {
	StepInstanceID string `json:"step_instance_id"`
	StageID        string `json:"stage_id"`
	DeclaredEffect
}

// declaredEffect reads a step definition sealed in this Run. The sealed bytes
// answer, not the installed package: a package updated after the Run may
// declare something else, and the Run ran under what it sealed.
func (r Run) declaredEffect(ref flow.Ref) (DeclaredEffect, bool) {
	for _, definition := range r.Definitions {
		if definition.Ref != ref || definition.Kind != "step" {
			continue
		}
		var step flow.StepDefinition
		if json.Unmarshal(definition.Bytes, &step) != nil {
			return DeclaredEffect{}, false
		}
		return DeclaredEffect{Class: step.Effects.Class, RetryClass: step.Effects.RetryClass, ExternalWrite: step.ExternalWrite}, true
	}
	return DeclaredEffect{}, false
}

// StepEffects lists every step instance of this Run with its declared effect,
// in the order the instances were created. The boundary of an external write
// was sealed from the first release that allowed it and reachable only as raw
// definition bytes; a pilot looking for it in run status and events found
// nothing.
func (r Run) StepEffects() []StepEffect {
	effects := make([]StepEffect, 0, len(r.Steps))
	for id, step := range r.Steps {
		if step == nil {
			continue
		}
		declared, ok := r.declaredEffect(step.Ref)
		if !ok {
			continue
		}
		stage := ""
		if activation := r.Activations[step.ActivationID]; activation != nil {
			stage = activation.StageID
		}
		effects = append(effects, StepEffect{StepInstanceID: id, StageID: stage, DeclaredEffect: declared})
	}
	created := func(id string) time.Time {
		at, _ := time.Parse(time.RFC3339Nano, r.Steps[id].Created.UTC)
		return at
	}
	slices.SortFunc(effects, func(a, b StepEffect) int {
		if order := created(a.StepInstanceID).Compare(created(b.StepInstanceID)); order != 0 {
			return order
		}
		return strings.Compare(a.StepInstanceID, b.StepInstanceID)
	})
	return effects
}

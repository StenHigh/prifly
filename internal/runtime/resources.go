package runtime

import (
	"fmt"
	"regexp"
	"slices"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// MaxResourceLimit bounds a declared limit: past the admission capacity it
// changes nothing, and the capacity itself is bounded well below this.
const MaxResourceLimit = 64

var resourceName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

func isResourceState(version string) bool { return atLeast(version, CoreResourceStateVersion) }

// checkStageResources holds a launch's declaration to what can be sealed: each
// stage exists in the root workflow, each name is a plain identifier, and each
// named resource has a limit within bounds.
func checkStageResources(p *flow.Plan, stages map[string][]string, limits map[string]int64) error {
	for stage, names := range stages {
		if _, exists := p.Workflow.Definition.Stages[stage]; !exists {
			return fault("unknown_resource_stage", fmt.Sprintf("stage %s holds a resource but workflow %s has no such stage", stage, p.Workflow.ID))
		}
		for _, name := range names {
			if !resourceName.MatchString(name) {
				return fault("invalid_resource", fmt.Sprintf("resource name %q is not a lowercase identifier", name))
			}
			if limit, declared := limits[name]; !declared || limit < 1 || limit > MaxResourceLimit {
				return fault("invalid_resource", fmt.Sprintf("resource %s needs a limit from 1 to %d", name, MaxResourceLimit))
			}
		}
	}
	return nil
}

// resourcesFor is what an attempt of this activation holds: the resources of
// the root stage it runs under, directly or inside the calls that stage made.
func (r Run) resourcesFor(activationID string) []local.ResourceClaim {
	if !isResourceState(r.SchemaVersion) || len(r.StageResources) == 0 {
		return nil
	}
	activation := r.Activations[activationID]
	for activation != nil {
		invocation := r.Invocations[activation.InvocationID]
		if invocation == nil || invocation.ParentInvocationID == "" || activation.InvocationID == r.RootInvocationID {
			break
		}
		activation = r.Activations[invocation.CallerActivationID]
	}
	if activation == nil {
		return nil
	}
	names := slices.Clone(r.StageResources[activation.StageID])
	slices.Sort(names)
	names = slices.Compact(names)
	claims := make([]local.ResourceClaim, 0, len(names))
	for _, name := range names {
		claims = append(claims, local.ResourceClaim{Name: name, Limit: r.ResourceLimits[name]})
	}
	return claims
}

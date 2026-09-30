package main

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// projectResource is one exclusive resource as extend.yaml declares it, or as
// local.yaml overrides its limit.
type projectResource struct {
	Limit  int64
	Stages []string
}

var projectResourceName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

// projectReadResources reads `resources: {NAME: {limit: N, stages: [...]}}`.
// In local.yaml only the limit is read: which stages hold a resource is the
// workflow's shape, shared by the team, and a machine says only how much of
// it it can take.
func projectReadResources(raw any, local bool) (map[string]projectResource, error) {
	file := "extend.yaml"
	code := "project_extension_invalid"
	if local {
		file, code = "local.yaml", "project_local_invalid"
	}
	object, ok := raw.(map[string]any)
	if !ok {
		return nil, refusal(code, file+" resources maps a resource name to {limit, stages}")
	}
	result := map[string]projectResource{}
	for name, rawResource := range object {
		if !projectResourceName.MatchString(name) {
			return nil, refusal(code, fmt.Sprintf("%s resource %q: a name is lowercase letters, digits, - and _", file, name))
		}
		fields, ok := rawResource.(map[string]any)
		if !ok {
			return nil, refusal(code, file+" resource "+name+" is an object")
		}
		resource := projectResource{Limit: 1}
		for key, value := range fields {
			switch {
			case key == "limit":
				limit, ok := wholeNumber(value)
				if !ok || limit < 1 || limit > prifly.MaxResourceLimit {
					return nil, refusal(code, fmt.Sprintf("%s resource %s: limit is a whole number from 1 to %d", file, name, prifly.MaxResourceLimit))
				}
				resource.Limit = limit
			case key == "stages" && !local:
				items, ok := value.([]any)
				if !ok || len(items) == 0 {
					return nil, refusal(code, file+" resource "+name+": stages is a non-empty list of stage ids")
				}
				for _, item := range items {
					stage, ok := item.(string)
					if !ok || stage == "" || slices.Contains(resource.Stages, stage) {
						return nil, refusal(code, file+" resource "+name+": stages lists each stage id once")
					}
					resource.Stages = append(resource.Stages, stage)
				}
			default:
				return nil, refusal(code, fmt.Sprintf("%s resource %s has unknown field %s", file, name, key))
			}
		}
		if !local && len(resource.Stages) == 0 {
			return nil, refusal(code, file+" resource "+name+" names no stages that hold it")
		}
		result[name] = resource
	}
	return result, nil
}

// projectLaunchResources turns the declaration into what a Run seals: for each
// root stage, the resources it holds, and each resource's limit, with this
// machine's local.yaml limit in place of the shared one. A stage the workflow
// does not have is refused before anything is created.
func projectLaunchResources(root string, declared map[string]projectResource, plan *flow.Plan) (map[string][]string, map[string]int64, error) {
	if len(declared) == 0 {
		return nil, nil, nil
	}
	_, local, err := readProjectLocalExecutionAll(root)
	if err != nil {
		return nil, nil, err
	}
	stages, limits := map[string][]string{}, map[string]int64{}
	for name, resource := range declared {
		limits[name] = resource.Limit
		if limit, overridden := local.ResourceLimits[name]; overridden {
			limits[name] = limit
		}
		for _, stage := range resource.Stages {
			if _, exists := plan.Workflow.Definition.Stages[stage]; !exists {
				return nil, nil, refusal("project_resource_stage_unknown", fmt.Sprintf("extend.yaml resource %s names stage %s, which workflow %s does not have", name, stage, plan.Workflow.ID))
			}
			stages[stage] = append(stages[stage], name)
		}
	}
	for stage := range stages {
		slices.Sort(stages[stage])
	}
	return stages, limits, nil
}

// wholeNumber reads an integer whichever numeric type the YAML reader chose.
func wholeNumber(value any) (int64, bool) {
	switch number := value.(type) {
	case int:
		return int64(number), true
	case int64:
		return number, true
	case uint64:
		return int64(number), number <= math.MaxInt64
	case float64:
		return int64(number), number == math.Trunc(number) && math.Abs(number) < 1<<53
	case json.Number:
		parsed, err := number.Int64()
		return parsed, err == nil
	}
	return 0, false
}

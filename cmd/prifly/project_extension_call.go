package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
)

func projectCallBindingExpression(expression string) bool {
	if !strings.HasPrefix(expression, projectExtensionStageSource) {
		return false
	}
	parts := strings.Split(strings.TrimPrefix(expression, projectExtensionStageSource), ".")
	return len(parts) == 2 && parts[0] != "" && parts[1] != ""
}

// Resolve against source declarations before compiling anything: a root need
// not contain a placeholder for the child an extension introduces.
func projectExtensionCallTargets(pending []projectPendingDocument, extensions []projectWorkflowExtension) (map[string]flow.Ref, error) {
	targets := map[string]flow.Ref{}
	for _, extension := range extensions {
		if extension.Call == "" {
			continue
		}
		candidates := []int{}
		wrongKind := false
		for i, item := range pending {
			object, _ := item.value.(map[string]any)
			id, _ := object["id"].(string)
			if projectExtensionWorkflowName(id) != extension.Call {
				continue
			}
			if item.document.Kind == "workflow" {
				candidates = append(candidates, i)
			} else {
				wrongKind = true
			}
		}
		if len(candidates) == 0 {
			if wrongKind {
				return nil, refusal("project_extension_call_wrong_kind", extension.ID+".call names a non-workflow component: "+extension.Call)
			}
			return nil, refusal("project_extension_unknown_call", extension.ID+".call is not a declared local workflow: "+extension.Call)
		}
		if len(candidates) != 1 {
			return nil, refusal("project_extension_call_ambiguous", extension.ID+".call matches multiple workflow declarations: "+extension.Call)
		}
		target := pending[candidates[0]]
		if target.document.FolderRoot || target.document.Extensions != "" {
			return nil, &flow.Problem{Code: "call_cycle", Path: "/extensions/" + extension.ID + "/call", Message: "an extension cannot call its own root workflow"}
		}
		object, _ := target.value.(map[string]any)
		id, _ := object["id"].(string)
		version, _ := object["version"].(string)
		targets[extension.ID] = flow.Ref{ID: id, Version: version}
	}
	if len(targets) > 0 {
		if err := projectExtensionDependencyCycles(pending, targets); err != nil {
			return nil, err
		}
	}
	return targets, nil
}

func projectExtensionDependencyCycles(pending []projectPendingDocument, targets map[string]flow.Ref) error {
	aliases := map[string]int{}
	identities := map[string]int{}
	for i, item := range pending {
		object, _ := item.value.(map[string]any)
		id, _ := object["id"].(string)
		version, _ := object["version"].(string)
		aliases[item.document.AliasPrefix+"_"+projectExtensionWorkflowName(id)] = i
		identities[id+"@"+version] = i
	}
	edges := make([][]int, len(pending))
	roots := []int{}
	for i, item := range pending {
		var visit func(any)
		visit = func(value any) {
			switch v := value.(type) {
			case string:
				if match := projectValueReference.FindStringSubmatch(v); match != nil {
					if j, ok := aliases[match[1]]; ok {
						edges[i] = append(edges[i], j)
					}
				}
			case []any:
				for _, child := range v {
					visit(child)
				}
			case map[string]any:
				for _, child := range v {
					visit(child)
				}
			}
		}
		visit(item.value)
		if item.document.Extensions != "" {
			roots = append(roots, i)
			for _, target := range targets {
				edges[i] = append(edges[i], identities[target.ID+"@"+target.Version])
			}
		}
	}
	state := make([]uint8, len(pending))
	var walk func(int) error
	walk = func(i int) error {
		if state[i] == 1 {
			return &flow.Problem{Code: "call_cycle", Path: "/extensions", Message: "extension and local definition dependencies form a cycle"}
		}
		if state[i] == 2 {
			return nil
		}
		state[i] = 1
		for _, j := range edges[i] {
			if err := walk(j); err != nil {
				return err
			}
		}
		state[i] = 2
		return nil
	}
	for _, root := range roots {
		if err := walk(root); err != nil {
			return err
		}
	}
	return nil
}

func projectExtensionCallsReady(components []projectCompileComponent, targets map[string]flow.Ref) bool {
	for _, target := range targets {
		found := false
		for _, c := range components {
			if c.Kind == "workflow" && c.Ref.ID == target.ID && c.Ref.Version == target.Version {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func projectApplyCallExtension(workflow map[string]any, extension projectWorkflowExtension, components []projectCompileComponent) error {
	var child projectCompileComponent
	for _, c := range components {
		if c.Kind == "workflow" && projectExtensionWorkflowName(c.Ref.ID) == extension.Call {
			if child.Ref.ID != "" {
				return refusal("project_extension_call_ambiguous", extension.ID+".call matches multiple workflows")
			}
			child = c
		}
	}
	if child.Ref.ID == "" {
		return refusal("project_extension_unknown_call", extension.ID+".call: "+extension.Call)
	}
	var definition flow.WorkflowRevision
	if err := json.Unmarshal(child.Bytes, &definition); err != nil {
		return err
	}
	for _, outcome := range definition.AllowedOutcomes {
		if extension.On[outcome] == "" {
			return refusal("project_extension_call_outcome_unmapped", fmt.Sprintf("extensions.%s.on.%s must route declared child outcome", extension.ID, outcome))
		}
	}
	return applyProjectExtension(workflow, extension, projectRefValue(child.Ref))
}

// Only definitions reachable through semantic refs are eligible. Ref-shaped
// literal data is not an execution edge; use the build's existing ref walker.
func projectValidateCallExtensionBindings(components []projectCompileComponent, options projectWorkflowOptions) error {
	if options.Authoring == "" || len(options.ExecutionBindings) == 0 {
		return nil
	}
	byRef := map[flow.Ref]projectCompileComponent{}
	allowed := map[flow.Ref]bool{}
	seen := map[flow.Ref]bool{}
	for _, c := range components {
		byRef[c.Ref] = c
	}
	var visit func(flow.Ref) error
	visit = func(ref flow.Ref) error {
		if seen[ref] {
			return nil
		}
		seen[ref] = true
		c, owned := byRef[ref]
		if !owned {
			return nil
		}
		if c.Kind == "step" {
			allowed[ref] = true
			return nil
		}
		if c.Kind != "workflow" {
			return nil
		}
		var value any
		if err := json.Unmarshal(c.Bytes, &value); err != nil {
			return err
		}
		_, err := projectMapDefinitionRefs(value, false, func(child flow.Ref) (any, error) {
			if err := visit(child); err != nil {
				return nil, err
			}
			return projectRefValue(child), nil
		})
		return err
	}
	for _, extension := range options.Extensions {
		if !projectExtensionEnabled(extension) {
			continue
		}
		for _, c := range components {
			if extension.Call != "" && c.Kind == "workflow" && projectExtensionWorkflowName(c.Ref.ID) == extension.Call {
				if err := visit(c.Ref); err != nil {
					return err
				}
			}
			if extension.Step != "" && c.Kind == "step" && projectExtensionWorkflowName(c.Ref.ID) == extension.Step {
				allowed[c.Ref] = true
			}
		}
	}
	for name := range options.ExecutionBindings {
		matches := []projectCompileComponent{}
		for _, c := range components {
			if c.Kind == "step" && projectExtensionWorkflowName(c.Ref.ID) == name {
				matches = append(matches, c)
			}
		}
		if len(matches) != 1 || matches[0].Kind != "step" || !allowed[matches[0].Ref] {
			return refusal("project_extension_invalid", "execution_bindings.steps."+name+" must name one owned step reachable from an inserted step/call closure")
		}
		var step flow.StepDefinition
		if err := json.Unmarshal(matches[0].Bytes, &step); err != nil {
			return err
		}
		if step.Executor.Operation != "process" {
			return refusal("project_extension_invalid", "execution_bindings.steps."+name+" must name a program step")
		}
	}
	return nil
}

// Review binds effective local limits as well as the declared contract sealed
// into the package. Changing local.yaml cannot reuse consent to another limit.
func projectCallExtensionReview(summary projectLaunchSummary, enabled bool, stages map[string][]string, limits map[string]int64) (string, error) {
	summary.WorktreeNamePreview = nil
	if !enabled {
		return projectReviewDigest(summary)
	}
	return projectReviewDigest(map[string]any{"launch": summary, "stage_resources": stages, "resource_limits": limits})
}

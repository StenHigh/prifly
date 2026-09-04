package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func projectValidateLaunch(ctx context.Context, engine *prifly.Engine, root string, compiled projectCompileResult, workflowPath, host, workspace string, allow bool, values map[string]json.RawMessage, refs map[string]prifly.ArtifactRef) (*prifly.ExecutionBindings, bool, error) {
	definitions, registry, resources, err := engine.CompilationInventory()
	if err != nil {
		return nil, false, err
	}
	var workflow []byte
	for _, component := range compiled.Components {
		if component.Path == workflowPath {
			workflow = component.Bytes
		}
		if component.Resource != nil {
			resources[component.Ref] = *component.Resource
			continue
		}
		registry[component.Ref] = component.Bytes
		definitions = append(definitions, prifly.PinnedDefinition{Ref: component.Ref, Kind: component.Kind, RawDigest: fmt.Sprintf("sha256:%x", sha256.Sum256(component.Bytes)), Bytes: component.Bytes})
	}
	if workflow == nil {
		return nil, false, usageError("project_start_invalid_root: compiled root not found")
	}
	plan, err := flow.CompileCore(workflow, "json", registry, resources)
	if err != nil {
		return nil, false, err
	}
	var assisted flow.Ref
	for _, definition := range definitions {
		if definition.Ref.ID == "core:adapter/assisted-session" {
			assisted = definition.Ref
		}
	}
	closure := map[flow.Ref]bool{}
	needsHost, needsWorkspace := false, false
	seen := map[*flow.Plan]bool{}
	var visit func(*flow.Plan)
	visit = func(current *flow.Plan) {
		if seen[current] {
			return
		}
		seen[current] = true
		for id, step := range current.Steps {
			closure[current.Workflow.Definition.Stages[id].StepRef] = true
			if step.Executor.AdapterRef == assisted && step.Executor.Operation == "session" {
				needsHost = true
				needsWorkspace = needsWorkspace || step.Effects.Class == "workspace_write" || len(step.WorkspaceTrees) != 0
			}
		}
		for _, child := range current.Calls {
			visit(child)
		}
		for _, child := range current.Repeats {
			visit(child)
		}
		for _, child := range current.Maps {
			visit(child)
		}
		for _, branches := range current.Branches {
			for _, child := range branches {
				visit(child)
			}
		}
	}
	visit(plan)
	for ref := range plan.Checks {
		closure[ref] = true
	}
	if needsHost && host == "" {
		return nil, false, usageError("project_start_host_required: selected workflow contains an assisted step; select a declared --host")
	}
	if needsWorkspace && workspace == "" {
		return nil, false, usageError("project_start_workspace_required: choose --workspace worktree or --workspace checkout before repository changes")
	}
	if !needsWorkspace && workspace != "" {
		return nil, false, usageError("project_start_workspace_unused: selected workflow does not need a Git workspace")
	}
	if needsWorkspace {
		gitRoot, err := projectRepositoryRoot(ctx, root)
		if err != nil {
			return nil, false, err
		}
		if gitRoot != root {
			return nil, false, usageError("project_start_git_root: assisted writes require the Project to be the Git root")
		}
	}
	payload, err := projectExecutionPayload(root, compiled, closure, allow)
	if err != nil {
		return nil, false, err
	}
	if err := engine.ValidateExecutionBindings(plan, definitions, registry, payload); err != nil {
		return nil, false, err
	}
	if err := engine.ValidateStartInputs(plan, values, refs); err != nil {
		return nil, false, err
	}
	return payload, needsWorkspace, nil
}

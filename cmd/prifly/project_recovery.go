package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// projectRecoverySource carries only sealed source bytes. The tree the stage
// runs in again is the source Run's own, which the plan hands over. Recovery
// takes a technical failure; resuming takes whatever the workflow declares,
// which the plan checks.
func projectRecoverySource(ctx context.Context, engine *prifly.Engine, runID, fromStage string, resuming bool) (prifly.RecoveryRequest, map[string]json.RawMessage, error) {
	var request prifly.RecoveryRequest
	view, err := engine.View(ctx, runID)
	if err != nil {
		return request, nil, err
	}
	run := view.Run
	if !resuming && (run.Status != "failed" || run.Outcome != nil) {
		return request, nil, refusal("recover_source_ineligible", "source Run must have a technical failure without an outcome")
	}
	values := map[string]json.RawMessage{}
	for name, ref := range run.Inputs {
		_, data, err := engine.Artifact(ref)
		if err != nil {
			return request, nil, err
		}
		values[name] = data
	}
	request = prifly.RecoveryRequest{SourceRunID: run.ID, SourceRunVersion: view.RunVersion, FromStage: fromStage}
	return request, values, nil
}

// projectResumes answers whether a continuation launch runs the source Run's
// own workflow, which makes it a resume rather than a continuation.
func projectResumes(ctx context.Context, engine *prifly.Engine, runID string, target *flow.Plan) (bool, error) {
	view, err := engine.View(ctx, runID)
	if err != nil {
		return false, err
	}
	return view.Run.WorkflowRef.ID == target.Workflow.ID, nil
}

// projectCarrySourceInputs adds the source Run's inputs, except those a
// decision answers: the answers must be the source's, which the plan checks.
func projectCarrySourceInputs(preflight projectPreflight, details projectLaunchDetail, carried, inputValues map[string]json.RawMessage) {
	decisionPorts := map[string]bool{}
	for _, decision := range preflight.Catalog.Decisions {
		if decision.Destination.Kind == "launch_input" {
			decisionPorts[decision.Destination.Name] = true
		}
	}
	for _, input := range details.Inputs {
		if input.Configured {
			decisionPorts[input.Name] = true
		}
	}
	for name, value := range carried {
		if !decisionPorts[name] {
			inputValues[name] = value
		}
	}
}

// projectSourceWorkspace is the mode of the tree the source Run still holds,
// or empty when it holds none.
func projectSourceWorkspace(ctx context.Context, engine *prifly.Engine, runID string) (string, error) {
	record, err := engine.Claims(ctx)
	if err != nil {
		return "", err
	}
	for _, claim := range record.Claims {
		if claim.RunID == runID && claim.Status == "active" {
			if claim.Mode == "" {
				return "worktree", nil
			}
			return claim.Mode, nil
		}
	}
	return "", nil
}

// projectSourcedAnswers are a source Run's sealed decisions written back as
// the flags that would seal them again.
type projectSourcedAnswers struct {
	preflight, runtime stringsFlag
	profile, policy    string
}

// projectSourceAnswers reads the answers a recovery or resume takes from its
// source: always for a recovery, and for a continuation only when the launch
// runs the source Run's own workflow -- read here from the launch's source,
// and checked against the compiled workflow once it exists. A source that
// sealed no decisions has none to give.
func projectSourceAnswers(ctx context.Context, engine *prifly.Engine, root string, launch projectLaunch, runID string, recovering bool) (projectSourcedAnswers, bool, error) {
	var sourced projectSourcedAnswers
	view, err := engine.View(ctx, runID)
	if err != nil {
		return sourced, false, err
	}
	run := view.Run
	if run.DecisionSheet == nil || run.DecisionCatalog == nil {
		return sourced, false, nil
	}
	if !recovering {
		data, err := os.ReadFile(filepath.Join(root, launch.Workflow))
		if err != nil {
			return sourced, false, err
		}
		value, err := flow.Parse(data, "yaml")
		if err != nil {
			return sourced, false, err
		}
		if document, _ := value.(map[string]any); document == nil || document["id"] != run.WorkflowRef.ID {
			return sourced, false, nil
		}
	}
	phases := map[string]prifly.DecisionDefinition{}
	for _, definition := range run.DecisionCatalog.Decisions {
		phases[definition.ID] = definition
	}
	for _, record := range run.DecisionSheet.Records {
		definition := phases[record.DefinitionID]
		if record.Status != "answered" || definition.Destination.Kind == "package_profile" {
			continue
		}
		answer := record.DefinitionID + "=" + string(record.Value)
		if definition.Phase == "runtime" {
			sourced.runtime = append(sourced.runtime, answer)
		} else {
			sourced.preflight = append(sourced.preflight, answer)
		}
	}
	sourced.profile, sourced.policy = run.DecisionSheet.PackageProfile, run.DecisionSheet.DecisionPolicy
	return sourced, true, nil
}

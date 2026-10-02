package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// Each entry keeps its actual Run; Sequence is meaningful only in that Run.
// This is prepare-local evidence, never new runtime executions or a cache.
type recoveryHistoryEntry struct {
	Source      Run
	Plan        *flow.Plan
	Sequences   map[string]int64
	Entry       RecoveryReuse
	Root        bool
	Revalidated bool
}

func recoveryPointWithHistory(source Run, oldPlan, target *flow.Plan, sequences map[string]int64, fromStage string, history []recoveryHistoryEntry) (recoveryPoint, int, error) {
	point, err := recoveryPointOf(source, oldPlan, target, sequences, "")
	if err != nil {
		return point, 0, err
	}
	if fromStage == "" {
		return point, len(history), nil
	}
	for i, row := range history {
		if row.Root && row.Entry.StageID == fromStage {
			switch target.Workflow.Definition.Stages[fromStage].Kind {
			case "step", "call", "repeat":
			default:
				return recoveryPoint{}, 0, local.Reject("resume_frontier_unsupported", "chosen inherited stage is not a step, call or repeat")
			}
			return recoveryPoint{StageID: fromStage, Cutoff: 0, Reason: "the operator chose to start again from this accepted inherited stage"}, i, nil
		}
	}
	point, err = recoveryPointOf(source, oldPlan, target, sequences, fromStage)
	return point, len(history), err
}

func recoveryTraceWithHistory(source Run, oldPlan, target *flow.Plan, definitions []PinnedDefinition, resources []PinnedResource, sequences map[string]int64, point recoveryPoint, history []recoveryHistoryEntry, inheritedCut int) ([]RecoveryReuse, map[string]map[string]ArtifactRef, error) {
	if oldPlan.Workflow.ID != target.Workflow.ID || oldPlan.Workflow.Definition.Entry != target.Workflow.Definition.Entry {
		return nil, nil, local.Reject("recover_prefix_changed", "root workflow or entry changed")
	}
	reused := []RecoveryReuse{}
	outputs := map[string]map[string]ArtifactRef{}
	// Keep the concatenated path in lineage order. Validate each origin against
	// the target independently; local journal numbers cannot order this list.
	for start := 0; start < inheritedCut; {
		if row := history[start]; row.Revalidated {
			oldEffective, err := recoveryEffectiveStage(row.Plan.Workflow.Definition.Stages[row.Entry.StageID], row.Source.Definitions, row.Source.ContextResources)
			if err != nil {
				return nil, nil, err
			}
			newEffective, err := recoveryEffectiveStage(target.Workflow.Definition.Stages[row.Entry.StageID], definitions, resources)
			if err != nil {
				return nil, nil, err
			}
			if !reflect.DeepEqual(oldEffective, newEffective) {
				return nil, nil, local.Reject("recover_prefix_changed", "revalidated inherited contract changed at stage "+row.Entry.StageID+"; resume with --from-stage "+row.Entry.StageID)
			}
			if ports := source.Recovery.RootOutputs[row.Entry.StageID]; len(ports) != 0 {
				outputs[row.Entry.StageID] = ports
			}
			start++
			continue
		}
		end := start + 1
		for end < inheritedCut && !history[end].Revalidated && history[end].Source.ID == history[start].Source.ID {
			end++
		}
		origin := history[start]
		accepted, _, err := recoveryAcceptedTrace(origin.Source, origin.Plan, target, definitions, resources, origin.Sequences, history[end-1].Entry.Sequence+1)
		if err != nil {
			return nil, nil, err
		}
		if len(accepted) != end-start {
			return nil, nil, local.Reject("recover_trace_invalid", "inherited origin does not contain the recorded accepted prefix")
		}
		for i, entry := range accepted {
			row := history[start+i]
			if !recoverySameTrace(entry, row.Entry) {
				return nil, nil, local.Reject("recover_trace_invalid", "inherited entry differs from its original accepted evidence")
			}
			reused = append(reused, entry)
			if row.Root && len(entry.Outputs) != 0 {
				outputs[entry.StageID] = source.Recovery.RootOutputs[entry.StageID]
			}
		}
		start = end
	}
	own, ownOutputs, err := recoveryAcceptedTrace(source, oldPlan, target, definitions, resources, sequences, point.Cutoff)
	if err != nil {
		return nil, nil, err
	}
	reused = append(reused, own...)
	for stage, ports := range ownOutputs {
		outputs[stage] = ports
	}
	if len(reused) == 0 {
		return nil, nil, local.Reject("resume_prefix_empty", "nothing was accepted before "+point.StageID+"; preserve the source workspace before reviewing a new start")
	}
	if err := recoveryFrontierShape(source, oldPlan, target, definitions, resources, point); err != nil {
		return nil, nil, err
	}
	return reused, outputs, nil
}

// The chain is loaded once from the immediate source backwards, then checked
// forwards. Neither an ancestor's claim nor its mutable filesystem is read.
func (e *Engine) recoveryHistory(ctx context.Context, source Run, visiting map[string]bool) ([]recoveryHistoryEntry, error) {
	if source.Recovery == nil {
		return nil, nil
	}
	provenance := source.Recovery
	if visiting[provenance.SourceRunID] {
		return nil, local.Reject("recover_trace_invalid", "recovery ancestry is cyclic")
	}
	visiting[provenance.SourceRunID] = true
	parent, view, err := e.load(ctx, provenance.SourceRunID)
	if err != nil {
		return nil, local.Reject("recover_evidence_unavailable", "recovery ancestor cannot be read: "+provenance.SourceRunID)
	}
	if view.Snapshot.Version != provenance.SourceRunVersion {
		return nil, local.Reject("recover_evidence_unavailable", "recovery ancestor version differs from the recorded source version")
	}
	parentPlan, err := parent.plan()
	if err != nil {
		return nil, local.Reject("recover_evidence_unavailable", "recovery ancestor contract cannot be read")
	}
	childPlan, err := source.plan()
	if err != nil {
		return nil, err
	}
	events := []local.Event{}
	for after := int64(0); ; {
		page, err := e.Events(ctx, parent.ID, after, 1000)
		if err != nil {
			return nil, err
		}
		events = append(events, page.Events...)
		if len(page.Events) == 0 || int64(len(events)) >= view.Snapshot.EventSeq {
			break
		}
		after = page.Events[len(page.Events)-1].Seq
	}
	sequences, err := activationSequences(events)
	if err != nil {
		return nil, err
	}
	inherited, err := e.recoveryHistory(ctx, parent, visiting)
	if err != nil {
		return nil, err
	}
	natural, _, err := recoveryPointWithHistory(parent, parentPlan, childPlan, sequences, "", inherited)
	if err != nil {
		return nil, err
	}
	chosen := provenance.FrontierStageID
	fromStage := ""
	if chosen != natural.StageID {
		fromStage = chosen
	}
	point, cut, err := recoveryPointWithHistory(parent, parentPlan, childPlan, sequences, fromStage, inherited)
	if err != nil {
		return nil, err
	}
	expected, outputs, err := recoveryTraceWithHistory(parent, parentPlan, childPlan, source.Definitions, source.ContextResources, sequences, point, inherited, cut)
	if err != nil {
		return nil, err
	}
	if !recoverySameTrace(expected, provenance.Reused) {
		return nil, local.Reject("recover_trace_invalid", "recorded recovery prefix differs from its ancestor's accepted path")
	}
	if provenance.FrontierAction == "revalidate" {
		if point.Attempt == nil || provenance.CandidateRef == nil {
			return nil, local.Reject("recover_trace_invalid", "revalidated origin has no failed Attempt or candidate")
		}
		candidate, err := recoveryCandidateRef(events, point.Attempt.ID)
		if err != nil || candidate != *provenance.CandidateRef {
			return nil, local.Reject("recover_candidate_invalid", "revalidated origin candidate differs from its journal")
		}
		_, data, err := e.Artifact(candidate)
		if err != nil {
			return nil, local.Reject("recover_evidence_unavailable", "revalidated candidate is unavailable")
		}
		var result Result
		if json.Unmarshal(data, &result) != nil || result.RunID != parent.ID || result.AttemptID != point.Attempt.ID || result.StepInstanceID != point.Attempt.StepID || result.EnvelopeDigest != point.Attempt.EnvelopeDigest {
			return nil, local.Reject("recover_candidate_invalid", "revalidated candidate identity differs from its Attempt")
		}
		if err := e.recoveryValidateCandidate(childPlan, point.StageID, result, data, func(port string, ref ArtifactRef) ([]byte, error) {
			copied := provenance.RootOutputs[point.StageID][port]
			if copied.Digest != ref.Digest {
				return nil, local.Reject("recover_candidate_invalid", "revalidated output differs from its sealed candidate")
			}
			_, data, err := e.Artifact(copied)
			return data, err
		}); err != nil {
			return nil, err
		}
		if len(result.Outputs) != 0 {
			outputs[point.StageID] = result.Outputs
		}
	}
	if len(outputs) != len(provenance.RootOutputs) {
		return nil, local.Reject("recover_trace_invalid", "carried root output stages differ from the accepted prefix")
	}
	for stage, ports := range outputs {
		copied := provenance.RootOutputs[stage]
		if len(ports) != len(copied) {
			return nil, local.Reject("recover_trace_invalid", "carried root output ports differ from the accepted prefix")
		}
		for port, ref := range ports {
			if copied[port].Digest != ref.Digest {
				return nil, local.Reject("recover_trace_invalid", "carried root output bytes differ from their accepted origin")
			}
			if _, _, err := e.Artifact(copied[port]); err != nil {
				return nil, local.Reject("recover_evidence_unavailable", "carried root output is unavailable: "+stage+"."+port)
			}
		}
	}
	history := append([]recoveryHistoryEntry{}, inherited[:cut]...)
	carriedEntries := 0
	for _, row := range history {
		if !row.Revalidated {
			carriedEntries++
		}
	}
	for _, entry := range expected[carriedEntries:] {
		history = append(history, recoveryHistoryEntry{Source: parent, Plan: parentPlan, Sequences: sequences, Entry: entry, Root: entry.InvocationID == parent.RootInvocationID})
	}
	if provenance.FrontierAction == "revalidate" {
		history = append(history, recoveryHistoryEntry{Source: source, Plan: childPlan, Entry: RecoveryReuse{StageID: point.StageID, Kind: childPlan.Workflow.Definition.Stages[point.StageID].Kind}, Root: true, Revalidated: true})
	}
	return history, nil
}

// Compare the persisted representation: omitted empty output maps decode nil.
func recoverySameTrace(a, b any) bool {
	x, err := json.Marshal(a)
	if err != nil {
		return false
	}
	y, err := json.Marshal(b)
	return err == nil && bytes.Equal(x, y)
}

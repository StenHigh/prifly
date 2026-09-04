package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// mapCollectionPort names the collection while it is being read. It is not a
// port of the body: the body receives items, never the list they came from.
const mapCollectionPort = "items"

// mapSealScope separates the artifacts one command prepares while sealing from
// the ones it prepares for a body's ordinary inputs.
const mapSealScope = "map_seal"

// SealedItem is one member of a sealed collection: the identity read out of the
// item, and the derived artifact that identity names. Both are fixed when the
// collection is sealed, so a later change to the source can neither rename a
// child of a fan-out already under way nor add one to it.
type SealedItem struct {
	// Key carries its own type, because identity is typed: the number 1 and
	// the string "1" are different items and must not collide.
	Key string `json:"key"`
	// Position is where the item sat in the sealed collection. It is evidence
	// about the collection, never a business identity: reordering the source
	// must not rename a child.
	Position int64       `json:"position"`
	Ref      ArtifactRef `json:"ref"`
}

// itemKey reads one item's identity and encodes its type into the key. A
// missing key, a key of another type, or a number that is not a safe integer
// refuses the whole expansion rather than falling back to the array position.
func itemKey(item any, pointer string) (string, error) {
	value, found := flow.JSONPointer(item, pointer)
	if !found {
		return "", faultf("item_key_missing", "no value at %s", pointer)
	}
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return "", fault("item_key_invalid", "an empty string is not an identity")
		}
		return "string:" + typed, nil
	case float64:
		if typed != math.Trunc(typed) || math.Abs(typed) > 1<<53-1 {
			return "", fault("item_key_invalid", "a numeric key must be a safe integer")
		}
		return "integer:" + strconv.FormatInt(int64(typed), 10), nil
	}
	return "", fault("item_key_invalid", "an item key is a string or a safe integer")
}

// sealCollection reads the whole collection, checks every item, and derives one
// artifact per item before any child is admitted. Nothing here creates work: a
// collection that fails anywhere fails entirely, so no half-processed expansion
// can be left behind.
func (e *Engine) sealCollection(r Run, p *flow.Plan, activation *Activation, commandID string) ([]SealedItem, error) {
	stage := p.Workflow.Definition.Stages[activation.StageID]
	body := p.Maps[activation.StageID]
	if stage.Kind != "map" || stage.Items == nil || body == nil {
		return nil, local.ErrIntegrity
	}
	source, guaranteed, err := p.CollectionPort(*stage.Items)
	if err != nil || !guaranteed {
		return nil, local.Reject("map_collection_unavailable", "the collection a map fans out over is not readable")
	}
	refs, err := e.prepareBindingsForBody(r, activation.InvocationID, "", map[string]flow.Binding{mapCollectionPort: *stage.Items}, map[string]flow.InputPort{mapCollectionPort: {Port: source, Required: true}}, commandID, mapSealScope)
	if err != nil {
		return nil, err
	}
	collection, ok := refs[mapCollectionPort]
	if !ok {
		return nil, local.Reject("map_collection_unavailable", "the collection a map fans out over is absent")
	}
	_, data, err := e.Artifact(collection)
	if err != nil {
		return nil, err
	}
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, local.Reject("map_collection_invalid", "a map fans out over a JSON array")
	}
	if len(items) > stage.MaxItems {
		return nil, local.Reject("map_items_exceeded", fmt.Sprintf("the collection holds %d items and the stage admits %d", len(items), stage.MaxItems))
	}
	if len(items) > flow.MaxMapItems {
		return nil, local.Reject("map_items_exceeded", fmt.Sprintf("the qualified profile admits at most %d items on one activation", flow.MaxMapItems))
	}
	itemPort := body.Workflow.Inputs[stage.ItemInput].Port
	sealed := make([]SealedItem, 0, len(items))
	seen := make(map[string]int64, len(items))
	producer := map[string]any{"kind": "authority", "authority_id": r.AuthorityID, "command_id": commandID, "port": stage.ItemInput}
	for position, raw := range items {
		var item any
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, local.Reject("map_item_invalid", fmt.Sprintf("item %d is not readable JSON", position))
		}
		key, err := itemKey(item, stage.ItemKeyPointer)
		if err != nil {
			return nil, local.Reject("map_item_key_invalid", fmt.Sprintf("item %d: %s", position, err.Error()))
		}
		if first, duplicate := seen[key]; duplicate {
			return nil, local.Reject("duplicate_item_key", fmt.Sprintf("items %d and %d share the identity %s", first, position, key))
		}
		seen[key] = int64(position)
		// Each item is validated against the port that will receive it, here
		// rather than at admission: an expansion that would fail on its ninth
		// item must not have run its first eight.
		if itemPort.SchemaRef != nil {
			if err := body.ValidateJSON(*itemPort.SchemaRef, raw); err != nil {
				return nil, local.Reject("map_item_invalid", fmt.Sprintf("item %d does not satisfy the body's item schema: %s", position, err.Error()))
			}
		}
		// Provenance names the collection this item was cut from, so an item
		// artifact can always be traced back to the exact sealed bytes.
		artifact, err := e.putArtifact(raw, "json", itemPort.SchemaRef, derivedID("artifact", commandID, activation.ID, key), producer, []ArtifactRef{collection}, body.Registry, portMedia(itemPort))
		if err != nil {
			return nil, err
		}
		sealed = append(sealed, SealedItem{Key: key, Position: int64(position), Ref: artifact.Ref()})
	}
	return sealed, nil
}

func (e *Engine) enterMap(ctx context.Context, loaded Run, view local.ReadView, p *flow.Plan, activation *Activation) error {
	if err := checkFanOutActivation(loaded, p, activation, "ready"); err != nil {
		return err
	}
	stage := p.Workflow.Definition.Stages[activation.StageID]
	body := p.Maps[activation.StageID]
	commandID := newID("command")
	sealed, err := e.sealCollection(loaded, p, activation, commandID)
	if err != nil {
		return e.failPreparation(ctx, loaded, view, p, activation, err, "map_expansion_failed")
	}
	if len(sealed) == 0 {
		return e.settleEmptyMap(ctx, loaded, view, p, activation, commandID)
	}
	opening := min(stage.MaxParallelism, len(sealed))
	inputs := make([]map[string]ArtifactRef, 0, opening)
	for _, item := range sealed[:opening] {
		refs, err := e.prepareBodyInputs(loaded, activation.InvocationID, "", body, stage.InputBindings, commandID, item.Key, stage.ItemInput)
		if err != nil {
			return e.failPreparation(ctx, loaded, view, p, activation, err, "branch_input_binding_failed")
		}
		refs[stage.ItemInput] = item.Ref
		inputs = append(inputs, refs)
	}
	opened := make([]string, 0, opening)
	for _, item := range sealed[:opening] {
		opened = append(opened, item.Key)
	}
	payload := map[string]any{"stage_activation_id": activation.ID, "item_keys": opened, "input_refs": inputs, "sealed": sealed}
	_, err = e.apply(ctx, e.owner, commandID, loaded.ID, "stage.map_entered", payload, &view.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		a := r.Activations[activation.ID]
		if err := checkFanOutActivation(*r, p, a, "ready"); err != nil {
			return local.Change{}, err
		}
		if a.Parallel.EnteredCount != 0 || a.Parallel.CurrentBranchInvocationID != "" || a.Parallel.LastDecision != nil || len(a.Parallel.Sealed) != 0 {
			return local.Change{}, local.Reject("stage_conflict", "map already sealed its collection")
		}
		// Sealing and entry are one change. A sealed collection with no branch
		// entered would be a fan-out nobody owns.
		a.Parallel.Sealed = sealed
		a.Parallel.BranchIDs = opened[:0:0]
		for _, item := range sealed {
			a.Parallel.BranchIDs = append(a.Parallel.BranchIDs, item.Key)
		}
		events := make([]local.EventInput, 0, opening+1)
		for i, item := range sealed[:opening] {
			created, err := r.createParallelBranch(a, item.Key, body, inputs[i], obs)
			if err != nil {
				return local.Change{}, err
			}
			events = append(events, created)
		}
		data, err := canonical(map[string]any{"run_id": r.ID, "stage_activation_id": a.ID, "stage_id": a.StageID, "workflow_invocation_id": a.InvocationID, "item_keys": a.Parallel.BranchIDs, "entered_item_keys": opened, "sealed": sealed, "join": stage.Join, "max_parallelism": stage.MaxParallelism, "observation": obs})
		if err != nil {
			return local.Change{}, err
		}
		return local.Change{RequireStorageBudget: true, Events: append([]local.EventInput{{Type: "stage.map_entered", Version: 1, Data: data}}, events...)}, nil
	})
	return err
}

// settleEmptyMap takes the one route a collection with no items can take. It
// records no join decision and no summary: there were no branches to observe,
// so there is no verdict about them and nothing to aggregate. Calling that a
// satisfied join would be a claim about work that never existed.
func (e *Engine) settleEmptyMap(ctx context.Context, loaded Run, view local.ReadView, p *flow.Plan, activation *Activation, commandID string) error {
	stage := p.Workflow.Definition.Stages[activation.StageID]
	next := stage.On["empty"]
	if next == "" {
		return local.Reject("unhandled_verdict", "a map stage declares where an empty collection leads")
	}
	payload := map[string]any{"stage_activation_id": activation.ID, "next_stage_id": next}
	_, err := e.apply(ctx, e.owner, commandID, loaded.ID, "stage.map_empty", payload, &view.Snapshot.Version, local.CommandCAS, func(r *Run, s local.Snapshot, obs Observation) (local.Change, error) {
		a := r.Activations[activation.ID]
		if err := checkFanOutActivation(*r, p, a, "ready"); err != nil {
			return local.Change{}, err
		}
		if a.Parallel.EnteredCount != 0 || len(a.Parallel.Sealed) != 0 {
			return local.Change{}, local.Reject("stage_conflict", "map already sealed a collection")
		}
		if err := r.chargeInvocation(a.InvocationID, 1, 0); err != nil {
			return local.Change{}, err
		}
		// The seal is recorded as what it is: an empty collection, checked and
		// found to hold nothing. That is a fact about the source, not a failure.
		a.Parallel.Sealed = []SealedItem{}
		a.Status, a.Settled = "completed", &obs
		if err := r.advanceInvocation(a.InvocationID, next); err != nil {
			return local.Change{}, err
		}
		data, err := canonical(map[string]any{"run_id": r.ID, "stage_activation_id": a.ID, "stage_id": a.StageID, "workflow_invocation_id": a.InvocationID, "item_count": 0, "next_stage_id": next, "observation": obs})
		if err != nil {
			return local.Change{}, err
		}
		return local.Change{RequireStorageBudget: true, Events: []local.EventInput{{Type: "stage.map_empty", Version: 1, Data: data}}}, nil
	})
	return err
}

// sealedItem finds one item of the sealed collection by its identity. A key
// that is not in the seal is an integrity failure, never a reason to read the
// source collection again.
func (progress *ParallelProgress) sealedItem(key string) (SealedItem, error) {
	for _, item := range progress.Sealed {
		if item.Key == key {
			return item, nil
		}
	}
	return SealedItem{}, local.ErrIntegrity
}

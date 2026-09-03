package runtime

import (
	"encoding/json"
	"fmt"

	"github.com/stenhigh/prifly/internal/flow"
)

const (
	CorePublicationSubscriptionReadVersion     = "core-read/14"
	CorePublicationSubscriptionStateVersion    = "core-state/14"
	CorePublicationSubscriptionNextVersion     = "core-next/14"
	CorePublicationSubscriptionPreviewVersion  = "core-preview/14"
	CorePublicationSubscriptionStepReadVersion = "core-step-read/14"

	PublicationSubscriptionVersion        = "1"
	PublicationNewOnlySubscriptionVersion = "2"
	PublicationAssignmentVersion          = "1"
	PublicationHandleVersion              = "publication-subscription-handle/1"
	PublicationCursorVersion              = "publication-cursor/1"
	PublicationDeliveryVersion            = "publication-delivery/1"

	publicationHandleSchemaID   = "core:schema/publication-subscription-handle"
	publicationCursorSchemaID   = "core:schema/publication-cursor"
	publicationDeliverySchemaID = "core:schema/publication-delivery"
	MaxPublicationSubscriptions = 64
	MaxPublicationAssignments   = MaxPublicationSubscriptions * 100
)

type PublicationSubscriptionHandle struct {
	SchemaVersion  string   `json:"schema_version"`
	SubscriptionID string   `json:"subscription_id"`
	RunID          string   `json:"run_id"`
	Generation     int64    `json:"generation"`
	SourceRef      flow.Ref `json:"source_ref"`
}

type PublicationCursor struct {
	SchemaVersion  string `json:"schema_version"`
	SubscriptionID string `json:"subscription_id"`
	Generation     int64  `json:"generation"`
	Position       int64  `json:"position"`
}

type PublicationDeliveryItem struct {
	PublicationID string      `json:"publication_id"`
	ItemKey       string      `json:"item_key"`
	Artifact      ArtifactRef `json:"artifact_ref"`
	SchemaRef     flow.Ref    `json:"schema_ref"`
}

type PublicationDeliveryClosure struct {
	ClosureID   string      `json:"closure_id"`
	Manifest    ArtifactRef `json:"manifest_ref"`
	ItemCount   int64       `json:"item_count"`
	CutSequence int64       `json:"cut_sequence"`
}

type PublicationDelivery struct {
	SchemaVersion  string                      `json:"schema_version"`
	Kind           string                      `json:"kind"`
	SubscriptionID string                      `json:"subscription_id"`
	Generation     int64                       `json:"generation"`
	Cursor         PublicationCursor           `json:"cursor"`
	NextCursor     PublicationCursor           `json:"next_cursor"`
	Publication    *PublicationDeliveryItem    `json:"publication,omitempty"`
	Closure        *PublicationDeliveryClosure `json:"closure,omitempty"`
	Reason         string                      `json:"reason,omitempty"`
}

// PublicationSubscription lives across repeat bodies. Cursor names the next
// unprocessed delivery; a pending assignment pins it until that body settles.
type PublicationSubscription struct {
	SchemaVersion      string   `json:"schema_version"`
	ID                 string   `json:"id"`
	RunID              string   `json:"run_id"`
	InvocationID       string   `json:"workflow_invocation_id"`
	RepeatActivationID string   `json:"repeat_activation_id"`
	SourceRef          flow.Ref `json:"source_ref"`
	Generation         int64    `json:"generation"`
	Cursor             int64    `json:"cursor"`
	// PublicationStartSequence is set only by a new_only source. It is an
	// authority cut, not a timestamp inferred from a producer workspace.
	PublicationStartSequence *int64      `json:"publication_start_sequence,omitempty"`
	PendingAssignmentID      string      `json:"pending_assignment_id,omitempty"`
	Status                   string      `json:"status"`
	Created                  Observation `json:"created"`
}

type PublicationAssignment struct {
	SchemaVersion    string       `json:"schema_version"`
	ID               string       `json:"id"`
	SubscriptionID   string       `json:"subscription_id"`
	Generation       int64        `json:"generation"`
	Cursor           int64        `json:"cursor"`
	NextCursor       int64        `json:"next_cursor"`
	Kind             string       `json:"kind"`
	PublicationID    string       `json:"publication_id,omitempty"`
	ClosureID        string       `json:"closure_id,omitempty"`
	Item             *ArtifactRef `json:"item_ref,omitempty"`
	Delivery         ArtifactRef  `json:"delivery_ref"`
	WaitActivationID string       `json:"wait_activation_id"`
	BodyInvocationID string       `json:"body_workflow_invocation_id"`
	Status           string       `json:"status"`
	Assigned         Observation  `json:"assigned"`
	Processed        *Observation `json:"processed,omitempty"`
}

func isPublicationSubscriptionState(version string) bool {
	return version == CorePublicationSubscriptionStateVersion || isPublicationChecksState(version)
}

func publicationSubscriptionID(runID, repeatActivationID string, source flow.Ref) string {
	return derivedID("subscription", runID, repeatActivationID, source.String())
}

func publicationAssignmentID(subscriptionID string, generation, cursor int64) string {
	return derivedID("assignment", subscriptionID, fmt.Sprint(generation), fmt.Sprint(cursor))
}

func publicationCursor(subscription *PublicationSubscription, position int64) PublicationCursor {
	return PublicationCursor{PublicationCursorVersion, subscription.ID, subscription.Generation, position}
}

func publicationTransportSchemas() (handle, cursor, delivery []byte, err error) {
	ref := func(name string) any { return map[string]any{"$ref": "#/$defs/" + name} }
	integer := map[string]any{"type": "integer", "minimum": 0, "maximum": int64(9007199254740991)}
	cursorProperties := map[string]any{
		"schema_version": map[string]any{"const": PublicationCursorVersion}, "subscription_id": ref("Identifier"),
		"generation": map[string]any{"type": "integer", "minimum": 1, "maximum": int64(9007199254740991)}, "position": integer,
	}
	cursorObject := map[string]any{"type": "object", "properties": cursorProperties, "required": []string{"schema_version", "subscription_id", "generation", "position"}, "additionalProperties": false}
	handleDocument, err := sourceContractSchema("urn:prifly:publication-subscription-handle:1", map[string]any{
		"schema_version": map[string]any{"const": PublicationHandleVersion}, "subscription_id": ref("Identifier"), "run_id": ref("Identifier"),
		"generation": map[string]any{"type": "integer", "minimum": 1, "maximum": int64(9007199254740991)}, "source_ref": ref("ImmutableRef"),
	}, []string{"schema_version", "subscription_id", "run_id", "generation", "source_ref"})
	if err != nil {
		return nil, nil, nil, err
	}
	cursorDocument, err := sourceContractSchema("urn:prifly:publication-cursor:1", cursorProperties, []string{"schema_version", "subscription_id", "generation", "position"})
	if err != nil {
		return nil, nil, nil, err
	}
	deliveryDocument, err := sourceContractSchema("urn:prifly:publication-delivery:1", map[string]any{
		"schema_version": map[string]any{"const": PublicationDeliveryVersion}, "kind": map[string]any{"enum": []string{"Item", "Closed", "Interrupted"}},
		"subscription_id": ref("Identifier"), "generation": map[string]any{"type": "integer", "minimum": 1, "maximum": int64(9007199254740991)},
		"cursor": cursorObject, "next_cursor": cursorObject,
		"publication": map[string]any{"type": "object", "properties": map[string]any{
			"publication_id": ref("Identifier"), "item_key": ref("Identifier"), "artifact_ref": ref("ArtifactRef"), "schema_ref": ref("ImmutableRef"),
		}, "required": []string{"publication_id", "item_key", "artifact_ref", "schema_ref"}, "additionalProperties": false},
		"closure": map[string]any{"type": "object", "properties": map[string]any{
			"closure_id": ref("Identifier"), "manifest_ref": ref("ArtifactRef"), "item_count": integer, "cut_sequence": integer,
		}, "required": []string{"closure_id", "manifest_ref", "item_count", "cut_sequence"}, "additionalProperties": false},
		"reason": map[string]any{"type": "string", "minLength": 1, "maxLength": 128},
	}, []string{"schema_version", "kind", "subscription_id", "generation", "cursor", "next_cursor"})
	if err != nil {
		return nil, nil, nil, err
	}
	deliveryDocument["oneOf"] = []any{
		map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "Item"}}, "required": []string{"publication"}, "not": map[string]any{"anyOf": []any{map[string]any{"required": []string{"closure"}}, map[string]any{"required": []string{"reason"}}}}},
		map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "Closed"}}, "required": []string{"closure"}, "not": map[string]any{"anyOf": []any{map[string]any{"required": []string{"publication"}}, map[string]any{"required": []string{"reason"}}}}},
		map[string]any{"properties": map[string]any{"kind": map[string]any{"const": "Interrupted"}}, "required": []string{"reason"}, "not": map[string]any{"anyOf": []any{map[string]any{"required": []string{"publication"}}, map[string]any{"required": []string{"closure"}}}}},
	}
	encode := func(value map[string]any) ([]byte, error) {
		data, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		return flow.Canonical(data)
	}
	if handle, err = encode(handleDocument); err != nil {
		return nil, nil, nil, err
	}
	if cursor, err = encode(cursorDocument); err != nil {
		return nil, nil, nil, err
	}
	delivery, err = encode(deliveryDocument)
	return
}

func publicationSubscriptionInvariant(r Run) error {
	if !isPublicationSubscriptionState(r.SchemaVersion) {
		if len(r.PublicationSubscriptions) != 0 || len(r.PublicationAssignments) != 0 {
			return fmt.Errorf("publication subscription invariant: older state contains stream records")
		}
		return nil
	}
	if len(r.PublicationSubscriptions) > MaxPublicationSubscriptions || len(r.PublicationAssignments) > MaxPublicationAssignments {
		return fmt.Errorf("publication subscription invariant: record limit exceeded")
	}
	assignments := map[string]*PublicationAssignment{}
	for i := range r.PublicationAssignments {
		a := &r.PublicationAssignments[i]
		if assignments[a.ID] != nil || a.SchemaVersion != PublicationAssignmentVersion || a.Generation < 1 || a.Cursor < 0 || a.NextCursor != a.Cursor+1 || a.ID != publicationAssignmentID(a.SubscriptionID, a.Generation, a.Cursor) || a.Status != "assigned" && a.Status != "processed" {
			return fmt.Errorf("publication subscription invariant: invalid assignment identity or cursor")
		}
		if a.Delivery.Revision != 1 || a.WaitActivationID == "" || a.BodyInvocationID == "" || a.Assigned.UTC == "" || a.Status == "processed" && a.Processed == nil || a.Status == "assigned" && a.Processed != nil {
			return fmt.Errorf("publication subscription invariant: invalid assignment lifecycle")
		}
		switch a.Kind {
		case "Item":
			if a.PublicationID == "" || a.ClosureID != "" || a.Item == nil {
				return fmt.Errorf("publication subscription invariant: item assignment lacks item")
			}
		case "Closed":
			if a.ClosureID == "" || a.PublicationID != "" || a.Item != nil {
				return fmt.Errorf("publication subscription invariant: closed assignment is not closure-only")
			}
		case "Interrupted":
			if a.PublicationID != "" || a.ClosureID != "" || a.Item != nil {
				return fmt.Errorf("publication subscription invariant: interrupted assignment carries data")
			}
		default:
			return fmt.Errorf("publication subscription invariant: unknown assignment kind")
		}
		assignments[a.ID] = a
	}
	for id, subscription := range r.PublicationSubscriptions {
		if subscription == nil {
			return fmt.Errorf("publication subscription invariant: nil subscription")
		}
		activation := r.Activations[subscription.RepeatActivationID]
		newOnly := subscription.SchemaVersion == PublicationNewOnlySubscriptionVersion && subscription.PublicationStartSequence != nil && *subscription.PublicationStartSequence >= 0
		retained := subscription.SchemaVersion == PublicationSubscriptionVersion && subscription.PublicationStartSequence == nil
		if id != subscription.ID || (!newOnly && !retained) || (newOnly && !isPublicationNewOnlyState(r.SchemaVersion)) || subscription.RunID != r.ID || subscription.Generation < 1 || subscription.Cursor < 0 || subscription.Status != "open" && subscription.Status != "closed" && subscription.Status != "interrupted" || activation == nil || activation.Kind != "repeat" || activation.InvocationID != subscription.InvocationID || subscription.ID != publicationSubscriptionID(r.ID, subscription.RepeatActivationID, subscription.SourceRef) {
			return fmt.Errorf("publication subscription invariant: invalid subscription")
		}
		if subscription.PendingAssignmentID != "" {
			assignment := assignments[subscription.PendingAssignmentID]
			if assignment == nil || assignment.SubscriptionID != subscription.ID || assignment.Generation != subscription.Generation || assignment.Cursor != subscription.Cursor || assignment.Status != "assigned" {
				return fmt.Errorf("publication subscription invariant: pending assignment differs")
			}
		}
	}
	return nil
}

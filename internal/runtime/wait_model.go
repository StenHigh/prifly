package runtime

import (
	"fmt"

	"github.com/stenhigh/prifly/internal/flow"
)

const (
	WaitRegistrationVersion = "1"
	EventEnvelopeVersion    = "1"
	// MaxInboxEvents is what one Run's inbox holds. An event arriving over the
	// cap is refused with a receipt rather than silently dropped or allowed to
	// grow without bound: a full inbox is a fact the sender must be told.
	MaxInboxEvents = 64
	// MaxWaitRegistrations bounds how many reservations one Run may hold at
	// once. A branch that is never taken cancels its own, so this is a ceiling
	// on outstanding promises, not on how many waits a workflow may contain.
	MaxWaitRegistrations = 64
)

// WaitRegistration is the durable promise that one exact wait may be resolved.
// It exists before the wait is entered so that a callback arriving immediately
// after an external job was started has somewhere to land. Reserving is not
// activating: a reserved registration routes nothing.
type WaitRegistration struct {
	SchemaVersion  string   `json:"schema_version"`
	ID             string   `json:"id"`
	RunID          string   `json:"run_id"`
	InvocationID   string   `json:"workflow_invocation_id"`
	TargetStageID  string   `json:"target_stage_id"`
	ActivationID   string   `json:"stage_activation_id"`
	SourceRef      flow.Ref `json:"source_ref"`
	EventSchemaRef flow.Ref `json:"event_schema_ref"`
	// Generation separates one attempt at this wait from another. An event
	// carrying an old generation is late, not a resolution.
	Generation int64 `json:"wait_generation"`
	// Nonce is issued by the authority. It correlates a signal to this exact
	// wait; it is not authentication, and knowing it does not make a sender
	// trusted. The source adapter still has to say who is speaking.
	Nonce string `json:"nonce"`
	// Status is the whole lifecycle: reserved before entry, active while the
	// wait holds the frontier, and then exactly one terminal word.
	Status    string `json:"status"`
	ExpiresAt string `json:"expires_at"`
	// PublicationStartSequence is the authority event cut before a new_only
	// source may observe a publication. Nil keeps the frozen retained contract.
	PublicationStartSequence *int64 `json:"publication_start_sequence,omitempty"`
}

// EventEnvelope is one delivered signal. The payload is an artifact of its own
// declared schema, never text handed to the next worker. Occurrence and receipt
// are different times, and only the receipt is the authority's to assign.
type EventEnvelope struct {
	SchemaVersion    string      `json:"schema_version"`
	SourceRef        flow.Ref    `json:"source_ref"`
	EventID          string      `json:"event_id"`
	EventType        string      `json:"event_type"`
	PayloadSchemaRef flow.Ref    `json:"payload_schema_ref"`
	PayloadRef       ArtifactRef `json:"payload_ref"`
	RunID            string      `json:"run_id"`
	ActivationID     string      `json:"stage_activation_id"`
	Generation       int64       `json:"wait_generation"`
	Nonce            string      `json:"nonce"`
	ReceivedAt       string      `json:"received_at"`
}

// InboxEvent is a delivered envelope together with what the authority made of
// it. A refused delivery is kept with its reason rather than discarded: "we
// never got it" and "we got it and would not use it" are different answers.
type InboxEvent struct {
	Envelope EventEnvelope `json:"envelope"`
	// RegistrationID is the reservation this event was matched to, empty when
	// it matched none.
	RegistrationID string `json:"wait_registration_id,omitempty"`
	// Disposition is one of held, consumed or refused. A held event is one that
	// arrived before its wait was entered and is waiting to be applied once.
	Disposition string      `json:"disposition"`
	Reason      string      `json:"reason,omitempty"`
	Observed    Observation `json:"observation"`
}

// WaitProgress is what one wait activation owns: the registration it holds and,
// once it has one, the single resolution. A wait resolves exactly once.
type WaitProgress struct {
	RegistrationID string `json:"wait_registration_id"`
	Generation     int64  `json:"wait_generation"`
	// Resolution is empty while waiting, then event, timeout, an interruption,
	// or a v17 terminal producer failure.
	Resolution string `json:"resolution,omitempty"`
	// EventRef is the accepted event's payload, present only for an event
	// resolution. A timeout produces no event, so it produces no artifact.
	EventRef *ArtifactRef `json:"event_ref,omitempty"`
	// PublicationAssignmentID is present only for a v14 stream delivery. The
	// assignment owns the cursor/item; this one-shot wait only points at it.
	PublicationAssignmentID string `json:"publication_assignment_id,omitempty"`
	// ResolvedAt is when the authority decided, not when the sender acted.
	ResolvedAt string `json:"resolved_at,omitempty"`
}

// Wait state carries the durable registrations and the inbox; guard state adds
// the live start/stop rules on top of everything they carried.
func isWaitState(version string) bool { return atLeast(version, CoreWaitStateVersion) }

func waitRegistrationID(runID, activationID string, generation int64) string {
	return derivedID("registration", runID, activationID, fmt.Sprint(generation))
}

// waitProgressInvariant checks the saved projection without reading artifacts.
// It never repairs a status or invents a resolution.
func (r Run) waitProgressInvariant(a *Activation) error {
	if a == nil {
		return fmt.Errorf("wait invariant: missing activation")
	}
	if a.Kind != "wait" {
		if a.Wait != nil {
			return fmt.Errorf("wait invariant: progress on another stage kind")
		}
		return nil
	}
	if !isWaitState(r.SchemaVersion) || a.Wait == nil || a.StepID != "" {
		return fmt.Errorf("wait invariant: a wait activation owns a registration")
	}
	registration := r.Waits[a.Wait.RegistrationID]
	if registration == nil || registration.ActivationID != a.ID || registration.RunID != r.ID ||
		registration.InvocationID != a.InvocationID || registration.TargetStageID != a.StageID ||
		registration.Generation != a.Wait.Generation {
		return fmt.Errorf("wait invariant: registration does not belong to this activation")
	}
	if registration.PublicationStartSequence != nil && (!isPublicationNewOnlyState(r.SchemaVersion) || *registration.PublicationStartSequence < 0) {
		return fmt.Errorf("wait invariant: unsupported publication start cut")
	}
	switch a.Wait.Resolution {
	case "":
		if registration.Status != "reserved" && registration.Status != "active" {
			return fmt.Errorf("wait invariant: unresolved wait holds a terminal registration")
		}
		if a.Wait.EventRef != nil || a.Wait.PublicationAssignmentID != "" || a.Wait.ResolvedAt != "" {
			return fmt.Errorf("wait invariant: unresolved wait carries a resolution")
		}
	case "event":
		// An event resolution has its payload. Claiming an event without one
		// would be a route taken on evidence nobody can read.
		if registration.Status != "consumed" || a.Wait.EventRef == nil || a.Wait.ResolvedAt == "" {
			return fmt.Errorf("wait invariant: event resolution has no consumed event")
		}
		if a.Wait.PublicationAssignmentID != "" {
			if !isPublicationSubscriptionState(r.SchemaVersion) {
				return fmt.Errorf("wait invariant: older state contains stream assignment")
			}
			found := false
			for i := range r.PublicationAssignments {
				assignment := &r.PublicationAssignments[i]
				found = found || assignment.ID == a.Wait.PublicationAssignmentID && assignment.WaitActivationID == a.ID && assignment.Delivery == *a.Wait.EventRef
			}
			if !found {
				return fmt.Errorf("wait invariant: stream resolution has no matching assignment")
			}
		}
	case "timeout":
		// Expiry creates no event. A timeout carrying a payload would be a
		// fabricated signal, which is exactly what expiry must never be.
		if registration.Status != "expired" || a.Wait.EventRef != nil || a.Wait.PublicationAssignmentID != "" || a.Wait.ResolvedAt == "" {
			return fmt.Errorf("wait invariant: timeout resolution invented an event")
		}
	case "producer_failed":
		if !isPublicationFailureState(r.SchemaVersion) || registration.Status != "interrupted" || a.Wait.EventRef != nil || a.Wait.PublicationAssignmentID != "" || a.Wait.ResolvedAt == "" {
			return fmt.Errorf("wait invariant: producer failure is not a v17 payload-free interruption")
		}
	case "interrupted":
		if !isPublicationSubscriptionState(r.SchemaVersion) || (registration.Status != "expired" && (!isPublicationFailureState(r.SchemaVersion) || registration.Status != "interrupted")) || a.Wait.EventRef == nil || a.Wait.PublicationAssignmentID == "" || a.Wait.ResolvedAt == "" {
			return fmt.Errorf("wait invariant: stream interruption lacks its tagged assignment")
		}
		found := false
		for i := range r.PublicationAssignments {
			assignment := &r.PublicationAssignments[i]
			found = found || assignment.ID == a.Wait.PublicationAssignmentID && assignment.Kind == "Interrupted" && assignment.WaitActivationID == a.ID && assignment.Delivery == *a.Wait.EventRef
		}
		if !found {
			return fmt.Errorf("wait invariant: stream interruption assignment is missing")
		}
	case "cancelled":
		if registration.Status != "cancelled" || a.Wait.EventRef != nil || a.Wait.PublicationAssignmentID != "" {
			return fmt.Errorf("wait invariant: cancelled wait carries a resolution")
		}
	default:
		return fmt.Errorf("wait invariant: unknown resolution %s", a.Wait.Resolution)
	}
	return nil
}

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// Approvals live in the same authority row as the objects they protect, because
// consuming one and changing the protected object must be a single transaction.
// A separate "check then admit" pair is not that, and CTRL-006 forbids treating
// it as one.
const (
	ControlApprovalPolicyVersion = "control-approval-policy/1"
	ControlOperationPolicy       = "control.policy"
	ControlOperationApprove      = "control.approve"

	MaxControlApprovals = 128
	// ponytail: one fixed maximum approval lifetime until a policy contract can
	// state its own; a shorter consume_before is always accepted.
	maxApprovalLifetime = 15 * time.Minute
)

// ControlApprovalPolicy is authority-side. The published PolicyRevision states
// approval requirements for effect classes, not for control operations, so this
// requirement is recorded here instead of being read into that contract.
type ControlApprovalPolicy struct {
	SchemaVersion string   `json:"schema_version"`
	Operations    []string `json:"operations"`
	Quorum        int      `json:"quorum"`
	Independence  string   `json:"independence"`
}

func (p ControlApprovalPolicy) requires(operation string) bool {
	for _, candidate := range p.Operations {
		if candidate == operation {
			return true
		}
	}
	return false
}

// Approval is the published contract, stored as authority state. Votes are
// separate sealed artifacts; this record references them.
type Approval struct {
	SchemaVersion         string        `json:"schema_version"`
	ID                    string        `json:"id"`
	IntentDigest          string        `json:"intent_digest"`
	RequestedBy           string        `json:"requested_by"`
	PolicyRef             flow.Ref      `json:"policy_ref"`
	Status                string        `json:"status"`
	RequiredApprovals     int           `json:"required_approvals"`
	RequiredRoles         []string      `json:"required_roles"`
	Independence          string        `json:"independence"`
	VoteRefs              []EvidenceRef `json:"vote_refs"`
	ConsumedByAdmissionID string        `json:"consumed_by_admission_id,omitempty"`
	ConsumeBefore         string        `json:"consume_before"`
	DispatchNotAfter      string        `json:"dispatch_not_after"`
	Reason                string        `json:"reason"`
}

// approvalVote is the published ApprovalVote contract. It records the actor,
// never the human: independence is decided from enrolled identities held by the
// authority, so a vote payload cannot claim to be a different person.
type approvalVote struct {
	SchemaVersion string      `json:"schema_version"`
	ID            string      `json:"id"`
	ApprovalID    string      `json:"approval_id"`
	IntentDigest  string      `json:"intent_digest"`
	ActorID       string      `json:"actor_id"`
	Decision      string      `json:"decision"`
	ProofRef      EvidenceRef `json:"proof_ref"`
	Reason        string      `json:"reason"`
}

func (c AuthorityControl) humanFor(principal string) string {
	for _, enrolled := range c.Principals {
		if enrolled.ID == principal {
			return enrolled.HumanID
		}
	}
	return ""
}

func (c AuthorityControl) approval(id string) *Approval {
	for i := range c.Approvals {
		if c.Approvals[i].ID == id {
			return &c.Approvals[i]
		}
	}
	return nil
}

// approvalPolicy returns the recorded requirement. An absent record means no
// control operation requires approval, which is what an installation that never
// selected one honestly means.
func (c AuthorityControl) approvalPolicy() ControlApprovalPolicy {
	if c.ApprovalPolicy.SchemaVersion != ControlApprovalPolicyVersion {
		return ControlApprovalPolicy{SchemaVersion: ControlApprovalPolicyVersion, Operations: []string{}, Quorum: 1, Independence: "different_from_proposer"}
	}
	return c.ApprovalPolicy
}

type ControlApprovalPolicyRequest struct {
	CommandID    string
	Operations   []string
	Quorum       int
	Independence string
	Reason       string
}

// SetControlApprovalPolicy records which control operations need approval. It
// is itself a control decision over an immutable intent, so the requirement
// cannot be lowered by an unrecorded edit.
func (e *Engine) SetControlApprovalPolicy(ctx context.Context, c ControlApprovalPolicyRequest) (local.AuthorityApplyResult, error) {
	if c.CommandID == "" || c.Reason == "" || len(c.Reason) > 4096 {
		return local.AuthorityApplyResult{}, errors.New("explicit command and reason required")
	}
	if c.Quorum < 1 || c.Quorum > 1000 || len(c.Operations) > 8 {
		return local.AuthorityApplyResult{}, errors.New("quorum must be 1..1000 over at most eight operations")
	}
	switch c.Independence {
	case "none", "different_from_proposer", "pairwise_distinct":
	default:
		return local.AuthorityApplyResult{}, errors.New("unsupported independence for a control approval policy")
	}
	for _, operation := range c.Operations {
		// Gating grant.issue is what keeps CTRL-007: a grant bounds when a
		// decision is made, it never becomes the way to skip one.
		if operation != "stop.release" && operation != "package.trust" && operation != "grant.issue" && operation != "action.admit" {
			return local.AuthorityApplyResult{}, errors.New("unsupported_operation: this installation gates stop.release, package.trust, grant.issue and action.admit")
		}
	}
	policy := ControlApprovalPolicy{SchemaVersion: ControlApprovalPolicyVersion, Operations: append([]string{}, c.Operations...), Quorum: c.Quorum, Independence: c.Independence}
	if policy.Operations == nil {
		policy.Operations = []string{}
	}
	control, _, err := e.ensureControl(ctx)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationPolicy) {
		return local.AuthorityApplyResult{}, local.Reject("object_access_denied", "the session principal cannot select a control approval policy")
	}
	_, reg, err := e.Inventory()
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	payload, err := canonical(policy)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	intent, intentBytes, expiry, err := e.controlIntentFor("policy.activate", "project", e.Config.ID, e.Config.DefaultPolicyRef, reg, c.CommandID, payload)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	command, err := canonical(map[string]any{"operation": "policy.activate", "command_id": c.CommandID, "policy": policy, "control_intent_ref": intent.Ref(), "reason": c.Reason})
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	return e.applyControl(ctx, c.CommandID, command, func(control *AuthorityControl, obs Observation) (json.RawMessage, error) {
		if err := controlIntentCurrent(intent, expiry, obs); err != nil {
			return nil, err
		}
		control.ApprovalPolicy = policy
		control.ControlEpoch++
		return e.controlAdmission(control, "project", e.Config.ID, c.CommandID, rawDigest(intentBytes), nil, obs)
	})
}

// applyControl is the shared envelope for a control-row mutation: it decodes,
// checks compatibility and writes back one canonical state.
func (e *Engine) applyControl(ctx context.Context, commandID string, payload []byte, mutate func(*AuthorityControl, Observation) (json.RawMessage, error)) (local.AuthorityApplyResult, error) {
	return e.applyControlCapacity(ctx, commandID, payload, nil, mutate)
}

// applyControlCapacity records a control decision and, when the decision is
// about admission capacity, changes that capacity in the same transaction. The
// recorded decision and the capacity it describes cannot then disagree.
func (e *Engine) applyControlCapacity(ctx context.Context, commandID string, payload []byte, capacity *int64, mutate func(*AuthorityControl, Observation) (json.RawMessage, error)) (local.AuthorityApplyResult, error) {
	return e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: commandID, Actor: e.owner, Key: AuthorityControlKey, Payload: payload}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
		control, err := decodeControl(s.Data)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		if err := e.controlCompatible(control); err != nil {
			return local.AuthorityChange{}, err
		}
		result, err := mutate(&control, e.clock.now())
		if err != nil {
			return local.AuthorityChange{}, err
		}
		data, err := canonicalState(control)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		return local.AuthorityChange{Data: data, Result: result, SetCapacity: capacity}, nil
	})
}

func (e *Engine) controlAdmission(control *AuthorityControl, scope, scopeID, commandID, intentDigest string, approvals []string, obs Observation) (json.RawMessage, error) {
	if approvals == nil {
		approvals = []string{}
	}
	admission := map[string]any{"schema_version": "1", "id": derivedID("control-admission", e.owner, commandID), "scope": scope, "scope_id": scopeID, "command_id": commandID, "intent_digest": intentDigest, "approval_refs": approvals, "control_epoch": control.ControlEpoch, "admitted_at": obs.UTC}
	encoded, err := canonical(admission)
	if err != nil {
		return nil, err
	}
	if err := flow.ValidateProtocol("ControlAdmission", encoded); err != nil {
		return nil, err
	}
	return encoded, nil
}

type ApprovalRequest struct {
	CommandID    string
	Operation    string
	IntentDigest string
	Reason       string
}

// RequestControlApproval opens an approval bound to one exact intent digest.
// The quorum and independence rule are frozen into the record at creation, so a
// later policy change cannot retroactively lower what this decision required.
func (e *Engine) RequestControlApproval(ctx context.Context, c ApprovalRequest) (local.AuthorityApplyResult, error) {
	if c.CommandID == "" || c.Reason == "" || len(c.Reason) > 4096 || c.IntentDigest == "" {
		return local.AuthorityApplyResult{}, errors.New("explicit command, intent digest and reason required")
	}
	control, _, err := e.ensureControl(ctx)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationApprove) {
		return local.AuthorityApplyResult{}, local.Reject("object_access_denied", "the session principal cannot request approvals for this project")
	}
	policy := control.approvalPolicy()
	if !policy.requires(c.Operation) {
		return local.AuthorityApplyResult{}, local.Reject("approval_not_required", "the selected control policy does not gate this operation")
	}
	command, err := canonical(map[string]any{"operation": "approval.request", "command_id": c.CommandID, "control_operation": c.Operation, "intent_digest": c.IntentDigest, "reason": c.Reason})
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	return e.applyControl(ctx, c.CommandID, command, func(control *AuthorityControl, obs Observation) (json.RawMessage, error) {
		if len(control.Approvals) >= MaxControlApprovals {
			return nil, local.Reject("approval_capacity", "resolve recorded approvals before opening another")
		}
		now, err := time.Parse(time.RFC3339Nano, obs.UTC)
		if err != nil {
			return nil, local.ErrIntegrity
		}
		frozen := control.approvalPolicy()
		approval := Approval{
			SchemaVersion: "1", ID: derivedID("approval", e.owner, c.CommandID), IntentDigest: c.IntentDigest,
			RequestedBy: e.owner, PolicyRef: control.PolicyRef, Status: "pending",
			RequiredApprovals: frozen.Quorum, RequiredRoles: []string{}, Independence: frozen.Independence,
			VoteRefs: []EvidenceRef{}, ConsumeBefore: now.Add(maxApprovalLifetime).Format(time.RFC3339Nano),
			DispatchNotAfter: now.Add(maxApprovalLifetime).Format(time.RFC3339Nano), Reason: c.Reason,
		}
		for _, existing := range control.Approvals {
			if existing.IntentDigest == approval.IntentDigest && (existing.Status == "pending" || existing.Status == "approved") {
				return nil, local.Reject("approval_present", "an open approval already covers this exact intent")
			}
		}
		encoded, err := canonical(approval)
		if err != nil {
			return nil, err
		}
		if err := flow.ValidateProtocol("Approval", encoded); err != nil {
			return nil, err
		}
		control.Approvals = append(control.Approvals, approval)
		return encoded, nil
	})
}

type ApprovalDecision struct {
	CommandID  string
	ApprovalID string
	Decision   string
	Reason     string
}

// DecideControlApproval records one vote. Independence compares human_id taken
// from the enrolled principals, never an actor string from the payload, so a
// second technical account of the same person is not a second approver.
func (e *Engine) DecideControlApproval(ctx context.Context, c ApprovalDecision) (local.AuthorityApplyResult, error) {
	if c.CommandID == "" || c.ApprovalID == "" || c.Reason == "" || len(c.Reason) > 4096 {
		return local.AuthorityApplyResult{}, errors.New("explicit command, approval and reason required")
	}
	if c.Decision != "approve" && c.Decision != "reject" {
		return local.AuthorityApplyResult{}, errors.New("a vote is approve or reject")
	}
	control, _, err := e.ensureControl(ctx)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationApprove) {
		return local.AuthorityApplyResult{}, local.Reject("object_access_denied", "the session principal cannot vote on approvals for this project")
	}
	existing := control.approval(c.ApprovalID)
	if existing == nil {
		return local.AuthorityApplyResult{}, local.Reject("not_found", "no approval with this identity")
	}
	voteID := derivedID("vote", e.owner, c.CommandID)
	vote := approvalVote{
		SchemaVersion: "1", ID: voteID, ApprovalID: c.ApprovalID, IntentDigest: existing.IntentDigest,
		ActorID: e.owner, Decision: c.Decision, ProofRef: EvidenceRef{ID: voteID, Digest: rawDigest([]byte(voteID))},
		Reason: c.Reason,
	}
	// The vote is sealed under an identity derived from the command, so its
	// bytes must be derived from the command too. When the vote was stamped
	// with the clock, retrying one command produced different bytes under the
	// same artifact identity. The commit time is recorded by the control
	// journal entry this vote is attached to.
	encoded, err := canonical(vote)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	if err := flow.ValidateProtocol("ApprovalVote", encoded); err != nil {
		return local.AuthorityApplyResult{}, err
	}
	_, reg, err := e.Inventory()
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	artifact, err := e.putArtifact(encoded, "blob", nil, derivedID("artifact", voteID), map[string]any{"kind": "authority", "authority_id": e.Installation.ID, "command_id": c.CommandID, "port": "approval_vote"}, nil, reg)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	// Who already voted is read here, not from inside the decision: a transform
	// compares and mutates, it does not go to storage. A vote recorded after
	// this read is caught in the transform, which refuses to decide on a list
	// it cannot account for.
	voters := map[string]string{}
	for _, recorded := range existing.VoteRefs {
		cast, err := e.recordedVote(recorded)
		if err != nil {
			return local.AuthorityApplyResult{}, err
		}
		voters[recorded.ID] = cast.ActorID
	}
	command, err := canonical(map[string]any{"operation": "approval.decide", "command_id": c.CommandID, "approval_id": c.ApprovalID, "decision": c.Decision, "vote_ref": artifact.Ref(), "reason": c.Reason})
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	return e.applyControl(ctx, c.CommandID, command, func(control *AuthorityControl, obs Observation) (json.RawMessage, error) {
		approval := control.approval(c.ApprovalID)
		if approval == nil {
			return nil, local.Reject("not_found", "no approval with this identity")
		}
		expired, err := approvalExpired(*approval, obs)
		if err != nil {
			return nil, err
		}
		if expired {
			return nil, local.Reject("approval_expired", "the consume window of this decision has closed")
		}
		if approval.Status != "pending" {
			return nil, local.Reject("approval_terminal", "this approval no longer accepts votes")
		}
		human := control.humanFor(e.owner)
		if human == "" {
			return nil, local.Reject("unknown_principal", "the voter is not an enrolled principal")
		}
		if approval.Independence != "none" && human == control.humanFor(approval.RequestedBy) {
			return nil, local.Reject("independence_violated", "the requester of this decision cannot also approve it")
		}
		for _, recorded := range approval.VoteRefs {
			actor, read := voters[recorded.ID]
			if !read {
				return nil, local.Reject("approval_changed", "a vote was recorded after this decision was prepared")
			}
			if control.humanFor(actor) == human {
				return nil, local.Reject("duplicate_vote", "this human has already voted on this approval")
			}
		}
		approval.VoteRefs = append(approval.VoteRefs, EvidenceRef{ID: artifact.ID, Digest: artifact.Digest})
		if c.Decision == "reject" {
			approval.Status = "rejected"
		} else if len(approval.VoteRefs) >= approval.RequiredApprovals {
			approval.Status = "approved"
		}
		encoded, err := canonical(approval)
		if err != nil {
			return nil, err
		}
		if err := flow.ValidateProtocol("Approval", encoded); err != nil {
			return nil, err
		}
		return encoded, nil
	})
}

// recordedVote reads a sealed vote back. A vote reference that no longer
// resolves is an integrity failure, not an absent vote.
func (e *Engine) recordedVote(ref EvidenceRef) (approvalVote, error) {
	_, data, err := e.Artifact(ArtifactRef{ArtifactID: ref.ID, Revision: 1, Digest: ref.Digest})
	if err != nil {
		return approvalVote{}, err
	}
	var vote approvalVote
	if err := decode(data, &vote); err != nil {
		return approvalVote{}, err
	}
	return vote, nil
}

// approvalExpired reports whether an open decision has passed its consume
// window. Expiry is evaluated at every decision point and reported by the view;
// it is never a stored approval, because time alone approves nothing and a
// refused command writes no state.
func approvalExpired(approval Approval, obs Observation) (bool, error) {
	if approval.Status != "pending" && approval.Status != "approved" {
		return false, nil
	}
	due, err := time.Parse(time.RFC3339Nano, approval.ConsumeBefore)
	now, nowErr := time.Parse(time.RFC3339Nano, obs.UTC)
	if err != nil || nowErr != nil {
		return false, local.ErrIntegrity
	}
	return !now.Before(due), nil
}

// effectiveStatus is what the decision means now, including a window that has
// closed since it was written.
func effectiveStatus(approval Approval, obs Observation) string {
	if expired, err := approvalExpired(approval, obs); err == nil && expired {
		return "expired"
	}
	return approval.Status
}

type ApprovalRevoke struct {
	CommandID  string
	ApprovalID string
	Reason     string
}

// RevokeControlApproval ends an approval before it is consumed. A consumed
// approval is not rewritten: history keeps what was actually admitted.
func (e *Engine) RevokeControlApproval(ctx context.Context, c ApprovalRevoke) (local.AuthorityApplyResult, error) {
	if c.CommandID == "" || c.ApprovalID == "" || c.Reason == "" || len(c.Reason) > 4096 {
		return local.AuthorityApplyResult{}, errors.New("explicit command, approval and reason required")
	}
	control, _, err := e.ensureControl(ctx)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationApprove) {
		return local.AuthorityApplyResult{}, local.Reject("object_access_denied", "the session principal cannot revoke approvals for this project")
	}
	command, err := canonical(map[string]any{"operation": "approval.revoke", "command_id": c.CommandID, "approval_id": c.ApprovalID, "reason": c.Reason})
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	return e.applyControl(ctx, c.CommandID, command, func(control *AuthorityControl, obs Observation) (json.RawMessage, error) {
		approval := control.approval(c.ApprovalID)
		if approval == nil {
			return nil, local.Reject("not_found", "no approval with this identity")
		}
		if approval.Status == "consumed" {
			return nil, local.Reject("approval_consumed", "a consumed approval is terminal; its admission already happened")
		}
		if approval.Status != "pending" && approval.Status != "approved" {
			return nil, local.Reject("approval_terminal", "this approval is already resolved")
		}
		approval.Status = "revoked"
		encoded, err := canonical(approval)
		return encoded, err
	})
}

// consumeApprovals turns approved decisions into one admitted control change.
// Consumption happens inside the same transform as the protected mutation, so a
// decision cannot be spent twice or spent after it stopped being valid.
func consumeApprovals(control *AuthorityControl, operation, intentDigest, admissionID string, requested []string, obs Observation) ([]string, error) {
	policy := control.approvalPolicy()
	if !policy.requires(operation) {
		if len(requested) != 0 {
			return nil, local.Reject("approval_not_required", "the selected control policy does not gate this operation")
		}
		return []string{}, nil
	}
	if len(requested) == 0 {
		return nil, local.Reject("approval_required", "the selected control policy requires an approved decision for this operation")
	}
	consumed := []string{}
	seen := map[string]bool{}
	for _, id := range requested {
		if seen[id] {
			return nil, local.Reject("duplicate_approval", "the same approval was named twice")
		}
		seen[id] = true
		approval := control.approval(id)
		if approval == nil {
			return nil, local.Reject("not_found", "no approval with this identity")
		}
		expired, err := approvalExpired(*approval, obs)
		if err != nil {
			return nil, err
		}
		if expired {
			return nil, local.Reject("approval_expired", "the consume window of this decision has closed")
		}
		if approval.Status != "approved" {
			return nil, local.Reject("approval_not_admissible", "this approval is not an approved decision at consume time")
		}
		if approval.IntentDigest != intentDigest {
			return nil, local.Reject("approval_intent_conflict", "this approval covers a different protected payload")
		}
		approval.Status, approval.ConsumedByAdmissionID = "consumed", admissionID
		consumed = append(consumed, approval.ID)
	}
	return consumed, nil
}

// ControlApprovalView is the reference CLI projection of open decisions. It
// reports the effective status, so a closed window is visible without a stored
// transition nobody committed.
func (e *Engine) ControlApprovalView(control AuthorityControl) map[string]any {
	obs := e.clock.now()
	approvals := []map[string]any{}
	for _, approval := range control.Approvals {
		approvals = append(approvals, map[string]any{"approval": approval, "effective_status": effectiveStatus(approval, obs)})
	}
	return map[string]any{"schema_version": "foundation-approvals/1", "policy": control.approvalPolicy(), "approvals": approvals}
}

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// The authority control plane is one state row beside the Runs. An installation
// or project stop is a control object, so releasing it never invents a Run,
// StepInstance or worker. Approvals, grants and waivers are later P2-06 slices;
// this row deliberately holds only the single-owner facts the pilot needs.
const (
	AuthorityControlKey     = "control"
	AuthorityControlVersion = "authority-control/1"

	ControlOperationStop    = "control.stop"
	ControlOperationRelease = "control.release"
	ControlOperationAdmit   = "run.admit"
	ControlOperationResolve = "run.resolve"
	ControlOperationTrust   = "package.trust"
	ControlOperationRead    = "object.read"

	MaxAuthorityStops = 128
)

type AuthorityControl struct {
	SchemaVersion string               `json:"schema_version"`
	AuthorityID   string               `json:"authority_id"`
	ControlEpoch  int64                `json:"control_epoch"`
	PolicyRef     flow.Ref             `json:"policy_ref"`
	Principals    []AuthorityPrincipal `json:"principals"`
	Access        []AuthorityAccess    `json:"object_access"`
	Stops         []AuthorityStop      `json:"stops"`
	// A row written before control approvals existed simply carries neither
	// field, which honestly means no operation is gated and none is open.
	ApprovalPolicy ControlApprovalPolicy `json:"approval_policy,omitempty"`
	Approvals      []Approval            `json:"approvals,omitempty"`
	Grants         []ControlGrant        `json:"grants,omitempty"`
	TrustRoots     []TrustRoot           `json:"trust_roots,omitempty"`
}

// HumanID is what separation of duties compares. A second technical account of
// the same person keeps this value, so it cannot become a second approver.
type AuthorityPrincipal struct {
	ID       string      `json:"id"`
	HumanID  string      `json:"human_id"`
	Kind     string      `json:"kind"`
	OwnerUID int         `json:"owner_uid"`
	Enrolled Observation `json:"enrolled"`
}

type AuthorityAccess struct {
	PrincipalID string   `json:"principal_id"`
	Scope       string   `json:"scope"`
	ScopeID     string   `json:"scope_id"`
	Operations  []string `json:"operations"`
}

type AuthorityStop struct {
	ID         string       `json:"id"`
	Scope      string       `json:"scope"`
	ScopeID    string       `json:"scope_id"`
	Generation int64        `json:"generation"`
	Epoch      int64        `json:"control_epoch"`
	Actor      string       `json:"actor_id"`
	Reason     string       `json:"reason"`
	Status     string       `json:"status"`
	Created    Observation  `json:"created"`
	Released   *Observation `json:"released,omitempty"`
}

func (s AuthorityStop) record() map[string]any {
	return map[string]any{"schema_version": "1", "id": s.ID, "scope": s.Scope, "scope_id": s.ScopeID, "generation": s.Generation, "control_epoch": s.Epoch, "actor_id": s.Actor, "reason": s.Reason, "status": s.Status, "created_at": s.Created.UTC}
}

func (c AuthorityControl) allows(principal, scope, scopeID, operation string) bool {
	for _, a := range c.Access {
		if a.PrincipalID != principal || a.Scope != scope || a.ScopeID != scopeID {
			continue
		}
		for _, granted := range a.Operations {
			if granted == operation {
				return true
			}
		}
	}
	return false
}

// blockingStop reports the active control stop covering this project. An
// installation stop covers every project; a project stop covers only its own.
func (c AuthorityControl) blockingStop(installationID, projectID string) *AuthorityStop {
	for i := range c.Stops {
		stop := &c.Stops[i]
		if stop.Status != "active" {
			continue
		}
		if stop.Scope == "installation" && stop.ScopeID == installationID || stop.Scope == "project" && stop.ScopeID == projectID {
			return stop
		}
	}
	return nil
}

// The owner operation set is defined by core, not by a command payload. A
// control plane enrolled by an older build is reconciled for the same
// authenticated owner instead of leaving the installation unable to proceed.
func ownerOperations() []string {
	return []string{ControlOperationAdmit, ControlOperationApprove, ControlOperationGrant, ControlOperationPolicy, ControlOperationRead, ControlOperationRelease, ControlOperationResolve, ControlOperationStop, ControlOperationTrust}
}

func (e *Engine) controlScope(scope string) (string, error) {
	switch scope {
	case "installation":
		return e.Installation.ID, nil
	case "project":
		return e.Config.ID, nil
	}
	return "", errors.New("unsupported_scope: authority control targets installation or project")
}

func approvalRefs(ids []string) []any {
	refs := []any{}
	for _, id := range ids {
		refs = append(refs, id)
	}
	return refs
}

func decodeControl(data []byte) (AuthorityControl, error) {
	var control AuthorityControl
	if err := decode(data, &control); err != nil {
		return AuthorityControl{}, err
	}
	return control, nil
}

// A different selected policy is a policy.activate decision, not a silent
// re-pin: the enrolled control plane keeps the policy it was admitted under.
func (e *Engine) controlCompatible(control AuthorityControl) error {
	if control.SchemaVersion != AuthorityControlVersion || control.AuthorityID != e.Installation.ID {
		return errors.New("unsupported or foreign authority control state")
	}
	if control.PolicyRef != e.Config.DefaultPolicyRef {
		return local.Reject("policy_conflict", "the project selects a policy the enrolled control plane was not admitted under")
	}
	return nil
}

// Control reads the current control plane without enrolling it. A read-only
// installation that has never written one honestly reports an absent plane.
func (e *Engine) Control(ctx context.Context) (AuthorityControl, int64, error) {
	snapshot, err := e.Store.ReadAuthority(ctx, AuthorityControlKey)
	if errors.Is(err, local.ErrNotFound) {
		return AuthorityControl{}, 0, nil
	}
	if err != nil {
		return AuthorityControl{}, 0, err
	}
	control, err := decodeControl(snapshot.Data)
	if err != nil {
		return AuthorityControl{}, 0, err
	}
	return control, snapshot.Version, e.controlCompatible(control)
}

// ensureControl enrols the local session principal from the authenticated OS
// owner. The identity comes from the process, never from a command payload, so
// no --actor flag or JSON field can select a different principal.
func (e *Engine) ensureControl(ctx context.Context) (AuthorityControl, int64, error) {
	control, version, err := e.Control(ctx)
	if err != nil {
		return control, version, err
	}
	if version > 0 {
		return e.reconcileOwnerOperations(ctx, control, version)
	}
	if e.ReadOnly {
		return AuthorityControl{}, 0, local.ErrReadOnly
	}
	principal := AuthorityPrincipal{ID: e.owner, HumanID: "local:human:owner", Kind: "human", OwnerUID: e.Installation.OwnerUID}
	operations := ownerOperations()
	payload, err := canonical(map[string]any{"operation": "control.enrol", "authority_id": e.Installation.ID, "project_id": e.Config.ID, "principal_id": principal.ID, "human_id": principal.HumanID, "owner_uid": principal.OwnerUID, "policy_ref": e.Config.DefaultPolicyRef, "operations": operations})
	if err != nil {
		return AuthorityControl{}, 0, err
	}
	zero := int64(0)
	// The command identity is derived, so a concurrent second enrolment returns
	// the committed receipt instead of a second owner.
	_, err = e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: derivedID("command", e.Installation.ID, "control-enrolment"), Actor: e.owner, Key: AuthorityControlKey, Payload: payload, ExpectedVersion: &zero}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
		principal.Enrolled = e.clock.now()
		enrolled := AuthorityControl{
			SchemaVersion: AuthorityControlVersion, AuthorityID: e.Installation.ID, ControlEpoch: 0, PolicyRef: e.Config.DefaultPolicyRef,
			Principals: []AuthorityPrincipal{principal},
			Access: []AuthorityAccess{
				{PrincipalID: principal.ID, Scope: "installation", ScopeID: e.Installation.ID, Operations: operations},
				{PrincipalID: principal.ID, Scope: "project", ScopeID: e.Config.ID, Operations: operations},
			},
			Stops: []AuthorityStop{},
		}
		data, err := canonicalState(enrolled)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		return local.AuthorityChange{Data: data, Result: json.RawMessage(`{"enrolled":true}`)}, nil
	})
	if err != nil {
		return AuthorityControl{}, 0, err
	}
	return e.Control(ctx)
}

// reconcileOwnerOperations records the core-defined owner operation set when a
// control plane enrolled by an earlier build lacks one. It widens nothing that
// core does not define and only for the exact enrolled owner principal, so an
// old installation upgrades instead of becoming unusable.
func (e *Engine) reconcileOwnerOperations(ctx context.Context, control AuthorityControl, version int64) (AuthorityControl, int64, error) {
	wanted, missing := ownerOperations(), false
	for _, operation := range wanted {
		missing = missing || !control.allows(e.owner, "installation", e.Installation.ID, operation) || !control.allows(e.owner, "project", e.Config.ID, operation)
	}
	if !missing {
		return control, version, nil
	}
	owner := false
	for _, principal := range control.Principals {
		owner = owner || principal.ID == e.owner && principal.OwnerUID == e.Installation.OwnerUID
	}
	if !owner {
		return control, version, local.Reject("object_access_denied", "only the enrolled owner principal receives the core operation set")
	}
	if e.ReadOnly {
		return control, version, local.ErrReadOnly
	}
	payload, err := canonical(map[string]any{"operation": "control.reconcile_owner_operations", "principal_id": e.owner, "operations": wanted})
	if err != nil {
		return control, version, err
	}
	if _, err := e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: derivedID("command", e.Installation.ID, "control-owner-operations", strings.Join(wanted, ",")), Actor: e.owner, Key: AuthorityControlKey, Payload: payload}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
		current, err := decodeControl(s.Data)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		if err := e.controlCompatible(current); err != nil {
			return local.AuthorityChange{}, err
		}
		for _, scope := range []struct{ name, id string }{{"installation", e.Installation.ID}, {"project", e.Config.ID}} {
			found := false
			for i := range current.Access {
				access := &current.Access[i]
				if access.PrincipalID != e.owner || access.Scope != scope.name || access.ScopeID != scope.id {
					continue
				}
				access.Operations, found = wanted, true
			}
			if !found {
				current.Access = append(current.Access, AuthorityAccess{PrincipalID: e.owner, Scope: scope.name, ScopeID: scope.id, Operations: wanted})
			}
		}
		data, err := canonicalState(current)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		return local.AuthorityChange{Data: data, Result: json.RawMessage(`{"reconciled":true}`)}, nil
	}); err != nil {
		return control, version, err
	}
	return e.Control(ctx)
}

// readAccess is evaluated before any record is selected, so a reader never
// counts what it may not see. It never enrols: reading must work on a read-only
// installation. An installation that never recorded a control plane grants its
// verified OS owner read access, which is exactly what opening it proved; once
// a plane exists, the recorded object access decides.
func (e *Engine) readAccess(ctx context.Context) (string, error) {
	control, version, err := e.Control(ctx)
	if err != nil {
		return "", err
	}
	if version == 0 {
		return "local-owner/1", nil
	}
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationRead) {
		return "", local.Reject("object_access_denied", "the session principal cannot read this project's records")
	}
	return "local-owner/1", nil
}

// admissionGate evaluates the control plane once, outside the run transaction,
// and returns both the pin that keeps that evaluation current at commit and the
// rejection the caller's transform must return. A rejection is a recorded
// control decision, not a transport failure.
func (e *Engine) admissionGate(ctx context.Context) (*local.ControlPin, error, error) {
	control, version, err := e.ensureControl(ctx)
	if err != nil {
		var rejection *local.Rejection
		if errors.As(err, &rejection) {
			return nil, err, nil
		}
		return nil, nil, err
	}
	pin := &local.ControlPin{Key: AuthorityControlKey, Version: version}
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationAdmit) {
		return pin, local.Reject("object_access_denied", "the session principal has no admission access to this project"), nil
	}
	if stop := control.blockingStop(e.Installation.ID, e.Config.ID); stop != nil {
		return pin, local.Reject("control_stop_active", "an active "+stop.Scope+" stop forbids new admissions"), nil
	}
	return pin, nil, nil
}

type ControlRestrictRequest struct {
	CommandID string
	Scope     string
	Reason    string
}

// RestrictControl records an installation or project stop in the control plane.
// It forbids new ordinary admissions from their next current check; it does not
// cancel already dispatched work and does not touch any Run snapshot.
func (e *Engine) RestrictControl(ctx context.Context, c ControlRestrictRequest) (local.AuthorityApplyResult, error) {
	scopeID, err := e.controlScope(c.Scope)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	if c.CommandID == "" || c.Reason == "" || len(c.Reason) > 4096 {
		return local.AuthorityApplyResult{}, errors.New("explicit command and reason required")
	}
	command := map[string]any{"schema_version": "1", "command_id": c.CommandID, "scope": c.Scope, "scope_id": scopeID, "kind": "pause", "reason": c.Reason}
	b, err := canonical(command)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	if err := flow.ValidateProtocol("RestrictCommand", b); err != nil {
		return local.AuthorityApplyResult{}, err
	}
	if _, _, err := e.ensureControl(ctx); err != nil {
		return local.AuthorityApplyResult{}, err
	}
	return e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: c.CommandID, Actor: e.owner, Key: AuthorityControlKey, Payload: b}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
		control, err := decodeControl(s.Data)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		if err := e.controlCompatible(control); err != nil {
			return local.AuthorityChange{}, err
		}
		if !control.allows(e.owner, c.Scope, scopeID, ControlOperationStop) {
			return local.AuthorityChange{}, local.Reject("object_access_denied", "the session principal cannot stop this object")
		}
		if len(control.Stops) >= MaxAuthorityStops {
			return local.AuthorityChange{}, local.Reject("stop_capacity", "release existing control stops before recording another")
		}
		obs := e.clock.now()
		control.ControlEpoch++
		stop := AuthorityStop{ID: derivedID("stop", e.owner, c.CommandID), Scope: c.Scope, ScopeID: scopeID, Generation: 1, Epoch: control.ControlEpoch, Actor: e.owner, Reason: c.Reason, Status: "active", Created: obs}
		record, err := canonical(stop.record())
		if err != nil {
			return local.AuthorityChange{}, err
		}
		if err := flow.ValidateProtocol("StopRecord", record); err != nil {
			return local.AuthorityChange{}, err
		}
		control.Stops = append(control.Stops, stop)
		data, err := canonicalState(control)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		return local.AuthorityChange{Data: data, Result: record}, nil
	})
}

type ControlReleaseRequest struct {
	CommandID            string
	Scope                string
	ExpectedControlEpoch int64
	Stops                []StopGeneration
	Approvals            []string
	GrantID              string
	Reason               string
}

// ReleaseControl consumes an immutable ControlIntent and commits a
// ControlAdmission in the same transaction as the released stops. A stale
// expected epoch cannot remove a later restriction. The empty approval list is
// admitted only because Open pins a local policy that requires no human
// approval for control; quorum arrives with the approvals slice of P2-06.
func (e *Engine) ReleaseControl(ctx context.Context, c ControlReleaseRequest) (local.AuthorityApplyResult, error) {
	scopeID, err := e.controlScope(c.Scope)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	if c.CommandID == "" || c.Reason == "" || len(c.Reason) > 4096 || len(c.Stops) == 0 || len(c.Stops) > MaxAuthorityStops {
		return local.AuthorityApplyResult{}, errors.New("explicit command, reason and exact stop generations required")
	}
	if _, _, err := e.ensureControl(ctx); err != nil {
		return local.AuthorityApplyResult{}, err
	}
	_, reg, err := e.Inventory()
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	intentPayload, err := canonical(map[string]any{"scope": c.Scope, "scope_id": scopeID, "expected_control_epoch": c.ExpectedControlEpoch, "stop_generations": c.Stops, "reason": c.Reason})
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	artifact, ib, expiry, err := e.controlIntentFor("stop.release", c.Scope, scopeID, e.Config.DefaultPolicyRef, reg, c.CommandID, intentPayload)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	command := map[string]any{"schema_version": "1", "command_id": c.CommandID, "scope": c.Scope, "scope_id": scopeID, "expected_control_epoch": c.ExpectedControlEpoch, "stop_generations": c.Stops, "control_intent_ref": artifact.Ref(), "approval_refs": approvalRefs(c.Approvals), "reason": c.Reason}
	cb, err := canonical(command)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	if err := flow.ValidateProtocol("ReleaseStopCommand", cb); err != nil {
		return local.AuthorityApplyResult{}, err
	}
	return e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: c.CommandID, Actor: e.owner, Key: AuthorityControlKey, Payload: cb}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
		control, err := decodeControl(s.Data)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		if err := e.controlCompatible(control); err != nil {
			return local.AuthorityChange{}, err
		}
		if !control.allows(e.owner, c.Scope, scopeID, ControlOperationRelease) {
			return local.AuthorityChange{}, local.Reject("object_access_denied", "the session principal cannot release this object")
		}
		obs := e.clock.now()
		if err := controlIntentCurrent(artifact, expiry, obs); err != nil {
			return local.AuthorityChange{}, err
		}
		if control.ControlEpoch != c.ExpectedControlEpoch {
			return local.AuthorityChange{}, local.Reject("control_epoch_conflict", "a newer restriction exists")
		}
		seen := map[string]bool{}
		for _, requested := range c.Stops {
			if seen[requested.ID] {
				return local.AuthorityChange{}, local.Reject("duplicate_stop", "duplicate stop reference")
			}
			seen[requested.ID] = true
			found := false
			for i := range control.Stops {
				stop := &control.Stops[i]
				if stop.ID == requested.ID && stop.Generation == requested.Generation && stop.Status == "active" && stop.Scope == c.Scope && stop.ScopeID == scopeID {
					stop.Status, stop.Released = "released", &obs
					found = true
					break
				}
			}
			if !found {
				return local.AuthorityChange{}, local.Reject("stop_generation_conflict", "stop is absent, changed, out of scope or already released")
			}
		}
		control.ControlEpoch++
		admissionID := derivedID("control-admission", e.owner, c.CommandID)
		// A decision is bound to the protected payload, not to the intent
		// document: the document carries an expiry, so its digest is not
		// something a requester could name before the release is built.
		var consumed []string
		if c.GrantID != "" {
			if len(c.Approvals) != 0 {
				return local.AuthorityChange{}, local.Reject("decision_conflict", "a release presents either approvals or one grant, not both")
			}
			if err := consumeGrant(&control, e.owner, "stop.release", rawDigest(intentPayload), admissionID, c.GrantID, nil, obs); err != nil {
				return local.AuthorityChange{}, err
			}
			consumed = []string{}
		} else if consumed, err = consumeApprovals(&control, "stop.release", rawDigest(intentPayload), admissionID, c.Approvals, obs); err != nil {
			return local.AuthorityChange{}, err
		}
		ab, err := e.controlAdmission(&control, c.Scope, scopeID, c.CommandID, rawDigest(ib), consumed, obs)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		data, err := canonicalState(control)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		return local.AuthorityChange{Data: data, Result: ab}, nil
	})
}

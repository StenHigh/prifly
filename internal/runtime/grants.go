package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// A Grant is a decision made once for a bounded set of later uses. It does not
// cancel a required approval: issuing it is itself a gated control operation,
// so the human decision moves to issuance and each use only draws the bound
// down. CTRL-007 forbids a grant widening itself, so its constraints are sealed
// and only the accounting is mutable.
const (
	ControlOperationGrant = "control.grant"
	// ponytail: one fixed maximum grant lifetime until a policy contract can
	// state its own; a shorter expiry is always accepted.
	maxGrantLifetime   = 24 * time.Hour
	MaxControlGrants   = 64
	maxGrantOperations = 1000
)

// ControlGrant pairs the published Grant with the authority-side accounting the
// contract deliberately does not carry: a counter is not part of the decision.
type ControlGrant struct {
	Grant     Grant        `json:"grant"`
	UsedCount int64        `json:"used_logical_operations"`
	Issued    Observation  `json:"issued"`
	Revoked   *Observation `json:"revoked,omitempty"`
	Uses      []GrantUse   `json:"uses"`
}

type GrantUse struct {
	AdmissionID       string      `json:"control_admission_id,omitempty"`
	ActionAdmissionID string      `json:"action_admission_id,omitempty"`
	Operation         string      `json:"operation"`
	IntentDigest      string      `json:"intent_digest"`
	Observed          Observation `json:"observed"`
}

type Grant struct {
	SchemaVersion             string             `json:"schema_version"`
	ID                        string             `json:"id"`
	IssuerID                  string             `json:"issuer_id"`
	SubjectID                 string             `json:"subject_id"`
	Capabilities              []string           `json:"capabilities"`
	ResourceScopes            []ResourceIdentity `json:"resource_scopes"`
	ConstraintsRef            flow.Ref           `json:"constraints_ref"`
	ExpiresAt                 string             `json:"expires_at"`
	MaxLogicalOperations      int64              `json:"max_logical_operations"`
	MaxDeliveriesPerOperation int64              `json:"max_deliveries_per_operation"`
	AllowUnattended           bool               `json:"allow_unattended"`
	Status                    string             `json:"status"`
}

type ResourceIdentity struct {
	ProviderRef flow.Ref `json:"provider_ref"`
	CanonicalID string   `json:"canonical_id"`
	Scope       string   `json:"scope"`
}

type grantConstraints struct {
	SchemaVersion         string             `json:"schema_version"`
	ID                    string             `json:"id"`
	Version               string             `json:"version"`
	ProjectIDs            []string           `json:"project_ids"`
	AllowedDataClasses    []string           `json:"allowed_data_classes"`
	AllowedDestinations   []ResourceIdentity `json:"allowed_destinations"`
	MaxParallelOperations int64              `json:"max_parallel_operations"`
	BudgetLimits          []grantBudget      `json:"budget_limits"`
}

type grantBudget struct {
	Unit        string `json:"unit"`
	Amount      string `json:"amount"`
	Enforcement string `json:"enforcement"`
}

// controlResource names the authority's own control plane as the provider of a
// control object. Without it a control ResourceIdentity would have to borrow an
// adapter that does not own the object.
func (e *Engine) controlResource(scope, scopeID string) ResourceIdentity {
	return ResourceIdentity{ProviderRef: builtinRef(builtinDefinitionsOrPanic(), "core:resource/authority-control"), CanonicalID: scopeID, Scope: scope}
}

func builtinDefinitionsOrPanic() []PinnedDefinition {
	defs, _, err := Builtins()
	if err != nil {
		panic(err)
	}
	return defs
}

func (c AuthorityControl) grant(id string) *ControlGrant {
	for i := range c.Grants {
		if c.Grants[i].Grant.ID == id {
			return &c.Grants[i]
		}
	}
	return nil
}

type ControlGrantRequest struct {
	CommandID      string
	SubjectID      string
	Capabilities   []string
	ResourceScopes []ResourceIdentity
	MaxOperations  int64
	LifetimeMS     int64
	Approvals      []string
	Reason         string
}

// IssueControlGrant records delegated authority for bounded later use. Issuing
// is itself gated by the control approval policy, so a grant cannot be the way
// its own subject escapes a required decision.
func (e *Engine) IssueControlGrant(ctx context.Context, c ControlGrantRequest) (local.AuthorityApplyResult, error) {
	if c.CommandID == "" || c.Reason == "" || len(c.Reason) > 4096 || c.SubjectID == "" {
		return local.AuthorityApplyResult{}, errors.New("explicit command, subject and reason required")
	}
	if len(c.Capabilities) == 0 || len(c.Capabilities) > 8 {
		return local.AuthorityApplyResult{}, errors.New("a grant names 1..8 control operations")
	}
	for _, capability := range c.Capabilities {
		if capability != "stop.release" && capability != "package.trust" && capability != "action.admit" {
			return local.AuthorityApplyResult{}, errors.New("unsupported_operation: this installation delegates stop.release, package.trust and action.admit")
		}
	}
	if contains(c.Capabilities, "action.admit") {
		if len(c.Capabilities) != 1 || len(c.ResourceScopes) == 0 {
			return local.AuthorityApplyResult{}, errors.New("an action.admit grant names one capability and its exact resource scopes")
		}
		for _, resource := range c.ResourceScopes {
			data, err := canonical(resource)
			if err != nil || flow.ValidateProtocol("ResourceIdentity", data) != nil {
				return local.AuthorityApplyResult{}, errors.New("an action.admit grant requires closed resource identities")
			}
		}
	} else if len(c.ResourceScopes) != 0 {
		return local.AuthorityApplyResult{}, errors.New("control grants derive their authority resource scope")
	}
	if c.MaxOperations < 1 || c.MaxOperations > maxGrantOperations {
		return local.AuthorityApplyResult{}, errors.New("a grant bounds 1..1000 logical operations")
	}
	lifetime := time.Duration(c.LifetimeMS) * time.Millisecond
	if lifetime <= 0 || lifetime > maxGrantLifetime {
		return local.AuthorityApplyResult{}, errors.New("a grant lifetime is bounded and explicit")
	}
	control, _, err := e.ensureControl(ctx)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationGrant) {
		return local.AuthorityApplyResult{}, local.Reject("object_access_denied", "the session principal cannot issue control grants")
	}
	// A grant may not deliver rights its subject does not already hold: it
	// bounds when a decision is made, never who may hold the object.
	for _, capability := range c.Capabilities {
		operation := ControlOperationRelease
		if capability == "package.trust" {
			operation = ControlOperationTrust
		} else if capability == "action.admit" {
			operation = ControlOperationAdmit
		}
		if !control.allows(c.SubjectID, "project", e.Config.ID, operation) {
			return local.AuthorityApplyResult{}, local.Reject("object_access_denied", "the subject has no current access to "+capability)
		}
	}
	_, reg, err := e.Inventory()
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	constraints := grantConstraints{
		SchemaVersion: "1", ID: derivedID("constraints", e.owner, c.CommandID), Version: "1.0.0",
		ProjectIDs: []string{e.Config.ID}, AllowedDataClasses: []string{"restricted"},
		AllowedDestinations: append([]ResourceIdentity{}, c.ResourceScopes...), MaxParallelOperations: 1,
		BudgetLimits: []grantBudget{{Unit: "requests", Amount: itoa(c.MaxOperations), Enforcement: "hard"}},
	}
	constraintBytes, err := canonical(constraints)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	if err := flow.ValidateProtocol("GrantConstraints", constraintBytes); err != nil {
		return local.AuthorityApplyResult{}, err
	}
	sealed, err := e.putArtifact(constraintBytes, "blob", nil, derivedID("artifact", constraints.ID), map[string]any{"kind": "authority", "authority_id": e.Installation.ID, "command_id": c.CommandID, "port": "grant_constraints"}, nil, reg)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	constraintsRef := flow.Ref{ID: constraints.ID, Version: constraints.Version, Digest: sealed.Digest}
	payload, err := canonical(map[string]any{"subject_id": c.SubjectID, "capabilities": c.Capabilities, "max_logical_operations": c.MaxOperations, "lifetime_ms": c.LifetimeMS, "constraints_ref": constraintsRef, "reason": c.Reason})
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	intent, intentBytes, expiry, err := e.controlIntentFor("grant.issue", "project", e.Config.ID, e.Config.DefaultPolicyRef, reg, c.CommandID, payload)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	_ = intentBytes
	command, err := canonical(map[string]any{"operation": "grant.issue", "command_id": c.CommandID, "subject_id": c.SubjectID, "capabilities": c.Capabilities, "control_intent_ref": intent.Ref(), "approval_refs": approvalRefs(c.Approvals), "reason": c.Reason})
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	return e.applyControl(ctx, c.CommandID, command, func(control *AuthorityControl, obs Observation) (json.RawMessage, error) {
		if err := controlIntentCurrent(intent, expiry, obs); err != nil {
			return nil, err
		}
		if len(control.Grants) >= MaxControlGrants {
			return nil, local.Reject("grant_capacity", "revoke recorded grants before issuing another")
		}
		now, err := time.Parse(time.RFC3339Nano, obs.UTC)
		if err != nil {
			return nil, local.ErrIntegrity
		}
		admissionID := derivedID("control-admission", e.owner, c.CommandID)
		consumed, err := consumeApprovals(control, "grant.issue", rawDigest(payload), admissionID, c.Approvals, obs)
		if err != nil {
			return nil, err
		}
		grant := Grant{
			SchemaVersion: "1", ID: derivedID("grant", e.owner, c.CommandID), IssuerID: e.owner, SubjectID: c.SubjectID,
			Capabilities: append([]string{}, c.Capabilities...), ResourceScopes: []ResourceIdentity{e.controlResource("project", e.Config.ID)},
			ConstraintsRef: constraintsRef, ExpiresAt: now.Add(lifetime).Format(time.RFC3339Nano),
			MaxLogicalOperations: c.MaxOperations, MaxDeliveriesPerOperation: 1, AllowUnattended: false, Status: "active",
		}
		if len(c.ResourceScopes) != 0 {
			grant.ResourceScopes = append([]ResourceIdentity{}, c.ResourceScopes...)
		}
		encoded, err := canonical(grant)
		if err != nil {
			return nil, err
		}
		if err := flow.ValidateProtocol("Grant", encoded); err != nil {
			return nil, err
		}
		control.Grants = append(control.Grants, ControlGrant{Grant: grant, Issued: obs, Uses: []GrantUse{}})
		control.ControlEpoch++
		if _, err := e.controlAdmission(control, "project", e.Config.ID, c.CommandID, rawDigest(payload), consumed, obs); err != nil {
			return nil, err
		}
		return encoded, nil
	})
}

func itoa(value int64) string {
	if value == 0 {
		return "0"
	}
	digits := []byte{}
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}

// grantStatus is what the grant means now. Expiry and exhaustion are derived,
// because a refused use writes no state and time approves nothing.
func grantStatus(recorded ControlGrant, obs Observation) (string, error) {
	if recorded.Grant.Status != "active" {
		return recorded.Grant.Status, nil
	}
	due, err := time.Parse(time.RFC3339Nano, recorded.Grant.ExpiresAt)
	now, nowErr := time.Parse(time.RFC3339Nano, obs.UTC)
	if err != nil || nowErr != nil {
		return "", local.ErrIntegrity
	}
	if !now.Before(due) {
		return "expired", nil
	}
	if recorded.UsedCount >= recorded.Grant.MaxLogicalOperations {
		return "exhausted", nil
	}
	return "active", nil
}

// consumeGrant spends exactly one logical operation. The counter moves in the
// same transaction as the protected change, so a grant cannot be spent twice
// for one admission or spent after it stopped being current.
func consumeGrant(control *AuthorityControl, subject, operation, intentDigest, admissionID, grantID string, resources []ResourceIdentity, obs Observation) error {
	recorded := control.grant(grantID)
	if recorded == nil {
		return local.Reject("not_found", "no grant with this identity")
	}
	status, err := grantStatus(*recorded, obs)
	if err != nil {
		return err
	}
	if status != "active" {
		return local.Reject("grant_not_admissible", "this grant is "+status)
	}
	if recorded.Grant.SubjectID != subject {
		return local.Reject("grant_subject_conflict", "this grant was issued to another subject")
	}
	held := false
	for _, capability := range recorded.Grant.Capabilities {
		held = held || capability == operation
	}
	if !held {
		return local.Reject("grant_capability_conflict", "this grant does not cover the operation")
	}
	if len(resources) == 0 {
		covered := false
		for _, scope := range recorded.Grant.ResourceScopes {
			covered = covered || scope.Scope == "project" && scope.CanonicalID == control.projectID()
		}
		if !covered {
			return local.Reject("grant_scope_conflict", "this grant does not cover the object")
		}
	} else {
		for _, resource := range resources {
			if !containsResource(recorded.Grant.ResourceScopes, resource) {
				return local.Reject("grant_scope_conflict", "this grant does not cover every action target")
			}
		}
	}
	recorded.UsedCount++
	use := GrantUse{Operation: operation, IntentDigest: intentDigest, Observed: obs}
	if operation == "action.admit" {
		use.ActionAdmissionID = admissionID
	} else {
		use.AdmissionID = admissionID
	}
	recorded.Uses = append(recorded.Uses, use)
	if recorded.UsedCount >= recorded.Grant.MaxLogicalOperations {
		recorded.Grant.Status = "exhausted"
	}
	return nil
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func containsResource(values []ResourceIdentity, wanted ResourceIdentity) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// projectID is the object a control grant is scoped to. The control row records
// its own access entries, so the project need not be passed in from a caller.
func (c AuthorityControl) projectID() string {
	for _, access := range c.Access {
		if access.Scope == "project" {
			return access.ScopeID
		}
	}
	return ""
}

type ControlGrantRevoke struct {
	CommandID string
	GrantID   string
	Reason    string
}

// RevokeControlGrant blocks later use. Recorded uses are not rewritten: the
// admissions they produced already happened.
func (e *Engine) RevokeControlGrant(ctx context.Context, c ControlGrantRevoke) (local.AuthorityApplyResult, error) {
	if c.CommandID == "" || c.GrantID == "" || c.Reason == "" || len(c.Reason) > 4096 {
		return local.AuthorityApplyResult{}, errors.New("explicit command, grant and reason required")
	}
	control, _, err := e.ensureControl(ctx)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationGrant) {
		return local.AuthorityApplyResult{}, local.Reject("object_access_denied", "the session principal cannot revoke control grants")
	}
	command, err := canonical(map[string]any{"operation": "grant.revoke", "command_id": c.CommandID, "grant_id": c.GrantID, "reason": c.Reason})
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	return e.applyControl(ctx, c.CommandID, command, func(control *AuthorityControl, obs Observation) (json.RawMessage, error) {
		recorded := control.grant(c.GrantID)
		if recorded == nil {
			return nil, local.Reject("not_found", "no grant with this identity")
		}
		if recorded.Grant.Status == "revoked" {
			return nil, local.Reject("grant_terminal", "this grant is already revoked")
		}
		recorded.Grant.Status, recorded.Revoked = "revoked", &obs
		control.ControlEpoch++
		return canonical(recorded.Grant)
	})
}

// ControlGrantView reports the effective status beside the recorded decision.
func (e *Engine) ControlGrantView(control AuthorityControl) map[string]any {
	obs := e.clock.now()
	grants := []map[string]any{}
	for _, recorded := range control.Grants {
		status, err := grantStatus(recorded, obs)
		if err != nil {
			status = "unknown"
		}
		grants = append(grants, map[string]any{"grant": recorded.Grant, "effective_status": status, "used_logical_operations": recorded.UsedCount, "uses": len(recorded.Uses)})
	}
	return map[string]any{"schema_version": "foundation-grants/1", "grants": grants}
}

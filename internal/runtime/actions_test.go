package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

func TestParseActionIntentRejectsUnsealedOrAmbiguousOperation(t *testing.T) {
	ref := flow.Ref{ID: "test:tool/write", Version: "1.0.0", Digest: "sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	resource := ResourceIdentity{ProviderRef: ref, CanonicalID: "file:report", Scope: "project"}
	intent := ActionIntent{SchemaVersion: "1", ID: "intent:test", RunID: "run:test", StepInstanceID: "step:test", OriginatingAttempt: "attempt:test", OperationID: "operation:test", ToolRef: ref, Operation: "write", ArgumentsSchemaRef: ref, Arguments: json.RawMessage(`{"text":"sealed"}`), Targets: []ResourceIdentity{resource}, InputArtifacts: []ArtifactRef{}, ExpectedOutputs: map[string]ActionExpectedOutput{}, EffectClass: "external_write", RetryClass: "deduplicated", Preconditions: []ActionPrecondition{{Resource: resource, ExpectedVersion: "7"}}, DispatchNotAfter: "2026-08-30T12:00:00Z"}
	data, err := canonical(intent)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseActionIntent(data)
	if err != nil || parsed.OperationID != intent.OperationID || string(parsed.Arguments) != `{"text":"sealed"}` {
		t.Fatalf("valid action intent was not retained: %+v %v", parsed, err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"missing_operation": func(v map[string]any) { delete(v, "operation_id") },
		"unbounded_effect":  func(v map[string]any) { v["targets"] = []any{} },
		"extra_authority":   func(v map[string]any) { v["approval"] = "all future work" },
	} {
		t.Run(name, func(t *testing.T) {
			var value map[string]any
			if err := json.Unmarshal(data, &value); err != nil {
				t.Fatal(err)
			}
			mutate(value)
			bad, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ParseActionIntent(bad); err == nil {
				t.Fatal("unsafe action intent was accepted")
			}
		})
	}
}

func TestSessionActionProposalIsDurableAndBlockedBeforeDelivery(t *testing.T) {
	ctx := context.Background()
	e, runID, _ := assistedFixture(t)
	task := handOver(t, e, runID)
	r, view, err := e.load(ctx, runID)
	if err != nil || r.SchemaVersion != CoreActionDeliveryStateVersion {
		t.Fatalf("assisted run did not select action-admission state: %s %v", r.SchemaVersion, err)
	}
	definitions, _, _, err := e.inventoryResources()
	if err != nil {
		t.Fatal(err)
	}
	var toolRef flow.Ref
	for _, definition := range definitions {
		if definition.Kind == "tool" {
			toolRef = definition.Ref
			break
		}
	}
	if toolRef.ID == "" {
		t.Fatal("assisted Run did not pin its tool descriptor")
	}
	resource := ResourceIdentity{ProviderRef: toolRef, CanonicalID: "git:claimed-worktree", Scope: "project"}
	intent := ActionIntent{SchemaVersion: "1", ID: "intent:commit", RunID: runID, StepInstanceID: task.StepInstanceID, OriginatingAttempt: task.AttemptID, OperationID: "operation:commit", ToolRef: toolRef, Operation: "commit", ArgumentsSchemaRef: builtinRef(definitions, "aif:schema/plan"), Arguments: json.RawMessage(`{"summary":"commit reviewed plan"}`), Targets: []ResourceIdentity{resource}, InputArtifacts: []ArtifactRef{}, ExpectedOutputs: map[string]ActionExpectedOutput{}, EffectClass: "workspace_write", RetryClass: "never", Preconditions: []ActionPrecondition{{Resource: resource, ExpectedVersion: "HEAD"}}, DispatchNotAfter: "2030-01-01T00:00:00Z"}
	command := ProposeActionCommand{SchemaVersion: "1", CommandID: "command:action-proposal", RunID: runID, ExpectedRunVersion: view.Snapshot.Version, Payload: intent}
	badArguments := command
	badArguments.CommandID = "command:action-proposal-invalid-arguments"
	badArguments.Payload.Arguments = json.RawMessage(`{"unexpected":true}`)
	if _, err := e.ProposeSessionAction(ctx, badArguments); err == nil {
		t.Fatal("arguments outside the sealed tool schema were proposed")
	} else {
		rejectionCode(t, err, "action_argument_invalid")
	}
	badInput := command
	badInput.CommandID = "command:action-proposal-missing-input"
	badInput.Payload.InputArtifacts = []ArtifactRef{{ArtifactID: "artifact:missing", Revision: 1, Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}
	if _, err := e.ProposeSessionAction(ctx, badInput); err == nil {
		t.Fatal("proposal accepted an unavailable input artifact")
	} else {
		rejectionCode(t, err, "action_input_unavailable")
	}
	if _, err := e.ProposeSessionAction(ctx, command); err != nil {
		t.Fatal(err)
	}
	after, afterView, err := e.load(ctx, runID)
	if err != nil || len(after.ActionIntents) != 1 || after.ActionIntents[intent.ID].Digest == "" || !slices.Contains(after.Active, task.AttemptID) {
		t.Fatalf("proposal was not retained without settling the attempt: %+v %v", after.ActionIntents, err)
	}
	if err := validatePublic(t, "ActionIntentRecord", after.ActionIntents[intent.ID]); err != nil {
		t.Fatalf("sealed proposal is outside its public contract: %v", err)
	}
	if err := validatePublic(t, "ProposeActionCommand", command); err != nil {
		t.Fatalf("proposal command is outside its public contract: %v", err)
	}
	actionView, err := e.View(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePublic(t, "CoreRunViewV21", actionView); err != nil {
		t.Fatalf("action-admission read view is outside its public contract: %v", err)
	}
	if _, err := e.SetControlApprovalPolicy(ctx, ControlApprovalPolicyRequest{CommandID: "command:action-policy", Operations: []string{"action.admit"}, Quorum: 1, Independence: "none", Reason: "the action needs a recorded decision"}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.RequestControlApproval(ctx, ApprovalRequest{CommandID: "command:action-approval-request", Operation: "action.admit", IntentDigest: after.ActionIntents[intent.ID].Digest, Reason: "reviewed exact workspace change"}); err != nil {
		t.Fatal(err)
	}
	control, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	approval := control.Approvals[len(control.Approvals)-1]
	if _, err := e.DecideControlApproval(ctx, ApprovalDecision{CommandID: "command:action-approval-vote", ApprovalID: approval.ID, Decision: "approve", Reason: "approved exact action"}); err != nil {
		t.Fatal(err)
	}
	admit := AdmitActionCommand{SchemaVersion: "1", CommandID: "command:action-admit", RunID: runID, ExpectedRunVersion: afterView.Snapshot.Version, Payload: AdmitActionPayload{IntentID: intent.ID, IntentDigest: after.ActionIntents[intent.ID].Digest, ApprovalRefs: []string{approval.ID}, GrantRefs: []string{}}}
	bad := admit
	bad.CommandID, bad.Payload.ApprovalRefs = "command:action-admit-bad", []string{"approval:missing"}
	if _, err := e.AdmitSessionAction(ctx, bad); err == nil {
		t.Fatal("missing approval admitted an action")
	} else {
		rejectionCode(t, err, "not_found")
	}
	refused, refusedView, err := e.load(ctx, runID)
	if err != nil || len(refused.ActionAdmissions) != 0 || refusedView.Snapshot.Version != afterView.Snapshot.Version {
		t.Fatalf("refused admission changed the Run ledger: %+v %d -> %d, %v", refused.ActionAdmissions, afterView.Snapshot.Version, refusedView.Snapshot.Version, err)
	}
	control, _, err = e.Control(ctx)
	if err != nil || control.approval(approval.ID).Status != "approved" {
		t.Fatalf("refused admission consumed approval: %+v %v", control.approval(approval.ID), err)
	}
	if _, err := e.AdmitSessionAction(ctx, admit); err != nil {
		t.Fatal(err)
	}
	admitted, admittedView, err := e.load(ctx, runID)
	if err != nil || len(admitted.ActionAdmissions) != 1 || admitted.ActionAdmissions[intent.ID].ID == "" || !slices.Contains(admitted.Active, task.AttemptID) {
		t.Fatalf("admission was not retained without delivery: %+v %v", admitted.ActionAdmissions, err)
	}
	if err := validatePublic(t, "ActionAdmission", admitted.ActionAdmissions[intent.ID]); err != nil {
		t.Fatalf("action admission is outside its public contract: %v", err)
	}
	delivery, exists := admitted.ActionDeliveries[admitted.ActionAdmissions[intent.ID].ID]
	if !exists || delivery.DeliveryStatus != "prepared" || delivery.EffectStatus == nil || *delivery.EffectStatus != "not_started" || delivery.ReceiptRef != nil {
		t.Fatalf("admission did not retain a pre-dispatch delivery: %+v", delivery)
	}
	if err := validatePublic(t, "AdmitActionCommand", admit); err != nil {
		t.Fatalf("action admission command is outside its public contract: %v", err)
	}
	control, _, err = e.Control(ctx)
	if err != nil || control.approval(approval.ID).Status != "consumed" || control.approval(approval.ID).ConsumedByAdmissionID != admitted.ActionAdmissions[intent.ID].ID {
		t.Fatalf("admission did not atomically consume its approval: %+v %v", control.approval(approval.ID), err)
	}
	if _, err := e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: "command:action-stop", Scope: "run", ScopeID: runID, Kind: "pause", Reason: "hold new actions"}); err != nil {
		t.Fatal(err)
	}
	blocked, blockedView, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	command.CommandID, command.ExpectedRunVersion = "command:blocked-action", blockedView.Snapshot.Version
	command.Payload.ID, command.Payload.OperationID = "intent:blocked", "operation:blocked"
	if _, err := e.ProposeSessionAction(ctx, command); err == nil {
		t.Fatal("stop admitted a new action proposal")
	} else {
		rejectionCode(t, err, "dispatch_blocked")
	}
	if len(blocked.ActionIntents) != 1 {
		t.Fatalf("stop changed the durable proposal: %+v", blocked.ActionIntents)
	}
	if err := os.Remove(filepath.Join(e.Root, "tools", "commit.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := e.ProposeSessionAction(ctx, ProposeActionCommand{SchemaVersion: "1", CommandID: "command:action-proposal", RunID: runID, ExpectedRunVersion: view.Snapshot.Version, Payload: intent}); err != nil {
		t.Fatalf("exact proposal retry did not return its receipt after the registry changed: %v", err)
	}
	retried, retriedView, err := e.load(ctx, runID)
	if err != nil || retriedView.Snapshot.Version != blockedView.Snapshot.Version || len(retried.ActionIntents) != 1 || len(retried.ActionAdmissions) != 1 || admittedView.Snapshot.Version >= blockedView.Snapshot.Version {
		t.Fatalf("exact retry changed the action ledgers: version %d -> %d, intents=%+v admissions=%+v, err=%v", blockedView.Snapshot.Version, retriedView.Snapshot.Version, retried.ActionIntents, retried.ActionAdmissions, err)
	}
}

func TestActionAdmissionRejectsRevokedToolPackage(t *testing.T) {
	ctx := context.Background()
	e, runID, _ := assistedFixture(t)
	definitions, _, err := Builtins()
	if err != nil {
		t.Fatal(err)
	}
	source, toolRef := toolPackage(t, builtinRef(definitions, "core:adapter/assisted-session"), "commit", "workspace_write", "never")
	if _, err := e.ImportPackage(ctx, PackageImportRequest{CommandID: "command:action-tool", Directory: source, Reason: "seal the proposed action tool"}); err != nil {
		t.Fatal(err)
	}
	// Package definitions become available on a newly opened engine, while the
	// Run and its assisted session remain the same durable records.
	opened, err := Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	task := handOver(t, opened, runID)
	_, view, err := opened.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	resource := ResourceIdentity{ProviderRef: toolRef, CanonicalID: "git:claimed-worktree", Scope: "project"}
	intent := ActionIntent{SchemaVersion: "1", ID: "intent:revoked-tool", RunID: runID, StepInstanceID: task.StepInstanceID, OriginatingAttempt: task.AttemptID, OperationID: "operation:revoked-tool", ToolRef: toolRef, Operation: "commit", ArgumentsSchemaRef: builtinRef(definitions, "core:schema/context-json"), Arguments: json.RawMessage(`{"summary":"reviewed plan"}`), Targets: []ResourceIdentity{resource}, InputArtifacts: []ArtifactRef{}, ExpectedOutputs: map[string]ActionExpectedOutput{}, EffectClass: "workspace_write", RetryClass: "never", Preconditions: []ActionPrecondition{{Resource: resource, ExpectedVersion: "HEAD"}}, DispatchNotAfter: "2030-01-01T00:00:00Z"}
	if _, err := opened.ProposeSessionAction(ctx, ProposeActionCommand{SchemaVersion: "1", CommandID: "command:revoked-tool-proposal", RunID: runID, ExpectedRunVersion: view.Snapshot.Version, Payload: intent}); err != nil {
		t.Fatal(err)
	}
	run, actionView, err := opened.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := opened.SetControlApprovalPolicy(ctx, ControlApprovalPolicyRequest{CommandID: "command:revoked-tool-policy", Operations: []string{"action.admit"}, Quorum: 1, Independence: "none", Reason: "the action needs a recorded decision"}); err != nil {
		t.Fatal(err)
	}
	if _, err := opened.RequestControlApproval(ctx, ApprovalRequest{CommandID: "command:revoked-tool-request", Operation: "action.admit", IntentDigest: run.ActionIntents[intent.ID].Digest, Reason: "review the exact action"}); err != nil {
		t.Fatal(err)
	}
	control, _, err := opened.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	approval := control.Approvals[len(control.Approvals)-1]
	if _, err := opened.DecideControlApproval(ctx, ApprovalDecision{CommandID: "command:revoked-tool-vote", ApprovalID: approval.ID, Decision: "approve", Reason: "approved exact action"}); err != nil {
		t.Fatal(err)
	}
	packages, err := opened.Packages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := opened.SetPackageStatus(ctx, PackageLifecycleRequest{CommandID: "command:revoke-action-tool", ID: packages.Packages[0].Ref.ID, Version: packages.Packages[0].Ref.Version, Status: PackageRevoked, Reason: "security incident"}); err != nil {
		t.Fatal(err)
	}
	later := intent
	later.ID, later.OperationID = "intent:revoked-tool-later", "operation:revoked-tool-later"
	_, err = opened.ProposeSessionAction(ctx, ProposeActionCommand{SchemaVersion: "1", CommandID: "command:revoked-tool-later", RunID: runID, ExpectedRunVersion: actionView.Snapshot.Version, Payload: later})
	rejectionCode(t, err, "package_not_resolvable")
	admit := AdmitActionCommand{SchemaVersion: "1", CommandID: "command:revoked-tool-admit", RunID: runID, ExpectedRunVersion: actionView.Snapshot.Version, Payload: AdmitActionPayload{IntentID: intent.ID, IntentDigest: run.ActionIntents[intent.ID].Digest, ApprovalRefs: []string{approval.ID}, GrantRefs: []string{}}}
	_, err = opened.AdmitSessionAction(ctx, admit)
	rejectionCode(t, err, "package_revoked")
	after, _, err := opened.load(ctx, runID)
	if err != nil || len(after.ActionAdmissions) != 0 {
		t.Fatalf("revoked tool admitted an action: %+v %v", after.ActionAdmissions, err)
	}
	control, _, err = opened.Control(ctx)
	if err != nil || control.approval(approval.ID).Status != "approved" {
		t.Fatalf("revoked tool consumed approval: %+v %v", control.approval(approval.ID), err)
	}
}

func TestActionAdmissionConsumesExactResourceGrant(t *testing.T) {
	ctx := context.Background()
	e, runID, _ := assistedFixture(t)
	task := handOver(t, e, runID)
	_, view, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	definitions, _, _, err := e.inventoryResources()
	if err != nil {
		t.Fatal(err)
	}
	var toolRef flow.Ref
	for _, definition := range definitions {
		if definition.Kind == "tool" {
			toolRef = definition.Ref
			break
		}
	}
	resource := ResourceIdentity{ProviderRef: toolRef, CanonicalID: "git:claimed-worktree", Scope: "project"}
	intent := ActionIntent{SchemaVersion: "1", ID: "intent:grant", RunID: runID, StepInstanceID: task.StepInstanceID, OriginatingAttempt: task.AttemptID, OperationID: "operation:grant", ToolRef: toolRef, Operation: "commit", ArgumentsSchemaRef: builtinRef(definitions, "aif:schema/plan"), Arguments: json.RawMessage(`{"summary":"commit reviewed plan"}`), Targets: []ResourceIdentity{resource}, InputArtifacts: []ArtifactRef{}, ExpectedOutputs: map[string]ActionExpectedOutput{}, EffectClass: "workspace_write", RetryClass: "never", Preconditions: []ActionPrecondition{{Resource: resource, ExpectedVersion: "HEAD"}}, DispatchNotAfter: "2030-01-01T00:00:00Z"}
	if _, err := e.ProposeSessionAction(ctx, ProposeActionCommand{SchemaVersion: "1", CommandID: "command:action-grant-proposal", RunID: runID, ExpectedRunVersion: view.Snapshot.Version, Payload: intent}); err != nil {
		t.Fatal(err)
	}
	run, runView, err := e.load(ctx, runID)
	if err != nil {
		t.Fatal(err)
	}
	badScope := resource
	badScope.CanonicalID = "git:other-worktree"
	if _, err := e.IssueControlGrant(ctx, ControlGrantRequest{CommandID: "command:action-grant-wrong", SubjectID: e.owner, Capabilities: []string{"action.admit"}, ResourceScopes: []ResourceIdentity{badScope}, MaxOperations: 1, LifetimeMS: 60_000, Reason: "bound to another worktree"}); err != nil {
		t.Fatal(err)
	}
	control, _, err := e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wrong := control.Grants[len(control.Grants)-1].Grant
	bad := AdmitActionCommand{SchemaVersion: "1", CommandID: "command:action-grant-refused", RunID: runID, ExpectedRunVersion: runView.Snapshot.Version, Payload: AdmitActionPayload{IntentID: intent.ID, IntentDigest: run.ActionIntents[intent.ID].Digest, ApprovalRefs: []string{}, GrantRefs: []string{wrong.ID}}}
	if _, err := e.AdmitSessionAction(ctx, bad); err == nil {
		t.Fatal("grant for another resource admitted the action")
	} else {
		rejectionCode(t, err, "grant_scope_conflict")
	}
	refused, refusedView, err := e.load(ctx, runID)
	if err != nil || len(refused.ActionAdmissions) != 0 || refusedView.Snapshot.Version != runView.Snapshot.Version {
		t.Fatalf("refused grant changed the action ledger: %+v %v", refused.ActionAdmissions, err)
	}
	control, _, err = e.Control(ctx)
	if err != nil || control.grant(wrong.ID).UsedCount != 0 {
		t.Fatalf("refused grant was consumed: %+v %v", control.grant(wrong.ID), err)
	}
	if _, err := e.IssueControlGrant(ctx, ControlGrantRequest{CommandID: "command:action-grant-right", SubjectID: e.owner, Capabilities: []string{"action.admit"}, ResourceScopes: []ResourceIdentity{resource}, MaxOperations: 1, LifetimeMS: 60_000, Reason: "bound to the exact worktree"}); err != nil {
		t.Fatal(err)
	}
	control, _, err = e.Control(ctx)
	if err != nil {
		t.Fatal(err)
	}
	grant := control.Grants[len(control.Grants)-1].Grant
	admit := bad
	admit.CommandID, admit.Payload.GrantRefs = "command:action-grant-admit", []string{grant.ID}
	if _, err := e.AdmitSessionAction(ctx, admit); err != nil {
		t.Fatal(err)
	}
	admitted, _, err := e.load(ctx, runID)
	if err != nil || admitted.ActionAdmissions[intent.ID].GrantRefs[0] != grant.ID || !slices.Contains(admitted.Active, task.AttemptID) {
		t.Fatalf("grant admission was not retained without delivery: %+v %v", admitted.ActionAdmissions, err)
	}
	control, _, err = e.Control(ctx)
	used := control.grant(grant.ID)
	if err != nil || used.UsedCount != 1 || len(used.Uses) != 1 || used.Uses[0].ActionAdmissionID != admitted.ActionAdmissions[intent.ID].ID || used.Uses[0].AdmissionID != "" {
		t.Fatalf("exact grant was not atomically consumed by the action admission: %+v %v", used, err)
	}
}

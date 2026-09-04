package runtime

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// A command is identified by its id. Repeating one with the same id must return
// the recorded receipt rather than a second decision, which fails the moment a
// payload carries a clock reading: the retry then describes different input.
// Each case binds its own fixture and is then invoked twice with one id.
func TestEveryCommandRetryIsDuplicate(t *testing.T) {
	// The two receipt shapes are compared by their recorded content, which is
	// what a retry must reproduce exactly.
	type invoke func(commandID string) (bool, any, error)
	authority := func(result local.AuthorityApplyResult, err error) (bool, any, error) {
		return result.Duplicate, result.Receipt, err
	}
	ordinary := func(result local.ApplyResult, err error) (bool, any, error) {
		return result.Duplicate, result.Receipt, err
	}
	for _, c := range []struct {
		name string
		bind func(t *testing.T) invoke
	}{
		{"capacity", func(t *testing.T) invoke {
			e, _ := emptyRuntime(t)
			return func(id string) (bool, any, error) {
				return authority(e.SetAdmissionCapacity(context.Background(), CapacityRequest{CommandID: id, Capacity: MaxAdmissionCapacity, Reason: "run the qualified maximum"}))
			}
		}},
		{"approval policy", func(t *testing.T) invoke {
			e, _ := emptyRuntime(t)
			return func(id string) (bool, any, error) {
				return authority(e.SetControlApprovalPolicy(context.Background(), ControlApprovalPolicyRequest{CommandID: id, Operations: []string{"stop.release"}, Quorum: 1, Independence: "none", Reason: "releases need a recorded decision"}))
			}
		}},
		{"control stop", func(t *testing.T) invoke {
			e, _ := emptyRuntime(t)
			return func(id string) (bool, any, error) {
				return authority(e.RestrictControl(context.Background(), ControlRestrictRequest{CommandID: id, Scope: "project", Reason: "held for the decision"}))
			}
		}},
		{"control release", func(t *testing.T) invoke {
			e, _ := emptyRuntime(t)
			ctx := context.Background()
			stop := openStop(t, e, ctx)
			control, _, err := e.Control(ctx)
			if err != nil {
				t.Fatal(err)
			}
			return func(id string) (bool, any, error) {
				return authority(e.ReleaseControl(ctx, ControlReleaseRequest{CommandID: id, Scope: "project", ExpectedControlEpoch: control.ControlEpoch, Stops: []StopGeneration{{ID: stop.ID, Generation: stop.Generation}}, Reason: "the incident is closed"}))
			}
		}},
		{"grant", func(t *testing.T) invoke {
			e, _ := emptyRuntime(t)
			return func(id string) (bool, any, error) {
				return authority(e.IssueControlGrant(context.Background(), ControlGrantRequest{CommandID: id, SubjectID: e.owner, Capabilities: []string{"stop.release"}, MaxOperations: 1, LifetimeMS: 60000, Reason: "delegate one bounded release"}))
			}
		}},
		{"grant revoke", func(t *testing.T) invoke {
			e, _ := emptyRuntime(t)
			ctx := context.Background()
			grant := issuedGrant(t, e, ctx, 1)
			return func(id string) (bool, any, error) {
				return authority(e.RevokeControlGrant(ctx, ControlGrantRevoke{CommandID: id, GrantID: grant.Grant.ID, Reason: "the delegation is no longer needed"}))
			}
		}},
		{"approval request", func(t *testing.T) invoke {
			e, ctx := gatedRuntime(t, 1, "none")
			stop := openStop(t, e, ctx)
			intent := stopReleaseIntentDigest(t, e, ctx, "command:release", stop)
			return func(id string) (bool, any, error) {
				return authority(e.RequestControlApproval(ctx, ApprovalRequest{CommandID: id, Operation: "stop.release", IntentDigest: intent, Reason: "release the pilot stop"}))
			}
		}},
		{"approval vote", func(t *testing.T) invoke {
			e, ctx := gatedRuntime(t, 1, "none")
			stop := openStop(t, e, ctx)
			intent := stopReleaseIntentDigest(t, e, ctx, "command:release", stop)
			if _, err := e.RequestControlApproval(ctx, ApprovalRequest{CommandID: "command:request", Operation: "stop.release", IntentDigest: intent, Reason: "release the pilot stop"}); err != nil {
				t.Fatal(err)
			}
			control, _, err := e.Control(ctx)
			if err != nil {
				t.Fatal(err)
			}
			approval := control.Approvals[0]
			return func(id string) (bool, any, error) {
				return authority(e.DecideControlApproval(ctx, ApprovalDecision{CommandID: id, ApprovalID: approval.ID, Decision: "approve", Reason: "the incident is closed"}))
			}
		}},
		{"trust root", func(t *testing.T) invoke {
			e, _ := emptyRuntime(t)
			public, _, err := ed25519.GenerateKey(nil)
			if err != nil {
				t.Fatal(err)
			}
			return func(id string) (bool, any, error) {
				return authority(e.SetTrustRoot(context.Background(), TrustRootRequest{CommandID: id, ID: "root:release", PublicKey: hex.EncodeToString(public), Note: "release signer", Reason: "accept the release signer"}))
			}
		}},
		{"waiver", func(t *testing.T) invoke {
			// The waived step is absent on purpose: the refusal happens inside
			// the transform, so the command is recorded and its retry must be
			// the same recorded refusal, not a new decision.
			e, runID, _, _ := waiverAwareFixture(t, "succeeded")
			return func(id string) (bool, any, error) {
				return ordinary(e.Waive(context.Background(), WaiveRequest{CommandID: id, RunID: runID, StepID: "step:absent", CheckRef: flow.Ref{ID: "demo:check/quality", Version: "1.0.0", Digest: rawDigest([]byte("q"))}, Reason: "the reviewer accepted this gap"}))
			}
		}},
		{"run start", func(t *testing.T) invoke {
			e, options := emptyRuntime(t)
			return func(id string) (bool, any, error) {
				options.CommandID = id
				result, err := e.Start(context.Background(), options)
				return result.Duplicate, result.Receipt, err
			}
		}},
		{"obligation resolve", func(t *testing.T) invoke {
			e, runID, attemptID := uncertainAssistedRun(t)
			ctx := context.Background()
			view, err := e.View(ctx, runID)
			if err != nil {
				t.Fatal(err)
			}
			return func(id string) (bool, any, error) {
				return ordinary(e.ResolveObligation(ctx, runID, id, attemptID, "", ResolveOutcomeNotApplied, "the host was stopped before it wrote anything", view.RunVersion))
			}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			invoked := c.bind(t)
			id := newID("command")
			// A recorded refusal is a recorded decision: the retry must return
			// that same receipt rather than deciding again.
			recorded := func(err error) error {
				var rejection *local.Rejection
				if err == nil || errors.As(err, &rejection) {
					return nil
				}
				return err
			}
			duplicate, first, err := invoked(id)
			if err := recorded(err); err != nil {
				t.Fatalf("the command failed: %v", err)
			}
			if duplicate {
				t.Fatal("a first command reported itself as a repeat")
			}
			duplicate, second, err := invoked(id)
			if err := recorded(err); err != nil {
				t.Fatalf("the retry was refused instead of returning its receipt: %v", err)
			}
			if !duplicate {
				t.Fatalf("the retry was applied as a second command: %+v", second)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("the retry returned a different receipt:\n%+v\n%+v", first, second)
			}
		})
	}
}

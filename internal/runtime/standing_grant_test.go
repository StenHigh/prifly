package runtime

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// A brief recording confirmation as "standing_grant" describes something true
// about the world -- the owner delegated in advance -- and until 0.13.11 the
// engine refused it, because a document that grants itself an exemption is the
// caller granting itself one. The value is now accepted against a grant the
// owner issued into the authority, and each start spends one of its operations:
// verifying without spending would publish a bound that does not bind.
func standingGrantFixture(t *testing.T, operations int64) (*Engine, StartOptions, string) {
	t.Helper()
	e, options := emptyRuntime(t)
	brief := Brief{"1", "test:brief/standing", "Work the owner delegated in advance", "Return no_work without a worker", []string{"Local state only"}, []string{"Network"}, []string{"Finish with no_work"}, []ArtifactRef{}, []string{}, "standing_grant"}
	writeRuntimeJSON(t, filepath.Join(e.Root, "brief.json"), brief)
	if _, err := e.IssueControlGrant(context.Background(), ControlGrantRequest{CommandID: newID("command"), SubjectID: e.owner, Capabilities: []string{ControlCapabilityRunStart}, MaxOperations: operations, LifetimeMS: 60_000, Reason: "the owner delegated starting Runs while away"}); err != nil {
		t.Fatal(err)
	}
	control, _, err := e.Control(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(control.Grants) == 0 {
		t.Fatal("the authority recorded no grant")
	}
	return e, options, control.Grants[len(control.Grants)-1].Grant.ID
}

func TestStandingGrantIsSpentByEachStartAndRefusedWithoutOne(t *testing.T) {
	ctx := context.Background()

	t.Run("a brief cannot grant itself the exemption", func(t *testing.T) {
		e, options, _ := standingGrantFixture(t, 1)
		options.CommandID = newID("command")
		_, err := e.Start(ctx, options)
		if err == nil {
			t.Fatal("a standing_grant brief started a Run with no grant named")
		}
		if !strings.Contains(err.Error(), "start_confirmation_required") || !strings.Contains(err.Error(), "--grant") {
			t.Fatalf("the refusal does not name the missing authority: %v", err)
		}
	})

	t.Run("a named grant admits the start and loses one operation", func(t *testing.T) {
		e, options, grantID := standingGrantFixture(t, 2)
		options.CommandID, options.GrantID = newID("command"), grantID
		if _, err := e.Start(ctx, options); err != nil {
			t.Fatalf("a start backed by a live grant was refused: %v", err)
		}
		control, _, err := e.Control(ctx)
		if err != nil {
			t.Fatal(err)
		}
		recorded := control.grant(grantID)
		if recorded == nil || recorded.UsedCount != 1 {
			t.Fatalf("the grant was checked and not spent: %+v", recorded)
		}
		if recorded.Grant.Status != "active" {
			t.Fatalf("a grant with an operation left was retired: %s", recorded.Grant.Status)
		}
	})

	// The whole point: a grant for one start authorises one start. Checking
	// without spending would make three issued starts mean unlimited starts.
	t.Run("the second start over a one-operation grant is refused", func(t *testing.T) {
		e, options, grantID := standingGrantFixture(t, 1)
		first := options
		first.CommandID, first.GrantID = newID("command"), grantID
		if _, err := e.Start(ctx, first); err != nil {
			t.Fatalf("the first start was refused: %v", err)
		}
		second := options
		second.CommandID, second.GrantID = newID("command"), grantID
		_, err := e.Start(ctx, second)
		if err == nil {
			t.Fatal("a grant bounded to one operation authorised two starts")
		}
		if !strings.Contains(err.Error(), "grant_not_admissible") || !strings.Contains(err.Error(), "exhausted") {
			t.Fatalf("the refusal does not say the grant is spent: %v", err)
		}
	})

	t.Run("a grant for another operation does not admit a start", func(t *testing.T) {
		e, options := emptyRuntime(t)
		brief := Brief{"1", "test:brief/standing", "Delegated work", "Return no_work", []string{"Local"}, []string{"Network"}, []string{"no_work"}, []ArtifactRef{}, []string{}, "standing_grant"}
		writeRuntimeJSON(t, filepath.Join(e.Root, "brief.json"), brief)
		if _, err := e.IssueControlGrant(ctx, ControlGrantRequest{CommandID: newID("command"), SubjectID: e.owner, Capabilities: []string{"stop.release"}, MaxOperations: 1, LifetimeMS: 60_000, Reason: "a release, not a start"}); err != nil {
			t.Fatal(err)
		}
		control, _, err := e.Control(ctx)
		if err != nil {
			t.Fatal(err)
		}
		options.CommandID, options.GrantID = newID("command"), control.Grants[len(control.Grants)-1].Grant.ID
		if _, err := e.Start(ctx, options); err == nil || !strings.Contains(err.Error(), "grant_capability_conflict") {
			t.Fatalf("a stop.release grant authorised a start: %v", err)
		}
	})

	// An explicitly confirmed Run must not touch a grant, and must keep the
	// empty grant_refs its contract has always carried.
	t.Run("explicit confirmation spends nothing", func(t *testing.T) {
		e, options, grantID := standingGrantFixture(t, 1)
		brief := Brief{"1", "test:brief/explicit", "Confirmed here and now", "Return no_work", []string{"Local"}, []string{"Network"}, []string{"no_work"}, []ArtifactRef{}, []string{}, "explicit"}
		writeRuntimeJSON(t, filepath.Join(e.Root, "brief.json"), brief)
		options.CommandID = newID("command")
		if _, err := e.Start(ctx, options); err != nil {
			t.Fatalf("an explicit start was refused: %v", err)
		}
		control, _, err := e.Control(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if recorded := control.grant(grantID); recorded == nil || recorded.UsedCount != 0 {
			t.Fatalf("an explicit start spent a grant it never named: %+v", recorded)
		}
	})
}

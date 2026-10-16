package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/local"
)

func saturateStorageBudget(t *testing.T, e *Engine) {
	t.Helper()
	usage, err := e.Store.StorageUsage(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if usage.SoftLimitBytes > 8<<20 {
		t.Fatal("fixture must bootstrap a small finite budget")
	}
	data, _ := json.Marshal(map[string]string{"fixture_padding": strings.Repeat("x", int(usage.SoftLimitBytes+64<<10))})
	result, err := e.Store.ApplyAuthority(context.Background(), local.AuthorityCommand{ID: newID("command"), Actor: e.owner, Key: "fixture:budget_padding", Payload: json.RawMessage(`{"operation":"fixture.saturate"}`)}, func(local.AuthoritySnapshot) (local.AuthorityChange, error) {
		return local.AuthorityChange{Data: data}, nil
	})
	if err != nil || result.Receipt.Rejection != nil {
		t.Fatal(err, result.Receipt.Rejection)
	}
	usage, err = e.Store.StorageUsage(context.Background())
	if err != nil || usage.AllocatedBytes < usage.SoftLimitBytes {
		t.Fatal("fixture did not saturate", usage, err)
	}
}

func raiseStorageBudget(t *testing.T, e *Engine, target int64) StorageBudgetResult {
	t.Helper()
	review, err := e.PrepareStorageBudget(context.Background(), target, "preserve accepted Run evidence")
	if err != nil {
		t.Fatal(err)
	}
	result, err := e.SetStorageBudget(context.Background(), StorageBudgetRequest{CommandID: newID("command"), TargetBytes: target, Reason: review.Reason, ExpectedReviewDigest: review.ReviewDigest})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestStorageBudgetReviewedRaiseSurvivesRestartAndOpenDriver(t *testing.T) {
	t.Parallel()
	e := artifactEngine(t, 512<<10)
	ctx := context.Background()
	other, err := Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	review, err := e.PrepareStorageBudget(ctx, 1<<20, "retain accepted work")
	if err != nil {
		t.Fatal(err)
	}
	request := StorageBudgetRequest{CommandID: "command:budget-raise", TargetBytes: review.TargetBytes, Reason: review.Reason, ExpectedReviewDigest: review.ReviewDigest}
	first, err := e.SetStorageBudget(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := e.SetStorageBudget(ctx, request)
	if err != nil || first != duplicate {
		t.Fatal("exact retry lost recorded result", duplicate, err)
	}
	status, err := other.StorageBudgetStatus(ctx)
	if err != nil || status.Control.LimitBytes != 1<<20 || status.Control.Generation != 1 || status.Usage.SoftLimitBytes != 1<<20 {
		t.Fatal("open driver kept stale ceiling", status, err)
	}
	if _, err := other.SetStorageBudget(ctx, StorageBudgetRequest{CommandID: "command:stale", TargetBytes: request.TargetBytes, Reason: request.Reason, ExpectedReviewDigest: request.ExpectedReviewDigest}); driverFailureCode(err, "") != "storage_budget_review_conflict" {
		t.Fatal("stale review allowed mutation", err)
	}
	changed := request
	changed.Reason = "different request"
	if _, err := e.SetStorageBudget(ctx, changed); err != local.ErrCommandConflict {
		t.Fatal("command payload conflict lost", err)
	}
	if err := other.Store.Verify(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestStorageBudgetEmittedContractsAndUsageStableReview(t *testing.T) {
	t.Parallel()
	e := artifactEngine(t, 512<<10)
	ctx := context.Background()
	before, err := e.StorageBudgetStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePublic(t, "StorageBudgetStatus", before); err != nil {
		t.Fatal(err)
	}
	review, err := e.PrepareStorageBudget(ctx, 1<<20, "preserve evidence")
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePublic(t, "StorageBudgetReview", review); err != nil {
		t.Fatal(err)
	}
	// Ordinary committed telemetry changes cut/usage without changing the setting.
	if _, err := e.Store.AppendSamples(ctx, []local.SampleInput{{ID: "sample:review-traffic", Data: json.RawMessage(`{"traffic":true}`)}}); err != nil {
		t.Fatal(err)
	}
	after, err := e.PrepareStorageBudget(ctx, 1<<20, review.Reason)
	if err != nil || after.ReviewDigest != review.ReviewDigest || after.Status.Cut <= review.Status.Cut {
		t.Fatal("telemetry invalidated owner intent", after, err)
	}
	result, err := e.SetStorageBudget(ctx, StorageBudgetRequest{CommandID: "command:schemas", TargetBytes: review.TargetBytes, Reason: review.Reason, ExpectedReviewDigest: review.ReviewDigest})
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePublic(t, "StorageBudgetResult", result); err != nil {
		t.Fatal(err)
	}
	if err := validatePublic(t, "AuthorityStorageBudget", result.Control); err != nil {
		t.Fatal(err)
	}
}

func TestStorageBudgetPendingCheckerResumesWithoutProducerReplay(t *testing.T) {
	t.Parallel()
	e, options := acceptanceProject(t, []string{"step_output", "step_result"}, "", "pass", false, 2<<20)
	ctx := context.Background()
	start, err := e.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	runID := start.Receipt.RunID
	var attemptID, workspace string
	func() {
		lock, err := e.driverLock(runID)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Close()
		socket, closeServer, err := e.serveSteps()
		if err != nil {
			t.Fatal(err)
		}
		defer closeServer()
		attempt := driverAdmit(t, e, runID)
		attemptID = attempt.ID
		workspace = attempt.Workspace
		saturateStorageBudget(t, e)
		r, view, err := e.load(ctx, runID)
		if err != nil {
			t.Fatal(err)
		}
		if err := e.executePending(ctx, r, view, attempt, socket); err != nil {
			t.Fatal(err)
		}
	}()
	pending := driverRun(t, e, runID)
	if pending.PendingAcceptance == nil || pending.Attempts[attemptID].Accepted != nil || len(pending.CheckExecutions) != 0 {
		t.Fatal("fixture skipped pending checker")
	}
	if err := e.Drive(ctx, runID); driverFailureCode(err, "") != "storage_budget_exhausted" {
		t.Fatal(err)
	}
	raiseStorageBudget(t, e, 4<<20)
	afterSet := driverRun(t, e, runID)
	if afterSet.PendingAcceptance == nil || afterSet.Attempts[attemptID].Accepted != nil || len(afterSet.CheckExecutions) != 0 {
		t.Fatal("setting accepted unchecked output")
	}
	if err := e.Drive(ctx, runID); err != nil {
		t.Fatal(err)
	}
	final := driverRun(t, e, runID)
	if final.Status != "completed" || final.PendingAcceptance != nil || len(final.Attempts) != 1 || final.Attempts[attemptID].Accepted == nil || len(final.CheckExecutions) == 0 {
		t.Fatal("checker recovery did not finish", final.Status)
	}
	if starts, err := os.ReadFile(filepath.Join(workspace, "worker-starts")); err != nil || string(starts) != "start\n" {
		t.Fatal("producer replay", err)
	}
}

func TestStorageBudgetRaisePreservesStopAndUnknownEffect(t *testing.T) {
	t.Parallel()
	for _, unknown := range []bool{false, true} {
		t.Run(fmt.Sprint(unknown), func(t *testing.T) {
			t.Parallel()
			e, runID := driverProject(t, "pass", 5000, driverShape{StorageBudgetBytes: 2 << 20})
			ctx := context.Background()
			if unknown {
				attempt := driverAdmit(t, e, runID)
				driverDispatchFixture(t, e, runID, attempt.ID)
				if err := e.settle(ctx, runID, attempt.ID, local.ProcessOutcome{Started: true, WaitReturned: true, GroupEmpty: false, Uncertain: true}, nil); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := e.Restrict(ctx, RestrictCommand{SchemaVersion: "1", CommandID: newID("command"), Scope: "run", ScopeID: runID, Kind: "pause", Reason: "owner pause"}); err != nil {
					t.Fatal(err)
				}
			}
			before := driverRun(t, e, runID)
			raiseStorageBudget(t, e, 4<<20)
			_ = e.Drive(ctx, runID)
			after := driverRun(t, e, runID)
			if len(after.Attempts) != len(before.Attempts) || unknown && (!after.HasUnresolvedEffects || after.Status != "uncertain") || !unknown && after.Status == "completed" {
				t.Fatal("budget raise bypassed safety state", after.Status)
			}
		})
	}
}

func TestStorageBudgetReviewProtectsIdentityEpochConfigurationAndRequest(t *testing.T) {
	t.Parallel()
	e := artifactEngine(t)
	ctx := context.Background()
	review, err := e.PrepareStorageBudget(ctx, 1<<30, "preserve evidence")
	if err != nil {
		t.Fatal(err)
	}
	original, err := e.Store.StorageBudget(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"authority", "epoch", "generation", "limit", "target", "reason"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			observation := original
			request := StorageBudgetRequest{CommandID: "command:review", TargetBytes: review.TargetBytes, Reason: review.Reason, ExpectedReviewDigest: review.ReviewDigest}
			switch field {
			case "authority":
				observation.Control.AuthorityID += "different"
			case "epoch":
				observation.Control.Epoch++
			case "generation":
				observation.Control.Generation++
			case "limit":
				observation.Control.LimitBytes++
			case "target":
				request.TargetBytes++
			case "reason":
				request.Reason += "changed"
			}
			if err := validateStorageBudgetReview(observation, request); driverFailureCode(err, "") != "storage_budget_review_conflict" {
				t.Fatal("unprotected selector", field, err)
			}
		})
	}
}

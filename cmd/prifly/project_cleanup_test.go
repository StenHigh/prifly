package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/local"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

type cleanupFixture struct {
	run                                string
	outcomeErr, releaseErr, packageErr error
	released, packages                 int
	claim                              prifly.WorktreeClaim
}

func (f *cleanupFixture) StartOutcome(context.Context, string) (string, error) {
	return f.run, f.outcomeErr
}
func (f *cleanupFixture) RollbackWorktreeClaim(_ context.Context, r prifly.ClaimReleaseRequest) (prifly.WorktreeClaim, error) {
	f.released++
	if r.Generation != f.claim.Generation || r.ClaimID != f.claim.ID {
		panic("wrong cleanup identity")
	}
	if f.releaseErr == nil {
		f.claim.Status = "released"
	}
	return f.claim, f.releaseErr
}
func (f *cleanupFixture) Claims(context.Context) (prifly.ClaimRecord, error) {
	return prifly.ClaimRecord{Claims: []prifly.WorktreeClaim{f.claim}}, nil
}
func (f *cleanupFixture) SetPackageStatus(context.Context, prifly.PackageLifecycleRequest) (local.AuthorityApplyResult, error) {
	f.packages++
	return local.AuthorityApplyResult{}, f.packageErr
}

func TestProjectStartCleanupPreservesOutcomeAndOriginalProblem(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"released", "existing", "committed-response-lost", "unknown", "release-failed", "package-failed", "before-start", "import-failed", "installed-path-failed"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			fixture := &cleanupFixture{claim: prifly.WorktreeClaim{ID: "claim:cleanup", Generation: 7, Status: "active"}}
			created := true
			imported := true
			originalCode := "definition_drift"
			originalExit := 3
			switch scenario {
			case "existing":
				created = false
			case "committed-response-lost":
				fixture.run = "run:admitted"
				fixture.claim.RunID = fixture.run
			case "unknown":
				fixture.outcomeErr = local.ErrIntegrity
			case "release-failed":
				fixture.releaseErr = &prifly.Fault{Code: "claim_identity_conflict", Message: "directory changed"}
				fixture.claim.Status = "releasing"
			case "package-failed":
				fixture.packageErr = &prifly.Fault{Code: "storage_busy", Message: "busy"}
			case "before-start":
			case "import-failed":
				imported = false
				originalCode = "dependency_limit"
				originalExit = 2
			case "installed-path-failed":
				originalCode = "project_start_package_not_installed"
				originalExit = 2
			}
			original := &prifly.Fault{Code: originalCode, Message: "original refusal"}
			err := projectStartCleanupWith(context.Background(), fixture, "command:start", &fixture.claim, created, imported, "example:package/work", "1.0.0", original)
			if !errors.Is(err, original) {
				t.Fatal("original cause lost")
			}
			problem, exit := makeProblem(err)
			if problem.Code != originalCode || exit != originalExit || !strings.Contains(problem.Message, "original refusal") {
				t.Fatalf("original Problem changed: %+v exit=%d", problem, exit)
			}
			preserve := scenario == "existing" || scenario == "committed-response-lost" || scenario == "unknown"
			if preserve && (fixture.released != 0 || (scenario != "existing" && fixture.packages != 0)) {
				t.Fatal("preserved ownership was mutated")
			}
			if scenario == "release-failed" && (fixture.packages != 0 || !strings.Contains(problem.Message, "observed status releasing") || !strings.Contains(problem.Message, "claim_identity_conflict")) {
				t.Fatalf("hidden cleanup failure: %+v", problem)
			}
			if scenario == "package-failed" && !strings.Contains(problem.Message, "package cleanup failed: storage_busy") {
				t.Fatal("package failure hidden")
			}
			if scenario == "released" && !strings.Contains(problem.Message, "released claim claim:cleanup generation 7") {
				t.Fatal("release not confirmed")
			}
			data, _ := json.Marshal(problem)
			var doc map[string]any
			_ = json.Unmarshal(data, &doc)
			if len(doc) != 7 {
				t.Fatalf("Problem/1 changed: %s", data)
			}
		})
	}
}

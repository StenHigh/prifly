package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func TestCLIProjectSessionLimitsPrepareShowsPinnedPolicies(t *testing.T) {
	root, authority, legacy := projectSessionLimitsFixture(t)
	const folder = ".prifly/workflows/limits/"
	timed := strings.Replace(legacy, "prifly-step/1", "prifly-step/2", 1)
	writeFixtureFile(t, root, folder+"steps/inspect.yaml", timed)
	writeFixtureFile(t, root, folder+"steps/legacy.yaml", strings.Replace(legacy, "test:step/inspect", "test:step/legacy", 1))
	// An owned definition is not necessarily reachable from the selected root.
	writeFixtureFile(t, root, folder+"steps/unused.yaml", strings.Replace(timed, "test:step/inspect", "test:step/unused", 1)+"session_limits: {active_timeout_ms: 99999999}\n")
	workflow, err := os.ReadFile(filepath.Join(root, folder, "workflow.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(workflow), "on: {pass: done}", "on: {pass: legacy}", 1)
	text = strings.Replace(text, "max_step_instances: 1", "max_step_instances: 2", 1)
	text = strings.Replace(text, "stages:\n", "stages:\n  legacy: {kind: step, step_ref: \"{{step_legacy}}\", on: {pass: done}}\n", 1)
	writeFixtureFile(t, root, folder+"workflow.yaml", text)
	prepare := func() projectLaunchSummary {
		t.Helper()
		code, out, stderr := runCLI(t, "project", "questionnaire", "--repository", root, "--launch", "limits", "--host", "codex-app", "--prepare")
		if code != 0 {
			t.Fatalf("prepare timed launch: %d %s", code, stderr)
		}
		var summary projectLaunchSummary
		if err := json.Unmarshal([]byte(out), &summary); err != nil {
			t.Fatal(err)
		}
		return summary
	}
	reviewed := prepare()
	before, _ := json.Marshal(reviewed)
	if reviewed.SchemaVersion != "project-launch-summary/2" || len(reviewed.SessionLimits) != 2 {
		t.Fatalf("summary omitted selected time policies or included an unused definition: %+v", reviewed.SessionLimits)
	}
	for _, item := range reviewed.SessionLimits {
		if !strings.HasPrefix(item.DefinitionRef.Version, "0.0.0-b1.") || item.DefinitionRef.Digest == "" {
			t.Fatalf("policy did not name its exact compiled definition: %+v", item)
		}
		switch item.DefinitionRef.ID {
		case "test:step/inspect":
			if item.Limits == nil || item.Limits.ActiveTimeoutMS != 3600000 || item.Limits.DecisionWaitTimeoutMS != nil || item.LegacyAbsoluteTimeoutMS != 0 {
				t.Fatalf("timed defaults were not distinguished from an absolute deadline: %+v", item)
			}
		case "test:step/legacy":
			if item.Limits != nil || item.LegacyAbsoluteTimeoutMS != 3600000 {
				t.Fatalf("legacy deadline was falsely labelled pause-aware: %+v", item)
			}
		default:
			t.Fatalf("summary includes an unselected definition: %+v", item)
		}
	}
	writeFixtureFile(t, root, folder+"steps/inspect.yaml", timed+"session_limits: {active_timeout_ms: 7200000, decision_wait_timeout_ms: 86400000}\n")
	changed := prepare()
	if changed.ReviewDigest == reviewed.ReviewDigest || changed.Workflow == reviewed.Workflow {
		t.Fatal("changed pinned time policy kept the previous reviewed launch")
	}
	found := false
	for _, item := range changed.SessionLimits {
		if item.DefinitionRef.ID == "test:step/inspect" {
			found = item.Limits != nil && item.Limits.ActiveTimeoutMS == 7200000 && item.Limits.DecisionWaitTimeoutMS != nil && *item.Limits.DecisionWaitTimeoutMS == 86400000
		}
	}
	if !found {
		t.Fatal("summary did not use the newly compiled time policy")
	}
	after, _ := json.Marshal(reviewed)
	if !bytes.Equal(before, after) {
		t.Fatal("a later preparation mutated the prior review")
	}
	code, _, stderr := runCLI(t, "project", "start", "--repository", root, "--launch", "limits", "--host", "codex-app", "--expected-launch-digest", reviewed.ReviewDigest)
	if code == 0 || !strings.Contains(stderr, "project_start_stale_launch") {
		t.Fatalf("stale reviewed time policy was admitted: %d %s", code, stderr)
	}
	engine, err := prifly.Open(authority, true)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	packages, err := engine.Packages(context.Background())
	if err != nil || len(packages.Packages) != 0 {
		t.Fatalf("review or stale launch imported packages: %+v %v", packages, err)
	}
	runs, err := engine.Runs(context.Background())
	if err != nil || len(runs) != 0 {
		t.Fatalf("review or stale launch created a Run: %+v %v", runs, err)
	}
}

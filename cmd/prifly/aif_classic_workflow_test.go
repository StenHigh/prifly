package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAIFClassicGateFindingsReachRepairDecision(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("test source path unavailable")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	for _, name := range []string{"verify-once.yaml", "review-once.yaml"} {
		raw, err := projectYAMLDocument(filepath.Join(root, ".prifly", "workflows", "aif-classic", "workflows", name))
		if err != nil {
			t.Fatal(err)
		}
		workflow := raw.(map[string]any)
		verify := workflow["stages"].(map[string]any)[strings.TrimSuffix(name, "-once.yaml")].(map[string]any)
		if got := verify["on"].(map[string]any)["needs_revision"]; got != "decide" {
			t.Errorf("%s routes a usable gate needs_revision to %q, want decide", name, got)
		}
	}
}

func TestAIFClassicAndRunnerDescribeOneControlLoop(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("test source path unavailable")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	bridge, err := os.ReadFile(filepath.Join(root, ".prifly", "workflows", "aif-classic", "contexts", "aif-verify-bridge.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(bridge), "`needs_revision` skips") {
		t.Fatal("verify bridge still documents the terminal repair bypass")
	}
	for _, required := range []string{"After every accepted session report", "`program`, call", "waiting or terminal state", "actually provides a separate-session mechanism"} {
		if !strings.Contains(projectRunnerSkill(projectHostByID(t, "codex-app")), required) {
			t.Errorf("Codex runner lacks %q", required)
		}
	}
}

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

// A runner is a generated file, and `project runners update` exists to replace
// exactly those. The accepted set was derived from the current template by
// removing its most recent blocks, so editing the template stopped recognising
// every file earlier releases had written: update refused it as customized, and
// deleting it refused too. A repository whose runner was one release old had no
// way forward at all.
func TestProjectRunnerUpdateReplacesEveryReleasedRunner(t *testing.T) {
	for index := range projectKnownRunnerSkills(projectHosts[0]) {
		root := t.TempDir()
		for _, host := range projectHosts {
			known := projectKnownRunnerSkills(host)[index]
			if !projectRunnerSkillAccepted(host, known) {
				t.Fatalf("a runner an earlier release wrote is not accepted: host=%s variant=%d", host.ID, index)
			}
			path := filepath.Join(projectRunnerPath(root, host), "SKILL.md")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(known), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		updated, err := updateProjectRunners(root, projectHosts...)
		if err != nil {
			t.Fatalf("variant %d could not be updated: %v", index, err)
		}
		if len(updated) != len(projectHosts) {
			t.Fatalf("variant %d updated %d of %d runners", index, len(updated), len(projectHosts))
		}
		for _, host := range projectHosts {
			data, err := os.ReadFile(filepath.Join(projectRunnerPath(root, host), "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != projectRunnerSkill(host) {
				t.Fatalf("variant %d left %s on an older runner", index, host.ID)
			}
		}
	}
}

// The pin is how the next edit finds out that it has to freeze the text it is
// replacing. Changing the runner without adding the old text to
// projectKnownRunnerSkills leaves every installed runner unreplaceable.
func TestProjectRunnerTextIsPinned(t *testing.T) {
	pinned := map[string]string{
		"codex-cli":   "sha256:ad7b4782ffa2d341350a2ef6890da52ff19d70bb3da05f08f0e4ee52a2ae74dc",
		"codex-app":   "sha256:0fecbf3f6b3b67b2347896025b6f0e28f64d7cf6002b5151790bcb8352623376",
		"claude-code": "sha256:416af8429794e5adef4b7180427c3b74b517404b44f36be226f752aa0f61196d",
	}
	for _, host := range projectHosts {
		sum := sha256.Sum256([]byte(projectRunnerSkill(host)))
		digest := "sha256:" + hex.EncodeToString(sum[:])
		if pinned[host.ID] != digest {
			t.Errorf("the runner for %s changed to %s; freeze the text it replaces in projectKnownRunnerSkills, then update this pin", host.ID, digest)
		}
	}
}

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
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
		"codex-cli":   "sha256:89bbc29743b3f7cf76949561a7ebde59e8a11ff5925b0ba87680f78bfb9fa3e0",
		"codex-app":   "sha256:623f100a194d00bca69181fd59af5fa776d88bd9115c57b8d65b133923fc4ef1",
		"claude-code": "sha256:49504df04d59ad25fe877095892dac4b7bed707b7e57447637358d0b82b87884",
	}
	for _, host := range projectHosts {
		sum := sha256.Sum256([]byte(projectRunnerSkill(host)))
		digest := "sha256:" + hex.EncodeToString(sum[:])
		if pinned[host.ID] != digest {
			t.Errorf("the runner for %s changed to %s; freeze the text it replaces in projectKnownRunnerSkills, then update this pin", host.ID, digest)
		}
	}
}

func TestProjectFrozenRunnerTextIsPinned(t *testing.T) {
	// Freeze all five pre-neutral forms, not only the latest released runner.
	pinned := map[string][]string{
		"codex-cli": {
			"sha256:ad7b4782ffa2d341350a2ef6890da52ff19d70bb3da05f08f0e4ee52a2ae74dc",
			"sha256:fa0879021b938800b69fe6bd301839a75d0b581ff7ca8f19c0a16a29c61abb4c",
			"sha256:4e10bebe6b55967cf05003f63dea57ab113506fdd69d63675d84db7c45f7ca89",
			"sha256:5a111db67776578d799fc288752a9c2ee1fdb6ca1d9e5f1bf964f19104187765",
			"sha256:20604265f5d5ad0f4f75d0131b0e8064aa698c6649eadc98c76e8e81957e74b2",
		},
		"codex-app": {
			"sha256:0fecbf3f6b3b67b2347896025b6f0e28f64d7cf6002b5151790bcb8352623376",
			"sha256:79cbfffcd27e4a71ea97aed386f8672f91f2a2edcd1df577aaefdc661a01e912",
			"sha256:7347b203d297e07689b96ea682cf3b482354538acb58ecdff3bd8ae2294e9ebc",
			"sha256:2f8a6075ebb11414d06863af864fd45f035e15fe45dc38678d2ab662b39567e2",
			"sha256:8c4e2ed9eb6f3b461316cb32c63476bfa1987eeb62bc33243d682434f43b971f",
		},
		"claude-code": {
			"sha256:416af8429794e5adef4b7180427c3b74b517404b44f36be226f752aa0f61196d",
			"sha256:2560471f7497503f8d91f9d0fd871f0045ed0ce840d0ba44ee830eb11d2f8ae7",
			"sha256:9851536cfbffa7a42e783499cfbb12d2aebdfa6d2741271003b5d9f6cd548d57",
			"sha256:abed5e09e3a3cae2946798cfb3399a4c3b63d883ab81a690f4850aa918cc5af3",
			"sha256:2817611582ef5058c919a8061f3761a737cba66b45f91ac47943cb4605d8dc3d",
		},
	}
	for _, host := range projectHosts {
		variants := projectKnownRunnerSkills(host)
		for index, text := range variants {
			sum := sha256.Sum256([]byte(text))
			digest := "sha256:" + hex.EncodeToString(sum[:])
			if len(pinned[host.ID]) != len(variants) || pinned[host.ID][index] != digest {
				t.Errorf("frozen host=%s variant=%d digest=%s", host.ID, index, digest)
			}
		}
	}
}

func TestProjectCurrentRunnerIsWorkflowNeutral(t *testing.T) {
	for _, host := range projectHosts {
		t.Run(host.ID, func(t *testing.T) {
			skill := projectRunnerSkill(host)
			// A one-task package must not acquire an industry process from its
			// host instructions. Its actual graph remains the package's concern.
			for _, absent := range []string{"AI Factory", "aif-", "reviewer", "improve", "commit", "successful exits are silent", "{{host}}", "{{question_tool}}"} {
				if strings.Contains(skill, absent) {
					t.Errorf("generic runner adds %q", absent)
				}
			}
			for _, required := range []string{
				"--host " + host.ID, "--prepare", "--expected-launch-digest DIGEST", "review_digest",
				"--expected-decision-catalog-digest DIGEST", "--preflight-answer ID=JSON", "--runtime-answer ID=JSON",
				"No Git work in /3 means no workspace\n   question or claim", "optional runtime",
				"Respect declared defaults", "not a guessed value", "Prepare is read-only",
				"same selected host", "exactly the prepared arguments", "never drop a stale-digest check",
				"session task --run RUN_ID --all", "permitted_effects", "pending_request_digest",
				"For an undeclared native skill question, stop that task", "do not choose a hidden model answer",
				"not present actor provenance as proof", "local owner and host can share the same OS",
				"launch_summary when present and decision ledger", "project_profile_version",
				"For legacy /2, do not pass --prepare or --expected-launch-digest",
			} {
				if !strings.Contains(strings.ToLower(skill), strings.ToLower(required)) {
					t.Errorf("generic runner lost %q", required)
				}
			}
			prepare := strings.Index(skill, "project questionnaire --repository \"$PWD\" --launch ID --prepare")
			show := strings.Index(skill, "show the returned project-launch-summary/1 before starting")
			start := strings.Index(skill, "project start --repository")
			if prepare < 0 || show <= prepare || start <= show {
				t.Fatal("start can precede preparation and presentation of the summary")
			}
			tool, other := "request_user_input", "AskUserQuestion"
			if host.ID == "claude-code" {
				tool, other = other, tool
			}
			if !strings.Contains(skill, tool) || strings.Contains(skill, other) {
				t.Fatal("runner selected the wrong native question tool")
			}
		})
	}
}

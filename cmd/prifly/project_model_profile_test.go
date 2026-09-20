package main

import (
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

// The exact block the packaging session will ship in aif-profiled: three
// hosts, nine entries. The project that installs it usually declares one of
// those hosts, which is why the key is checked against the hosts this build
// knows rather than the hosts the project declared -- the first reading would
// refuse every installation missing codex-app.
const packagedModelProfiles = `model_profiles:
  claude-code:
    deep-reasoning: {model: opus, effort: high}
    careful-review: {model: opus, effort: medium}
    fast-draft: {model: haiku, effort: low}
  codex-cli:
    deep-reasoning: {model: gpt-5, reasoning_effort: high}
    careful-review: {model: gpt-5, reasoning_effort: medium}
    fast-draft: {model: gpt-5-mini, reasoning_effort: low}
  codex-app:
    deep-reasoning: {model: gpt-5, reasoning_effort: high}
    careful-review: {model: gpt-5, reasoning_effort: medium}
    fast-draft: {model: gpt-5-mini, reasoning_effort: low}
`

func readModelProfilesFromYAML(t *testing.T, source string) (map[string]map[string]map[string]string, error) {
	t.Helper()
	value, err := flow.Parse([]byte(source), "yaml")
	if err != nil {
		t.Fatal(err)
	}
	return projectReadModelProfiles(value.(map[string]any)["model_profiles"])
}

func TestAPackageCarriesModelProfilesForEveryHostItMayMeet(t *testing.T) {
	profiles, err := readModelProfilesFromYAML(t, packagedModelProfiles)
	if err != nil {
		t.Fatalf("the block a shared package ships was refused: %v", err)
	}
	if len(profiles) != 3 {
		t.Fatalf("three hosts were declared and %d survived", len(profiles))
	}
	if got := profiles["claude-code"]["careful-review"]["model"]; got != "opus" {
		t.Fatalf("claude-code careful-review reads %q", got)
	}
	if got := profiles["codex-cli"]["fast-draft"]["reasoning_effort"]; got != "low" {
		t.Fatalf("codex-cli fast-draft reads %q", got)
	}
	// Every key is carried as written: this tool reads no meaning into them,
	// so it must not normalise, lowercase or drop one it does not recognise.
	if len(profiles["codex-app"]["deep-reasoning"]) != 2 {
		t.Fatalf("an opaque entry lost a key: %+v", profiles["codex-app"]["deep-reasoning"])
	}
}

func TestModelProfilesRefuseOnlyWhatIsNotAHostThisBuildKnows(t *testing.T) {
	for _, c := range []struct{ name, source, code string }{
		{"typo in a host", "model_profiles:\n  claude_code:\n    fast-draft: {model: haiku}\n", "project_model_profile_unknown_host"},
		{"profile name a step could not declare", "model_profiles:\n  claude-code:\n    Fast_Draft: {model: haiku}\n", "project_model_profile_invalid"},
		{"value that is not a string", "model_profiles:\n  claude-code:\n    fast-draft: {effort: 3}\n", "project_model_profile_invalid"},
		{"entry with nothing in it", "model_profiles:\n  claude-code:\n    fast-draft: {}\n", "project_model_profile_invalid"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := readModelProfilesFromYAML(t, c.source)
			if err == nil || !strings.Contains(err.Error(), c.code) {
				t.Fatalf("expected %s, got %v", c.code, err)
			}
		})
	}
}

// A machine entry replaces the package entry whole. Merging key by key would
// produce a value neither file contains -- and `source`, which names one file,
// would then be describing half of it.
func TestAMachineEntryReplacesThePackageEntryWhole(t *testing.T) {
	packaged := map[string]map[string]map[string]string{
		"claude-code": {"deep-reasoning": {"model": "opus", "effort": "high"}, "fast-draft": {"model": "haiku"}},
	}
	local := map[string]map[string]map[string]string{
		"claude-code": {"deep-reasoning": {"effort": "medium"}},
	}
	merged := mergeModelProfiles(packaged, local, "claude-code")
	deep := merged["deep-reasoning"]
	if deep.Source != "local" {
		t.Fatalf("the machine entry did not win: %+v", deep)
	}
	if len(deep.Values) != 1 || deep.Values["effort"] != "medium" {
		t.Fatalf("the two entries were mixed into a value nobody wrote: %+v", deep.Values)
	}
	untouched := merged["fast-draft"]
	if untouched.Source != "project_default" || untouched.Values["model"] != "haiku" {
		t.Fatalf("an entry the machine said nothing about changed: %+v", untouched)
	}
	if got := mergeModelProfiles(packaged, local, "codex-cli"); len(got) != 0 {
		t.Fatalf("a host neither file describes produced %d entries", len(got))
	}
}

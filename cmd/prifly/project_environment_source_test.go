package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

const environmentSourceSecret = "p@ss=word#1"

func environmentSourceProject(t *testing.T) (root, dotenv, keyfile string) {
	t.Helper()
	root, state := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	if code, _, stderr := runCLI(t, "project", "init", "--repository", root, "--state-root", state); code != 0 {
		t.Fatalf("init: %d %s", code, stderr)
	}
	secrets := t.TempDir()
	writeFixtureFile(t, secrets, "env", "# the machine's own file\nOTHER=ignored\nPASSWORD="+environmentSourceSecret+"\nPASSWORD=second\n")
	writeFixtureFile(t, secrets, "token", environmentSourceSecret+"\n")
	// The tool records the path it resolved, so the test asks for the same one.
	resolved, err := filepath.EvalSymlinks(secrets)
	if err != nil {
		t.Fatal(err)
	}
	return root, filepath.Join(resolved, "env"), filepath.Join(resolved, "token")
}

func TestProjectLocalEnvironmentSourcesRecordThePlaceNotTheValue(t *testing.T) {
	root, dotenv, keyfile := environmentSourceProject(t)
	code, stdout, stderr := runCLI(t, "project", "local", "set", "--repository", root,
		"--env", "APP_ENV=test",
		"--env-from", "UPSTREAM_TOKEN=env:CI_TOKEN",
		"--env-from", "KEY_FILE=file:"+keyfile,
		"--env-from", "DB_PASSWORD=dotenv:"+dotenv+":PASSWORD")
	if code != 0 {
		t.Fatalf("declare sources: %d %s", code, stderr)
	}
	for _, place := range []string{"env:CI_TOKEN", "file:" + keyfile, "dotenv:" + dotenv + ":PASSWORD"} {
		if !strings.Contains(stdout, place) {
			t.Fatalf("receipt hid the declared place %q: %s", place, stdout)
		}
	}
	if strings.Contains(stdout, environmentSourceSecret) {
		t.Fatalf("receipt printed the value: %s", stdout)
	}
	local, err := os.ReadFile(filepath.Join(root, ".prifly", "local.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(local), environmentSourceSecret) {
		t.Fatalf("declaring a source copied the value into local.yaml: %s", local)
	}
	environment, err := projectLocalEnvironment(root)
	if err != nil || len(environment) != 1 || environment["APP_ENV"] != "test" {
		t.Fatalf("literal environment changed: %#v %v", environment, err)
	}
	sources, err := projectLocalEnvironmentSources(root)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]prifly.EnvironmentSource{
		"UPSTREAM_TOKEN": {Env: "CI_TOKEN"},
		"KEY_FILE":       {File: keyfile},
		"DB_PASSWORD":    {DotEnv: dotenv, Key: "PASSWORD"},
	}
	if len(sources) != len(want) {
		t.Fatalf("declared sources: %#v", sources)
	}
	for name, source := range want {
		if sources[name] != source {
			t.Fatalf("%s read back as %#v, expected %#v", name, sources[name], source)
		}
	}
}

func TestProjectLocalEnvironmentSourceRefusesUnusableDeclarations(t *testing.T) {
	root, dotenv, keyfile := environmentSourceProject(t)
	for _, test := range []struct{ name, argument string }{
		{"no-source", "TOKEN="},
		{"unknown-kind", "TOKEN=vault:secret/token"},
		{"dotenv-without-key", "TOKEN=dotenv:" + dotenv},
		{"relative-file", "TOKEN=file:relative/path"},
		{"engine-name", "PRIFLY_TOKEN=env:CI_TOKEN"},
		{"no-name", "=env:CI_TOKEN"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(root, ".prifly", "local.yaml")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			code, _, stderr := runCLI(t, "project", "local", "set", "--repository", root, "--env-from", test.argument)
			if code == 0 || !strings.Contains(stderr, "project_local_invalid") {
				t.Fatalf("unusable declaration admitted: %d %s", code, stderr)
			}
			after, err := os.ReadFile(path)
			if err != nil || string(before) != string(after) {
				t.Fatalf("refused declaration still changed local.yaml: %v", err)
			}
		})
	}
	if code, _, stderr := runCLI(t, "project", "local", "set", "--repository", root, "--env-from", "TOKEN=env:CI_TOKEN", "--env-from", "TOKEN=file:"+keyfile); code == 0 || !strings.Contains(stderr, "duplicate name") {
		t.Fatalf("two sources for one name admitted: %d %s", code, stderr)
	}
	// A hand-edited file is refused on the next read rather than at the start
	// of a Run that has already claimed a workspace.
	base, err := os.ReadFile(filepath.Join(root, ".prifly", "local.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ name, text string }{
		{"value-and-source", "environment: {TOKEN: written-down}\nenvironment_from: {TOKEN: {env: CI_TOKEN}}\n"},
		{"two-places", "environment_from: {TOKEN: {env: CI_TOKEN, file: " + keyfile + "}}\n"},
		{"unknown-field", "environment_from: {TOKEN: {vault: secret/token}}\n"},
		{"not-an-object", "environment_from: {TOKEN: env:CI_TOKEN}\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			writeFixtureFile(t, root, ".prifly/local.yaml", string(base)+test.text)
			if _, err := projectLocalEnvironmentSources(root); err == nil {
				t.Fatal("unusable hand-written declaration admitted")
			}
		})
	}
}

// The reviewed launch is invalidated by a changed source and not by a changed
// value: the value is never part of what was reviewed.
func TestProjectLaunchDigestFollowsTheSourceNotTheValue(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	secrets := t.TempDir()
	dotenv := filepath.Join(secrets, "env")
	writeFixtureFile(t, secrets, "env", "PASSWORD=first\n")
	config := prifly.ExecutorConfig{Executable: executable, Args: []string{}, Files: map[string]string{}, Environment: map[string]string{}, TimeoutMS: 1000, GraceMS: 30, MaxOutputBytes: 1 << 20,
		EnvironmentFrom: map[string]prifly.EnvironmentSource{"DB_PASSWORD": {DotEnv: dotenv, Key: "PASSWORD"}}}
	review := func(config prifly.ExecutorConfig) projectExecutionReview {
		t.Helper()
		items, err := projectReviewExecutors(&prifly.ExecutionBindings{SchemaVersion: prifly.ExecutionBindingsSourceVersion, Bindings: []prifly.ExecutionBinding{{Config: config, Files: map[string][]byte{}}}})
		if err != nil || len(items) != 1 {
			t.Fatalf("review executors: %#v %v", items, err)
		}
		return items[0]
	}
	before := review(config)
	writeFixtureFile(t, secrets, "env", "PASSWORD=second\n")
	after := review(config)
	if before.ConfigurationDigest != after.ConfigurationDigest {
		t.Fatal("a changed value invalidated a reviewed launch")
	}
	if len(after.EnvironmentSources) != 1 || after.EnvironmentSources["DB_PASSWORD"] != "dotenv:"+dotenv+":PASSWORD" {
		t.Fatalf("review hid where the value comes from: %#v", after.EnvironmentSources)
	}
	elsewhere := config
	elsewhere.EnvironmentFrom = map[string]prifly.EnvironmentSource{"DB_PASSWORD": {DotEnv: dotenv, Key: "OTHER"}}
	if review(elsewhere).ConfigurationDigest == before.ConfigurationDigest {
		t.Fatal("a changed source left the reviewed launch valid")
	}
}

// The receipt describes the file, not the arguments of this call: printing
// only what was passed read as "the rest is gone" to a cold start that had
// just declared a source and saw its allowed programs disappear.
func TestProjectLocalReceiptDescribesTheFile(t *testing.T) {
	root, dotenv, _ := environmentSourceProject(t)
	if code, stdout, stderr := runCLI(t, "project", "local", "set", "--repository", root, "--allow-executable", "shell=/bin/sh", "--env", "APP_ENV=testing"); code != 0 || !strings.Contains(stdout, `"shell":"/bin/sh"`) {
		t.Fatalf("allow an executable: %d %s %s", code, stdout, stderr)
	}
	code, stdout, stderr := runCLI(t, "project", "local", "set", "--repository", root, "--env-from", "DB_PASSWORD=dotenv:"+dotenv+":PASSWORD")
	if code != 0 {
		t.Fatalf("declare a source: %d %s", code, stderr)
	}
	// The change reaches the next launch, not a Run already sealed, and the
	// note says so where it is made — on stderr, so the document is untouched.
	if !strings.Contains(stderr, "sealed when its Run starts") || strings.Contains(stdout, "sealed when its Run starts") {
		t.Fatalf("the note is missing or in the document: out=%s err=%s", stdout, stderr)
	}
	for _, kept := range []string{`"shell":"/bin/sh"`, `"APP_ENV":"testing"`, `"DB_PASSWORD":"dotenv:` + dotenv + `:PASSWORD"`} {
		if !strings.Contains(stdout, kept) {
			t.Fatalf("the receipt reads as dropping %s: %s", kept, stdout)
		}
	}
	if strings.Contains(stdout, environmentSourceSecret) {
		t.Fatalf("the receipt printed the value: %s", stdout)
	}
}

// An authority a launch cannot use is replaced by the one command that writes a
// usable one. A cold start read the code in the message, "help" in the actions,
// and went looking for a profile flag on prifly init, which is not it.
func TestIncompatibleAuthorityNamesTheCommandThatWritesOne(t *testing.T) {
	state := filepath.Join(t.TempDir(), "authority")
	if code, _, stderr := runCLI(t, "init", state); code != 0 {
		t.Fatalf("plain authority: %d %s", code, stderr)
	}
	engine, err := prifly.Open(state, true)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	err = checkProjectAuthority(engine)
	if err == nil {
		t.Fatal("an authority a launch cannot use was accepted")
	}
	problem, _ := prifly.ProblemFor(err)
	if problem.Code != "authority_profile_incompatible" && problem.Code != "authority_configuration_incompatible" {
		t.Fatalf("the refusal is not named: %+v", problem)
	}
	if !slices.Contains(problem.SafeNextActions, "project.init") {
		t.Fatalf("the refusal does not name the command that writes a usable authority: %+v", problem.SafeNextActions)
	}
	if !strings.Contains(problem.Message, "project init") {
		t.Fatalf("the message does not spell the command: %s", problem.Message)
	}
}

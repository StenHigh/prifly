package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func gitFixture(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func fixtureWorkflowYAML(name string) string {
	return `authoring: prifly-project-workflow/1
package:
  id: test:package/` + name + `
  version: 1.0.0
  description: Fixture workflow ` + name + `.
  requires_core_protocol: "1"
  references:
    local_policy: core:policy/local@3.0.0
id: test:workflow/` + name + `
version: 1.0.0
title: Fixture ` + name + `
refs:
  local_policy: "{{local_policy}}"
inputs: {}
outputs: {}
entry: done
limits: {max_step_instances: 1, max_control_transitions: 1}
policy_ref: local_policy
stages:
  done: {kind: finish, outcome: succeeded}
`
}

func writeFixtureFile(t *testing.T, root, name, text string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
}

func writeFixtureWorkflowFolder(t *testing.T, root, folder, name string) {
	t.Helper()
	writeFixtureFile(t, root, folder+"/workflow.yaml", fixtureWorkflowYAML(name))
	writeFixtureFile(t, root, folder+"/extend.yaml", "extensions: []\n")
	writeFixtureFile(t, root, folder+"/README.md", "# "+name+"\n")
}

// newWorkflowRepositoryFixture builds a local Git repository that plays the
// remote. allowAnySHA1InWant mirrors what hosted services permit for shallow
// fetches by commit.
func newWorkflowRepositoryFixture(t *testing.T, folders ...string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	source := filepath.Join(t.TempDir(), "source")
	if err := os.MkdirAll(source, 0755); err != nil {
		t.Fatal(err)
	}
	gitFixture(t, source, "init", "-q", "-b", "main")
	writeFixtureFile(t, source, "README.md", "# fixture\n")
	for _, folder := range folders {
		writeFixtureWorkflowFolder(t, source, folder, filepath.Base(folder))
	}
	gitFixture(t, source, "add", ".")
	gitFixture(t, source, "commit", "-q", "-m", "fixture")
	gitFixture(t, source, "config", "uploadpack.allowAnySHA1InWant", "true")
	return source
}

func newProjectFixture(t *testing.T) (repository, authority string) {
	t.Helper()
	repository = filepath.Join(t.TempDir(), "repository")
	if err := os.MkdirAll(repository, 0755); err != nil {
		t.Fatal(err)
	}
	gitFixture(t, repository, "init", "-q", "-b", "main")
	authority = filepath.Join(t.TempDir(), "authority")
	var out, errout bytes.Buffer
	if code := execute(context.Background(), []string{"project", "init", "--repository", repository, "--state-root", authority, "--json"}, &out, &errout); code != 0 {
		t.Fatalf("project init %d: %s", code, errout.String())
	}
	return repository, authority
}

func runCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errout bytes.Buffer
	code := execute(context.Background(), append(args, "--json"), &out, &errout)
	return code, out.String(), errout.String()
}

func TestProjectWorkflowSourceParsing(t *testing.T) {
	accepted := map[string]projectWorkflowSource{
		"aif-classic":                            {catalogEntry: "aif-classic"},
		"owner/repo":                             {repository: "https://github.com/owner/repo.git"},
		"owner/repo.git":                         {repository: "https://github.com/owner/repo.git"},
		"https://github.com/stenhigh/prifly.git": {repository: "https://github.com/stenhigh/prifly.git"},
		"ssh://git@github.com/owner/repo.git":    {repository: "ssh://git@github.com/owner/repo.git"},
		"git@github.com:owner/repo.git":          {repository: "git@github.com:owner/repo.git"},
		"file:///srv/git/workflows.git":          {repository: "file:///srv/git/workflows.git"},
		"/srv/git/workflows":                     {repository: "/srv/git/workflows"},
	}
	for input, want := range accepted {
		got, err := parseProjectWorkflowSource(input)
		if err != nil || got != want {
			t.Fatalf("%q: %v %+v", input, err, got)
		}
	}
	for _, input := range []string{"", "-flag", "./workflows", "../workflows", "Owner", "https://user:token@github.com/owner/repo.git", "https://token@github.com/owner/repo.git", "ext::sh -c id", "git://github.com/owner/repo.git", "http://github.com/owner/repo.git", "https://github.com/owner/repo with space"} {
		if _, err := parseProjectWorkflowSource(input); err == nil || !strings.Contains(err.Error(), "project_workflow_source_invalid") {
			t.Fatalf("%q was accepted: %v", input, err)
		}
	}
	// A bare lowercase word is a catalog entry, never a relative path.
	if source, err := parseProjectWorkflowSource("workflows"); err != nil || source.catalogEntry != "workflows" {
		t.Fatalf("bare name is a catalog entry: %v %+v", err, source)
	}
	if projectRedactCredentials("fatal: https://user:token@example.com/x failed") != "fatal: https://***@example.com/x failed" {
		t.Fatal("credentials were not redacted from git output")
	}
}

func TestProjectProfileOriginIsStrict(t *testing.T) {
	repository, _ := newProjectFixture(t)
	writeFixtureWorkflowFolder(t, repository, ".prifly/workflows/sample", "sample")
	commit := strings.Repeat("a", 40)
	digest := "sha256:" + strings.Repeat("b", 64)
	profile := func(origin string) string {
		return "schema_version: prifly-project-profile/2\n" + projectHostsYAML + "packages:\n  sample:\n    source: .prifly/workflows/sample\n" + origin + "launches:\n  sample:\n    title: Sample\n    description: Sample workflow.\n    kind: workflow\n    workflow: .prifly/workflows/sample/workflow.yaml\n"
	}
	valid := "    origin:\n      repository: /srv/git/workflows\n      path: flows/sample\n      ref: v1\n      commit: " + commit + "\n      digest: " + digest + "\n      catalog: https://github.com/StenHigh/prifly-workflows.git\n"
	writeFixtureFile(t, repository, ".prifly/project.yaml", profile(valid))
	parsed, err := readProjectProfile(repository)
	if err != nil || parsed.Packages["sample"].Origin == nil || parsed.Packages["sample"].Origin.Commit != commit || parsed.Packages["sample"].Origin.Catalog != projectDefaultWorkflowCatalog {
		t.Fatalf("valid origin was not read: %v %+v", err, parsed.Packages["sample"])
	}
	writeFixtureFile(t, repository, ".prifly/project.yaml", profile(""))
	if parsed, err := readProjectProfile(repository); err != nil || parsed.Packages["sample"].Origin != nil {
		t.Fatalf("profile without origin must stay valid: %v", err)
	}
	for name, origin := range map[string]string{
		"unknown field": strings.Replace(valid, "      catalog:", "      signed_by: x\n      catalog:", 1),
		"short commit":  strings.Replace(valid, commit, "abc", 1),
		"bad digest":    strings.Replace(valid, digest, "md5:00", 1),
		"credentials":   strings.Replace(valid, "/srv/git/workflows", "https://user:pw@example.com/repo.git", 1),
		"shorthand":     strings.Replace(valid, "/srv/git/workflows", "owner/repo", 1),
		"traversal":     strings.Replace(valid, "flows/sample", "../outside", 1),
		"extra key":     strings.Replace(valid, "    origin:", "    extra: 1\n    origin:", 1),
	} {
		writeFixtureFile(t, repository, ".prifly/project.yaml", profile(origin))
		if _, err := readProjectProfile(repository); err == nil || !strings.Contains(err.Error(), "project_profile_invalid") {
			t.Fatalf("%s was accepted: %v", name, err)
		}
	}
}

func TestProjectWorkflowTreeDigestIgnoresExtendAndOrder(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "folder")
	writeFixtureWorkflowFolder(t, folder, ".", "sample")
	writeFixtureFile(t, folder, "a-b.yaml", "x\n")
	writeFixtureFile(t, folder, "a/b.yaml", "y\n")
	first, err := projectWorkflowTreeDigest(folder)
	if err != nil || !projectDigestPattern.MatchString(first) {
		t.Fatalf("digest: %v %q", err, first)
	}
	writeFixtureFile(t, folder, "extend.yaml", "exclude: [something]\n")
	if second, err := projectWorkflowTreeDigest(folder); err != nil || second != first {
		t.Fatalf("extend.yaml changed the tree digest: %v %q %q", err, first, second)
	}
	writeFixtureFile(t, folder, "README.md", "# changed\n")
	if third, err := projectWorkflowTreeDigest(folder); err != nil || third == first {
		t.Fatalf("content change did not change the digest: %v", err)
	}
	if err := os.Symlink("README.md", filepath.Join(folder, "link.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := projectWorkflowTreeDigest(folder); err == nil {
		t.Fatal("symlink was hashed as content")
	}
}

func TestCLIProjectWorkflowsAddInstallsFolderFromRepository(t *testing.T) {
	source := newWorkflowRepositoryFixture(t, "flows/sample")
	gitFixture(t, source, "tag", "-a", "v1", "-m", "release")
	commit := gitFixture(t, source, "rev-parse", "v1^{commit}")
	repository, authority := newProjectFixture(t)
	profilePath := filepath.Join(repository, ".prifly", "project.yaml")
	original, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	commented := "# Team profile, keep this comment.\n" + strings.Replace(string(original), "schema_version: prifly-project-profile/2\n", "schema_version: prifly-project-profile/2 # pinned\n", 1)
	if err := os.WriteFile(profilePath, []byte(commented), 0644); err != nil {
		t.Fatal(err)
	}
	code, out, errout := runCLI(t, "project", "workflows", "add", source, "--ref", "v1", "--repository", repository)
	if code != 0 {
		t.Fatalf("add %d: %s", code, errout)
	}
	var result projectWorkflowAddResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if result.SchemaVersion != "project-workflow-add/1" || result.Name != "sample" || result.Folder != ".prifly/workflows/sample" || result.Launch != "sample" || result.Package.ID != "test:package/sample" || result.Package.Title != "Fixture sample" || result.Package.References["local_policy"] != "core:policy/local@3.0.0" || len(result.Next) != 3 {
		t.Fatalf("unexpected add result: %+v", result)
	}
	if result.Origin.Repository != source || result.Origin.Path != "flows/sample" || result.Origin.Ref != "v1" || result.Origin.Commit != commit || !projectDigestPattern.MatchString(result.Origin.Digest) || !projectDigestPattern.MatchString(result.Origin.ExtendDigest) || result.Origin.Catalog != "" {
		t.Fatalf("unexpected origin: %+v", result.Origin)
	}
	for _, name := range []string{"workflow.yaml", "extend.yaml", "README.md"} {
		if _, err := os.Stat(filepath.Join(repository, ".prifly", "workflows", "sample", name)); err != nil {
			t.Fatalf("installed folder misses %s: %v", name, err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(repository, ".prifly", "workflows"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("staging leftovers remain: %v %v", err, entries)
	}
	profile, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Team profile, keep this comment.", "# pinned", "source: .prifly/workflows/sample", "commit: " + commit, "path: flows/sample", "ref: v1", "title: Fixture sample", "workflow: .prifly/workflows/sample/workflow.yaml"} {
		if !strings.Contains(string(profile), want) {
			t.Fatalf("profile misses %q:\n%s", want, profile)
		}
	}
	parsed, err := readProjectProfile(repository)
	if err != nil || parsed.Packages["sample"].Origin == nil || parsed.Packages["sample"].Origin.Commit != commit {
		t.Fatalf("edited profile is not readable: %v", err)
	}
	code, out, errout = runCLI(t, "project", "workflows", "--repository", repository)
	var launches projectWorkflowList
	if code != 0 || json.Unmarshal([]byte(out), &launches) != nil || len(launches.Launches) != 1 || launches.Launches[0].ID != "sample" {
		t.Fatalf("installed launch is not listed: %d %s %s", code, out, errout)
	}
	output := filepath.Join(t.TempDir(), "package")
	if code, _, errout := runCLI(t, "--project", authority, "project", "compile", "--repository", repository, "--package", "sample", "--host", "codex-cli", "--output", output); code != 0 {
		t.Fatalf("installed folder does not compile %d: %s", code, errout)
	}
	if _, err := os.Stat(filepath.Join(output, prifly.PackageManifestFile)); err != nil {
		t.Fatalf("compile produced no sealed manifest: %v", err)
	}
	if code, _, errout := runCLI(t, "project", "workflows", "add", source, "--ref", "v1", "--repository", repository); code == 0 || !strings.Contains(errout, "project_workflow_exists") {
		t.Fatalf("second add overwrote the folder: %d %s", code, errout)
	}
	if code, _, errout := runCLI(t, "project", "workflows", "add", source, "--ref", "v1", "--name", "other", "--repository", repository); code == 0 || !strings.Contains(errout, "project_workflow_package_conflict") {
		t.Fatalf("same package id under another name was accepted: %d %s", code, errout)
	}
	if _, err := os.Stat(filepath.Join(repository, ".prifly", "workflows", "other")); !os.IsNotExist(err) {
		t.Fatalf("refused add left a folder behind: %v", err)
	}
	if code, _, errout := runCLI(t, "project", "workflows", "add", source, "--ref", "v9", "--name", "missing", "--repository", repository); code == 0 || !strings.Contains(errout, "project_workflow_repository_unreachable") {
		t.Fatalf("unknown ref was accepted: %d %s", code, errout)
	}
	// Installing a second copy by exact commit works once the id differs.
	writeFixtureFile(t, source, "flows/sample/workflow.yaml", strings.ReplaceAll(fixtureWorkflowYAML("sample"), "test:package/sample", "test:package/sample-two"))
	gitFixture(t, source, "commit", "-qam", "second package id")
	second := gitFixture(t, source, "rev-parse", "HEAD")
	code, out, errout = runCLI(t, "project", "workflows", "add", source, "--ref", second, "--name", "sample-two", "--repository", repository)
	if code != 0 {
		t.Fatalf("add by commit %d: %s", code, errout)
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil || result.Origin.Ref != second || result.Origin.Commit != second || result.Package.ID != "test:package/sample-two" {
		t.Fatalf("add by commit recorded a different origin: %v %+v", err, result.Origin)
	}
}

func TestCLIProjectWorkflowsAddDiscoveryAndRefusals(t *testing.T) {
	source := newWorkflowRepositoryFixture(t, "flows/alpha", "flows/beta")
	repository, _ := newProjectFixture(t)
	code, _, errout := runCLI(t, "project", "workflows", "add", source, "--repository", repository)
	if code == 0 || !strings.Contains(errout, "project_workflow_repository_ambiguous") || !strings.Contains(errout, "flows/alpha") || !strings.Contains(errout, "flows/beta") {
		t.Fatalf("ambiguous repository was not listed: %d %s", code, errout)
	}
	code, out, errout := runCLI(t, "project", "workflows", "add", source, "--path", "flows/beta", "--repository", repository)
	var result projectWorkflowAddResult
	if code != 0 || json.Unmarshal([]byte(out), &result) != nil || result.Name != "beta" || result.Origin.Ref != "main" || result.Origin.Path != "flows/beta" {
		t.Fatalf("--path install: %d %s %s", code, out, errout)
	}
	if code, _, errout := runCLI(t, "project", "workflows", "add", source, "--path", "flows", "--name", "flows", "--repository", repository); code == 0 || !strings.Contains(errout, "project_workflow_folder_invalid") {
		t.Fatalf("--path without marker was accepted: %d %s", code, errout)
	}
	empty := newWorkflowRepositoryFixture(t)
	if code, _, errout := runCLI(t, "project", "workflows", "add", empty, "--repository", repository); code == 0 || !strings.Contains(errout, "project_workflow_repository_empty") {
		t.Fatalf("empty repository was accepted: %d %s", code, errout)
	}
	linked := newWorkflowRepositoryFixture(t, "flows/linked")
	if err := os.Symlink("README.md", filepath.Join(linked, "flows", "linked", "link.md")); err != nil {
		t.Fatal(err)
	}
	gitFixture(t, linked, "add", ".")
	gitFixture(t, linked, "commit", "-qm", "symlink")
	if code, _, errout := runCLI(t, "project", "workflows", "add", linked, "--repository", repository); code == 0 || !strings.Contains(errout, "symlinks are not allowed") {
		t.Fatalf("symlink was copied: %d %s", code, errout)
	}
	if _, err := os.Stat(filepath.Join(repository, ".prifly", "workflows", "linked")); !os.IsNotExist(err) {
		t.Fatalf("refused folder was left behind: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(repository, ".prifly", "workflows"))
	if err != nil || len(entries) != 1 || entries[0].Name() != "beta" {
		t.Fatalf("staging leftovers after refusals: %v %v", err, entries)
	}
	for _, source := range []string{"./relative", "https://user:token@example.com/repo.git", "ext::sh -c id", "Owner/Repo/extra"} {
		if code, _, errout := runCLI(t, "project", "workflows", "add", source, "--repository", repository); code == 0 || !strings.Contains(errout, "project_workflow_source_invalid") {
			t.Fatalf("%q reached the network: %d %s", source, code, errout)
		}
	}
	if code, _, errout := runCLI(t, "project", "workflows", "add", "--repository", repository, source); code == 0 || !strings.Contains(errout, "invalid_usage") {
		t.Fatalf("flags before SOURCE were accepted: %d %s", code, errout)
	}
	if code, _, errout := runCLI(t, "project", "workflows", "bogus", "--repository", repository); code == 0 || !strings.Contains(errout, "invalid_usage") {
		t.Fatalf("unknown subcommand was accepted: %d %s", code, errout)
	}
}

func TestProjectWorkflowCatalogParsing(t *testing.T) {
	valid := `schema_version: prifly-workflow-catalog/1
title: Test catalog
categories:
  delivery: {title: Delivery, description: Cycles.}
  ops: {title: Operations}
workflows:
  zeta:
    title: Zeta
    description: Last in delivery.
    category: delivery
    repository: https://example.com/zeta.git
    path: flows/zeta
    tags: [z, delivery]
  alpha:
    title: Alpha
    description: First in delivery.
    category: delivery
    repository: /srv/git/alpha
    path: .
    ref: v2
    commit: ` + strings.Repeat("c", 40) + `
  ops-check:
    title: Ops check
    description: Operations workflow.
    category: ops
    repository: git@github.com:owner/ops.git
    path: check
`
	catalog, err := parseProjectWorkflowCatalog([]byte(valid))
	if err != nil {
		t.Fatalf("valid catalog: %v", err)
	}
	if catalog.Title != "Test catalog" || len(catalog.Categories) != 2 || catalog.Categories[0].ID != "delivery" || len(catalog.Workflows) != 3 || catalog.Workflows[0].Name != "alpha" || catalog.Workflows[1].Name != "zeta" || catalog.Workflows[2].Name != "ops-check" || catalog.Workflows[0].Commit != strings.Repeat("c", 40) || len(catalog.Workflows[1].Tags) != 2 || len(catalog.Workflows[0].Tags) != 0 {
		t.Fatalf("unexpected catalog: %+v", catalog)
	}
	for name, mutate := range map[string]func(string) string{
		"unknown top field": func(s string) string { return s + "extra: 1\n" },
		"schema version":    func(s string) string { return strings.Replace(s, "catalog/1", "catalog/2", 1) },
		"unknown entry field": func(s string) string {
			return strings.Replace(s, "    path: check\n", "    path: check\n    signature: x\n", 1)
		},
		"unknown category":     func(s string) string { return strings.Replace(s, "category: ops", "category: nope", 1) },
		"relative repository":  func(s string) string { return strings.Replace(s, "/srv/git/alpha", "./alpha", 1) },
		"shorthand repository": func(s string) string { return strings.Replace(s, "/srv/git/alpha", "owner/alpha", 1) },
		"credentials": func(s string) string {
			return strings.Replace(s, "https://example.com/zeta.git", "https://u:p@example.com/zeta.git", 1)
		},
		"bad name":       func(s string) string { return strings.Replace(s, "  zeta:", "  Zeta:", 1) },
		"bad commit":     func(s string) string { return strings.Replace(s, strings.Repeat("c", 40), "abc", 1) },
		"traversal path": func(s string) string { return strings.Replace(s, "path: check", "path: ../check", 1) },
		"missing title":  func(s string) string { return strings.Replace(s, "    title: Ops check\n", "", 1) },
		"duplicate tag":  func(s string) string { return strings.Replace(s, "tags: [z, delivery]", "tags: [z, z]", 1) },
		"not an object":  func(string) string { return "- list\n" },
	} {
		if _, err := parseProjectWorkflowCatalog([]byte(mutate(valid))); err == nil || !strings.Contains(err.Error(), "project_workflow_catalog_invalid") {
			t.Fatalf("%s was accepted: %v", name, err)
		}
	}
	if projectDefaultWorkflowCatalog != "https://github.com/StenHigh/prifly-workflows.git" {
		t.Fatalf("default catalog changed: %s", projectDefaultWorkflowCatalog)
	}
}

func TestCLIProjectWorkflowsSearchAndAddByCatalogEntry(t *testing.T) {
	source := newWorkflowRepositoryFixture(t, "flows/sample", "flows/other")
	gitFixture(t, source, "tag", "v1")
	commit := gitFixture(t, source, "rev-parse", "HEAD")
	catalog := filepath.Join(t.TempDir(), "catalog")
	if err := os.MkdirAll(catalog, 0755); err != nil {
		t.Fatal(err)
	}
	gitFixture(t, catalog, "init", "-q", "-b", "main")
	writeFixtureFile(t, catalog, "catalog.yaml", `schema_version: prifly-workflow-catalog/1
title: Fixture catalog
categories:
  delivery: {title: Delivery}
  ops: {title: Ops}
workflows:
  sample:
    title: Sample workflow
    description: Pinned fixture.
    category: delivery
    repository: `+source+`
    path: flows/sample
    ref: v1
    commit: `+commit+`
    tags: [fixture]
  other:
    title: Other workflow
    description: Wrong pin.
    category: ops
    repository: `+source+`
    path: flows/other
    ref: v1
    commit: `+strings.Repeat("ab", 20)+`
`)
	gitFixture(t, catalog, "add", ".")
	gitFixture(t, catalog, "commit", "-qm", "catalog")
	catalogCommit := gitFixture(t, catalog, "rev-parse", "HEAD")
	code, out, errout := runCLI(t, "project", "workflows", "search", "--catalog", catalog)
	var result projectWorkflowCatalogResult
	if code != 0 || json.Unmarshal([]byte(out), &result) != nil || result.SchemaVersion != "project-workflow-catalog/1" || result.Catalog.Repository != catalog || result.Catalog.Commit != catalogCommit || result.Catalog.Title != "Fixture catalog" || len(result.Categories) != 2 || len(result.Workflows) != 2 || result.Workflows[0].Name != "sample" || result.Workflows[1].Name != "other" {
		t.Fatalf("search: %d %s %s", code, out, errout)
	}
	code, out, _ = runCLI(t, "project", "workflows", "search", "PINNED", "--catalog", catalog)
	if code != 0 || json.Unmarshal([]byte(out), &result) != nil || len(result.Workflows) != 1 || result.Workflows[0].Name != "sample" {
		t.Fatalf("query filter: %d %s", code, out)
	}
	code, out, _ = runCLI(t, "project", "workflows", "search", "--category", "ops", "--catalog", catalog)
	if code != 0 || json.Unmarshal([]byte(out), &result) != nil || len(result.Workflows) != 1 || result.Workflows[0].Name != "other" {
		t.Fatalf("category filter: %d %s", code, out)
	}
	if code, _, errout := runCLI(t, "project", "workflows", "search", "--catalog", "https://user:pw@example.com/c.git"); code == 0 || !strings.Contains(errout, "project_workflow_source_invalid") {
		t.Fatalf("catalog with credentials reached the network: %d %s", code, errout)
	}
	repository, _ := newProjectFixture(t)
	code, out, errout = runCLI(t, "project", "workflows", "add", "sample", "--catalog", catalog, "--repository", repository)
	var added projectWorkflowAddResult
	if code != 0 || json.Unmarshal([]byte(out), &added) != nil || added.Name != "sample" || added.Origin.Catalog != catalog || added.Origin.Ref != "v1" || added.Origin.Commit != commit || added.Origin.Path != "flows/sample" {
		t.Fatalf("add by catalog entry: %d %s %s", code, out, errout)
	}
	if code, _, errout := runCLI(t, "project", "workflows", "add", "other", "--catalog", catalog, "--repository", repository); code == 0 || !strings.Contains(errout, "project_workflow_commit_mismatch") {
		t.Fatalf("pinned commit mismatch was accepted: %d %s", code, errout)
	}
	if code, _, errout := runCLI(t, "project", "workflows", "add", "missing", "--catalog", catalog, "--repository", repository); code == 0 || !strings.Contains(errout, "project_workflow_catalog_entry_unknown") {
		t.Fatalf("unknown entry was accepted: %d %s", code, errout)
	}
	if code, _, errout := runCLI(t, "project", "workflows", "add", "sample", "--path", "flows/other", "--catalog", catalog, "--repository", repository); code == 0 || !strings.Contains(errout, "project_workflow_source_invalid") {
		t.Fatalf("--path with a catalog entry was accepted: %d %s", code, errout)
	}
	if code, _, errout := runCLI(t, "project", "workflows", "search", "--catalog", source); code == 0 || !strings.Contains(errout, "project_workflow_catalog_invalid") {
		t.Fatalf("repository without catalog.yaml was read as a catalog: %d %s", code, errout)
	}
}

func TestCLIProjectWorkflowsUpdateAndRemove(t *testing.T) {
	source := newWorkflowRepositoryFixture(t, "flows/sample")
	repository, _ := newProjectFixture(t)
	if code, _, errout := runCLI(t, "project", "workflows", "add", source, "--repository", repository); code != 0 {
		t.Fatalf("add %d: %s", code, errout)
	}
	folder := filepath.Join(repository, ".prifly", "workflows", "sample")
	code, out, errout := runCLI(t, "project", "workflows", "update", "sample", "--repository", repository)
	var result projectWorkflowUpdateResult
	if code != 0 || json.Unmarshal([]byte(out), &result) != nil || !result.UpToDate || result.Previous != result.Current || result.Origin.Ref != "main" {
		t.Fatalf("up-to-date update: %d %s %s", code, out, errout)
	}
	writeFixtureFile(t, folder, "README.md", "# edited locally\n")
	if code, _, errout := runCLI(t, "project", "workflows", "update", "sample", "--repository", repository); code == 0 || !strings.Contains(errout, "project_workflow_modified") || !strings.Contains(errout, "README.md") {
		t.Fatalf("local drift was not refused with its path: %d %s", code, errout)
	}
	writeFixtureFile(t, folder, "README.md", "# sample\n")
	writeFixtureFile(t, folder, "extend.yaml", "extensions: []\n# team note\n")
	writeFixtureFile(t, source, "flows/sample/README.md", "# sample v2\n")
	writeFixtureFile(t, source, "flows/sample/extend.yaml", "extensions: []\n# upstream note\n")
	gitFixture(t, source, "commit", "-qam", "upstream change without version bump")
	upstream := gitFixture(t, source, "rev-parse", "HEAD")
	code, out, errout = runCLI(t, "project", "workflows", "update", "sample", "--repository", repository)
	if code != 0 || json.Unmarshal([]byte(out), &result) != nil {
		t.Fatalf("update %d: %s %s", code, out, errout)
	}
	if result.UpToDate || result.Current.Commit != upstream || result.Previous.Commit == upstream || !result.ExtendUpstreamChanged || !result.PackageVersionUnchanged || len(result.Next) != 1 || !strings.Contains(result.Next[0], "explicitly to prifly-project-profile/3") || strings.Contains(result.Next[0], "package remove") {
		t.Fatalf("unexpected update result: %+v", result)
	}
	if data, err := os.ReadFile(filepath.Join(folder, "README.md")); err != nil || string(data) != "# sample v2\n" {
		t.Fatalf("upstream change was not applied: %v %q", err, data)
	}
	if data, err := os.ReadFile(filepath.Join(folder, "extend.yaml")); err != nil || string(data) != "extensions: []\n# team note\n" {
		t.Fatalf("team extend.yaml was not kept: %v %q", err, data)
	}
	parsed, err := readProjectProfile(repository)
	if err != nil || parsed.Packages["sample"].Origin.Commit != upstream || parsed.Packages["sample"].Origin.Digest != result.Current.Digest {
		t.Fatalf("origin was not updated: %v %+v", err, parsed.Packages["sample"].Origin)
	}
	entries, err := os.ReadDir(filepath.Join(repository, ".prifly", "workflows"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("swap left temporary folders: %v %v", err, entries)
	}
	gitFixture(t, source, "tag", "-a", "v2", "-m", "annotated release")
	code, out, errout = runCLI(t, "project", "workflows", "update", "sample", "--ref", "v2", "--repository", repository)
	if code != 0 || json.Unmarshal([]byte(out), &result) != nil || !result.UpToDate {
		t.Fatalf("switching to an annotated tag at the same commit is read-only: %d %s %s", code, out, errout)
	}
	if peeled, err := projectRemoteCommit(context.Background(), source, "v2"); err != nil || peeled != upstream {
		t.Fatalf("annotated tag was not peeled to its commit: %v %s", err, peeled)
	}
	writeFixtureWorkflowFolder(t, repository, ".prifly/workflows/manual", "manual")
	profilePath := filepath.Join(repository, ".prifly", "project.yaml")
	profile, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profilePath, []byte(strings.Replace(string(profile), "packages:\n", "packages:\n  manual:\n    source: .prifly/workflows/manual\n", 1)), 0644); err != nil {
		t.Fatal(err)
	}
	if code, _, errout := runCLI(t, "project", "workflows", "update", "manual", "--repository", repository); code == 0 || !strings.Contains(errout, "project_workflow_origin_missing") {
		t.Fatalf("hand-written folder was updated: %d %s", code, errout)
	}
	if code, _, errout := runCLI(t, "project", "workflows", "update", "absent", "--repository", repository); code == 0 || !strings.Contains(errout, "project_workflow_not_installed") {
		t.Fatalf("unknown package was updated: %d %s", code, errout)
	}
	code, out, errout = runCLI(t, "project", "workflows", "remove", "sample", "--repository", repository)
	var removed projectWorkflowRemoveResult
	if code != 0 || json.Unmarshal([]byte(out), &removed) != nil || removed.SchemaVersion != "project-workflow-remove/1" || len(removed.RemovedLaunches) != 1 || removed.RemovedLaunches[0] != "sample" {
		t.Fatalf("remove: %d %s %s", code, out, errout)
	}
	if _, err := os.Stat(folder); !os.IsNotExist(err) {
		t.Fatalf("folder was not removed: %v", err)
	}
	parsed, err = readProjectProfile(repository)
	if err != nil || len(parsed.Packages) != 1 || len(parsed.Launches) != 0 {
		t.Fatalf("profile still declares the removed workflow: %v %+v", err, parsed)
	}
	if code, _, errout := runCLI(t, "project", "workflows", "remove", "sample", "--repository", repository); code == 0 || !strings.Contains(errout, "project_workflow_not_installed") {
		t.Fatalf("second remove succeeded: %d %s", code, errout)
	}
	if _, err := flow.Parse(profile, "yaml"); err != nil {
		t.Fatal(err)
	}
}

func TestCLIProjectWorkflowsUpdateExplainsCompiledVariant(t *testing.T) {
	source := newWorkflowRepositoryFixture(t, "flows/sample")
	repository, _ := newProjectFixture(t)
	profilePath := filepath.Join(repository, ".prifly", "project.yaml")
	profile, err := os.ReadFile(profilePath)
	if err != nil {
		t.Fatal(err)
	}
	writeFixtureFile(t, repository, ".prifly/project.yaml", strings.Replace(string(profile), "prifly-project-profile/2", "prifly-project-profile/3", 1))
	if code, _, errout := runCLI(t, "project", "workflows", "add", source, "--repository", repository); code != 0 {
		t.Fatalf("add %d: %s", code, errout)
	}
	writeFixtureFile(t, source, "flows/sample/README.md", "# changed without an author version bump\n")
	gitFixture(t, source, "commit", "-qam", "upstream change without version bump")
	code, out, errout := runCLI(t, "project", "workflows", "update", "sample", "--repository", repository)
	var result projectWorkflowUpdateResult
	if code != 0 || json.Unmarshal([]byte(out), &result) != nil || result.UpToDate || !result.PackageVersionUnchanged || result.Package.Version != "1.0.0" || len(result.Next) != 1 {
		t.Fatalf("variant update: %d %s %s", code, out, errout)
	}
	if !strings.Contains(result.Next[0], "distinct sealed build") || !strings.Contains(result.Next[0], "existing packages and Runs remain unchanged") || strings.Contains(result.Next[0], "remove") || strings.Contains(result.Next[0], "Migrate") {
		t.Fatalf("variant update must preserve previous builds, not request removal or migration: %+v", result.Next)
	}
	parsed, err := readProjectProfile(repository)
	if err != nil || parsed.SchemaVersion != "prifly-project-profile/3" {
		t.Fatalf("update changed the explicit profile: %v %+v", err, parsed)
	}
}

package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func runnerTreeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	if err := filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, _ := filepath.Rel(root, name)
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(name)
			if err != nil {
				return err
			}
			result[relative] = "link:" + target
			return nil
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		result[relative] = runnerDigest(data)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestRunnerManualInstallsForEveryHostAndRepairsMissingPages(t *testing.T) {
	t.Parallel()
	for _, host := range projectHosts {
		t.Run(host.ID, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			if err := writeProjectRunners(root, host); err != nil {
				t.Fatal(err)
			}
			skillRoot := projectRunnerPath(root, host)
			assets, err := projectRunnerAssets()
			if err != nil {
				t.Fatal(err)
			}
			if len(assets) < 50 {
				t.Fatalf("manual included only %d files", len(assets))
			}
			for name, want := range assets {
				got, err := runnerReadFile(skillRoot, name)
				if err != nil || string(got) != string(want) {
					t.Fatalf("missing or wrong installed asset %s: %v", name, err)
				}
			}
			for _, name := range []string{
				"references/examples/compile-parameters/README.md",
				"references/examples/compile-parameters/project.yaml",
				"references/examples/compile-parameters/package/compile-parameters.yaml",
				"references/examples/compile-parameters/package/project/compile-values.yaml",
				"references/examples/compile-parameters/package/project/steps/worker.yaml",
			} {
				if data, err := runnerReadFile(skillRoot, name); err != nil || len(data) == 0 {
					t.Fatalf("compile parameter example is not installed: %s: %v", name, err)
				}
			}
			updated, missing, err := checkProjectRunnerUpdates(root, host)
			if err != nil || len(updated) != 0 || len(missing) != 0 {
				t.Fatalf("fresh install is stale: %v %v %v", updated, missing, err)
			}
			removed := filepath.Join(skillRoot, "references", "manual", "hosting.md")
			if err := os.Remove(removed); err != nil {
				t.Fatal(err)
			}
			before := runnerTreeSnapshot(t, root)
			updated, _, err = checkProjectRunnerUpdates(root, host)
			if err != nil || len(updated) != 1 {
				t.Fatalf("missing page was not detected: %v %v", updated, err)
			}
			if !reflect.DeepEqual(before, runnerTreeSnapshot(t, root)) {
				t.Fatal("read-only check changed files")
			}
			updated, _, err = updateProjectRunners(root, host)
			if err != nil || len(updated) != 1 {
				t.Fatalf("repair failed: %v %v", updated, err)
			}
			got, err := os.ReadFile(removed)
			if err != nil || string(got) != string(assets["references/manual/hosting.md"]) {
				t.Fatal("repair did not restore exact page")
			}
		})
	}
}

func TestRunnerManualConflictPreservesEveryHostAndAuthorFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := writeProjectRunners(root, projectHosts...); err != nil {
		t.Fatal(err)
	}
	for _, host := range projectHosts {
		base := projectRunnerPath(root, host)
		writeFixtureFile(t, base, "PROJECT.md", "# Author rules\n")
		writeFixtureFile(t, base, "references/custom.md", "author bytes\n")
	}
	last := projectRunnerPath(root, projectHosts[len(projectHosts)-1])
	writeFixtureFile(t, last, "references/manual/recovery.md", "local edits which must survive\n")
	before := runnerTreeSnapshot(t, root)
	if _, _, err := updateProjectRunners(root, projectHosts...); err == nil {
		t.Fatal("modified managed page was overwritten")
	}
	if !reflect.DeepEqual(before, runnerTreeSnapshot(t, root)) {
		t.Fatal("a late conflict partially changed a host or author file")
	}
	if _, _, err := checkProjectRunnerUpdates(root, projectHosts...); err == nil {
		t.Fatal("read-only check concealed the conflict")
	}
	if !reflect.DeepEqual(before, runnerTreeSnapshot(t, root)) {
		t.Fatal("failed read-only check changed bytes")
	}
}

func TestRunnerManualUpgradePreservesCustomFilesAndRetiresOwnedPages(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	host := projectHosts[0]
	if err := writeProjectRunners(root, host); err != nil {
		t.Fatal(err)
	}
	base := projectRunnerPath(root, host)
	data, err := os.ReadFile(filepath.Join(base, filepath.FromSlash(runnerManualReceipt)))
	if err != nil {
		t.Fatal(err)
	}
	var version runnerManualVersion
	if err := json.Unmarshal(data, &version); err != nil {
		t.Fatal(err)
	}
	previousPage := []byte("an earlier released procedure\n")
	writeFixtureFile(t, base, "references/manual/hosting.md", string(previousPage))
	version.Files["references/manual/hosting.md"] = runnerDigest(previousPage)
	writeFixtureFile(t, base, "references/manual/retired.md", "retired owned page\n")
	version.Files["references/manual/retired.md"] = runnerDigest([]byte("retired owned page\n"))
	receipt, _ := json.Marshal(version)
	writeFixtureFile(t, base, runnerManualReceipt, string(receipt))
	writeFixtureFile(t, base, "PROJECT.md", "author overlay\n")
	writeFixtureFile(t, base, "references/manual/custom.md", "author page\n")
	updated, _, err := updateProjectRunners(root, host)
	if err != nil || len(updated) != 1 {
		t.Fatalf("old manifest did not update: %v %v", updated, err)
	}
	for name, want := range map[string]string{"PROJECT.md": "author overlay\n", "references/manual/custom.md": "author page\n"} {
		got, err := os.ReadFile(filepath.Join(base, filepath.FromSlash(name)))
		if err != nil || string(got) != want {
			t.Fatalf("author file %s changed", name)
		}
	}
	if _, err := os.Lstat(filepath.Join(base, "references/manual/retired.md")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("retired owned page remains")
	}
	if updated, _, err = checkProjectRunnerUpdates(root, host); err != nil || len(updated) != 0 {
		t.Fatalf("updated manual is not current: %v %v", updated, err)
	}
}

func TestRunnerManualRefusesManagedSymlinksAndUnownedCollisions(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"root-link", "parent-link", "file-link", "skill-link", "unowned", "invalid-receipt"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			host := projectHosts[0]
			if err := writeProjectRunners(root, host); err != nil {
				t.Fatal(err)
			}
			base := projectRunnerPath(root, host)
			outside := t.TempDir()
			writeFixtureFile(t, outside, "outside.md", "outside bytes\n")
			switch kind {
			case "root-link":
				saved := base + "-saved"
				if err := os.Rename(base, saved); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(saved, base); err != nil {
					t.Fatal(err)
				}
			case "parent-link":
				refs := filepath.Join(base, "references")
				if err := os.Rename(refs, filepath.Join(outside, "references")); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(outside, "references"), refs); err != nil {
					t.Fatal(err)
				}
			case "file-link":
				page := filepath.Join(base, "references/manual/hosting.md")
				if err := os.Remove(page); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(outside, "outside.md"), page); err != nil {
					t.Fatal(err)
				}
			case "skill-link":
				page := filepath.Join(base, "SKILL.md")
				writeFixtureFile(t, outside, "skill.md", projectRunnerSkillBeforeHandbook(host))
				if err := os.Remove(page); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(outside, "skill.md"), page); err != nil {
					t.Fatal(err)
				}
			case "unowned":
				if err := os.Remove(filepath.Join(base, filepath.FromSlash(runnerManualReceipt))); err != nil {
					t.Fatal(err)
				}
				writeFixtureFile(t, base, "references/manual/hosting.md", "unowned custom reference\n")
			case "invalid-receipt":
				writeFixtureFile(t, base, runnerManualReceipt, `{"schema_version":"prifly-runner-manual/1","files":{"../../escape":"sha256:bad"}}`)
			}
			before := runnerTreeSnapshot(t, root)
			outsideBefore := runnerTreeSnapshot(t, outside)
			if _, _, err := updateProjectRunners(root, host); err == nil {
				t.Fatal("unsafe/colliding managed path was accepted")
			}
			if !reflect.DeepEqual(before, runnerTreeSnapshot(t, root)) || !reflect.DeepEqual(outsideBefore, runnerTreeSnapshot(t, outside)) {
				t.Fatal("conflict changed an original or external file")
			}
		})
	}
}

func TestRunnerBundleFailedStageLeavesOriginalIntact(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	host := projectHosts[0]
	if err := writeProjectRunners(root, host); err != nil {
		t.Fatal(err)
	}
	base := projectRunnerPath(root, host)
	before := runnerTreeSnapshot(t, root)
	if err := installRunnerBundle(base, host, map[string][]byte{"invalid\x00path": []byte("cannot be written")}, nil); err == nil {
		t.Fatal("invalid stage unexpectedly succeeded")
	}
	if !reflect.DeepEqual(before, runnerTreeSnapshot(t, root)) {
		t.Fatal("failed stage altered the visible runner")
	}
}

func TestRunnerManualConcurrentUpdatesDoNotMixFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	host := projectHosts[0]
	writeFixtureFile(t, root, filepath.ToSlash(filepath.Join(host.SkillsRoot, "prifly-run/SKILL.md")), projectRunnerSkillBeforeHandbook(host))
	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() { defer workers.Done(); <-start; _, _, err := updateProjectRunners(root, host); results <- err }()
	}
	close(start)
	workers.Wait()
	close(results)
	success := 0
	for err := range results {
		if err == nil {
			success++
		}
	}
	if success < 1 {
		t.Fatal("neither concurrent updater installed the manual")
	}
	updated, _, err := checkProjectRunnerUpdates(root, host)
	if err != nil || len(updated) != 0 {
		t.Fatalf("concurrent writers left mixed assets: %v %v", updated, err)
	}
}

func runnerBrokenLinks(files map[string][]byte) []string {
	var failures []string
	for name, data := range files {
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		for _, match := range runnerMarkdownLink.FindAllStringSubmatch(string(data), -1) {
			target := strings.SplitN(match[1], "#", 2)[0]
			if target == "" || strings.Contains(target, ":") || strings.HasPrefix(target, "//") {
				continue
			}
			destination := filepath.ToSlash(filepath.Clean(filepath.Join(filepath.Dir(name), filepath.FromSlash(target))))
			if _, ok := files[destination]; !ok {
				failures = append(failures, name+" -> "+target)
			}
		}
	}
	return failures
}

func TestInstalledRunnerLinksNeedNoCoreCheckout(t *testing.T) {
	t.Parallel()
	assets, err := projectRunnerAssets()
	if err != nil {
		t.Fatal(err)
	}
	if broken := runnerBrokenLinks(assets); len(broken) != 0 {
		t.Fatalf("broken installed references: %v", broken)
	}
	delete(assets, "references/manual/hosting.md")
	if broken := runnerBrokenLinks(assets); len(broken) == 0 {
		t.Fatal("link gate did not notice a missing supplied page")
	}
}

func TestRunnerManualVersionRecordsInstalledDigests(t *testing.T) {
	t.Parallel()
	assets, err := projectRunnerAssets()
	if err != nil {
		t.Fatal(err)
	}
	var version runnerManualVersion
	if err := json.Unmarshal(assets[runnerManualReceipt], &version); err != nil {
		t.Fatal(err)
	}
	if version.BinaryVersion == "" || version.SourceRef == "" {
		t.Fatal("manual has no version provenance")
	}
	for name, digest := range version.Files {
		if runnerDigest(assets[name]) != digest {
			t.Fatalf("manifest does not describe supplied bytes: %s", name)
		}
	}
	if len(version.Files) != len(assets)-1 {
		t.Fatal("not every supplied reference is owned")
	}
	if !version.Development && !regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`).MatchString(version.SourceRef) {
		t.Fatal("stable links are not pinned to the release")
	}
}

func TestTrackedRunnerBundlesMatchTemplateAndOwnedDigests(t *testing.T) {
	t.Parallel()
	for _, host := range []projectHost{projectHosts[1], projectHosts[2]} {
		root := filepath.Join("../..", host.SkillsRoot, "prifly-run")
		skill, err := runnerReadFile(root, "SKILL.md")
		if err != nil || string(skill) != projectRunnerSkill(host) {
			t.Fatalf("tracked %s runner differs from its canonical template: %v", host.ID, err)
		}
		data, err := runnerReadFile(root, runnerManualReceipt)
		if err != nil {
			t.Fatal(err)
		}
		var version runnerManualVersion
		if err := json.Unmarshal(data, &version); err != nil {
			t.Fatal(err)
		}
		for name, digest := range version.Files {
			if !runnerManagedPath(name) {
				t.Fatalf("tracked ownership escapes references: %s", name)
			}
			got, err := runnerReadFile(root, name)
			if err != nil || runnerDigest(got) != digest {
				t.Fatalf("tracked %s reference differs from receipt: %s (%v)", host.ID, name, err)
			}
		}
	}
}

func TestRunnerAssetCallersCannotAlterAnotherInstallation(t *testing.T) {
	t.Parallel()
	first, err := projectRunnerAssets()
	if err != nil {
		t.Fatal(err)
	}
	name := "references/manual/index.md"
	want := runnerDigest(first[name])
	first[name][0] ^= 1
	delete(first, runnerManualReceipt)
	second, err := projectRunnerAssets()
	if err != nil {
		t.Fatal(err)
	}
	if runnerDigest(second[name]) != want || len(second[runnerManualReceipt]) == 0 {
		t.Fatal("one caller changed the cached assets for another installation")
	}
}

func TestRunnerBundleFailureReportsPreviouslyCompletedHost(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("permission fixture requires an unprivileged process")
	}
	root := t.TempDir()
	for _, host := range projectHosts[:2] {
		writeFixtureFile(t, root, filepath.ToSlash(filepath.Join(host.SkillsRoot, "prifly-run/SKILL.md")), projectRunnerSkillBeforeHandbook(host))
	}
	parent := filepath.Dir(projectRunnerPath(root, projectHosts[1]))
	if err := os.Chmod(parent, 0555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0755) })
	_, _, err := updateProjectRunners(root, projectHosts[:2]...)
	if err == nil || !strings.Contains(err.Error(), "already updated hosts: [codex-cli]") || !strings.Contains(err.Error(), "codex-app") {
		t.Fatalf("partial update was not diagnosed: %v", err)
	}
	for index, host := range projectHosts[:2] {
		got, readErr := runnerReadFile(projectRunnerPath(root, host), "SKILL.md")
		want := projectRunnerSkillBeforeHandbook(host)
		if index == 0 {
			want = projectRunnerSkill(host)
		}
		if readErr != nil || string(got) != want {
			t.Fatalf("unexpected host bytes after failure: %s %v", host.ID, readErr)
		}
	}
}

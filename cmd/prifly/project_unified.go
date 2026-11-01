package main

import (
	"crypto/sha256"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
)

const projectUnifiedExtensionVersion = "prifly-extension/5"
const projectUnifiedFolderVersion = "prifly-project-workflow/3"

// These are source compiler records. They never become Run state or authority.
type projectUnifiedDefinition struct {
	Owner         string         `json:"owner"`
	Source        string         `json:"source"`
	SourceDigest  string         `json:"source_digest"`
	AuthorID      string         `json:"author_id"`
	AuthorVersion string         `json:"author_version"`
	Alias         string         `json:"-"`
	VariantKey    string         `json:"variant_key"`
	Settings      map[string]any `json:"settings"`
}
type projectUnifiedPoint struct {
	Export   string
	Identity string
	Owner    string
	Name     string
	Target   string
	Scope    string
	Contract projectExtensionPoint
}
type projectUnifiedPointExport struct {
	Workflow string
	Point    projectUnifiedPoint
}
type projectUnifiedBuild struct {
	Points      map[string][]projectWorkflowExtension
	Scopes      map[string][]string
	ScopeLabels map[string]string
	Definitions map[string]*projectUnifiedDefinition
}

func projectUnifiedHash(value any) string {
	data, _ := projectCanonicalJSON(value)
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data))
}
func projectUnifiedAlias(value any) string {
	return "uv" + strings.TrimPrefix(projectUnifiedHash(value), "sha256:")
}

func projectUnifiedOptIn(root, folder string) (bool, error) {
	relative, err := projectRelativePath(root, filepath.Join(folder, "workflow.yaml"))
	if err != nil {
		return false, nil
	}
	data, err := projectExecutionSource(root, root, relative)
	if err != nil {
		return false, nil
	}
	value, err := flow.Parse(data, "yaml")
	if err != nil {
		return false, nil
	}
	object, _ := value.(map[string]any)
	if object["authoring"] == projectUnifiedFolderVersion {
		return true, nil
	}
	relative, err = projectRelativePath(root, filepath.Join(folder, "extend.yaml"))
	if err != nil {
		return false, nil
	}
	if _, err := os.Lstat(filepath.Join(root, relative)); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, nil
	}
	data, err = projectExecutionSource(root, root, relative)
	if err != nil {
		return false, nil
	}
	value, err = flow.Parse(data, "yaml")
	if err != nil {
		return false, nil
	}
	object, _ = value.(map[string]any)
	return object["authoring"] == projectUnifiedExtensionVersion, nil
}
func projectReadUnifiedFolder(root, folder string) (projectPackageSource, error) {
	profile, err := readProjectProfile(root)
	if err != nil {
		return projectPackageSource{}, err
	}
	r := &projectCompositionReader{unified: true, root: root, profile: profile, nodes: map[string]*projectCompositionNode{}, active: map[string]bool{}, files: map[string]string{}, layers: map[string]bool{}, folders: map[string]bool{}}
	name := "root"
	for _, key := range slices.Sorted(maps.Keys(profile.Packages)) {
		p, e := projectPackageSourceLocation(root, profile.Packages[key].Source)
		if e == nil && filepath.Clean(p) == filepath.Clean(folder) {
			name = key
			break
		}
	}
	node, err := r.load(name, folder, true, 0)
	if err != nil {
		return projectPackageSource{}, err
	}
	return r.finishUnified(node)
}
func (r *projectCompositionReader) checkPointBoundary(object map[string]any, source string) error {
	if r.unified {
		return nil
	}
	return projectCompositionCheckPointBoundary(object, source)
}

func projectPrepareCompilationPoints(source projectPackageSource, pending []projectPendingDocument, extensions []projectWorkflowExtension) (map[string][]projectWorkflowExtension, error) {
	if source.Unified == nil {
		return projectPreparePointExtensions(source, pending, extensions)
	}
	result := map[string][]projectWorkflowExtension{}
	for target, entries := range source.Unified.Points {
		for _, e := range entries {
			if projectExtensionEnabled(e) {
				result[target] = append(result[target], e)
			}
		}
	}
	return result, nil
}

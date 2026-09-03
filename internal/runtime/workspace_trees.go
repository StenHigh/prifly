package runtime

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

const (
	WorkspaceTreeManifestVersion = "workspace-tree-manifest/1"
	MaxWorkspaceTreeFiles        = 256
)

// WorkspaceTreeManifest is the one immutable index for a native workspace
// document or a small direct-child bundle. File contents remain raw artifacts.
type WorkspaceTreeManifest struct {
	SchemaVersion string               `json:"schema_version"`
	Root          string               `json:"root"`
	Entrypoint    string               `json:"entrypoint"`
	Files         []WorkspaceTreeEntry `json:"files"`
}

type WorkspaceTreeEntry struct {
	Path string      `json:"path"`
	Ref  ArtifactRef `json:"ref"`
}

// WorkspaceTreeHandoff is the constrained repository boundary recorded before
// an assisted host receives its claim. It never gives the host artifact access.
type WorkspaceTreeHandoff struct {
	InputPort        string                          `json:"input_port,omitempty"`
	OutputPort       string                          `json:"output_port"`
	Capture          flow.WorkspaceTreeCapturePolicy `json:"capture"`
	InputManifest    *ArtifactRef                    `json:"input_manifest,omitempty"`
	InputLocation    string                          `json:"input_location,omitempty"`
	ExistingChildren []string                        `json:"existing_children,omitempty"`
}

// WorkspaceTreeLocation is the only tree value an output-only host may report.
// The runtime chooses all ArtifactRefs and seals all bytes itself.
type WorkspaceTreeLocation struct {
	OutputPort string `json:"output_port"`
	Path       string `json:"path"`
}

func workspaceTreeManifestSchema() map[string]any {
	ref := map[string]any{"type": "object", "required": []string{"artifact_id", "revision", "digest"}, "properties": map[string]any{
		"artifact_id": map[string]any{"type": "string", "pattern": "^[A-Za-z][A-Za-z0-9._:/-]{0,127}$"},
		"revision":    map[string]any{"const": 1},
		"digest":      map[string]any{"type": "string", "pattern": "^sha256:[0-9a-f]{64}$"},
	}, "additionalProperties": false}
	return map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object", "required": []string{"schema_version", "root", "entrypoint", "files"}, "properties": map[string]any{
		"schema_version": map[string]any{"const": WorkspaceTreeManifestVersion},
		"root":           map[string]any{"type": "string", "minLength": 1, "maxLength": 1024},
		"entrypoint":     map[string]any{"type": "string", "minLength": 1, "maxLength": 255},
		"files": map[string]any{"type": "array", "minItems": 1, "maxItems": MaxWorkspaceTreeFiles, "items": map[string]any{"type": "object", "required": []string{"path", "ref"}, "properties": map[string]any{
			"path": map[string]any{"type": "string", "minLength": 1, "maxLength": 255}, "ref": ref,
		}, "additionalProperties": false}},
	}, "additionalProperties": false}
}

func validateWorkspaceTreeManifest(manifest WorkspaceTreeManifest) error {
	return validateWorkspaceTreeShape(manifest, true)
}

func validateWorkspaceTreeShape(manifest WorkspaceTreeManifest, requireRefs bool) error {
	if manifest.SchemaVersion != WorkspaceTreeManifestVersion || !safeRelative(manifest.Root) || !directChildName(manifest.Entrypoint) || len(manifest.Files) == 0 || len(manifest.Files) > MaxWorkspaceTreeFiles {
		return errors.New("invalid_workspace_tree_manifest")
	}
	seen, entrypoint := map[string]bool{}, false
	for _, file := range manifest.Files {
		if !directChildName(file.Path) || seen[file.Path] || requireRefs && (file.Ref.ArtifactID == "" || file.Ref.Revision != 1 || !strings.HasPrefix(file.Ref.Digest, "sha256:")) {
			return errors.New("invalid_workspace_tree_manifest")
		}
		seen[file.Path] = true
		entrypoint = entrypoint || file.Path == manifest.Entrypoint
	}
	if !entrypoint || !sort.SliceIsSorted(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path }) {
		return errors.New("invalid_workspace_tree_manifest")
	}
	return nil
}

func directChildName(name string) bool { return safeRelative(name) && filepath.Base(name) == name }

func workspaceTreePolicyLocation(policy flow.WorkspaceTreeCapturePolicy, manifest WorkspaceTreeManifest) (string, error) {
	policy.Path = filepath.ToSlash(policy.Path)
	switch policy.Kind {
	case "exact_file":
		if filepath.ToSlash(filepath.Dir(policy.Path)) != manifest.Root || filepath.Base(policy.Path) != manifest.Entrypoint || len(manifest.Files) != 1 {
			return "", errors.New("workspace_tree_policy_mismatch")
		}
		return policy.Path, nil
	case "direct_child_file":
		if manifest.Root != policy.Path || len(manifest.Files) != 1 {
			return "", errors.New("workspace_tree_policy_mismatch")
		}
		return filepath.ToSlash(filepath.Join(manifest.Root, manifest.Entrypoint)), nil
	case "direct_child_tree":
		if filepath.ToSlash(filepath.Dir(manifest.Root)) != policy.Path || policy.Entrypoint != manifest.Entrypoint || !directChildName(filepath.Base(manifest.Root)) {
			return "", errors.New("workspace_tree_policy_mismatch")
		}
		return manifest.Root, nil
	default:
		return "", errors.New("workspace_tree_policy_mismatch")
	}
}

func (e *Engine) readWorkspaceTreeManifest(r Run, ref ArtifactRef) (WorkspaceTreeManifest, error) {
	artifact, data, err := e.Artifact(ref)
	if err != nil {
		return WorkspaceTreeManifest{}, err
	}
	schema := builtinRef(r.Definitions, flow.WorkspaceTreeManifestSchemaID)
	if schema.ID == "" || artifact.Format != "json" || !sameRef(artifact.SchemaRef, &schema) || flow.ValidateSchema(r.registry(), schema, data) != nil {
		return WorkspaceTreeManifest{}, errors.New("workspace_tree_manifest_contract_mismatch")
	}
	var manifest WorkspaceTreeManifest
	if err := decode(data, &manifest); err != nil {
		return WorkspaceTreeManifest{}, err
	}
	return manifest, validateWorkspaceTreeManifest(manifest)
}

func workspaceTreeBinding(step flow.StepDefinition, output string) *flow.WorkspaceTreeBinding {
	for index := range step.WorkspaceTrees {
		if step.WorkspaceTrees[index].OutputPort == output {
			return &step.WorkspaceTrees[index]
		}
	}
	return nil
}

func requiresWorkspaceTreeState(plan *flow.Plan) bool {
	for _, candidate := range workflowPlans(plan) {
		for _, step := range candidate.Steps {
			if len(step.WorkspaceTrees) != 0 {
				return true
			}
		}
	}
	return false
}

func treeParent(policy flow.WorkspaceTreeCapturePolicy) string {
	if policy.Kind == "exact_file" {
		return filepath.ToSlash(filepath.Dir(policy.Path))
	}
	return filepath.ToSlash(policy.Path)
}

func checkedDirectory(root *os.Root, path string) error {
	if !safeRelative(path) {
		return local.ErrUnsafePath
	}
	current := ""
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return local.ErrUnsafePath
		}
	}
	return nil
}

func childNames(root *os.Root, path string) ([]string, error) {
	if err := checkedDirectory(root, path); err != nil {
		return nil, err
	}
	dir, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !directChildName(entry.Name()) {
			return nil, local.ErrUnsafePath
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

func ensureTreeDirectory(root *os.Root, path string, created *[]string) error {
	if !safeRelative(path) {
		return local.ErrUnsafePath
	}
	current := ""
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		current = filepath.Join(current, part)
		info, err := root.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			if err := root.Mkdir(current, 0700); err != nil {
				return err
			}
			*created = append(*created, current)
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return local.ErrUnsafePath
		}
	}
	return nil
}

func writeRootExclusive(root *os.Root, path string, data []byte) error {
	f, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	return f.Sync()
}

func (e *Engine) materializeWorkspaceTree(root *os.Root, manifest WorkspaceTreeManifest) (func(), error) {
	createdFiles, createdDirs := []string{}, []string{}
	cleanup := func() {
		for index := len(createdFiles) - 1; index >= 0; index-- {
			_ = root.Remove(createdFiles[index])
		}
		for index := len(createdDirs) - 1; index >= 0; index-- {
			_ = root.Remove(createdDirs[index])
		}
	}
	for _, entry := range manifest.Files {
		artifact, data, err := e.Artifact(entry.Ref)
		if err != nil || artifact.Ref() != entry.Ref {
			cleanup()
			return nil, errors.New("workspace_tree_entry_unavailable")
		}
		target := filepath.ToSlash(filepath.Join(manifest.Root, entry.Path))
		if existing, err := root.Lstat(target); err == nil {
			if existing.Mode()&os.ModeSymlink != 0 || !existing.Mode().IsRegular() {
				cleanup()
				return nil, local.ErrUnsafePath
			}
			current, err := readLocal(root.Name(), target, MaxArtifactBytes)
			if err != nil || rawDigest(current) != entry.Ref.Digest {
				cleanup()
				return nil, errors.New("workspace_tree_input_drift")
			}
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			cleanup()
			return nil, err
		}
		if err := ensureTreeDirectory(root, filepath.ToSlash(filepath.Dir(target)), &createdDirs); err != nil {
			cleanup()
			return nil, err
		}
		if err := writeRootExclusive(root, target, data); err != nil {
			cleanup()
			return nil, err
		}
		createdFiles = append(createdFiles, target)
	}
	return cleanup, nil
}

func (e *Engine) prepareWorkspaceTrees(r Run, step flow.StepDefinition, inputs map[string]ArtifactRef, claim WorktreeClaim) ([]WorkspaceTreeHandoff, func(), error) {
	if len(step.WorkspaceTrees) == 0 {
		return nil, func() {}, nil
	}
	workspace, err := e.claimWorkspacePath(claim)
	if err != nil {
		return nil, nil, err
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()
	cleanups := []func(){}
	cleanup := func() {
		for index := len(cleanups) - 1; index >= 0; index-- {
			cleanups[index]()
		}
	}
	handoffs := make([]WorkspaceTreeHandoff, 0, len(step.WorkspaceTrees))
	for _, binding := range step.WorkspaceTrees {
		handoff := WorkspaceTreeHandoff{InputPort: binding.InputPort, OutputPort: binding.OutputPort, Capture: binding.Capture}
		if binding.InputPort != "" {
			ref, ok := inputs[binding.InputPort]
			if !ok {
				cleanup()
				return nil, nil, errors.New("workspace_tree_input_missing")
			}
			manifest, err := e.readWorkspaceTreeManifest(r, ref)
			if err != nil {
				cleanup()
				return nil, nil, err
			}
			location, err := workspaceTreePolicyLocation(binding.Capture, manifest)
			if err != nil {
				cleanup()
				return nil, nil, err
			}
			entryCleanup, err := e.materializeWorkspaceTree(root, manifest)
			if err != nil {
				cleanup()
				return nil, nil, err
			}
			cleanups = append(cleanups, entryCleanup)
			handoff.InputManifest, handoff.InputLocation = &ref, location
		} else {
			created := []string{}
			if err := ensureTreeDirectory(root, treeParent(binding.Capture), &created); err != nil {
				cleanup()
				return nil, nil, err
			}
			if len(created) != 0 {
				cleanups = append(cleanups, func() {
					for index := len(created) - 1; index >= 0; index-- {
						_ = os.Remove(filepath.Join(workspace, created[index]))
					}
				})
			}
			if binding.Capture.Kind == "exact_file" {
				if _, err := root.Lstat(binding.Capture.Path); !errors.Is(err, os.ErrNotExist) {
					cleanup()
					return nil, nil, errors.New("workspace_tree_output_exists")
				}
			}
		}
		if binding.Capture.Kind != "exact_file" {
			names, err := childNames(root, treeParent(binding.Capture))
			if err != nil {
				cleanup()
				return nil, nil, err
			}
			handoff.ExistingChildren = names
		}
		handoffs = append(handoffs, handoff)
	}
	return handoffs, cleanup, nil
}

// treeProblem names the reported location an intake refusal is about. A host
// that never sees which entry was wrong has no way back except guessing.
func treeProblem(code, pointer, message string) error {
	if pointer == "" {
		pointer = "/workspace_trees"
	}
	return &flow.Problem{Code: code, Path: pointer, Message: message}
}

// selectedTreeLocation resolves the one location a binding may be captured at.
// A host reports a location only where it actually chooses one: an input tree
// is already materialized at a fixed path, and an exact-file policy admits a
// single path, so in both cases the runtime supplies it. Repeating that path is
// allowed, which keeps one submission form for every binding kind; naming a
// different one is refused.
func selectedTreeLocation(handoff WorkspaceTreeHandoff, supplied map[string]string, pointer string) (string, error) {
	location, exists := supplied[handoff.OutputPort]
	if handoff.InputManifest != nil {
		if exists && location != handoff.InputLocation {
			return "", treeProblem("workspace_tree_input_location_mismatch", pointer, "this port's tree is materialized at the location named by the handoff")
		}
		return handoff.InputLocation, nil
	}
	policy := handoff.Capture
	if !exists && policy.Kind == "exact_file" {
		return policy.Path, nil
	}
	if !exists || !safeRelative(location) {
		return "", treeProblem("workspace_tree_location_missing", pointer, "this port needs one relative location inside the claimed workspace")
	}
	switch policy.Kind {
	case "exact_file":
		if location != policy.Path {
			return "", treeProblem("workspace_tree_policy_escape", pointer, "this port is captured at the exact path named by its declared policy")
		}
	case "direct_child_file", "direct_child_tree":
		if filepath.ToSlash(filepath.Dir(location)) != policy.Path || !directChildName(filepath.Base(location)) || slices.Contains(handoff.ExistingChildren, filepath.Base(location)) {
			return "", treeProblem("workspace_tree_policy_escape", pointer, "this port is captured as a new direct child of the parent named by its declared policy")
		}
	default:
		return "", treeProblem("workspace_tree_policy_escape", pointer, "this binding has no supported capture policy")
	}
	return location, nil
}

func captureWorkspaceTree(root string, handoff WorkspaceTreeHandoff, location string) (WorkspaceTreeManifest, map[string][]byte, error) {
	policy := handoff.Capture
	files := map[string][]byte{}
	manifest := WorkspaceTreeManifest{SchemaVersion: WorkspaceTreeManifestVersion}
	switch policy.Kind {
	case "exact_file", "direct_child_file":
		data, err := readLocal(root, location, MaxArtifactBytes)
		if err != nil {
			return WorkspaceTreeManifest{}, nil, err
		}
		manifest.Root, manifest.Entrypoint = filepath.ToSlash(filepath.Dir(location)), filepath.Base(location)
		files[manifest.Entrypoint] = data
	case "direct_child_tree":
		tree, err := os.OpenRoot(root)
		if err != nil {
			return WorkspaceTreeManifest{}, nil, err
		}
		defer tree.Close()
		if err := checkedDirectory(tree, location); err != nil {
			return WorkspaceTreeManifest{}, nil, err
		}
		dir, err := tree.Open(location)
		if err != nil {
			return WorkspaceTreeManifest{}, nil, err
		}
		entries, err := dir.ReadDir(-1)
		dir.Close()
		if err != nil || len(entries) == 0 || len(entries) > MaxWorkspaceTreeFiles {
			if err != nil {
				return WorkspaceTreeManifest{}, nil, err
			}
			return WorkspaceTreeManifest{}, nil, errors.New("workspace_tree_entry_limit")
		}
		manifest.Root, manifest.Entrypoint = location, policy.Entrypoint
		for _, entry := range entries {
			if !directChildName(entry.Name()) || entry.Type()&os.ModeSymlink != 0 || entry.IsDir() || !entry.Type().IsRegular() {
				return WorkspaceTreeManifest{}, nil, local.ErrUnsafePath
			}
			data, err := readLocal(root, filepath.ToSlash(filepath.Join(location, entry.Name())), MaxArtifactBytes)
			if err != nil {
				return WorkspaceTreeManifest{}, nil, err
			}
			files[entry.Name()] = data
		}
	default:
		return WorkspaceTreeManifest{}, nil, errors.New("workspace_tree_policy_escape")
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		manifest.Files = append(manifest.Files, WorkspaceTreeEntry{Path: name})
	}
	if err := validateWorkspaceTreeShape(manifest, false); err != nil {
		return WorkspaceTreeManifest{}, nil, err
	}
	return manifest, files, nil
}

func treeCapturePath(port, name string) string {
	return filepath.ToSlash(filepath.Join("tmp", "workspace-trees", port, name))
}

func writeCapturedTreeFile(workspace, path string, data []byte) error {
	if err := os.MkdirAll(filepath.Join(workspace, filepath.Dir(path)), 0700); err != nil {
		return err
	}
	if existing, err := readLocal(workspace, path, MaxArtifactBytes); err == nil {
		if bytes.Equal(existing, data) {
			return nil
		}
		return errors.New("workspace_tree_capture_conflict")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		return err
	}
	defer root.Close()
	return writeRootExclusive(root, path, data)
}

func (e *Engine) captureTreeOutput(a *Attempt, handoff WorkspaceTreeHandoff, location string) (ArtifactRef, error) {
	claim, err := e.claim(context.Background(), a.Session.ClaimID)
	if err != nil {
		return ArtifactRef{}, err
	}
	workspace, err := e.claimWorkspacePath(claim)
	if err != nil {
		return ArtifactRef{}, err
	}
	manifest, files, err := captureWorkspaceTree(workspace, handoff, location)
	if err != nil {
		return ArtifactRef{}, err
	}
	for index := range manifest.Files {
		entry := &manifest.Files[index]
		data := files[entry.Path]
		entry.Ref = ArtifactRef{ArtifactID: derivedID("artifact", a.ID, "workspace-tree", handoff.OutputPort, entry.Path), Revision: 1, Digest: rawDigest(data)}
		if err := writeCapturedTreeFile(a.Workspace, treeCapturePath(handoff.OutputPort, entry.Path), data); err != nil {
			return ArtifactRef{}, err
		}
	}
	data, err := canonical(manifest)
	if err != nil {
		return ArtifactRef{}, err
	}
	slot, exists := a.Context.Outputs[handoff.OutputPort]
	if !exists {
		return ArtifactRef{}, local.ErrIntegrity
	}
	if err := writeCapturedTreeFile(a.Workspace, slot.Path, data); err != nil {
		return ArtifactRef{}, err
	}
	return ArtifactRef{ArtifactID: slot.ArtifactID, Revision: slot.Revision, Digest: rawDigest(data)}, nil
}

func (e *Engine) captureWorkspaceTreeOutputs(a *Attempt, step flow.StepDefinition, result Result, locations []WorkspaceTreeLocation) (Result, error) {
	if len(step.WorkspaceTrees) == 0 {
		if len(locations) != 0 {
			return Result{}, errors.New("workspace_tree_location_unexpected")
		}
		return result, nil
	}
	if a.Session == nil || len(a.Session.WorkspaceTrees) != len(step.WorkspaceTrees) {
		return Result{}, local.ErrIntegrity
	}
	supplied := map[string]string{}
	pointers := map[string]string{}
	for index, location := range locations {
		pointer := "/workspace_trees/" + strconv.Itoa(index)
		if location.OutputPort == "" || location.Path == "" || supplied[location.OutputPort] != "" {
			return Result{}, treeProblem("workspace_tree_location_invalid", pointer, "each reported location names one distinct output port and one path")
		}
		supplied[location.OutputPort] = location.Path
		pointers[location.OutputPort] = pointer + "/path"
	}
	allowed := map[string]bool{}
	for _, handoff := range a.Session.WorkspaceTrees {
		allowed[handoff.OutputPort] = true
		if _, exists := result.Outputs[handoff.OutputPort]; exists {
			return Result{}, treeProblem("workspace_tree_output_host_supplied", "/result/outputs/"+handoff.OutputPort, "the runtime seals this port's tree itself; a report does not carry its artifact")
		}
		location, err := selectedTreeLocation(handoff, supplied, pointers[handoff.OutputPort])
		if err != nil {
			return Result{}, err
		}
		ref, err := e.captureTreeOutput(a, handoff, location)
		if err != nil {
			return Result{}, err
		}
		result.Outputs[handoff.OutputPort] = ref
	}
	for port := range supplied {
		if !allowed[port] {
			return Result{}, treeProblem("workspace_tree_location_invalid", pointers[port], "this step declares no workspace tree binding for that output port")
		}
	}
	return result, nil
}

func (e *Engine) sealWorkspaceTreeOutput(r Run, a *Attempt, step flow.StepDefinition, definition flow.OutputPort, port string, ref ArtifactRef, data []byte) (Artifact, error) {
	if definition.SchemaRef == nil || definition.SchemaRef.ID != flow.WorkspaceTreeManifestSchemaID || workspaceTreeBinding(step, port) == nil || a.Session == nil {
		return Artifact{}, local.ErrIntegrity
	}
	var manifest WorkspaceTreeManifest
	if err := decode(data, &manifest); err != nil || validateWorkspaceTreeManifest(manifest) != nil {
		return Artifact{}, errors.New("workspace_tree_manifest_invalid")
	}
	inputProvenance := []ArtifactRef{}
	for _, handoff := range a.Session.WorkspaceTrees {
		if handoff.OutputPort == port && handoff.InputManifest != nil {
			inputProvenance = append(inputProvenance, *handoff.InputManifest)
		}
	}
	activation := r.Activations[a.ActivationID]
	if activation == nil {
		return Artifact{}, local.ErrIntegrity
	}
	producer := map[string]any{"kind": "step", "run_id": r.ID, "workflow_invocation_id": activation.InvocationID, "stage_activation_id": a.ActivationID, "step_instance_id": a.StepID, "attempt_id": a.ID, "port": port}
	entryRefs := make([]ArtifactRef, 0, len(manifest.Files))
	for _, entry := range manifest.Files {
		captured, err := readLocal(a.Workspace, treeCapturePath(port, entry.Path), MaxArtifactBytes)
		if err != nil || rawDigest(captured) != entry.Ref.Digest {
			return Artifact{}, errors.New("workspace_tree_capture_drift")
		}
		artifact, err := e.putArtifact(captured, "blob", nil, entry.Ref.ArtifactID, producer, inputProvenance, r.registry())
		if err != nil || artifact.Ref() != entry.Ref {
			if err != nil {
				return Artifact{}, err
			}
			return Artifact{}, local.ErrIntegrity
		}
		entryRefs = append(entryRefs, entry.Ref)
	}
	provenance := append(inputProvenance, entryRefs...)
	artifact, err := e.putArtifact(data, "json", definition.SchemaRef, ref.ArtifactID, producer, provenance, r.registry())
	if err != nil || artifact.Ref() != ref {
		if err != nil {
			return Artifact{}, err
		}
		return Artifact{}, local.ErrIntegrity
	}
	return artifact, nil
}

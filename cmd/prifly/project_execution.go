package main

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

const projectExecutionFile = "execution-bindings.json"
const projectExecutionVersion = "prifly-package-execution/1"

// The package is inert: executable is a logical owner-approved name here.
// Only projectExecutionPayload substitutes the machine-local absolute path.
type projectPackageExecution struct {
	SchemaVersion string                    `json:"schema_version"`
	Bindings      []prifly.ExecutionBinding `json:"bindings"`
}

func (c *cli) projectAuthority(root string, profile projectProfile) error {
	if profile.SchemaVersion != projectVariantProfileVersion || c.projectExplicit || c.project != "." && c.project != "" {
		return nil
	}
	authority, _, err := projectLocalExecution(root)
	if err == nil {
		c.project = authority
	}
	return err
}

func projectReadExecution(projectRoot string, source projectPackageSource, components []projectCompileComponent, values map[string]any) (*projectPackageExecution, error) {
	root, _ := source.RootValue.(map[string]any)
	raw, present := root["execution_bindings"]
	if !present {
		return nil, nil
	}
	rendered, missing, err := projectSubstituteValue(raw, values)
	if err != nil || len(missing) != 0 {
		return nil, usageError("project_execution_invalid: unresolved binding values")
	}
	object, ok := rendered.(map[string]any)
	if !ok {
		return nil, usageError("project_execution_invalid: execution_bindings must be an object")
	}
	result := &projectPackageExecution{SchemaVersion: projectExecutionVersion, Bindings: []prifly.ExecutionBinding{}}
	total := 0
	for group, rawBindings := range object {
		kind := strings.TrimSuffix(group, "s")
		if group != "steps" && group != "checks" {
			return nil, usageError("project_execution_invalid: unknown binding group " + group)
		}
		bindings, ok := rawBindings.(map[string]any)
		if !ok {
			return nil, usageError("project_execution_invalid: binding group must be an object")
		}
		for id, rawConfig := range bindings {
			var ref flow.Ref
			for _, component := range components {
				if component.Kind == kind && component.Ref.ID == id {
					if ref.ID != "" {
						return nil, usageError("project_execution_invalid: ambiguous component " + id)
					}
					ref = component.Ref
				}
			}
			if ref.ID == "" {
				return nil, usageError("project_execution_invalid: unknown owned component " + id)
			}
			fields, ok := rawConfig.(map[string]any)
			if !ok {
				return nil, usageError("project_execution_invalid: binding must be an object")
			}
			for key := range fields {
				if fields[key] == nil {
					return nil, usageError("project_execution_invalid: omit optional fields instead of null: " + key)
				}
				if key != "executable" && key != "args" && key != "files" && key != "timeout_ms" && key != "grace_ms" && key != "max_output_bytes" && key != "context_profile_ref" {
					return nil, usageError("project_execution_invalid: unknown binding field " + key)
				}
			}
			data, err := json.Marshal(fields)
			if err != nil {
				return nil, err
			}
			var config prifly.ExecutorConfig
			if err := json.Unmarshal(data, &config); err != nil {
				return nil, usageError("project_execution_invalid: " + err.Error())
			}
			if !projectLaunchID(config.Executable) {
				return nil, usageError("project_execution_invalid: executable must be a logical name")
			}
			if config.Args == nil {
				config.Args = []string{}
			}
			if config.Files == nil {
				config.Files = map[string]string{}
			}
			config.Environment = map[string]string{}
			binding := prifly.ExecutionBinding{DefinitionRef: ref, Config: config, Files: map[string][]byte{}}
			for _, name := range config.Files {
				if _, exists := binding.Files[name]; exists {
					continue
				}
				data, err := projectExecutionSource(projectRoot, source.Folder, name)
				if err != nil {
					return nil, err
				}
				total += len(data)
				if total > prifly.MaxArtifactBytes {
					return nil, usageError("project_execution_invalid: supporting files exceed the byte limit")
				}
				binding.Files[name] = data
			}
			result.Bindings = append(result.Bindings, binding)
		}
	}
	sort.Slice(result.Bindings, func(i, j int) bool {
		return result.Bindings[i].DefinitionRef.String() < result.Bindings[j].DefinitionRef.String()
	})
	if err := projectValidateExecution(result); err != nil {
		return nil, err
	}
	return result, nil
}

func projectExecutionSource(projectRoot, folder, source string) ([]byte, error) {
	if !fs.ValidPath(source) || source == "." || strings.Contains(source, "\\") {
		return nil, usageError("project_execution_invalid: supporting source must be a confined relative file")
	}
	base, err := os.OpenRoot(projectRoot)
	if err != nil {
		return nil, err
	}
	defer base.Close()
	relative, err := filepath.Rel(projectRoot, folder)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, usageError("project_execution_invalid: workflow folder escapes project")
	}
	// Open each directory without following links. Descriptor-relative traversal
	// keeps the same boundary even if a path is replaced while reading.
	parts := append(strings.Split(filepath.ToSlash(relative), "/"), strings.Split(source, "/")...)
	current := base
	for _, part := range parts[:len(parts)-1] {
		if part == "." {
			continue
		}
		info, err := current.Lstat(part)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, usageError("project_execution_invalid: supporting directories must not contain symlinks")
		}
		next, err := current.OpenRoot(part)
		if err != nil {
			return nil, err
		}
		defer next.Close()
		opened, err := next.Stat(".")
		if err != nil || !os.SameFile(info, opened) {
			return nil, usageError("project_execution_invalid: supporting directory changed while opening")
		}
		current = next
	}
	file, err := current.OpenFile(parts[len(parts)-1], os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > prifly.MaxArtifactBytes {
		return nil, usageError("project_execution_invalid: supporting source must be a bounded regular file")
	}
	data, err := io.ReadAll(io.LimitReader(file, prifly.MaxArtifactBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > prifly.MaxArtifactBytes {
		return nil, usageError("project_execution_invalid: supporting source exceeds byte limit")
	}
	return data, nil
}

// Reuse the closed runtime validator, but never persist or execute this
// synthetic absolute path; portable packages contain logical names only.
func projectValidateExecution(payload *projectPackageExecution) error {
	if payload.SchemaVersion != projectExecutionVersion {
		return usageError("project_execution_invalid: unsupported package execution version")
	}
	copy := prifly.ExecutionBindings{SchemaVersion: prifly.ExecutionBindingsVersion, Bindings: append([]prifly.ExecutionBinding{}, payload.Bindings...)}
	for i := range copy.Bindings {
		if !projectLaunchID(copy.Bindings[i].Config.Executable) {
			return usageError("project_execution_invalid: executable must be a logical name")
		}
		if len(copy.Bindings[i].Config.Environment) != 0 {
			return usageError("project_execution_invalid: package execution must not contain machine environment")
		}
		copy.Bindings[i].Config.Executable = "/" + copy.Bindings[i].Config.Executable
	}
	data, err := json.Marshal(copy)
	if err != nil {
		return err
	}
	return prifly.ValidateExecutionBindingsPayload(data)
}

func projectSealExecution(payload *projectPackageExecution, components []projectCompileComponent) error {
	if payload == nil {
		return nil
	}
	refs := make(map[flow.Ref]flow.Ref, len(components))
	for _, component := range components {
		refs[component.AuthorRef] = component.Ref
	}
	for i := range payload.Bindings {
		binding := &payload.Bindings[i]
		if ref, exists := refs[binding.DefinitionRef]; exists {
			binding.DefinitionRef = ref
		}
		if binding.Config.ContextProfileRef != nil {
			if ref, exists := refs[*binding.Config.ContextProfileRef]; exists {
				binding.Config.ContextProfileRef = &ref
			}
		}
	}
	return projectValidateExecution(payload)
}

func projectExecutionPayload(root string, compiled projectCompileResult, closure map[flow.Ref]bool, allow bool) (*prifly.ExecutionBindings, error) {
	result := &prifly.ExecutionBindings{SchemaVersion: prifly.ExecutionBindingsVersion, Bindings: []prifly.ExecutionBinding{}}
	if compiled.ExecutionBindings == nil {
		return result, nil
	}
	_, executables, err := projectLocalExecution(root)
	if err != nil {
		return nil, err
	}
	for _, binding := range compiled.ExecutionBindings.Bindings {
		if !closure[binding.DefinitionRef] {
			continue
		}
		if !allow {
			return nil, usageError("project_execution_approval_required: review the workflow programs, arguments and files, then pass --allow-execution")
		}
		path, allowed := executables[binding.Config.Executable]
		if !allowed {
			return nil, usageError("project_execution_not_allowed: use project local set --allow-executable " + binding.Config.Executable + "=/absolute/path")
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			return nil, usageError("project_execution_unavailable: selected executable is unavailable: " + binding.Config.Executable)
		}
		binding.Config.Executable = path
		result.Bindings = append(result.Bindings, binding)
	}
	return result, nil
}

func projectDecodeExecution(data []byte) (*projectPackageExecution, error) {
	if len(data) > prifly.MaxExecutionBindingsBytes {
		return nil, usageError("project_execution_invalid: metadata exceeds byte limit")
	}
	canonical, err := flow.Canonical(data)
	if err != nil {
		return nil, err
	}
	var payload projectPackageExecution
	decoder := json.NewDecoder(bytes.NewReader(canonical))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	if err := projectValidateExecution(&payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

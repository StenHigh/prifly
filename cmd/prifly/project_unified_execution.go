package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

func projectReadUnifiedExecution(root string, source projectPackageSource, components []projectCompileComponent, values map[string]any) (*projectPackageExecution, error) {
	result := &projectPackageExecution{SchemaVersion: projectExecutionVersion, Bindings: []prifly.ExecutionBinding{}}
	seen := map[flow.Ref]prifly.ExecutionBinding{}
	total := 0
	programs := slices.Clone(source.Composition.Programs)
	slices.SortStableFunc(programs, func(a, b projectCompositionProgram) int {
		if a.Extension != b.Extension {
			if a.Extension {
				return 1
			}
			return -1
		}
		return strings.Compare(a.Folder, b.Folder)
	})
	for _, program := range programs {
		groups, ok := program.Value.(map[string]any)
		if !ok {
			return nil, refusal("project_execution_invalid", "binding groups required")
		}
		for _, group := range slices.Sorted(maps.Keys(groups)) {
			if group != "steps" && group != "checks" {
				return nil, refusal("project_execution_invalid", "unknown binding group "+group)
			}
			entries, ok := groups[group].(map[string]any)
			if !ok {
				return nil, refusal("project_execution_invalid", "binding group must be object")
			}
			kind := strings.TrimSuffix(group, "s")
			for _, id := range slices.Sorted(maps.Keys(entries)) {
				aliases := program.Targets[kind+" "+id]
				if len(aliases) == 0 {
					return nil, refusal("project_execution_invalid", "unknown owned component "+id)
				}
				for _, component := range components {
					if component.Kind != kind || component.Ref.ID != id || !slices.ContainsFunc(aliases, func(alias string) bool { return projectCompositionComponentNamed(component, alias) }) {
						continue
					}
					if program.Extension {
						if component.Unified == nil || component.Unified.Owner != program.Owner || !strings.HasPrefix(component.Unified.Source, component.Unified.Owner+"/project/") {
							return nil, refusal("project_composition_program_override", id+": project program must belong to its declaring consumer")
						}
						var step flow.StepDefinition
						if err := json.Unmarshal(component.Bytes, &step); err != nil {
							return nil, err
						}
						if step.Executor.Operation != "process" {
							return nil, refusal("project_extension_invalid", id+": program binding requires process operation")
						}
					}
					config, err := projectUnifiedRewriteValue(entries[id], func(alias string) (string, error) {
						for _, c := range components {
							if projectCompositionComponentNamed(c, alias) {
								return c.Name, nil
							}
						}
						return alias, nil
					})
					if err != nil {
						return nil, err
					}
					part := source
					part.Composition = nil
					part.Unified = nil
					part.Folder = program.Folder
					part.RootValue = map[string]any{"execution_bindings": map[string]any{group: map[string]any{id: config}}}
					payload, err := projectReadExecution(root, part, []projectCompileComponent{component}, values, nil)
					if err != nil {
						return nil, err
					}
					for _, binding := range payload.Bindings {
						old, repeated := seen[binding.DefinitionRef]
						if repeated && (program.Extension || !projectCompositionEqual(old, binding)) {
							return nil, refusal("project_composition_program_conflict", id)
						}
						seen[binding.DefinitionRef] = binding
						for name, data := range binding.Files {
							if !repeated {
								total += len(data)
							}
							path, err := projectRelativePath(root, filepath.Join(program.Folder, filepath.FromSlash(name)))
							if err != nil {
								return nil, err
							}
							source.Composition.Files[path] = fmt.Sprintf("sha256:%x", sha256.Sum256(data))
						}
						if total > prifly.MaxArtifactBytes {
							return nil, refusal("project_execution_invalid", "supporting files exceed byte limit")
						}
						if !repeated {
							result.Bindings = append(result.Bindings, binding)
						}
					}
				}
			}
		}
	}
	if len(result.Bindings) == 0 {
		return nil, nil
	}
	slices.SortFunc(result.Bindings, func(a, b prifly.ExecutionBinding) int {
		return strings.Compare(a.DefinitionRef.String(), b.DefinitionRef.String())
	})
	if err := projectValidateExecution(result); err != nil {
		return nil, err
	}
	return result, nil
}

func projectValidateCompilationBindings(source projectPackageSource, components []projectCompileComponent, options projectWorkflowOptions) error {
	if source.Unified == nil {
		return projectValidateCallExtensionBindings(components, options)
	}
	// The new reader checks positive source ownership and selected closure per variant.
	return nil
}

func projectUnifiedProgramTargets(documents []projectPackageDocument, owner string) map[string][]string {
	owned := []projectPackageDocument{}
	for _, d := range documents {
		if d.Unified != nil && d.Unified.Owner == owner {
			owned = append(owned, d)
		}
	}
	return projectCompositionProgramTargets(owned)
}

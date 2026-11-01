package main

import (
	"maps"
	"slices"
)

func projectApplyCompilationOptions(source projectPackageSource, pending []projectPendingDocument, options projectWorkflowOptions) error {
	if source.Unified == nil {
		return projectApplyWorkflowOptions(pending, options)
	}
	// A source declares a feature once. Its isolated variants all retain that
	// declaration, and exclusion applies independently to each selected variant.
	owners := map[string][4]string{}
	workflows := map[string]bool{}
	for _, item := range pending {
		if item.document.Kind != "workflow" {
			continue
		}
		object, _ := item.value.(map[string]any)
		declared, _ := object["features"].(map[string]any)
		workflow := item.document.Name
		if workflow == "" {
			id, _ := object["id"].(string)
			workflow = projectExtensionWorkflowName(id)
		}
		workflows[workflow] = true
		local := options
		local.Settings = nil
		if settings, ok := options.Settings[workflow]; ok {
			local.Settings = map[string]map[string]any{workflow: settings}
		}
		local.Exclude = nil
		for _, name := range options.Exclude {
			if _, exists := declared[name]; exists {
				local.Exclude = append(local.Exclude, name)
			}
		}
		meta := item.document.Unified
		if meta != nil {
			for _, name := range local.Exclude {
				feature, _ := declared[name].(map[string]any)
				input, _ := feature["input"].(string)
				if value, exists := meta.Settings[input]; exists {
					if enabled, ok := value.(bool); !ok || enabled {
						return projectCompositionSourceError(item.document.Source, refusal("project_option_conflict", "exclude "+name+" conflicts with setting "+meta.AuthorID+"."+input))
					}
				}
			}
		}
		if err := projectApplyWorkflowOptions([]projectPendingDocument{item}, local); err != nil {
			return projectCompositionSourceError(item.document.Source, err)
		}
		origin := [4]string{item.document.Source, workflow, "", ""}
		if meta != nil {
			origin = [4]string{meta.Owner, meta.Source, meta.AuthorID, meta.AuthorVersion}
		}
		for _, name := range slices.Sorted(maps.Keys(declared)) {
			if prior, exists := owners[name]; exists && prior != origin {
				return refusal("project_feature_duplicate", name)
			}
			owners[name] = origin
		}
	}
	for _, name := range options.Exclude {
		if _, exists := owners[name]; !exists {
			return refusal("project_option_unknown_feature", name)
		}
	}
	for name := range options.Settings {
		if !workflows[name] {
			return refusal("project_option_unknown_workflow", name)
		}
	}
	return nil
}

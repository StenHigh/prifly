package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
	"go.yaml.in/yaml/v3"
)

// projectLocalSettings is what this machine, and only this machine, says about
// running a launch here: where the authority lives, which programs the owner
// allowed, the environment they get, and where a value that is not written
// down is read from.
type projectLocalSettings struct {
	Authority       string
	Executables     map[string]string
	Environment     map[string]string
	EnvironmentFrom map[string]prifly.EnvironmentSource
}

func projectLocalExecution(root string) (authority string, executables map[string]string, err error) {
	_, settings, err := readProjectLocalExecutionAll(root)
	return settings.Authority, settings.Executables, err
}

// projectLocalEnvironment is the machine's environment for the programs a
// launch runs here, from local.yaml; empty when none was set.
func projectLocalEnvironment(root string) (map[string]string, error) {
	_, settings, err := readProjectLocalExecutionAll(root)
	return settings.Environment, err
}

// projectLocalEnvironmentSources reads the declarations that name where a
// value comes from. The value itself is never here: that is the point.
func projectLocalEnvironmentSources(root string) (map[string]prifly.EnvironmentSource, error) {
	_, settings, err := readProjectLocalExecutionAll(root)
	return settings.EnvironmentFrom, err
}

func readProjectLocalExecution(root string) ([]byte, string, map[string]string, error) {
	data, settings, err := readProjectLocalExecutionAll(root)
	return data, settings.Authority, settings.Executables, err
}

// One read, one parse, one set of checks: a second reader of the same file is
// a second opinion about it, and the two drift.
func readProjectLocalExecutionAll(root string) ([]byte, projectLocalSettings, error) {
	profile := filepath.Join(root, ".prifly")
	info, err := os.Lstat(profile)
	if errors.Is(err, os.ErrNotExist) {
		return nil, projectLocalSettings{}, usageError("project_local_missing: run project init before using .prifly/local.yaml")
	}
	if err != nil {
		return nil, projectLocalSettings{}, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, projectLocalSettings{}, usageError("project_local_invalid: .prifly must be a real directory")
	}
	data, err := readFile(filepath.Join(profile, "local.yaml"), flow.MaxDocumentBytes)
	if errors.Is(err, os.ErrNotExist) {
		return nil, projectLocalSettings{}, usageError("project_local_missing: run project init before using .prifly/local.yaml")
	}
	if err != nil {
		return nil, projectLocalSettings{}, usageError("project_local_invalid: local.yaml must be a bounded regular file, not a symlink")
	}
	value, err := flow.Parse(data, "yaml")
	if err != nil {
		return nil, projectLocalSettings{}, usageError("project_local_invalid: " + err.Error())
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, projectLocalSettings{}, usageError("project_local_invalid: local.yaml must be an object")
	}
	for _, key := range []string{"authority_root", "prifly_executable"} {
		path, ok := object[key].(string)
		if !ok || !filepath.IsAbs(path) {
			return nil, projectLocalSettings{}, usageError("project_local_invalid: " + key + " must be an absolute path")
		}
	}
	authority, err := canonicalProjectPath(object["authority_root"].(string))
	if err != nil {
		return nil, projectLocalSettings{}, err
	}
	project, err := canonicalProjectPath(root)
	if err != nil {
		return nil, projectLocalSettings{}, err
	}
	if projectPathsOverlap(project, authority) {
		return nil, projectLocalSettings{}, usageError("unsafe_authority_root: local authority data must be outside the project")
	}
	executables := make(map[string]string)
	if raw, exists := object["executables"]; exists {
		mapping, ok := raw.(map[string]any)
		if !ok {
			return nil, projectLocalSettings{}, usageError("project_local_invalid: executables must be an object")
		}
		for name, rawPath := range mapping {
			path, ok := rawPath.(string)
			if !projectLaunchID(name) || !ok || !filepath.IsAbs(path) {
				return nil, projectLocalSettings{}, usageError("project_local_invalid: executables must map simple names to absolute paths")
			}
			executables[name] = path
		}
	}
	environment := map[string]string{}
	if raw, exists := object["environment"]; exists {
		mapping, ok := raw.(map[string]any)
		if !ok {
			return nil, projectLocalSettings{}, usageError("project_local_invalid: environment must be an object")
		}
		for name, rawValue := range mapping {
			value, ok := rawValue.(string)
			if !ok || name == "" || strings.ContainsAny(name, "=\x00") || strings.HasPrefix(name, "PRIFLY_") {
				return nil, projectLocalSettings{}, usageError("project_local_invalid: environment must map names to strings; PRIFLY_ names are the engine's")
			}
			environment[name] = value
		}
	}
	sources := map[string]prifly.EnvironmentSource{}
	if raw, exists := object["environment_from"]; exists {
		mapping, ok := raw.(map[string]any)
		if !ok {
			return nil, projectLocalSettings{}, usageError("project_local_invalid: environment_from must be an object")
		}
		for name, rawSource := range mapping {
			fields, ok := rawSource.(map[string]any)
			if !ok || name == "" || strings.ContainsAny(name, "=\x00") || strings.HasPrefix(name, "PRIFLY_") {
				return nil, projectLocalSettings{}, usageError("project_local_invalid: environment_from maps a name to one source object; PRIFLY_ names are the engine's")
			}
			if _, literal := environment[name]; literal {
				return nil, projectLocalSettings{}, usageError("project_local_invalid: " + name + " is given both a value and a source; keep one")
			}
			source := prifly.EnvironmentSource{}
			for key, rawValue := range fields {
				text, ok := rawValue.(string)
				if !ok {
					return nil, projectLocalSettings{}, usageError("project_local_invalid: environment_from fields are strings")
				}
				switch key {
				case "env":
					source.Env = text
				case "file":
					source.File = text
				case "dotenv":
					source.DotEnv = text
				case "key":
					source.Key = text
				default:
					return nil, projectLocalSettings{}, usageError("project_local_invalid: environment_from has unknown field " + key)
				}
			}
			// A hand-edited file is checked here, once, rather than at the
			// start of a Run that has already claimed a workspace.
			if err := prifly.ValidateEnvironmentSource(source); err != nil {
				return nil, projectLocalSettings{}, usageError("project_local_invalid: environment_from " + name + ": " + err.Error())
			}
			sources[name] = source
		}
	}
	return data, projectLocalSettings{Authority: authority, Executables: executables, Environment: environment, EnvironmentFrom: sources}, nil
}

// projectParseEnvironmentSource reads one NAME=SOURCE argument. The forms are
// env:VAR, file:/absolute/path and dotenv:/absolute/path:KEY, where the key is
// after the last colon because an environment name cannot contain one.
func projectParseEnvironmentSource(argument string) (string, prifly.EnvironmentSource, error) {
	name, spec, ok := strings.Cut(argument, "=")
	if !ok || name == "" || strings.ContainsAny(name, "=\x00") || strings.HasPrefix(name, "PRIFLY_") {
		return "", prifly.EnvironmentSource{}, usageError("project_local_invalid_environment: use NAME=env:VAR, NAME=file:/path or NAME=dotenv:/path:KEY; PRIFLY_ names are the engine's")
	}
	kind, rest, ok := strings.Cut(spec, ":")
	if !ok || rest == "" {
		return "", prifly.EnvironmentSource{}, usageError("project_local_invalid_environment: " + name + " names no source: use env:VAR, file:/path or dotenv:/path:KEY")
	}
	source := prifly.EnvironmentSource{}
	switch kind {
	case "env":
		source.Env = rest
	case "file":
		source.File = rest
	case "dotenv":
		path, key, found := lastCut(rest, ":")
		if !found || key == "" {
			return "", prifly.EnvironmentSource{}, usageError("project_local_invalid_environment: " + name + " needs the key to read: dotenv:/path:KEY")
		}
		source.DotEnv, source.Key = path, key
	default:
		return "", prifly.EnvironmentSource{}, usageError("project_local_invalid_environment: " + name + " names an unknown source " + kind)
	}
	if source.File != "" || source.DotEnv != "" {
		// A relative path would be resolved against whatever directory the
		// tool happened to run in, which is not where the owner thinks the
		// file is. The path is named absolutely or not at all.
		if !filepath.IsAbs(source.File + source.DotEnv) {
			return "", prifly.EnvironmentSource{}, usageError("project_local_invalid_environment: " + name + " names a file by an absolute path")
		}
		absolute, err := canonicalProjectPath(source.File + source.DotEnv)
		if err != nil {
			return "", prifly.EnvironmentSource{}, err
		}
		if source.File != "" {
			source.File = absolute
		} else {
			source.DotEnv = absolute
		}
	}
	return name, source, nil
}

// projectEnvironmentSourcePlace says where a value is read from, in the same
// words the owner wrote it in. It never reads the source.
func projectEnvironmentSourcePlace(source prifly.EnvironmentSource) string {
	switch {
	case source.Env != "":
		return "env:" + source.Env
	case source.File != "":
		return "file:" + source.File
	default:
		return "dotenv:" + source.DotEnv + ":" + source.Key
	}
}

func lastCut(value, separator string) (before, after string, found bool) {
	index := strings.LastIndex(value, separator)
	if index < 0 {
		return value, "", false
	}
	return value[:index], value[index+len(separator):], true
}

// The environment is the machine's like the executable paths are: a program
// the project binds needs the PATH its tools live on and the APP_ENV its suite
// expects, and both belong beside the binary in ignored local.yaml, never in
// the shared package or extend.yaml. Every program of a launch on this machine
// receives it; a package's own program is no less machine-bound at run time.
func (c *cli) projectLocalAllowExecutables(root string, current []byte, executable string, allowed, environment, environmentFrom []string) error {
	selectedSources := make(map[string]prifly.EnvironmentSource, len(environmentFrom))
	for _, argument := range environmentFrom {
		name, source, err := projectParseEnvironmentSource(argument)
		if err != nil {
			return err
		}
		if _, exists := selectedSources[name]; exists {
			return usageError("project_local_invalid_environment: duplicate name " + name)
		}
		if err := prifly.ValidateEnvironmentSource(source); err != nil {
			return usageError("project_local_invalid_environment: " + name + ": " + err.Error())
		}
		selectedSources[name] = source
	}
	selectedEnvironment := make(map[string]string, len(environment))
	for _, argument := range environment {
		name, value, ok := strings.Cut(argument, "=")
		if !ok || name == "" || strings.ContainsAny(name, "=\x00") || strings.HasPrefix(name, "PRIFLY_") || strings.ContainsRune(value, 0) {
			return usageError("project_local_invalid_environment: use NAME=VALUE; PRIFLY_ names are the engine's")
		}
		if _, exists := selectedEnvironment[name]; exists {
			return usageError("project_local_invalid_environment: duplicate name " + name)
		}
		selectedEnvironment[name] = value
	}
	selected := make(map[string]string, len(allowed))
	for _, argument := range allowed {
		name, path, ok := strings.Cut(argument, "=")
		if !ok || !projectLaunchID(name) || !filepath.IsAbs(path) {
			return usageError("project_local_invalid_executable: use a simple name and absolute path: NAME=/path/to/program")
		}
		if _, exists := selected[name]; exists {
			return usageError("project_local_invalid_executable: duplicate executable name " + name)
		}
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			return usageError("project_local_invalid_executable: " + name + " must name an existing executable regular file")
		}
		selected[name] = path
	}
	var document yaml.Node
	if err := yaml.Unmarshal(current, &document); err != nil {
		return err
	}
	object := document.Content[0]
	mapping := projectMappingValue(object, "executables")
	if mapping == nil {
		mapping = projectMappingNode()
		projectMappingSet(object, "executables", mapping)
	}
	for _, argument := range allowed {
		name, _, _ := strings.Cut(argument, "=")
		projectMappingSet(mapping, name, projectScalarNode(selected[name]))
	}
	if len(selectedEnvironment) != 0 {
		environmentNode := projectMappingValue(object, "environment")
		if environmentNode == nil {
			environmentNode = projectMappingNode()
			projectMappingSet(object, "environment", environmentNode)
		}
		for _, argument := range environment {
			name, _, _ := strings.Cut(argument, "=")
			projectMappingSet(environmentNode, name, projectScalarNode(selectedEnvironment[name]))
		}
	}
	if len(selectedSources) != 0 {
		sourcesNode := projectMappingValue(object, "environment_from")
		if sourcesNode == nil {
			sourcesNode = projectMappingNode()
			projectMappingSet(object, "environment_from", sourcesNode)
		}
		for name, source := range selectedSources {
			entry := projectMappingNode()
			for key, value := range map[string]string{"env": source.Env, "file": source.File, "dotenv": source.DotEnv, "key": source.Key} {
				if value != "" {
					projectMappingSet(entry, key, projectScalarNode(value))
				}
			}
			projectMappingSet(sourcesNode, name, entry)
		}
	}
	if executable != "" {
		projectMappingSet(object, "prifly_executable", projectScalarNode(executable))
	}
	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return err
	}
	if err := encoder.Close(); err != nil {
		return err
	}
	if err := replaceProjectRunner(filepath.Join(root, ".prifly", "local.yaml"), buffer.String()); err != nil {
		return err
	}
	_, settings, err := readProjectLocalExecutionAll(root)
	if err != nil {
		return err
	}
	// The receipt confirms the place, never the value: a name whose value is
	// read at start is shown as the source it will be read from.
	sources := make(map[string]string, len(settings.EnvironmentFrom))
	for name, source := range settings.EnvironmentFrom {
		sources[name] = projectEnvironmentSourcePlace(source)
	}
	// Both halves are read from the file, not from this call's arguments: a
	// receipt that printed only what was passed read as "the rest is gone"
	// after a call that touched one of them.
	return c.emit(map[string]any{"schema_version": "prifly-project-local/3", "repository": root, "prifly_executable": projectMappingValue(object, "prifly_executable").Value, "allowed_executables": settings.Executables, "environment": settings.Environment, "environment_from": sources})
}

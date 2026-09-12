package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	"go.yaml.in/yaml/v3"
)

func projectLocalExecution(root string) (authority string, executables map[string]string, err error) {
	_, authority, executables, err = readProjectLocalExecution(root)
	return
}

// projectLocalEnvironment is the machine's environment for the programs a
// launch runs here, from local.yaml; empty when none was set.
func projectLocalEnvironment(root string) (map[string]string, error) {
	_, _, _, environment, err := readProjectLocalExecutionAll(root)
	return environment, err
}

func readProjectLocalExecution(root string) ([]byte, string, map[string]string, error) {
	data, authority, executables, _, err := readProjectLocalExecutionAll(root)
	return data, authority, executables, err
}

func readProjectLocalExecutionAll(root string) ([]byte, string, map[string]string, map[string]string, error) {
	profile := filepath.Join(root, ".prifly")
	info, err := os.Lstat(profile)
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", nil, nil, usageError("project_local_missing: run project init before using .prifly/local.yaml")
	}
	if err != nil {
		return nil, "", nil, nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, "", nil, nil, usageError("project_local_invalid: .prifly must be a real directory")
	}
	data, err := readFile(filepath.Join(profile, "local.yaml"), flow.MaxDocumentBytes)
	if errors.Is(err, os.ErrNotExist) {
		return nil, "", nil, nil, usageError("project_local_missing: run project init before using .prifly/local.yaml")
	}
	if err != nil {
		return nil, "", nil, nil, usageError("project_local_invalid: local.yaml must be a bounded regular file, not a symlink")
	}
	value, err := flow.Parse(data, "yaml")
	if err != nil {
		return nil, "", nil, nil, usageError("project_local_invalid: " + err.Error())
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, "", nil, nil, usageError("project_local_invalid: local.yaml must be an object")
	}
	for _, key := range []string{"authority_root", "prifly_executable"} {
		path, ok := object[key].(string)
		if !ok || !filepath.IsAbs(path) {
			return nil, "", nil, nil, usageError("project_local_invalid: " + key + " must be an absolute path")
		}
	}
	authority, err := canonicalProjectPath(object["authority_root"].(string))
	if err != nil {
		return nil, "", nil, nil, err
	}
	project, err := canonicalProjectPath(root)
	if err != nil {
		return nil, "", nil, nil, err
	}
	if projectPathsOverlap(project, authority) {
		return nil, "", nil, nil, usageError("unsafe_authority_root: local authority data must be outside the project")
	}
	executables := make(map[string]string)
	if raw, exists := object["executables"]; exists {
		mapping, ok := raw.(map[string]any)
		if !ok {
			return nil, "", nil, nil, usageError("project_local_invalid: executables must be an object")
		}
		for name, rawPath := range mapping {
			path, ok := rawPath.(string)
			if !projectLaunchID(name) || !ok || !filepath.IsAbs(path) {
				return nil, "", nil, nil, usageError("project_local_invalid: executables must map simple names to absolute paths")
			}
			executables[name] = path
		}
	}
	environment := map[string]string{}
	if raw, exists := object["environment"]; exists {
		mapping, ok := raw.(map[string]any)
		if !ok {
			return nil, "", nil, nil, usageError("project_local_invalid: environment must be an object")
		}
		for name, rawValue := range mapping {
			value, ok := rawValue.(string)
			if !ok || name == "" || strings.ContainsAny(name, "=\x00") || strings.HasPrefix(name, "PRIFLY_") {
				return nil, "", nil, nil, usageError("project_local_invalid: environment must map names to strings; PRIFLY_ names are the engine's")
			}
			environment[name] = value
		}
	}
	return data, authority, executables, environment, nil
}

// The environment is the machine's like the executable paths are: a program
// the project binds needs the PATH its tools live on and the APP_ENV its suite
// expects, and both belong beside the binary in ignored local.yaml, never in
// the shared package or extend.yaml. Every program of a launch on this machine
// receives it; a package's own program is no less machine-bound at run time.
func (c *cli) projectLocalAllowExecutables(root string, current []byte, executable string, allowed, environment []string) error {
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
	currentEnvironment, err := projectLocalEnvironment(root)
	if err != nil {
		return err
	}
	return c.emit(map[string]any{"schema_version": "prifly-project-local/2", "repository": root, "prifly_executable": projectMappingValue(object, "prifly_executable").Value, "allowed_executables": selected, "environment": currentEnvironment})
}

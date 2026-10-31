package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

const projectPointExtensionVersion = "prifly-extension/4"
const projectExtensionPointsVersion = "prifly-extension-points/1"
const projectExtensionPointsFile = "extension-points.yaml"

type projectExtensionPoints struct {
	Authoring string                           `json:"authoring"`
	Points    map[string]projectExtensionPoint `json:"points"`
}
type projectExtensionPoint struct {
	Workflow      string                          `json:"workflow"`
	Entry         string                          `json:"entry,omitempty"`
	Between       *projectPointBetween            `json:"between,omitempty"`
	InputBindings map[string]string               `json:"input_bindings"`
	Consumers     map[string]projectPointConsumer `json:"consumers"`
	Routes        projectPointRoutes              `json:"routes"`
}
type projectPointBetween struct {
	From string `json:"from"`
	To   string `json:"to"`
}
type projectPointConsumer struct {
	Stage  string `json:"stage"`
	Input  string `json:"input,omitempty"`
	Output string `json:"output,omitempty"`
}
type projectPointRoutes struct {
	Step *projectPointRoute `json:"step,omitempty"`
	Call *projectPointRoute `json:"call,omitempty"`
}
type projectPointRoute struct {
	On                 map[string]string `json:"on"`
	OnError            string            `json:"on_error,omitempty"`
	ImpossibleVerdicts []string          `json:"impossible_verdicts,omitempty"`
}

// The embedded editor form is also the closed intake contract. JSON's
// decoder accepts null for optional scalars; authoring requires actual types.
func projectValidatePointShape(name string, value any) error {
	schema, err := prifly.AuthoringSchema(name)
	if err != nil {
		return err
	}
	schema, err = flow.Canonical(schema)
	if err != nil {
		return err
	}
	digest, err := flow.Digest(schema)
	if err != nil {
		return err
	}
	ref := flow.Ref{ID: "core:schema/project-point-authoring", Version: "1.0.0", Digest: digest}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := flow.ValidateSchema(flow.Registry{ref: schema}, ref, data); err != nil {
		return refusal("project_extension_point_invalid", err.Error())
	}
	return nil
}

func projectPointDecode(value any, target any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return refusal("project_extension_point_invalid", err.Error())
	}
	return nil
}

func projectReadExtensionPoints(root, folder string) (*projectExtensionPoints, string, error) {
	path := filepath.Join(folder, projectExtensionPointsFile)
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil, "", nil
	} else if err != nil {
		return nil, "", err
	}
	relative, err := projectRelativePath(root, path)
	if err != nil {
		return nil, "", err
	}
	checked, err := projectPackageSourcePath(root, relative)
	if err != nil {
		return nil, "", err
	}
	data, err := readFile(checked, flow.MaxDocumentBytes)
	if err != nil {
		return nil, "", err
	}
	value, err := flow.Parse(data, "yaml")
	if err != nil {
		return nil, "", refusal("project_extension_point_invalid", err.Error())
	}
	if err := projectValidatePointShape("extension-points-v1", value); err != nil {
		return nil, "", err
	}
	var points projectExtensionPoints
	if err := projectPointDecode(value, &points); err != nil {
		return nil, "", err
	}
	if points.Authoring != projectExtensionPointsVersion || len(points.Points) == 0 || len(points.Points) > 256 {
		return nil, "", refusal("project_extension_point_invalid", "extension-points.yaml requires authoring: prifly-extension-points/1 and 1..256 points")
	}
	for name, point := range points.Points {
		if !projectValueName.MatchString(name) || point.Workflow == "" || ((point.Entry == "") == (point.Between == nil)) || point.InputBindings == nil || point.Consumers == nil || len(point.InputBindings) > 256 || len(point.Consumers) > 256 {
			return nil, "", refusal("project_extension_point_invalid", name+" requires workflow, exactly one entry/between, input_bindings and consumers")
		}
		if point.Between != nil && (point.Between.From == "" || point.Between.To == "") {
			return nil, "", refusal("project_extension_point_invalid", name+".between requires from and to")
		}
		if point.Routes.Step == nil && point.Routes.Call == nil {
			return nil, "", refusal("project_extension_point_invalid", name+" requires a step or call route contract")
		}
		for port, expression := range point.InputBindings {
			if !projectValueName.MatchString(port) || !projectPointBindingExpression(expression) {
				return nil, "", refusal("project_extension_point_invalid", name+".input_bindings requires $inputs.PORT or $stages.STAGE.PORT")
			}
		}
		for alias, consumer := range point.Consumers {
			if !projectValueName.MatchString(alias) || consumer.Stage == "" || ((consumer.Input == "") == (consumer.Output == "")) {
				return nil, "", refusal("project_extension_point_invalid", name+".consumers requires named stage and exactly one input/output")
			}
		}
		for _, route := range []*projectPointRoute{point.Routes.Step, point.Routes.Call} {
			if route == nil {
				continue
			}
			if len(route.On) == 0 || len(route.On) > 256 {
				return nil, "", refusal("project_extension_point_invalid", name+".routes requires nonempty on")
			}
			for verdict, target := range route.On {
				if verdict == "" || target == "" {
					return nil, "", refusal("project_extension_point_invalid", name+" routes require nonempty strings")
				}
			}
			for _, verdict := range route.ImpossibleVerdicts {
				if !slices.Contains(flow.StepVerdicts, verdict) {
					return nil, "", refusal("project_extension_point_invalid", name+" has unknown impossible verdict")
				}
			}
		}
		if point.Routes.Call != nil && len(point.Routes.Call.ImpossibleVerdicts) > 0 {
			return nil, "", refusal("project_extension_point_invalid", name+" call routes cannot declare impossible_verdicts")
		}
	}
	return &points, relative, nil
}

func projectPointBindingExpression(expression string) bool {
	if strings.HasPrefix(expression, "$inputs.") {
		return projectValueName.MatchString(strings.TrimPrefix(expression, "$inputs."))
	}
	return projectCallBindingExpression(expression)
}

func projectParsePointExtension(object map[string]any) (projectWorkflowExtension, error) {
	type entry struct {
		ID             string            `json:"id"`
		Point          string            `json:"point"`
		Step           string            `json:"step,omitempty"`
		Call           string            `json:"call,omitempty"`
		OutputBindings map[string]string `json:"output_bindings"`
		Enabled        *bool             `json:"enabled,omitempty"`
	}
	var value entry
	if err := projectPointDecode(object, &value); err != nil {
		return projectWorkflowExtension{}, err
	}
	if !projectValueName.MatchString(value.ID) || !projectValueName.MatchString(value.Point) || ((value.Step == "") == (value.Call == "")) || value.OutputBindings == nil || len(value.OutputBindings) > 256 {
		return projectWorkflowExtension{}, refusal("project_extension_point_invalid", "point extension requires id, point, exactly one step/call and output_bindings")
	}
	implementation := value.Step
	if value.Call != "" {
		implementation = value.Call
	}
	if !projectValueName.MatchString(implementation) {
		return projectWorkflowExtension{}, refusal("project_extension_point_invalid", "step/call must name a project component")
	}
	for consumer, output := range value.OutputBindings {
		if !projectValueName.MatchString(consumer) || !projectValueName.MatchString(output) {
			return projectWorkflowExtension{}, refusal("project_extension_point_invalid", "output_bindings maps declared consumers to output port names")
		}
	}
	return projectWorkflowExtension{ID: value.ID, Point: value.Point, Step: value.Step, Call: value.Call, OutputBindings: value.OutputBindings, Enabled: value.Enabled}, nil
}

func projectExtensionEnabled(extension projectWorkflowExtension) bool {
	return extension.Enabled == nil || *extension.Enabled
}

// Resolve target and ownership before compilation, so a parent never receives
// the old child ref and no dependency is smuggled into an unrelated workflow.
func projectPreparePointExtensions(source projectPackageSource, pending []projectPendingDocument, extensions []projectWorkflowExtension) (map[string][]projectWorkflowExtension, error) {
	result := map[string][]projectWorkflowExtension{}
	ownedPrefix := ""
	for _, document := range source.Documents {
		if document.FolderRoot {
			ownedPrefix = filepath.ToSlash(filepath.Join(filepath.Dir(document.Source), projectOwnedFolder)) + "/"
			break
		}
	}
	byID := map[string][]int{}
	for i, item := range pending {
		if item.document.Kind == "workflow" {
			object, _ := item.value.(map[string]any)
			id, _ := object["id"].(string)
			byID[id] = append(byID[id], i)
		}
	}
	points := map[string]projectExtensionPoint{}
	if source.ExtensionPoints != nil {
		points = source.ExtensionPoints.Points
	}
	names := make([]string, 0, len(points))
	for name := range points {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		point := points[name]
		candidates := byID[point.Workflow]
		if len(candidates) == 0 {
			return nil, refusal("project_extension_point_unknown_workflow", name+": "+point.Workflow)
		}
		if len(candidates) != 1 {
			return nil, refusal("project_extension_point_ambiguous", name+": workflow identity is not unique")
		}
		item := pending[candidates[0]]
		if projectPointOwned(item.document.Source, ownedPrefix) {
			return nil, refusal("project_extension_point_invalid", name+": target must be package-owned")
		}
		object, _ := item.value.(map[string]any)
		graph := object
		if definition, ok := object["definition"].(map[string]any); ok {
			graph = definition
		}
		if err := projectPointValidateAnchor(graph, point); err != nil {
			return nil, err
		}
		stages, _ := graph["stages"].(map[string]any)
		for _, consumer := range point.Consumers {
			if err := projectPointConsumerBinding(stages, consumer); err != nil {
				return nil, err
			}
		}
		for _, routes := range []*projectPointRoute{point.Routes.Step, point.Routes.Call} {
			if routes == nil {
				continue
			}
			for _, target := range routes.On {
				if _, ok := stages[target]; !ok {
					return nil, refusal("project_extension_point_unknown_anchor", name+": route target "+target)
				}
			}
			if routes.OnError != "" {
				if _, ok := stages[routes.OnError]; !ok {
					return nil, refusal("project_extension_point_unknown_anchor", name+": on_error target "+routes.OnError)
				}
			}
		}
	}
	occupied := map[string]bool{}
	for _, extension := range extensions {
		if extension.Point == "" {
			continue
		}
		point, ok := points[extension.Point]
		if !ok {
			return nil, refusal("project_extension_point_unknown", extension.Point)
		}
		if len(extension.OutputBindings) != len(point.Consumers) {
			return nil, refusal("project_extension_point_binding_not_permitted", extension.Point+": bind every declared consumer exactly once")
		}
		for alias := range extension.OutputBindings {
			if _, ok := point.Consumers[alias]; !ok {
				return nil, refusal("project_extension_point_binding_not_permitted", extension.Point+": "+alias)
			}
		}
		if !projectExtensionEnabled(extension) {
			continue
		}
		anchor := point.Workflow + "/entry"
		if point.Between != nil {
			anchor = point.Workflow + "/" + point.Between.From + "/" + point.Between.To
		}
		if occupied[anchor] {
			return nil, refusal("project_extension_point_conflict", extension.Point+": anchor already selected")
		}
		occupied[anchor] = true
		for _, consumer := range point.Consumers {
			key := point.Workflow + "/consumer/" + consumer.Stage + "/" + consumer.Input + "/" + consumer.Output
			if occupied[key] {
				return nil, refusal("project_extension_point_conflict", extension.Point+": consumer already selected")
			}
			occupied[key] = true
		}
		kind, name := "step", extension.Step
		if extension.Call != "" {
			kind, name = "workflow", extension.Call
		}
		matches := 0
		for _, item := range pending {
			object, _ := item.value.(map[string]any)
			id, _ := object["id"].(string)
			if item.document.Kind == kind && projectExtensionWorkflowName(id) == name {
				if !projectPointOwned(item.document.Source, ownedPrefix) {
					return nil, refusal("project_extension_point_component_not_permitted", name+" must be declared in project/")
				}
				matches++
			}
		}
		if matches == 0 {
			return nil, refusal("project_extension_point_unknown_component", name)
		}
		if matches != 1 {
			return nil, refusal("project_extension_point_ambiguous", name)
		}
		routes := point.Routes.Step
		if kind == "workflow" {
			routes = point.Routes.Call
		}
		if routes == nil {
			return nil, refusal("project_extension_point_component_not_permitted", extension.Point+": implementation kind is not declared")
		}
		extension.ResolvedPoint = &point
		result[point.Workflow] = append(result[point.Workflow], extension)
	}
	if err := projectPointDependencyCycles(pending, result, projectPointRootExtensions(extensions)); err != nil {
		return nil, err
	}
	return result, nil
}

func projectPointOwned(source, prefix string) bool {
	return prefix != "" && strings.HasPrefix(filepath.ToSlash(source), prefix)
}

func projectPointValidateAnchor(graph map[string]any, point projectExtensionPoint) error {
	stages, ok := graph["stages"].(map[string]any)
	if !ok {
		return refusal("project_extension_point_invalid", "workflow requires stages")
	}
	if point.Entry != "" {
		if graph["entry"] != point.Entry {
			return refusal("project_extension_point_unknown_anchor", "entry must name the exact workflow entry")
		}
		if _, ok := stages[point.Entry]; !ok {
			return refusal("project_extension_point_unknown_anchor", point.Entry)
		}
		return nil
	}
	if _, ok := stages[point.Between.To]; !ok {
		return refusal("project_extension_point_unknown_anchor", point.Between.To)
	}
	from, ok := stages[point.Between.From].(map[string]any)
	if !ok {
		return refusal("project_extension_point_unknown_anchor", point.Between.From)
	}
	count := 0
	for _, group := range []string{"on", "on_complete"} {
		values, _ := from[group].(map[string]any)
		for _, target := range values {
			if target == point.Between.To {
				count++
			}
		}
	}
	for _, field := range []string{"on_limit", "on_error", "on_unknown", "default"} {
		if from[field] == point.Between.To {
			count++
		}
	}
	if count == 0 {
		return refusal("project_extension_point_unknown_anchor", point.Between.From+" -> "+point.Between.To)
	}
	if count != 1 {
		return refusal("project_extension_point_ambiguous", point.Between.From+" -> "+point.Between.To)
	}
	return nil
}

func projectPointConsumerBinding(stages map[string]any, consumer projectPointConsumer) error {
	stage, ok := stages[consumer.Stage].(map[string]any)
	if !ok {
		return refusal("project_extension_point_unknown_consumer", consumer.Stage)
	}
	field, port := "input_bindings", consumer.Input
	if consumer.Output != "" {
		field, port = "output_bindings", consumer.Output
		if stage["kind"] != "finish" {
			return refusal("project_extension_point_binding_not_permitted", "output consumer must be a finish stage")
		}
	}
	bindings, ok := stage[field].(map[string]any)
	if !ok {
		return refusal("project_extension_point_unknown_consumer", consumer.Stage+"."+port)
	}
	if _, ok := bindings[port]; !ok {
		return refusal("project_extension_point_unknown_consumer", consumer.Stage+"."+port)
	}
	return nil
}

func projectPointExtensionsReady(components []projectCompileComponent, extensions []projectWorkflowExtension) bool {
	for _, extension := range extensions {
		kind, name := "step", extension.Step
		if extension.Call != "" {
			kind, name = "workflow", extension.Call
		}
		found := false
		for _, c := range components {
			if c.Kind == kind && projectExtensionWorkflowName(c.Ref.ID) == name {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func projectApplyPointExtensions(component *projectCompileComponent, components []projectCompileComponent, extensions []projectWorkflowExtension) error {
	if len(extensions) == 0 {
		return nil
	}
	var workflow map[string]any
	if err := json.Unmarshal(component.Bytes, &workflow); err != nil {
		return err
	}
	graph, _ := workflow["definition"].(map[string]any)
	stages, _ := graph["stages"].(map[string]any)
	for _, extension := range extensions {
		point := extension.ResolvedPoint
		if err := projectPointValidateAnchor(graph, *point); err != nil {
			return err
		}
		if _, ok := stages[extension.ID]; ok {
			return refusal("project_extension_stage_conflict", extension.ID)
		}
		kind, name := "step", extension.Step
		routes := point.Routes.Step
		refField := "step_ref"
		if extension.Call != "" {
			kind, name = "workflow", extension.Call
			routes = point.Routes.Call
			refField = "workflow_ref"
		}
		var target projectCompileComponent
		for _, c := range components {
			if c.Kind == kind && projectExtensionWorkflowName(c.Ref.ID) == name {
				target = c
				break
			}
		}
		if kind == "workflow" {
			var child flow.WorkflowRevision
			if err := json.Unmarshal(target.Bytes, &child); err != nil {
				return err
			}
			for _, outcome := range child.AllowedOutcomes {
				if routes.On[outcome] == "" {
					return refusal("project_extension_call_outcome_unmapped", extension.Point+": route every declared child outcome: "+outcome)
				}
			}
		}
		bindings := map[string]any{}
		for port, expression := range point.InputBindings {
			if strings.HasPrefix(expression, "$inputs.") {
				bindings[port] = map[string]any{"from": "workflow_input", "port": strings.TrimPrefix(expression, "$inputs.")}
			} else {
				parts := strings.Split(strings.TrimPrefix(expression, projectExtensionStageSource), ".")
				bindings[port] = map[string]any{"from": "stage_output", "stage_id": parts[0], "port": parts[1]}
			}
		}
		on := map[string]any{}
		for outcome, destination := range routes.On {
			on[outcome] = destination
		}
		stageKind := "step"
		if kind == "workflow" {
			stageKind = "call"
		}
		stage := map[string]any{"kind": stageKind, refField: projectRefValue(target.Ref), "input_bindings": bindings, "on": on}
		if routes.OnError != "" {
			stage["on_error"] = routes.OnError
		}
		if len(routes.ImpossibleVerdicts) > 0 {
			stage["impossible_verdicts"] = routes.ImpossibleVerdicts
			current, _ := workflow["schema_version"].(string)
			if current == "" || flow.WorkflowRevisionAtLeast(flow.WorkflowRevisionVerdictVersion, current) {
				workflow["schema_version"] = flow.WorkflowRevisionVerdictVersion
			}
		}
		if point.Entry != "" {
			graph["entry"] = extension.ID
		} else {
			from := stages[point.Between.From].(map[string]any)
			for _, group := range []string{"on", "on_complete"} {
				values, _ := from[group].(map[string]any)
				for verdict, destination := range values {
					if destination == point.Between.To {
						values[verdict] = extension.ID
					}
				}
			}
			for _, field := range []string{"on_limit", "on_error", "on_unknown", "default"} {
				if from[field] == point.Between.To {
					from[field] = extension.ID
				}
			}
		}
		stages[extension.ID] = stage
		for alias, consumer := range point.Consumers {
			if err := projectPointConsumerBinding(stages, consumer); err != nil {
				return err
			}
			field, port := "input_bindings", consumer.Input
			if consumer.Output != "" {
				field, port = "output_bindings", consumer.Output
			}
			stages[consumer.Stage].(map[string]any)[field].(map[string]any)[port] = map[string]any{"from": "stage_output", "stage_id": extension.ID, "port": extension.OutputBindings[alias]}
		}
	}
	data, err := json.Marshal(workflow)
	if err != nil {
		return err
	}
	component.Bytes, err = flow.Canonical(data)
	if err != nil {
		return err
	}
	component.Ref.Digest, err = flow.Digest(component.Bytes)
	return err
}

func projectPointDependencyCycles(pending []projectPendingDocument, extensions map[string][]projectWorkflowExtension, rootExtensions []projectWorkflowExtension) error {
	if len(extensions) == 0 {
		return nil
	}
	aliases := map[string]int{}
	ids := map[string]int{}
	for i, item := range pending {
		object, _ := item.value.(map[string]any)
		id, _ := object["id"].(string)
		ids[id] = i
		aliases[item.document.AliasPrefix+"_"+projectExtensionWorkflowName(id)] = i
	}
	edges := make([][]int, len(pending))
	for i, item := range pending {
		var visit func(any)
		visit = func(value any) {
			switch v := value.(type) {
			case string:
				if match := projectValueReference.FindStringSubmatch(v); match != nil {
					if j, ok := aliases[match[1]]; ok {
						edges[i] = append(edges[i], j)
					}
				}
			case []any:
				for _, child := range v {
					visit(child)
				}
			case map[string]any:
				for _, child := range v {
					visit(child)
				}
			}
		}
		visit(item.value)
	}
	// Legacy root calls and point calls share one dependency graph. Each
	// separate subgraph can be acyclic while their union re-enters the root.
	for i, item := range pending {
		if item.document.Extensions == "" {
			continue
		}
		for _, extension := range rootExtensions {
			name, kind := extension.Step, "step"
			if extension.Call != "" {
				name, kind = extension.Call, "workflow"
			}
			for j, candidate := range pending {
				object, _ := candidate.value.(map[string]any)
				id, _ := object["id"].(string)
				if candidate.document.Kind == kind && projectExtensionWorkflowName(id) == name {
					edges[i] = append(edges[i], j)
				}
			}
		}
	}
	for target, items := range extensions {
		for _, extension := range items {
			name, kind := extension.Step, "step"
			if extension.Call != "" {
				name, kind = extension.Call, "workflow"
			}
			for j, item := range pending {
				object, _ := item.value.(map[string]any)
				id, _ := object["id"].(string)
				if item.document.Kind == kind && projectExtensionWorkflowName(id) == name {
					edges[ids[target]] = append(edges[ids[target]], j)
				}
			}
		}
	}
	state := make([]uint8, len(pending))
	var walk func(int) error
	walk = func(i int) error {
		if state[i] == 1 {
			return &flow.Problem{Code: "call_cycle", Path: "/extension_points", Message: "point and local definition dependencies form a cycle"}
		}
		if state[i] == 2 {
			return nil
		}
		state[i] = 1
		for _, j := range edges[i] {
			if err := walk(j); err != nil {
				return err
			}
		}
		state[i] = 2
		return nil
	}
	for target := range extensions {
		if err := walk(ids[target]); err != nil {
			return err
		}
	}
	return nil
}

func projectPointRootExtensions(extensions []projectWorkflowExtension) []projectWorkflowExtension {
	result := make([]projectWorkflowExtension, 0, len(extensions))
	for _, extension := range extensions {
		if extension.Point == "" {
			result = append(result, extension)
		}
	}
	return result
}

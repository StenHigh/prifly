package main

import (
	"encoding/json"
	"maps"
	"slices"

	"github.com/stenhigh/prifly/internal/flow"
)

func projectArrangeCompilationNode(n *projectCompositionNode) error {
	if !n.unified {
		return projectCompositionArrangeNode(n)
	}
	roots, points := []projectWorkflowExtension{}, []projectWorkflowExtension{}
	for _, e := range n.options.Extensions {
		if e.Point == "" {
			roots = append(roots, e)
		} else {
			points = append(points, e)
		}
	}
	groups, pointGroups := []any{}, []any{}
	for _, raw := range n.groups {
		object, _ := raw.(map[string]any)
		if _, ok := object["point"]; ok {
			pointGroups = append(pointGroups, raw)
		} else {
			groups = append(groups, raw)
		}
	}
	n.options.Extensions = roots
	n.groups = groups
	if err := projectCompositionArrangeNode(n); err != nil {
		return err
	}
	roots = n.options.Extensions
	used := map[string]bool{}
	grouped := []projectWorkflowExtension{}
	bySelector := map[string]projectWorkflowExtension{}
	for _, e := range points {
		bySelector[e.Selector] = e
	}
	for _, raw := range pointGroups {
		object := raw.(map[string]any)
		p, err := n.unifiedSelectPoint(object["point"])
		if err != nil {
			return err
		}
		if len(p.Contract.Consumers) != 0 {
			return refusal("project_composition_point_group_outputs_unsupported", p.Name)
		}
		call := p.Contract.Routes.Call
		if call == nil || call.On["succeeded"] == "" || call.On["rejected"] == "" || call.On["succeeded"] == call.On["rejected"] || call.OnError == "" {
			return refusal("project_composition_point_group_routes_incompatible", p.Name)
		}
		from, to := "entry", call.On["succeeded"]
		if p.Contract.Between != nil {
			from = p.Contract.Between.From
		}
		compatible := map[string]projectWorkflowExtension{}
		for selector, e := range bySelector {
			if e.UnifiedPoint.Identity != p.Identity || e.UnifiedPoint.Scope != p.Scope || e.UnifiedPoint.Target != p.Target {
				continue
			}
			if !projectExtensionEnabled(e) {
				continue
			}
			e.From = from
			e.To = to
			e.InputBindings = maps.Clone(p.Contract.InputBindings)
			routes := p.Contract.Routes.Step
			if e.Call != "" || e.Group != nil {
				routes = call
			}
			if routes == nil || routes.OnError != call.OnError {
				return refusal("project_composition_point_group_routes_incompatible", p.Name)
			}
			for outcome, destination := range routes.On {
				want := call.On["rejected"]
				if e.Step != "" && outcome == "pass" || e.Step == "" && outcome == "succeeded" {
					want = to
				}
				if destination != want && !slices.Contains(routes.ImpossibleVerdicts, outcome) {
					return refusal("project_composition_point_group_routes_incompatible", p.Name)
				}
			}
			e.On = maps.Clone(routes.On)
			e.OnError = routes.OnError
			e.ImpossibleVerdicts = slices.Clone(routes.ImpossibleVerdicts)
			compatible[selector] = e
		}
		group, err := projectCompositionReadGroup(object, compatible, used, from, to, 0)
		if err != nil {
			return err
		}
		id, _ := object["id"].(string)
		e := projectWorkflowExtension{ID: id, Point: p.Name, Composed: true, Workflow: p.Target, UnifiedPoint: &p, ResolvedPoint: &p.Contract, Source: n.rootDocument().Source + "#arrange/" + id, Selector: n.name + "." + id, Group: group, OutputBindings: map[string]string{}}
		grouped = append(grouped, e)
	}
	// Inherited containers remain whole; verify origins across new containers.
	for _, e := range points {
		if !used[e.Selector] {
			grouped = append(grouped, e)
		}
	}
	if err := projectCompositionValidateMembers(grouped, 0, map[string]string{}); err != nil {
		return err
	}
	occupied := map[string]bool{}
	for _, e := range grouped {
		if !projectExtensionEnabled(e) {
			continue
		}
		p := e.UnifiedPoint
		key := p.Scope + "/" + p.Target + "/" + p.Identity
		if occupied[key] {
			return refusal("project_composition_arrange_required", p.Name)
		}
		occupied[key] = true
	}
	n.options.Extensions = append(roots, grouped...)
	return nil
}

func projectApplyCompilationPoints(component *projectCompileComponent, components *[]projectCompileComponent, extensions []projectWorkflowExtension) error {
	if len(extensions) == 0 || extensions[0].UnifiedPoint == nil {
		return projectApplyPointExtensions(component, *components, extensions)
	}
	for _, e := range extensions {
		if e.Group != nil {
			var root flow.WorkflowRevision
			if err := json.Unmarshal(component.Bytes, &root); err != nil {
				return err
			}
			lowerer := projectCompositionLowerer{root: root, components: components, external: map[string]flow.Binding{}, ports: map[string]flow.InputPort{}}
			ref, err := lowerer.lower(e)
			if err != nil {
				return err
			}
			e.Group = nil
			e.Step = ""
			e.Call = projectExtensionWorkflowName(ref.ID)
			// The wrapper receives the lifted native inputs, rather than new implicit sources.
			p := *e.ResolvedPoint
			route := *p.Routes.Call
			route.On = map[string]string{"succeeded": route.On["succeeded"], "rejected": route.On["rejected"]}
			p.Routes.Call = &route
			p.InputBindings = map[string]string{}
			for port, b := range lowerer.external {
				if b.From == "workflow_input" {
					p.InputBindings[port] = "$inputs." + b.Port
				} else {
					p.InputBindings[port] = "$stages." + b.StageID + "." + b.Port
				}
			}
			e.ResolvedPoint = &p
		}
		if err := projectApplyPointExtensions(component, *components, []projectWorkflowExtension{e}); err != nil {
			return projectCompositionSourceError(e.Source, err)
		}
	}
	return nil
}

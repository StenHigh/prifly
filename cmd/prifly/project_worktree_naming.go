package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
	"github.com/stenhigh/prifly/internal/projectwire"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

type projectWorktreeBinding struct {
	NamingInput *string `json:"naming_input,omitempty"`
	Value       *string `json:"value,omitempty"`
	Input       *string `json:"input,omitempty"`
	Pointer     *string `json:"pointer,omitempty"`
}

type projectWorktreeNaming struct {
	Inputs   map[string]projectNamingDeclaration `json:"inputs,omitempty"`
	Template string                              `json:"template"`
	Bindings map[string]projectWorktreeBinding   `json:"bindings"`
}

var projectNameToken = regexp.MustCompile(`^[a-z][a-z0-9_]{0,31}$`)
var projectNameLiteral = regexp.MustCompile(`^[a-z0-9-]*$`)

func projectNamedObservationConflict(err error) bool {
	var fault *prifly.Fault
	var rejection *local.Rejection
	code := ""
	if errors.As(err, &fault) {
		code = fault.Code
	}
	if errors.As(err, &rejection) {
		code = rejection.Code
	}
	return code == "control_conflict" || code == "authority_state_conflict" || code == "claim_ownership_conflict"
}

func projectParseWorktreeNaming(raw any) (*projectWorktreeNaming, error) {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(data, &shape); err != nil {
		return nil, refusal("project_worktree_template_invalid", err.Error())
	}
	if raw, present := shape["inputs"]; present {
		var inputs map[string]json.RawMessage
		if json.Unmarshal(raw, &inputs) != nil || len(inputs) == 0 {
			return nil, refusal("project_worktree_template_invalid", "inputs requires 1..16 declarations")
		}
	}
	var bindings map[string]map[string]json.RawMessage
	if err := json.Unmarshal(shape["bindings"], &bindings); err == nil {
		for _, binding := range bindings {
			count := 0
			for _, key := range []string{"value", "input", "naming_input"} {
				if raw, ok := binding[key]; ok {
					count++
					var value string
					if json.Unmarshal(raw, &value) != nil || string(raw) == "null" {
						return nil, refusal("project_worktree_template_invalid", "binding source must be a string")
					}
				}
			}
			if count != 1 {
				return nil, refusal("project_worktree_template_invalid", "binding requires exactly one source")
			}
			if _, ok := binding["value"]; ok && len(binding) != 1 {
				return nil, refusal("project_worktree_template_invalid", "literal binding admits only value")
			}
			if _, ok := binding["value"]; !ok {
				raw, ok := binding["pointer"]
				var pointer string
				if !ok || string(raw) == "null" || json.Unmarshal(raw, &pointer) != nil {
					return nil, refusal("project_worktree_binding_invalid", "input binding requires a string pointer")
				}
			}
		}
	}
	var result projectWorktreeNaming
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return nil, refusal("project_worktree_template_invalid", err.Error())
	}
	if err := result.validate(); err != nil {
		return nil, err
	}
	return &result, nil
}

func (n projectWorktreeNaming) validate() error {
	bad := func(message string) error { return refusal("project_worktree_template_invalid", message) }
	if len(n.Template) == 0 || len(n.Template) > 256 || len(n.Bindings) > 16 {
		return bad("template requires 1..256 ASCII characters and at most 16 bindings")
	}
	used := map[string]bool{}
	remaining := n.Template
	for {
		literal, after, found := strings.Cut(remaining, "{")
		if !projectNameLiteral.MatchString(literal) {
			return bad("template literals allow only lowercase letters, digits and hyphens")
		}
		if !found {
			break
		}
		name, rest, closed := strings.Cut(after, "}")
		if !closed || !projectNameToken.MatchString(name) {
			return bad("invalid placeholder; expected {name}")
		}
		used[name], remaining = true, rest
	}
	if len(used) != len(n.Bindings) {
		return bad("bindings must exactly match the template placeholders")
	}
	declared := map[string]bool{}
	if len(n.Inputs) > 16 {
		return bad("at most 16 naming inputs are allowed")
	}
	for name, declaration := range n.Inputs {
		if !projectNameToken.MatchString(name) || declaration.Schema == "" {
			return bad("invalid naming input declaration")
		}
	}
	for name, binding := range n.Bindings {
		count := 0
		if binding.Value != nil {
			count++
		}
		if binding.Input != nil {
			count++
		}
		if binding.NamingInput != nil {
			count++
		}
		if count != 1 {
			return bad("binding requires exactly one source")
		}
		if binding.NamingInput != nil {
			if _, ok := n.Inputs[*binding.NamingInput]; !ok || binding.Pointer == nil {
				return bad("binding requires a declared naming_input and pointer")
			}
			declared[*binding.NamingInput] = true
			if !used[name] || !projectNamingPointer(*binding.Pointer) {
				return bad("invalid naming binding")
			}
			continue
		}
		if !used[name] || (binding.Value == nil) == (binding.Input == nil) || binding.Value != nil && binding.Pointer != nil || binding.Input != nil && (binding.Pointer == nil || *binding.Input == "") {
			return bad("binding " + name + " must contain only value, or input and pointer")
		}
		if binding.Pointer != nil && !projectNamingPointer(*binding.Pointer) {
			return refusal("project_worktree_binding_invalid", name+": invalid JSON pointer")
		}
	}
	if len(declared) != len(n.Inputs) {
		return bad("every naming input must be used by a binding")
	}
	return nil
}

func projectNamingPointer(pointer string) bool {
	if pointer != "" && !strings.HasPrefix(pointer, "/") {
		return false
	}
	for i := 0; i < len(pointer); i++ {
		if pointer[i] == '~' {
			i++
			if i == len(pointer) || pointer[i] != '0' && pointer[i] != '1' {
				return false
			}
		}
	}
	return true
}

func projectResolveWorktreeNaming(n *projectWorktreeNaming, engine *prifly.Engine, plan *flow.Plan, values map[string]json.RawMessage, refs map[string]prifly.ArtifactRef) (*projectwire.WorktreeNamingReview, error) {
	return projectResolveNaming(n, engine, plan, values, refs, nil)
}

func projectResolveNaming(n *projectWorktreeNaming, engine *prifly.Engine, plan *flow.Plan, values map[string]json.RawMessage, refs map[string]prifly.ArtifactRef, snapshot *prifly.WorktreeNamingSnapshot) (*projectwire.WorktreeNamingReview, error) {
	if n == nil {
		return nil, nil
	}
	resolved, err := engine.StartInputValues(plan, values, refs)
	if err != nil {
		return nil, err
	}
	result := &projectwire.WorktreeNamingReview{Template: n.Template, Bindings: map[string]json.RawMessage{}, Values: map[string]string{}, Root: prifly.ClaimRoot, BranchPrefix: "prifly/", Policy: prifly.WorktreeNamingPolicy, MaxCandidates: prifly.MaxWorktreeNames}
	result.Snapshot = snapshot
	base := n.Template
	for name, binding := range n.Bindings {
		value := ""
		if binding.Value != nil {
			value = *binding.Value
		} else if binding.NamingInput != nil {
			if snapshot == nil {
				return nil, refusal("project_worktree_input_missing", *binding.NamingInput)
			}
			input, ok := snapshot.Inputs[*binding.NamingInput]
			if !ok {
				return nil, refusal("project_worktree_input_missing", *binding.NamingInput)
			}
			var document any
			if err := json.Unmarshal(input.Value, &document); err != nil {
				return nil, err
			}
			selected, exists := flow.JSONPointer(document, *binding.Pointer)
			if !exists {
				return nil, refusal("project_worktree_value_missing", name+": pointer is absent")
			}
			value, ok = selected.(string)
			if !ok {
				return nil, refusal("project_worktree_value_invalid", name+": expected a string")
			}
		} else {
			port, declared := plan.Workflow.Inputs[*binding.Input]
			where := fmt.Sprintf("%s (input %s pointer %q)", name, *binding.Input, *binding.Pointer)
			if !declared || port.Format != "json" {
				return nil, refusal("project_worktree_binding_invalid", where+": expected a declared JSON input")
			}
			data, exists := resolved[*binding.Input]
			if !exists {
				return nil, refusal("project_worktree_value_missing", where+": input is absent")
			}
			var document any
			if err := json.Unmarshal(data, &document); err != nil {
				return nil, err
			}
			selected, exists := flow.JSONPointer(document, *binding.Pointer)
			if !exists {
				return nil, refusal("project_worktree_value_missing", where+": value is absent")
			}
			var ok bool
			value, ok = selected.(string)
			if !ok {
				return nil, refusal("project_worktree_value_invalid", where+": expected a string")
			}
		}
		if len(value) > 64 || prifly.ValidateWorktreeName(value) != nil {
			return nil, refusal("project_worktree_value_invalid", name+": expected 1..64 lowercase letters, digits and single hyphen separators")
		}
		result.Values[name] = value
		result.Bindings[name], err = json.Marshal(binding)
		if err != nil {
			return nil, err
		}
		base = strings.ReplaceAll(base, "{"+name+"}", value)
	}
	if err := prifly.ValidateWorktreeName(base); err != nil {
		return nil, refusal("project_worktree_value_invalid", "rendered name: "+err.Error())
	}
	result.Base = base
	return result, nil
}

package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
	"github.com/stenhigh/prifly/internal/projectwire"
	prifly "github.com/stenhigh/prifly/internal/runtime"
)

type projectNamingDeclaration struct {
	Schema string `json:"schema"`
}

func (n *projectWorktreeNaming) owned() bool {
	if n == nil {
		return false
	}
	if len(n.Inputs) > 0 {
		return true
	}
	for _, binding := range n.Bindings {
		if binding.NamingInput != nil {
			return true
		}
	}
	return false
}

// A separate flag parser gives typed naming refusals without changing --input.
type projectNamingFiles map[string]string

func (v projectNamingFiles) String() string { return fmt.Sprint(map[string]string(v)) }
func (v projectNamingFiles) Set(arg string) error {
	name, path, ok := strings.Cut(arg, "=")
	if !ok || !projectNameToken.MatchString(name) || path == "" {
		return refusal("project_worktree_input_invalid", "expected NAME=FILE")
	}
	if _, exists := v[name]; exists {
		return refusal("project_worktree_input_duplicate", name)
	}
	v[name] = path
	return nil
}

func projectReadNamingSnapshot(root string, n *projectWorktreeNaming, files projectNamingFiles) (*prifly.WorktreeNamingSnapshot, error) {
	if n == nil || !n.owned() {
		if len(files) > 0 {
			return nil, refusal("project_worktree_input_unknown", "no separate naming inputs are declared")
		}
		return nil, nil
	}
	for name := range files {
		if _, ok := n.Inputs[name]; !ok {
			return nil, refusal("project_worktree_input_unknown", name)
		}
	}
	snapshot := &prifly.WorktreeNamingSnapshot{SchemaVersion: "project-worktree-naming-snapshot/1", Inputs: map[string]prifly.WorktreeNamingInput{}}
	declaration, err := json.Marshal(n)
	if err != nil {
		return nil, err
	}
	snapshot.Declaration, err = flow.Canonical(declaration)
	if err != nil {
		return nil, err
	}
	for name, decl := range n.Inputs {
		file, ok := files[name]
		if !ok {
			return nil, refusal("project_worktree_input_missing", name)
		}
		// Only project-owned schemas are admitted. Package upstream remains sealed.
		path := decl.Schema
		if !fs.ValidPath(path) || path == "." || strings.Contains(path, "\\") || !strings.HasPrefix(path, ".prifly/") {
			return nil, refusal("project_worktree_schema_invalid", name+": schema must be a confined project-owned file")
		}
		if strings.HasPrefix(path, ".prifly/workflows/") {
			parts := strings.Split(path, "/")
			if len(parts) < 5 || parts[3] != "project" {
				return nil, refusal("project_worktree_schema_invalid", name+": upstream schemas cannot be naming declarations")
			}
		}
		schema, err := projectExecutionSource(root, root, path)
		if err != nil {
			return nil, refusal("project_worktree_schema_invalid", name+": "+err.Error())
		}
		schema, err = flow.Canonical(schema)
		if err != nil {
			return nil, refusal("project_worktree_schema_invalid", name+": "+err.Error())
		}
		var document any
		if err := json.Unmarshal(schema, &document); err != nil {
			return nil, err
		}
		if err := projectNamingSchemaRefs(document); err != nil {
			return nil, err
		}
		object, isObject := document.(map[string]any)
		if !isObject || object["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
			return nil, refusal("project_worktree_schema_invalid", name+": expected JSON Schema 2020-12")
		}
		value, err := projectReadNamingValue(root, file)
		if err != nil {
			return nil, err
		}
		schemaDigest, _ := flow.Digest(schema)
		valueDigest, _ := flow.Digest(value)
		ref := flow.Ref{ID: "prifly:naming/" + name, Version: "1.0.0", Digest: schemaDigest}
		if err := flow.ValidateSchema(flow.Registry{ref: schema}, ref, value); err != nil {
			return nil, refusal("project_worktree_input_invalid", name+": "+err.Error())
		}
		snapshot.Inputs[name] = prifly.WorktreeNamingInput{SchemaPath: path, Schema: schema, SchemaDigest: schemaDigest, Value: value, ValueDigest: valueDigest, ValueSource: projectRequestFile(root, file)}
	}
	if err := prifly.ValidateWorktreeNamingSnapshot(snapshot); err != nil {
		return nil, err
	}
	return snapshot, nil
}

func projectReadNamingValue(root, file string) ([]byte, error) {
	path := projectRequestFile(root, file)
	if path == "-" {
		return nil, refusal("project_worktree_input_invalid", "naming input requires a rereadable regular file")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	// Reuse the descriptor-relative, no-symlink reader for every path component.
	volume := filepath.VolumeName(absolute) + string(filepath.Separator)
	data, err := projectExecutionSource(volume, volume, filepath.ToSlash(strings.TrimPrefix(absolute, volume)))
	if err != nil {
		return nil, refusal("project_worktree_input_invalid", err.Error())
	}
	data, err = flow.Canonical(data)
	if err != nil {
		return nil, refusal("project_worktree_input_invalid", err.Error())
	}
	return data, nil
}

func projectNamingSchemaRefs(value any) error {
	switch v := value.(type) {
	case map[string]any:
		for key, child := range v {
			switch key {
			case "const", "enum", "default", "examples":
				continue
			case "$defs", "definitions", "properties", "patternProperties", "dependentSchemas":
				if schemas, ok := child.(map[string]any); ok {
					for _, schema := range schemas {
						if err := projectNamingSchemaRefs(schema); err != nil {
							return err
						}
					}
				}
				continue
			}
			if key == "$ref" || key == "$dynamicRef" || key == "$recursiveRef" {
				ref, ok := child.(string)
				if !ok || !strings.HasPrefix(ref, "#") {
					return refusal("project_worktree_schema_invalid", "only fragment schema references are allowed")
				}
			}
			if key == "$id" {
				return refusal("project_worktree_schema_invalid", "schema resource identifiers are not allowed; use fragment references")
			}
			if err := projectNamingSchemaRefs(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range v {
			if err := projectNamingSchemaRefs(child); err != nil {
				return err
			}
		}
	}
	return nil
}

func projectNamingFromClaim(claim *prifly.WorktreeClaim) (*projectwire.WorktreeNamingReview, error) {
	var n projectWorktreeNaming
	if err := json.Unmarshal(claim.NamingSnapshot.Declaration, &n); err != nil {
		return nil, err
	}
	bindings := map[string]json.RawMessage{}
	for name, binding := range n.Bindings {
		bindings[name], _ = json.Marshal(binding)
	}
	return &projectwire.WorktreeNamingReview{Snapshot: prifly.CloneWorktreeNamingSnapshot(claim.NamingSnapshot), Template: n.Template, Bindings: bindings, Values: map[string]string{}, Base: claim.Naming.Base, Policy: claim.Naming.Policy, Root: prifly.ClaimRoot, BranchPrefix: "prifly/", MaxCandidates: prifly.MaxWorktreeNames}, nil
}

func projectNamingLaunchBudget(summary projectLaunchSummary) error {
	data, err := json.Marshal(summary)
	if err != nil {
		return err
	}
	// The result carries both the reviewed summary and the allocated claim.
	// Queue finalization returns an intent containing summary_bytes and result_bytes.
	size := len(data)
	var snapshot *prifly.WorktreeNamingSnapshot
	if summary.WorktreeNaming != nil {
		snapshot = summary.WorktreeNaming.Snapshot
	}
	if summary.WorktreeClaim != nil {
		snapshot = summary.WorktreeClaim.NamingSnapshot
	}
	if snapshot != nil {
		raw, err := json.Marshal(snapshot)
		if err != nil {
			return err
		}
		size += len(raw)
	}
	if summary.QueueTask != nil {
		size += len(data)
		size = (size + 2) / 3 * 4
	}
	// Keep space for the initial Run view, generated identities and timestamps.
	if size+128*1024 > local.MaxCommandBytes {
		return refusal("project_worktree_snapshot_too_large", "serialized launch carriers exceed authority command budget")
	}
	return nil
}

func projectSummaryHasOwnedNaming(summary projectLaunchSummary) bool {
	return summary.WorktreeNaming != nil && summary.WorktreeNaming.Snapshot != nil || summary.WorktreeClaim != nil && summary.WorktreeClaim.NamingSnapshot != nil
}

package runtime

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

const SnapshotAuthorityClaimsVersion = "authority-claims/5"
const MaxWorktreeNamingSnapshotBytes = 128 * 1024

// WorktreeNamingSnapshot preserves project-owned naming data independently of Run inputs.
type WorktreeNamingSnapshot struct {
	SchemaVersion string                         `json:"schema_version"`
	Declaration   json.RawMessage                `json:"declaration"`
	Inputs        map[string]WorktreeNamingInput `json:"inputs"`
}

type WorktreeNamingInput struct {
	SchemaPath   string          `json:"schema_path"`
	ValueSource  string          `json:"value_source"`
	Schema       json.RawMessage `json:"schema"`
	SchemaDigest string          `json:"schema_digest"`
	Value        json.RawMessage `json:"value"`
	ValueDigest  string          `json:"value_digest"`
}

func ValidateWorktreeNamingSnapshot(snapshot *WorktreeNamingSnapshot) error {
	if snapshot == nil {
		return nil
	}
	data, err := canonical(snapshot)
	if err != nil {
		return err
	}
	if len(data) > MaxWorktreeNamingSnapshotBytes {
		return fault("project_worktree_snapshot_too_large", "complete canonical naming snapshot exceeds 131072 bytes")
	}
	if snapshot.SchemaVersion != "project-worktree-naming-snapshot/1" || len(snapshot.Inputs) == 0 || len(snapshot.Inputs) > 16 || !json.Valid(snapshot.Declaration) {
		return fault("project_worktree_binding_invalid", "invalid naming snapshot")
	}
	if err := validateNamingSnapshotDeclaration(snapshot); err != nil {
		return err
	}
	for _, input := range snapshot.Inputs {
		schemaDigest, err := flow.Digest(input.Schema)
		if err != nil {
			return err
		}
		valueDigest, err := flow.Digest(input.Value)
		if err != nil {
			return err
		}
		if schemaDigest != input.SchemaDigest || valueDigest != input.ValueDigest || input.SchemaPath == "" || input.ValueSource == "" {
			return fault("project_worktree_binding_invalid", "naming snapshot digest or source mismatch")
		}
	}
	return nil
}

func CloneWorktreeNamingSnapshot(snapshot *WorktreeNamingSnapshot) *WorktreeNamingSnapshot {
	if snapshot == nil {
		return nil
	}
	result := *snapshot
	result.Declaration = append(json.RawMessage(nil), snapshot.Declaration...)
	result.Inputs = make(map[string]WorktreeNamingInput, len(snapshot.Inputs))
	for name, input := range snapshot.Inputs {
		input.Schema = append(json.RawMessage(nil), input.Schema...)
		input.Value = append(json.RawMessage(nil), input.Value...)
		result.Inputs[name] = input
	}
	return &result
}

func EqualWorktreeNamingSnapshot(a, b *WorktreeNamingSnapshot) bool {
	x, ex := canonical(a)
	y, ey := canonical(b)
	return ex == nil && ey == nil && bytes.Equal(x, y)
}

// CheckWorktreeNamingCapacity is read-only. The transform repeats this check
// against its current record before committing a reservation.
func (e *Engine) CheckWorktreeNamingCapacity(ctx context.Context, naming WorktreeNamingRequest) error {
	if err := validateWorktreeNamingRequest(naming); err != nil {
		return err
	}
	record, _, err := e.readClaims(ctx)
	if err != nil {
		return err
	}
	return checkNamingRecordCapacity(record, WorktreeClaim{NamingSnapshot: naming.Snapshot})
}

func checkNamingRecordCapacity(record ClaimRecord, claim WorktreeClaim) error {
	record.Claims = append(append([]WorktreeClaim{}, record.Claims...), claim)
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	// Reserve bounded scalar metadata for the allocation and later lease changes.
	if len(data)+4096 > local.MaxSnapshotBytes {
		return fault("project_worktree_snapshot_too_large", "claim record cannot hold this naming snapshot")
	}
	return nil
}

// This bounded structural check stays pure. Schema compilation and value
// validation run before the claim command, outside the authority transform.
func validateNamingSnapshotDeclaration(snapshot *WorktreeNamingSnapshot) error {
	type declarationInput struct {
		Schema string `json:"schema"`
	}
	type binding struct {
		Value       *string `json:"value,omitempty"`
		Input       *string `json:"input,omitempty"`
		NamingInput *string `json:"naming_input,omitempty"`
		Pointer     *string `json:"pointer,omitempty"`
	}
	var declaration struct {
		Template string                      `json:"template"`
		Inputs   map[string]declarationInput `json:"inputs"`
		Bindings map[string]binding          `json:"bindings"`
	}
	decoder := json.NewDecoder(bytes.NewReader(snapshot.Declaration))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&declaration); err != nil {
		return fault("project_worktree_binding_invalid", "invalid closed naming declaration")
	}
	if declaration.Template == "" || len(declaration.Template) > 256 || len(declaration.Bindings) > 16 || len(declaration.Inputs) != len(snapshot.Inputs) {
		return fault("project_worktree_binding_invalid", "invalid naming declaration bounds")
	}
	used := map[string]bool{}
	for _, binding := range declaration.Bindings {
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
		if count != 1 || binding.Value != nil && binding.Pointer != nil || binding.Value == nil && binding.Pointer == nil {
			return fault("project_worktree_binding_invalid", "invalid naming binding sources")
		}
		if binding.NamingInput != nil {
			used[*binding.NamingInput] = true
		}
	}
	for name, input := range snapshot.Inputs {
		declared, ok := declaration.Inputs[name]
		if !ok || !used[name] || declared.Schema != input.SchemaPath {
			return fault("project_worktree_binding_invalid", "snapshot inputs differ from naming declaration")
		}
	}
	return nil
}

package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

const LaunchSchemaMigrationStateVersion = "launch-schema-migration-choice/1"

type LaunchSchemaMigrationScope struct {
	AuthorityID string `json:"authority_id"`
	ProjectID   string `json:"project_id"`
	Repository  string `json:"repository"`
	Launch      string `json:"launch"`
}

type LaunchSchemaMigrationBuild struct {
	BuildKey    string   `json:"build_key"`
	ConsumerRef flow.Ref `json:"consumer_ref"`
	WorkflowRef flow.Ref `json:"workflow_ref"`
}

// LaunchSchemaMigrationChoice selects an existing decision; it grants no new trust.
type LaunchSchemaMigrationChoice struct {
	LaunchSchemaMigrationBuild
	DecisionID   string `json:"decision_id"`
	ReviewDigest string `json:"review_digest"`
}

type LaunchSchemaMigrationState struct {
	SchemaVersion string                       `json:"schema_version"`
	Scope         LaunchSchemaMigrationScope   `json:"scope"`
	Generation    int64                        `json:"generation"`
	Choice        *LaunchSchemaMigrationChoice `json:"choice"`
	Actor         string                       `json:"actor"`
	Reason        string                       `json:"reason"`
	CommandID     string                       `json:"command_id"`
}

type LaunchSchemaMigrationInspection struct {
	SchemaVersion string                       `json:"schema_version"`
	State         LaunchSchemaMigrationState   `json:"state"`
	Valid         bool                         `json:"valid"`
	Reasons       []string                     `json:"reasons"`
	Current       *LaunchSchemaMigrationBuild  `json:"current,omitempty"`
	Candidate     *LaunchSchemaMigrationChoice `json:"candidate,omitempty"`
}

type LaunchSchemaMigrationResult struct {
	SchemaVersion string                     `json:"schema_version"`
	State         LaunchSchemaMigrationState `json:"state"`
	Receipt       local.AuthorityReceipt     `json:"receipt"`
	Duplicate     bool                       `json:"duplicate"`
}

type LaunchSchemaMigrationReview struct {
	Source     string                      `json:"source"`
	Scope      LaunchSchemaMigrationScope  `json:"scope"`
	Generation int64                       `json:"generation,omitempty"`
	Choice     LaunchSchemaMigrationChoice `json:"choice"`
}

type LaunchSchemaMigrationRequest struct {
	Scope              LaunchSchemaMigrationScope   `json:"scope"`
	ExpectedGeneration int64                        `json:"expected_generation"`
	Choice             *LaunchSchemaMigrationChoice `json:"choice"`
	CommandID          string                       `json:"command_id"`
	Reason             string                       `json:"reason"`
}

func launchSchemaMigrationKey(scope LaunchSchemaMigrationScope) string {
	data, _ := json.Marshal(scope)
	sum := sha256.Sum256(data)
	key := strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:]))
	return "launch_choice:h" + key[:26] + ":h" + key[26:]
}

func (e *Engine) validateLaunchSchemaMigrationScope(scope LaunchSchemaMigrationScope) error {
	if scope.AuthorityID != e.Installation.ID || scope.ProjectID != e.Config.ID || !filepath.IsAbs(scope.Repository) || filepath.Clean(scope.Repository) != scope.Repository || !utf8.ValidString(scope.Repository+scope.Launch) || scope.Launch == "" || len(scope.Launch) > 128 || len(scope.Repository) > 4096 || strings.ContainsAny(scope.Repository+scope.Launch, "\x00\r\n") {
		return fault("schema_migration_scope_mismatch", "launch choice must name this authority, project, canonical repository and launch")
	}
	return nil
}

func decodeLaunchSchemaMigration(data []byte, scope LaunchSchemaMigrationScope) (LaunchSchemaMigrationState, error) {
	state := LaunchSchemaMigrationState{SchemaVersion: LaunchSchemaMigrationStateVersion, Scope: scope}
	if len(data) == 0 {
		return state, nil
	}
	if err := validateSchemaMigrationJSON("LaunchSchemaMigrationStateV1", data); err != nil {
		return state, err
	}
	if err := decode(data, &state); err != nil {
		return state, err
	}
	if state.SchemaVersion != LaunchSchemaMigrationStateVersion || state.Scope != scope || state.Generation < 0 {
		return state, migrationInvalid("unsupported or inconsistent launch migration choice; inspect the recorded choice")
	}
	return state, nil
}

func (e *Engine) readLaunchSchemaMigration(ctx context.Context, scope LaunchSchemaMigrationScope) (LaunchSchemaMigrationState, int64, error) {
	snapshot, err := e.Store.ReadAuthority(ctx, launchSchemaMigrationKey(scope))
	if errors.Is(err, local.ErrNotFound) {
		state, decodeErr := decodeLaunchSchemaMigration(nil, scope)
		return state, 0, decodeErr
	}
	if err != nil {
		return LaunchSchemaMigrationState{}, 0, err
	}
	state, err := decodeLaunchSchemaMigration(snapshot.Data, scope)
	return state, snapshot.Version, err
}

// CheckLaunchSchemaMigration verifies the exact consumer and workflow without changing authority state.
func (e *Engine) CheckLaunchSchemaMigration(ctx context.Context, id string, build LaunchSchemaMigrationBuild) (LaunchSchemaMigrationChoice, error) {
	choice := LaunchSchemaMigrationChoice{LaunchSchemaMigrationBuild: build, DecisionID: id}
	if build.BuildKey == "" || flow.ValidateProtocol("Digest", []byte(strconv.Quote(build.BuildKey))) != nil {
		return choice, migrationInvalid("an exact build key is required")
	}
	if _, err := e.readAccess(ctx); err != nil {
		return choice, err
	}
	binding, err := e.schemaMigrationBinding(ctx, id, build.WorkflowRef, nil)
	if err != nil {
		return choice, err
	}
	if binding == nil || binding.ConsumerRef != build.ConsumerRef {
		return choice, fault("schema_migration_scope_mismatch", "decision belongs to another exact consumer; inspect and review the launch choice")
	}
	choice.ReviewDigest = binding.ReviewDigest
	return choice, nil
}

func (e *Engine) InspectLaunchSchemaMigration(ctx context.Context, scope LaunchSchemaMigrationScope) (LaunchSchemaMigrationInspection, error) {
	view := LaunchSchemaMigrationInspection{SchemaVersion: "launch-schema-migration-inspection/1", Reasons: []string{}}
	if _, err := e.readAccess(ctx); err != nil {
		return view, err
	}
	if err := e.validateLaunchSchemaMigrationScope(scope); err != nil {
		return view, err
	}
	state, _, err := e.readLaunchSchemaMigration(ctx, scope)
	if err != nil {
		return view, err
	}
	view.State = state
	if state.Choice == nil {
		return view, nil
	}
	current, err := e.CheckLaunchSchemaMigration(ctx, state.Choice.DecisionID, state.Choice.LaunchSchemaMigrationBuild)
	if err != nil {
		view.Reasons = append(view.Reasons, err.Error())
		return view, nil
	}
	if current != *state.Choice {
		view.Reasons = append(view.Reasons, "recorded choice differs from its migration decision")
		return view, nil
	}
	view.Valid = true
	return view, nil
}

// SetLaunchSchemaMigration records a reviewed exact choice, or clears it when Choice is nil.
func (e *Engine) SetLaunchSchemaMigration(ctx context.Context, request LaunchSchemaMigrationRequest) (LaunchSchemaMigrationResult, error) {
	result := LaunchSchemaMigrationResult{SchemaVersion: "launch-schema-migration-result/1"}
	if e.ReadOnly {
		return result, local.ErrReadOnly
	}
	if _, err := e.readAccess(ctx); err != nil {
		return result, err
	}
	if err := e.validateLaunchSchemaMigrationScope(request.Scope); err != nil {
		return result, err
	}
	if request.CommandID == "" || strings.TrimSpace(request.Reason) == "" || len(request.Reason) > 4096 || !utf8.ValidString(request.Reason) || request.ExpectedGeneration < 0 || request.ExpectedGeneration > 9007199254740991 {
		return result, migrationInvalid("command identity, reason and nonnegative expected generation are required")
	}
	command, err := e.launchSchemaMigrationCommand(request)
	if err != nil {
		return result, err
	}
	receipt, err := e.Store.LookupAuthorityReceipt(ctx, command)
	if err == nil {
		result.Receipt = receipt
		result.Duplicate = true
		if receipt.Rejection == nil {
			err = decode(receipt.Result, &result.State)
		}
		return result, err
	}
	if !errors.Is(err, local.ErrNotFound) {
		return result, err
	}
	// A first clear is a valid owner mutation even when no choice or control
	// object exists. Enroll without restoring any access removed later.
	if _, _, err := e.enrolControl(ctx); err != nil {
		return result, err
	}
	state, _, err := e.readLaunchSchemaMigration(ctx, request.Scope)
	if err != nil {
		return result, err
	}
	command.AuthorityGuards = []local.AuthorityGuard{{Key: AuthorityControlKey, Check: func(snapshot local.AuthoritySnapshot) error {
		control, err := decodeControl(snapshot.Data)
		if err != nil {
			return err
		}
		if !control.allows(e.owner, "project", e.Config.ID, ControlOperationTrust) {
			return local.Reject("object_access_denied", "the principal cannot change launch schema migration choices")
		}
		return e.controlCompatible(control)
	}}}
	if request.Choice != nil {
		_, packagesVersion, err := e.readPackages(ctx)
		if err != nil {
			return result, err
		}
		checked, err := e.CheckLaunchSchemaMigration(ctx, request.Choice.DecisionID, request.Choice.LaunchSchemaMigrationBuild)
		if err != nil {
			return result, err
		}
		if checked != *request.Choice {
			return result, fault("schema_migration_decision_invalid", "choice differs from the checked migration decision")
		}
		command.AuthorityGuards = append(command.AuthorityGuards, local.AuthorityGuard{Key: AuthorityPackagesKey, Check: func(snapshot local.AuthoritySnapshot) error {
			if snapshot.Version != packagesVersion {
				return local.Reject("schema_migration_choice_stale", "package lifecycle changed; inspect and set the choice again")
			}
			return nil
		}})
	}
	stale := state.Generation != request.ExpectedGeneration
	if !stale && (state.Choice != nil || request.Choice != nil) {
		if state.Generation >= 9007199254740991 {
			return result, migrationInvalid("launch choice generation exhausted")
		}
		state.Generation++
		state.Choice = request.Choice
		state.Actor = e.owner
		state.Reason = request.Reason
		state.CommandID = request.CommandID
	}
	prepared, err := canonical(state)
	if err != nil {
		return result, err
	}
	if err := validateSchemaMigrationJSON("LaunchSchemaMigrationStateV1", prepared); err != nil {
		return result, err
	}
	applied, err := e.Store.ApplyAuthority(ctx, command, func(snapshot local.AuthoritySnapshot) (local.AuthorityChange, error) {
		current, decodeErr := decodeLaunchSchemaMigration(snapshot.Data, request.Scope)
		if decodeErr != nil {
			return local.AuthorityChange{}, decodeErr
		}
		if stale || current.Generation != request.ExpectedGeneration {
			return local.AuthorityChange{}, local.Reject("schema_migration_choice_stale", "launch choice changed; inspect its current generation")
		}
		return local.AuthorityChange{Data: prepared, Result: prepared}, nil
	})
	result.Receipt = applied.Receipt
	result.Duplicate = applied.Duplicate
	if err == nil && applied.Receipt.Rejection == nil {
		err = decode(applied.Receipt.Result, &result.State)
	}
	return result, err
}

func (e *Engine) launchSchemaMigrationGuard(expected LaunchSchemaMigrationState) local.AuthorityGuard {
	return local.AuthorityGuard{Key: launchSchemaMigrationKey(expected.Scope), Check: func(snapshot local.AuthoritySnapshot) error {
		actual, err := decodeLaunchSchemaMigration(snapshot.Data, expected.Scope)
		if err != nil {
			return local.Reject("project_start_stale_launch", "saved migration choice cannot be read; inspect the choice")
		}
		if actual.Generation != expected.Generation || actual.Choice == nil || expected.Choice == nil || *actual.Choice != *expected.Choice {
			return local.Reject("project_start_stale_launch", "saved migration choice changed; repeat project questionnaire --prepare")
		}
		return nil
	}}
}

// CLI inputs, including the exact build key, determine mutation identity. The
// checked refs and decision digest are derived facts, not new caller choices.
func (e *Engine) launchSchemaMigrationCommand(request LaunchSchemaMigrationRequest) (local.AuthorityCommand, error) {
	var selection any
	if request.Choice != nil {
		selection = struct {
			DecisionID string `json:"decision_id"`
			BuildKey   string `json:"build_key"`
		}{request.Choice.DecisionID, request.Choice.BuildKey}
	}
	payload, err := canonical(struct {
		Scope      LaunchSchemaMigrationScope `json:"scope"`
		Generation int64                      `json:"expected_generation"`
		Selection  any                        `json:"selection"`
		Reason     string                     `json:"reason"`
	}{request.Scope, request.ExpectedGeneration, selection, request.Reason})
	return local.AuthorityCommand{ID: request.CommandID, Actor: e.owner, Key: launchSchemaMigrationKey(request.Scope), Payload: payload, StaleRejectionCode: "schema_migration_choice_stale"}, err
}

// LaunchSchemaMigrationReplay allows CLI retries before reading mutable sources.
func (e *Engine) LaunchSchemaMigrationReplay(ctx context.Context, request LaunchSchemaMigrationRequest) (LaunchSchemaMigrationResult, bool, error) {
	result := LaunchSchemaMigrationResult{SchemaVersion: "launch-schema-migration-result/1"}
	if _, err := e.readAccess(ctx); err != nil {
		return result, false, err
	}
	if err := e.validateLaunchSchemaMigrationScope(request.Scope); err != nil {
		return result, false, err
	}
	command, err := e.launchSchemaMigrationCommand(request)
	if err != nil {
		return result, false, err
	}
	receipt, err := e.Store.LookupAuthorityReceipt(ctx, command)
	if errors.Is(err, local.ErrNotFound) {
		return result, false, nil
	}
	if err != nil {
		return result, false, err
	}
	result.Receipt = receipt
	result.Duplicate = true
	if receipt.Rejection == nil {
		err = decode(receipt.Result, &result.State)
	}
	return result, true, err
}

func (e *Engine) savedLaunchResultCommand(ctx context.Context, scope LaunchSchemaMigrationScope, id, fingerprint string) (local.AuthorityCommand, error) {
	if _, err := e.readAccess(ctx); err != nil {
		return local.AuthorityCommand{}, err
	}
	if err := e.validateLaunchSchemaMigrationScope(scope); err != nil {
		return local.AuthorityCommand{}, err
	}
	payload, err := canonical(map[string]any{"operation": "project.saved_launch.result", "request_fingerprint": fingerprint})
	return local.AuthorityCommand{ID: derivedID("command", e.owner, "project.saved_launch.result", id), Actor: e.owner, Key: launchSchemaMigrationKey(scope), Payload: payload}, err
}

func (e *Engine) SavedLaunchSchemaMigrationResult(ctx context.Context, scope LaunchSchemaMigrationScope, id, fingerprint string) (json.RawMessage, error) {
	command, err := e.savedLaunchResultCommand(ctx, scope, id, fingerprint)
	if err != nil {
		return nil, err
	}
	receipt, err := e.Store.LookupAuthorityReceipt(ctx, command)
	if errors.Is(err, local.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if receipt.Rejection != nil {
		return nil, receipt.Rejection
	}
	return receipt.Result, nil
}

func (e *Engine) RecordSavedLaunchSchemaMigrationResult(ctx context.Context, scope LaunchSchemaMigrationScope, id, fingerprint string, response json.RawMessage) error {
	command, err := e.savedLaunchResultCommand(ctx, scope, id, fingerprint)
	if err != nil {
		return err
	}
	result, err := e.Store.ApplyAuthority(ctx, command, func(snapshot local.AuthoritySnapshot) (local.AuthorityChange, error) {
		if snapshot.Version == 0 {
			return local.AuthorityChange{}, local.ErrIntegrity
		}
		return local.AuthorityChange{Data: snapshot.Data, Result: response}, nil
	})
	if err != nil {
		return err
	}
	if result.Receipt.Rejection != nil {
		return result.Receipt.Rejection
	}
	return nil
}

// SavedLaunchSchemaMigrationReplay carries reviewed Project metadata only. The
// Run receipt commits it with admission; it cannot authorize another Run.
type SavedLaunchSchemaMigrationReplay struct {
	SchemaVersion string                     `json:"schema_version"`
	Scope         LaunchSchemaMigrationScope `json:"scope"`
	Fingerprint   string                     `json:"fingerprint"`
	Response      json.RawMessage            `json:"response"`
}

func (e *Engine) AcceptedLaunchSchemaMigrationReplay(ctx context.Context, scope LaunchSchemaMigrationScope, id, fingerprint string) (json.RawMessage, string, error) {
	if err := e.validateLaunchSchemaMigrationScope(scope); err != nil {
		return nil, "", err
	}
	receipt, err := e.Receipt(ctx, id)
	if errors.Is(err, local.ErrNotFound) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	if receipt.Rejection != nil {
		return nil, "", receipt.Rejection
	}
	var result struct {
		Replay *SavedLaunchSchemaMigrationReplay `json:"saved_launch_replay"`
	}
	if err := json.Unmarshal(receipt.Result, &result); err != nil {
		return nil, "", err
	}
	if result.Replay == nil {
		return nil, "", nil
	}
	replay := result.Replay
	if replay.Scope != scope || replay.Fingerprint != fingerprint {
		return nil, "", local.ErrCommandConflict
	}
	if receipt.RunID != e.IntendedRunID(id) || replay.SchemaVersion != "saved-launch-replay/1" || !json.Valid(replay.Response) {
		return nil, "", local.ErrIntegrity
	}
	return replay.Response, receipt.RunID, nil
}

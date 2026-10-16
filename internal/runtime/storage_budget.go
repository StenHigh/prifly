package runtime

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// StorageBudgetStatus keeps allocation separate from scan, snapshot and disk limits.
type StorageBudgetStatus struct {
	SchemaVersion   string                        `json:"schema_version"`
	AuthorityID     string                        `json:"authority_id"`
	Epoch           int64                         `json:"epoch"`
	StorageVersion  int                           `json:"storage_version"`
	Cut             int64                         `json:"cut"`
	Measurement     string                        `json:"measurement"`
	Reason          string                        `json:"reason"`
	Usage           *local.StorageUsage           `json:"usage"`
	Control         *local.AuthorityStorageBudget `json:"control"`
	SafeNextActions []string                      `json:"safe_next_actions"`
}

type StorageBudgetReview struct {
	SchemaVersion      string              `json:"schema_version"`
	Status             StorageBudgetStatus `json:"status"`
	TargetBytes        int64               `json:"target_bytes"`
	Reason             string              `json:"reason"`
	StorageFromVersion int                 `json:"storage_from_version"`
	StorageToVersion   int                 `json:"storage_to_version"`
	ReviewDigest       string              `json:"review_digest"`
}

type StorageBudgetRequest struct {
	CommandID            string `json:"command_id"`
	TargetBytes          int64  `json:"target_bytes"`
	Reason               string `json:"reason"`
	ExpectedReviewDigest string `json:"expected_review_digest"`
}

type StorageBudgetResult struct {
	SchemaVersion string                       `json:"schema_version"`
	CommandID     string                       `json:"command_id"`
	Cut           int64                        `json:"cut"`
	Control       local.AuthorityStorageBudget `json:"control"`
}

func (e *Engine) StorageBudgetStatus(ctx context.Context) (view StorageBudgetStatus, err error) {
	view = StorageBudgetStatus{SchemaVersion: "storage-budget-status/1", AuthorityID: e.Installation.ID, Epoch: e.Store.Info().Epoch, StorageVersion: e.Store.Info().StorageVersion, Measurement: "unknown", Reason: "storage_access_unavailable", SafeNextActions: []string{"storage.budget.show", "storage.budget.set"}}
	if _, err = e.readAccess(ctx); err != nil {
		return
	}
	observation, err := e.Store.StorageBudget(ctx)
	if err != nil {
		view.Reason = "storage_observation_unavailable"
		return view, err
	}
	view.StorageVersion = observation.StorageVersion
	view.Cut = observation.Cut
	view.Epoch = observation.Control.Epoch
	view.Measurement = "known"
	view.Reason = ""
	view.Usage = &observation.Usage
	view.Control = &observation.Control
	return view, nil
}

func validateStorageBudgetTarget(current, target int64, reason string) error {
	if target < local.MinStorageBudgetBytes || target > local.MaxStorageBudgetBytes || target <= current || !local.ValidStorageBudgetReason(reason) {
		return fault("storage_budget_invalid_limit", "choose an explicit finite increase within 64 KiB..64 GiB and a nonempty UTF-8 reason of at most 1024 bytes")
	}
	return nil
}

// The digest deliberately excludes usage and cut: traffic does not change owner intent.
func storageBudgetReviewDigest(control local.AuthorityStorageBudget, from int, target int64, reason string) (string, error) {
	data, err := canonical(map[string]any{"semantics": "finite-raise/1", "authority_id": control.AuthorityID, "epoch": control.Epoch, "generation": control.Generation, "current_limit_bytes": control.LimitBytes, "target_bytes": target, "reason": reason, "storage_from_version": from, "storage_to_version": local.StorageVersion})
	if err != nil {
		return "", err
	}
	return flow.Digest(data)
}

func (e *Engine) PrepareStorageBudget(ctx context.Context, target int64, reason string) (review StorageBudgetReview, err error) {
	status, err := e.StorageBudgetStatus(ctx)
	if err != nil {
		return review, err
	}
	if err = validateStorageBudgetTarget(status.Control.LimitBytes, target, reason); err != nil {
		return review, err
	}
	if target <= status.Usage.AllocatedBytes {
		return review, fault("storage_budget_target_exhausted", "the target leaves no headroom above allocated logical pages; inspect storage budget show and explicitly choose a sufficient finite target")
	}
	digest, err := storageBudgetReviewDigest(*status.Control, status.StorageVersion, target, reason)
	if err != nil {
		return review, err
	}
	return StorageBudgetReview{SchemaVersion: "storage-budget-review/1", Status: status, TargetBytes: target, Reason: reason, StorageFromVersion: status.StorageVersion, StorageToVersion: local.StorageVersion, ReviewDigest: digest}, nil
}

func (e *Engine) storageBudgetCommand(ctx context.Context, request StorageBudgetRequest) (local.AuthorityCommand, error) {
	if _, err := e.readAccess(ctx); err != nil {
		return local.AuthorityCommand{}, err
	}
	id, err := json.Marshal(request.CommandID)
	if err != nil {
		return local.AuthorityCommand{}, err
	}
	if flow.ValidateProtocol("Identifier", id) != nil || request.ExpectedReviewDigest == "" {
		return local.AuthorityCommand{}, fault("storage_budget_review_conflict", "set requires an explicit command identity and expected review digest from storage budget set --prepare")
	}
	payload, err := canonical(map[string]any{"operation": "storage.budget.set", "request": request})
	if err != nil {
		return local.AuthorityCommand{}, err
	}
	return local.AuthorityCommand{ID: request.CommandID, Actor: e.owner, Key: local.StorageBudgetKey, Payload: payload}, nil
}

func validateStorageBudgetReview(observation local.StorageBudgetObservation, request StorageBudgetRequest) error {
	c := observation.Control
	// A writer may already have performed exactly the reviewed bootstrap upgrade.
	// Only generation zero accepts that alternate plan; a configuration change never does.
	versions := []int{observation.StorageVersion}
	if c.Generation == 0 && observation.StorageVersion == local.StorageVersion && c.BootstrapStorageVersion < local.StorageVersion {
		versions = append(versions, c.BootstrapStorageVersion)
	}
	matched := false
	for _, from := range versions {
		digest, err := storageBudgetReviewDigest(c, from, request.TargetBytes, request.Reason)
		if err != nil {
			return err
		}
		matched = matched || digest == request.ExpectedReviewDigest
	}
	if !matched {
		return fault("storage_budget_review_conflict", "authority, epoch, configuration or protected request differs from the review; prepare again")
	}
	return validateStorageBudgetTarget(c.LimitBytes, request.TargetBytes, request.Reason)
}

func storageBudgetReceiptResult(receipt local.AuthorityReceipt) (result StorageBudgetResult, err error) {
	if receipt.Rejection != nil {
		return result, receipt.Rejection
	}
	var control local.AuthorityStorageBudget
	if err = decode(receipt.Result, &control); err != nil {
		return
	}
	if control.CommandID != receipt.ID || control.Generation != receipt.Version {
		return result, local.ErrIntegrity
	}
	return StorageBudgetResult{SchemaVersion: "storage-budget-result/1", CommandID: receipt.ID, Cut: receipt.Cut, Control: control}, nil
}

// CheckStorageBudgetSet is read-only, including on legacy storage. It reconciles
// exact retries before considering current configuration or any writer upgrade.
func (e *Engine) CheckStorageBudgetSet(ctx context.Context, request StorageBudgetRequest) (*StorageBudgetResult, error) {
	command, err := e.storageBudgetCommand(ctx, request)
	if err != nil {
		return nil, err
	}
	receipt, err := e.Store.LookupAuthorityReceipt(ctx, command)
	if err == nil {
		result, err := storageBudgetReceiptResult(receipt)
		return &result, err
	}
	if !errors.Is(err, local.ErrNotFound) {
		return nil, err
	}
	observation, err := e.Store.StorageBudget(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateStorageBudgetReview(observation, request); err != nil {
		return nil, err
	}
	control, version, err := e.Control(ctx)
	if err != nil {
		return nil, err
	}
	if version > 0 && !control.allows(e.owner, "project", e.Config.ID, ControlOperationResolve) {
		return nil, fault("object_access_denied", "the session principal cannot change authority storage budget")
	}
	return nil, nil
}

func (e *Engine) storageBudgetAccessGuard() local.AuthorityGuard {
	return local.AuthorityGuard{Key: AuthorityControlKey, Check: func(snapshot local.AuthoritySnapshot) error {
		if snapshot.Version == 0 {
			return nil
		}
		control, err := decodeControl(snapshot.Data)
		if err != nil {
			return err
		}
		if err := e.controlCompatible(control); err != nil {
			return err
		}
		if !control.allows(e.owner, "project", e.Config.ID, ControlOperationResolve) {
			return local.Reject("object_access_denied", "the session principal cannot change authority storage budget")
		}
		return nil
	}}
}

func (e *Engine) SetStorageBudget(ctx context.Context, request StorageBudgetRequest) (StorageBudgetResult, error) {
	command, err := e.storageBudgetCommand(ctx, request)
	if err != nil {
		return StorageBudgetResult{}, err
	}
	command.AuthorityGuards = []local.AuthorityGuard{e.storageBudgetAccessGuard()}
	result, err := e.Store.ApplyAuthority(ctx, command, func(snapshot local.AuthoritySnapshot) (local.AuthorityChange, error) {
		if snapshot.Budget == nil {
			return local.AuthorityChange{}, local.ErrIntegrity
		}
		if err := validateStorageBudgetReview(*snapshot.Budget, request); err != nil {
			var refusal *Fault
			if errors.As(err, &refusal) {
				return local.AuthorityChange{}, local.Reject(refusal.Code, refusal.Message)
			}
			return local.AuthorityChange{}, err
		}
		control := snapshot.Budget.Control
		control.Generation++
		control.PreviousLimitBytes = control.LimitBytes
		control.LimitBytes = request.TargetBytes
		control.Source = "owner"
		control.Actor = e.owner
		control.Reason = request.Reason
		control.CommandID = request.CommandID
		control.ObservedAllocatedBytes = snapshot.Budget.Usage.AllocatedBytes
		data, err := canonical(control)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		return local.AuthorityChange{Data: data, Result: data, SetStorageBudget: &request.TargetBytes}, nil
	})
	if err != nil {
		return StorageBudgetResult{}, err
	}
	return storageBudgetReceiptResult(result.Receipt)
}

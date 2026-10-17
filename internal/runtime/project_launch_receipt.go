package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"io"

	"github.com/stenhigh/prifly/internal/local"
)

// LockNamedProjectLaunch serializes only retries of one external command.
// Independent named launches and Run drivers retain their own locks.
func (e *Engine) LockNamedProjectLaunch(commandID string) (io.Closer, error) {
	return e.claimEffectLock(derivedID("claim", e.Installation.ID, commandID, "project-result"))
}

func (e *Engine) namedProjectResultCommand(commandID, fingerprint string) (local.AuthorityCommand, error) {
	payload, err := canonical(map[string]any{"operation": "project.named_launch_result", "command_id": commandID, "request_digest": fingerprint})
	return local.AuthorityCommand{ID: derivedID("command", commandID, "named-launch-result"), Actor: e.owner, Key: AuthorityClaimsKey, Payload: payload}, err
}

// NamedProjectLaunchResult returns a completed command's original response
// before prepare observes changed availability or a continuation's new owner.
func (e *Engine) NamedProjectLaunchResult(ctx context.Context, commandID, fingerprint string) (json.RawMessage, error) {
	command, err := e.namedProjectResultCommand(commandID, fingerprint)
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

// The receipt records the result without adding fields to a saved Run or a
// second ownership ledger. Claims retain their exact current bytes.
func (e *Engine) RecordNamedProjectLaunchResult(ctx context.Context, commandID, fingerprint string, response json.RawMessage) error {
	if e.ReadOnly {
		return local.ErrReadOnly
	}
	command, err := e.namedProjectResultCommand(commandID, fingerprint)
	if err != nil {
		return err
	}
	result, err := e.Store.ApplyAuthority(ctx, command, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
		if s.Version == 0 {
			return local.AuthorityChange{}, local.ErrIntegrity
		}
		return local.AuthorityChange{Data: s.Data, Result: response}, nil
	})
	if err != nil {
		return err
	}
	if result.Receipt.Rejection != nil {
		return result.Receipt.Rejection
	}
	return nil
}

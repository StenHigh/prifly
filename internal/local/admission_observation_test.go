//go:build cgo

package local

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestStaleAdmissionObservationLeavesNoReceiptOrRun(t *testing.T) {
	t.Parallel()
	s, _ := testStore(t)
	ctx := context.Background()
	_, err := s.ApplyAuthority(ctx, AuthorityCommand{ID: "command:seed-observation", Actor: "owner", Key: "claims", Payload: json.RawMessage(`{}`)}, func(AuthoritySnapshot) (AuthorityChange, error) {
		return AuthorityChange{Data: json.RawMessage(`{}`)}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	command := storeCommand("command:stale-observation", "run:observed", 0)
	command.Control = &ControlPin{Key: "claims", Version: 0}
	command.RetryAdmissionObservation = true
	_, err = s.Apply(ctx, command, func(Snapshot) (Change, error) { t.Fatal("stale reducer ran"); return Change{}, nil })
	if !errors.Is(err, ErrAdmissionSnapshotStale) {
		t.Fatalf("stale: %v", err)
	}
	if _, err := s.LookupReceipt(ctx, command.Actor, command.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stale observation wrote receipt: %v", err)
	}
	if _, err := s.Read(ctx, command.RunID, 0, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stale observation wrote Run: %v", err)
	}
	command.Control.Version = 1
	result := applyChange(t, s, command, storeChange(`{"value":1}`))
	if result.Receipt.Rejection != nil {
		t.Fatal(result.Receipt.Rejection)
	}
	duplicate, err := s.Apply(ctx, command, func(Snapshot) (Change, error) { t.Fatal("duplicate reducer ran"); return Change{}, nil })
	if err != nil || !duplicate.Duplicate {
		t.Fatalf("duplicate: %+v %v", duplicate, err)
	}
}

//go:build cgo

package local

import (
	"context"
	"encoding/json"
	"testing"
)

func TestAuthorityPreconditionRefusalIsNamedBeforeSavingAndReplays(t *testing.T) {
	t.Parallel()
	for _, guard := range []string{"version", "run", "cut"} {
		t.Run(guard, func(t *testing.T) {
			t.Parallel()
			s, _ := testStore(t)
			ctx := context.Background()
			seedQueueTransaction(t, s)
			applyChange(t, s, storeCommand("command:run", "run:guarded", 0), storeChange(`{"v":1}`))
			zero := int64(0)
			command := AuthorityCommand{ID: "command:migration", Actor: "owner", Key: "task_queue", Payload: json.RawMessage(`{"reviewed":true}`), StaleRejectionCode: "schema_migration_plan_stale"}
			switch guard {
			case "version":
				command.ExpectedVersion = &zero
			case "run":
				command.RunPins = []RunPin{{ID: "run:guarded", Version: 0}}
			case "cut":
				command.ExpectedCut = &zero
			}
			result, err := s.ApplyAuthority(ctx, command, func(AuthoritySnapshot) (AuthorityChange, error) {
				t.Error("stale proof entered transform")
				return AuthorityChange{}, nil
			})
			if err != nil || result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "schema_migration_plan_stale" {
				t.Fatalf("saved refusal: %v %+v", err, result.Receipt)
			}
			receipt, err := s.LookupAuthorityReceipt(ctx, command)
			if err != nil || receipt.Rejection.Code != "schema_migration_plan_stale" {
				t.Fatal("receipt lookup lost named refusal", err)
			}
			command.StaleRejectionCode = ""
			duplicate, err := s.ApplyAuthority(ctx, command, func(AuthoritySnapshot) (AuthorityChange, error) {
				t.Error("replay entered transform")
				return AuthorityChange{}, nil
			})
			if err != nil || !duplicate.Duplicate || duplicate.Receipt.Rejection.Code != "schema_migration_plan_stale" {
				t.Fatal("internal proof changed command identity", err)
			}
		})
	}
}

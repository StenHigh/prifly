//go:build cgo

package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stenhigh/prifly/internal/purity"
)

func seedQueueTransaction(t *testing.T, s *Store) {
	t.Helper()
	for _, key := range []string{"control", "claims", "task_queue"} {
		result, err := s.ApplyAuthority(context.Background(), AuthorityCommand{ID: "command:seed-" + key, Actor: "owner", Key: key, Payload: json.RawMessage(`{"seed":true}`)}, func(AuthoritySnapshot) (AuthorityChange, error) {
			return AuthorityChange{Data: json.RawMessage(`{"value":"before"}`)}, nil
		})
		if err != nil || result.Receipt.Rejection != nil {
			t.Fatalf("seed %s: %+v %v", key, result, err)
		}
	}
}

func guardedQueueCommand() Command {
	command := storeCommand("command:coupled-queue", "run:coupled-queue", 0)
	command.Control = &ControlPin{Key: "control", Version: 1}
	command.ControlMutation = func(AuthoritySnapshot) (json.RawMessage, error) { return json.RawMessage(`{"value":"after"}`), nil }
	for _, key := range []string{"claims", "task_queue"} {
		command.AuthorityGuards = append(command.AuthorityGuards, AuthorityGuard{Key: key, Check: func(snapshot AuthoritySnapshot) error {
			if !purity.Inside() {
				panic("authority guard escaped the purity region")
			}
			if snapshot.Version != 1 {
				return Reject("version_conflict", "unexpected authority version")
			}
			return nil
		}})
	}
	return command
}

func TestQueueAuthorityGuardsFenceAdmissionAndNeverReplay(t *testing.T) {
	t.Parallel()
	s, _ := testStore(t)
	ctx := context.Background()
	seedQueueTransaction(t, s)
	command := guardedQueueCommand()
	first, err := s.Apply(ctx, command, func(Snapshot) (Change, error) { return storeChange(`{"value":1}`), nil })
	if err != nil || first.Receipt.Rejection != nil {
		t.Fatalf("coupled admission: %+v %v", first, err)
	}
	for _, key := range []string{"control", "claims", "task_queue"} {
		state, err := s.ReadAuthority(ctx, key)
		if err != nil {
			t.Fatal(err)
		}
		if key == "control" {
			if state.Version != 2 || state.Cut != first.Receipt.Cut || string(state.Data) != `{"value":"after"}` {
				t.Fatalf("primary control differs from Run: %+v", state)
			}
		} else if state.Version != 1 || string(state.Data) != `{"value":"before"}` {
			t.Fatalf("guard mutated %s: %+v", key, state)
		}
	}
	command.ControlMutation = func(AuthoritySnapshot) (json.RawMessage, error) {
		t.Fatal("replay invoked primary authority reducer")
		return nil, nil
	}
	for i := range command.AuthorityGuards {
		command.AuthorityGuards[i].Check = func(AuthoritySnapshot) error {
			t.Fatal("replay invoked authority guard")
			return nil
		}
	}
	duplicate, err := s.Apply(ctx, command, func(Snapshot) (Change, error) { t.Fatal("replay invoked Run reducer"); return Change{}, nil })
	if err != nil || !duplicate.Duplicate {
		t.Fatalf("coupled replay: %+v %v", duplicate, err)
	}
	if err := s.Verify(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestQueueAuthorityGuardFailureRollsBackEveryParticipant(t *testing.T) {
	t.Parallel()
	for _, receiptFailure := range []bool{false, true} {
		t.Run(fmt.Sprint(receiptFailure), func(t *testing.T) {
			t.Parallel()
			s, _ := testStore(t)
			ctx := context.Background()
			seedQueueTransaction(t, s)
			command := guardedQueueCommand()
			if receiptFailure {
				if _, err := s.db.Exec(`CREATE TRIGGER refuse_queue_receipt BEFORE INSERT ON commands BEGIN SELECT RAISE(ABORT,'fixture receipt failure'); END`); err != nil {
					t.Fatal(err)
				}
			} else {
				command.AuthorityGuards[1].Check = func(AuthoritySnapshot) error {
					return Reject("queue_task_claim_conflict", "task ownership changed")
				}
			}
			result, err := s.Apply(ctx, command, func(Snapshot) (Change, error) { return storeChange(`{"value":1}`), nil })
			if receiptFailure && err == nil || !receiptFailure && (err != nil || result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "queue_task_claim_conflict") {
				t.Fatalf("failure was acknowledged: %+v %v", result, err)
			}
			if _, err := s.Read(ctx, command.RunID, 0, 1); !errors.Is(err, ErrNotFound) {
				t.Fatalf("failed admission left a Run: %v", err)
			}
			for _, key := range []string{"control", "claims", "task_queue"} {
				state, err := s.ReadAuthority(ctx, key)
				if err != nil || state.Version != 1 || string(state.Data) != `{"value":"before"}` {
					t.Fatalf("failure changed %s: %+v %v", key, state, err)
				}
			}
		})
	}
}

func TestQueueReceiptOnlySkipsAuthorityGuards(t *testing.T) {
	t.Parallel()
	s, _ := testStore(t)
	ctx := context.Background()
	seedQueueTransaction(t, s)
	initial := applyChange(t, s, storeCommand("command:initial-queue-run", "run:receipt-only", 0), storeChange(`{"value":1}`))
	command := Command{ID: "command:queue-receipt-only", Actor: "owner", RunID: "run:receipt-only", Mode: CommandPublication, Payload: json.RawMessage(`{"operation":"publication"}`), AuthorityGuards: []AuthorityGuard{{Key: "task_queue", Check: func(AuthoritySnapshot) error {
		t.Fatal("receipt-only publication invoked queue guard")
		return nil
	}}}}
	result, err := s.Apply(ctx, command, func(Snapshot) (Change, error) {
		return Change{ReceiptOnly: true, Result: json.RawMessage(`{"duplicate":true}`)}, nil
	})
	if err != nil || result.Receipt.Rejection != nil || result.Receipt.Version != initial.Receipt.Version {
		t.Fatalf("receipt-only result: %+v %v", result, err)
	}
	queue, err := s.ReadAuthority(ctx, "task_queue")
	if err != nil || queue.Version != 1 {
		t.Fatalf("receipt-only publication mutated queue: %+v %v", queue, err)
	}
}

func TestQueueAuthorityRunPinsFenceReleaseAndPreserveReceiptIdentity(t *testing.T) {
	t.Parallel()
	s, _ := testStore(t)
	ctx := context.Background()
	seedQueueTransaction(t, s)
	for _, id := range []string{"run:first", "run:second"} {
		result := applyChange(t, s, storeCommand("command:seed-"+id, id, 0), storeChange(`{"value":1}`))
		if result.Receipt.Rejection != nil {
			t.Fatal(result.Receipt.Rejection)
		}
	}
	command := AuthorityCommand{ID: "command:release-proof", Actor: "owner", Key: "task_queue", Payload: json.RawMessage(`{"operation":"release","owner":"host:first"}`), RunPins: []RunPin{{ID: "run:first", Version: 1}, {ID: "run:second", Version: 1}}}
	applyChange(t, s, storeCommand("command:concurrent-reopen", "run:second", 1), storeChange(`{"value":2}`))
	rejected, err := s.ApplyAuthority(ctx, command, func(AuthoritySnapshot) (AuthorityChange, error) {
		t.Fatal("stale Run proof reached queue reducer")
		return AuthorityChange{}, nil
	})
	if err != nil || rejected.Receipt.Rejection == nil || rejected.Receipt.Rejection.Code != "runs_changed" {
		t.Fatalf("stale release proof accepted: %+v %v", rejected, err)
	}
	command.RunPins[1].Version = 2
	duplicate, err := s.ApplyAuthority(ctx, command, func(AuthoritySnapshot) (AuthorityChange, error) {
		t.Fatal("new internal proof replayed rejected request")
		return AuthorityChange{}, nil
	})
	if err != nil || !duplicate.Duplicate || duplicate.Receipt.Rejection == nil {
		t.Fatalf("internal guards changed receipt identity: %+v %v", duplicate, err)
	}
	command.ID = "command:release-fresh-proof"
	accepted, err := s.ApplyAuthority(ctx, command, func(AuthoritySnapshot) (AuthorityChange, error) {
		return AuthorityChange{Data: json.RawMessage(`{"owner":null}`), Result: json.RawMessage(`{"released":true}`)}, nil
	})
	if err != nil || accepted.Receipt.Rejection != nil {
		t.Fatalf("current Run proof refused: %+v %v", accepted, err)
	}
	receipt, err := s.LookupAuthorityReceipt(ctx, command)
	if err != nil || string(receipt.Result) != string(accepted.Receipt.Result) {
		t.Fatalf("authority receipt lookup: %+v %v", receipt, err)
	}
	command.Payload = json.RawMessage(`{"operation":"release","owner":"host:changed"}`)
	if _, err := s.LookupAuthorityReceipt(ctx, command); !errors.Is(err, ErrCommandConflict) {
		t.Fatalf("changed owner reused authority receipt: %v", err)
	}
	if err := s.Verify(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestQueueReservationFencesRejectedRunCommandAndReplaysItsRefusal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _ := testStore(t)
	seedQueueTransaction(t, s)
	original := storeCommand("command:already-refused", "run:already-refused", 0)
	refused, err := s.Apply(ctx, original, func(Snapshot) (Change, error) { return Change{}, Reject("not_admitted", "fixture original refusal") })
	if err != nil || refused.Receipt.Rejection == nil {
		t.Fatalf("original refusal: %+v %v", refused, err)
	}
	command := AuthorityCommand{ID: "command:reserve-used-id", Actor: "owner", Key: "task_queue", Payload: json.RawMessage(`{"reserve":"command:already-refused"}`), AbsentCommandID: original.ID, RunPins: []RunPin{{ID: original.RunID, Version: 0}}}
	conflict, err := s.ApplyAuthority(ctx, command, func(AuthoritySnapshot) (AuthorityChange, error) {
		t.Fatal("used Run command reached reservation reducer")
		return AuthorityChange{}, nil
	})
	if err != nil || conflict.Receipt.Rejection == nil || conflict.Receipt.Rejection.Code != "command_conflict" {
		t.Fatalf("used identity reserved: %+v %v", conflict, err)
	}
	command.AbsentCommandID = "command:another-proof"
	duplicate, err := s.ApplyAuthority(ctx, command, func(AuthoritySnapshot) (AuthorityChange, error) {
		t.Fatal("retry re-evaluated reservation proof")
		return AuthorityChange{}, nil
	})
	if err != nil || !duplicate.Duplicate || duplicate.Receipt.Rejection == nil || duplicate.Receipt.Rejection.Code != "command_conflict" {
		t.Fatalf("internal namespace proof changed retry identity: %+v %v", duplicate, err)
	}
	queue, err := s.ReadAuthority(ctx, "task_queue")
	if err != nil || queue.Version != 1 {
		t.Fatalf("reservation refusal mutated queue: %+v %v", queue, err)
	}
}

func TestQueueGuardFencesLinkedRunAtomicallyWithoutConsumingItsIdentity(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _ := testStore(t)
	seedQueueTransaction(t, s)
	source := applyChange(t, s, storeCommand("command:linked-source", "run:linked-source", 0), storeChange(`{"value":1}`))
	command := LinkedRunCommand{ID: "command:linked-queue", Actor: "owner", SourceRunID: source.Receipt.RunID, NewRunID: "run:linked-queue", ExpectedVersion: 0, Payload: json.RawMessage(`{"operation":"linked"}`), AuthorityGuards: []AuthorityGuard{{Key: "task_queue", Check: func(snapshot AuthoritySnapshot) error {
		if !purity.Inside() {
			t.Fatal("linked guard escaped purity region")
		}
		if snapshot.Version == 1 {
			return ErrCommandConflict
		}
		return nil
	}}}}
	blocked, err := s.CreateLinkedRun(ctx, command, func(Snapshot) (Change, error) {
		t.Fatal("reserved linked identity reached reducer")
		return Change{}, nil
	})
	if !errors.Is(err, ErrCommandConflict) {
		t.Fatalf("linked guard: %+v %v", blocked, err)
	}
	if _, err := s.LookupReceipt(ctx, command.Actor, command.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("guard consumed Run command identity: %v", err)
	}
	if _, err := s.Read(ctx, command.NewRunID, 0, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("guard left linked Run: %v", err)
	}
	changed, err := s.ApplyAuthority(ctx, AuthorityCommand{ID: "command:release-linked-proof", Actor: "owner", Key: "task_queue", Payload: json.RawMessage(`{"fixture":"next cut"}`)}, func(AuthoritySnapshot) (AuthorityChange, error) {
		return AuthorityChange{Data: json.RawMessage(`{"value":"next"}`)}, nil
	})
	if err != nil || changed.Receipt.Rejection != nil {
		t.Fatal(err)
	}
	command.ExpectedVersion = source.Receipt.Version
	accepted, err := s.CreateLinkedRun(ctx, command, func(Snapshot) (Change, error) { return storeChange(`{"value":2}`), nil })
	if err != nil || accepted.Receipt.Rejection != nil {
		t.Fatalf("new proof did not allow original linked request: %+v %v", accepted, err)
	}
	command.AuthorityGuards[0].Check = func(AuthoritySnapshot) error { t.Fatal("linked replay invoked guard"); return nil }
	replay, err := s.CreateLinkedRun(ctx, command, func(Snapshot) (Change, error) { t.Fatal("linked replay invoked reducer"); return Change{}, nil })
	if err != nil || !replay.Duplicate {
		t.Fatalf("linked replay: %+v %v", replay, err)
	}
	unchanged, err := s.Read(ctx, command.SourceRunID, 0, 1)
	if err != nil || unchanged.Snapshot.Version != source.Receipt.Version {
		t.Fatal("linked guard changed source")
	}
	if err := s.Verify(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestQueueGuardChecksReservationBeforeRecordingStaleCASRefusal(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	s, _ := testStore(t)
	seedQueueTransaction(t, s)
	command := storeCommand("command:stale-reserved-create", "run:stale-reserved-create", 1)
	command.AuthorityGuards = []AuthorityGuard{{Key: "task_queue", Check: func(AuthoritySnapshot) error { return ErrCommandConflict }}}
	result, err := s.Apply(ctx, command, func(Snapshot) (Change, error) { t.Fatal("stale creation reached reducer"); return Change{}, nil })
	if !errors.Is(err, ErrCommandConflict) {
		t.Fatalf("stale precondition spent a reserved original identity: %+v %v", result, err)
	}
	if _, err := s.LookupReceipt(ctx, command.Actor, command.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong request recorded a receipt: %v", err)
	}
	if _, err := s.Read(ctx, command.RunID, 0, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stale creation left Run: %v", err)
	}
}

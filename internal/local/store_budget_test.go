//go:build cgo

package local

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mattn/go-sqlite3"
)

func applyFixtureStorageBudget(s *Store, id string, generation, target int64) (AuthorityApplyResult, error) {
	payload, _ := json.Marshal(map[string]any{"generation": generation, "target": target})
	return s.ApplyAuthority(context.Background(), AuthorityCommand{ID: id, Actor: "owner", Key: StorageBudgetKey, Payload: payload}, func(snapshot AuthoritySnapshot) (AuthorityChange, error) {
		if snapshot.Budget.Control.Generation != generation {
			return AuthorityChange{}, Reject("storage_budget_review_conflict", "fixture generation changed")
		}
		c := snapshot.Budget.Control
		c.Generation++
		c.PreviousLimitBytes = c.LimitBytes
		c.LimitBytes = target
		c.Source = "owner"
		c.Actor = "owner"
		c.CommandID = id
		c.Reason = "preserve evidence"
		c.ObservedAllocatedBytes = snapshot.Budget.Usage.AllocatedBytes
		data, _ := json.Marshal(c)
		return AuthorityChange{Data: data, Result: data, SetStorageBudget: &target}, nil
	})
}

func TestStorageBudgetBootstrapOpenAndConcurrentCAS(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "authority")
	opts := storeTestOptions
	opts.SoftLimitBytes = 256 << 10
	first, err := OpenStore(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := OpenStore(dir, storeTestOptions)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	initial, err := first.StorageBudget(ctx)
	if err != nil || initial.Control.Generation != 0 || initial.Control.Source != "bootstrap" || initial.Control.LimitBytes != opts.SoftLimitBytes {
		t.Fatal(initial, err)
	}
	var group sync.WaitGroup
	results := make(chan AuthorityApplyResult, 2)
	failures := make(chan error, 2)
	for index, store := range []*Store{first, second} {
		group.Add(1)
		go func(index int, store *Store) {
			defer group.Done()
			result, err := applyFixtureStorageBudget(store, fmt.Sprintf("raise-%d", index), 0, int64(index+2)<<20)
			results <- result
			failures <- err
		}(index, store)
	}
	group.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	winners, conflicts := 0, 0
	for r := range results {
		if r.Receipt.Rejection == nil {
			winners++
		} else if r.Receipt.Rejection.Code == "storage_budget_review_conflict" {
			conflicts++
		} else {
			t.Fatal(r)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatal(winners, conflicts)
	}
	a, err := first.StorageUsage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b, err := second.StorageUsage(ctx)
	if err != nil || a.SoftLimitBytes != b.SoftLimitBytes || a.SoftLimitBytes == opts.SoftLimitBytes {
		t.Fatal(a, b, err)
	}
	if _, err := OpenStore(dir, opts); err == nil {
		t.Fatal("bootstrap option silently overrode durable setting")
	}
	reopened, err := OpenStore(dir, storeTestOptions)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.Verify(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestStorageBudgetInvalidLimitsAndRecordedRetry(t *testing.T) {
	t.Parallel()
	s, _ := testStore(t)
	ctx := context.Background()
	for i, target := range []int64{0, -1, 64 << 10, DefaultSoftLimitBytes, MaxStorageBudgetBytes + 1} {
		result, err := applyFixtureStorageBudget(s, fmt.Sprintf("invalid-%d", i), 0, target)
		if err != nil || result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "storage_budget_invalid_limit" {
			t.Fatal(target, result, err)
		}
	}
	first, err := applyFixtureStorageBudget(s, "accepted", 0, 1<<30)
	if err != nil || first.Receipt.Rejection != nil {
		t.Fatal(first, err)
	}
	later, err := applyFixtureStorageBudget(s, "later", 1, 2<<30)
	if err != nil || later.Receipt.Rejection != nil {
		t.Fatal(later, err)
	}
	retry, err := applyFixtureStorageBudget(s, "accepted", 0, 1<<30)
	if err != nil || !retry.Duplicate || string(retry.Receipt.Result) != string(first.Receipt.Result) || retry.Receipt.Cut != first.Receipt.Cut {
		t.Fatal(retry, err)
	}
	control, err := s.StorageBudget(ctx)
	if err != nil || control.Control.Generation != 2 || control.Control.LimitBytes != 2<<30 {
		t.Fatal(control, err)
	}
}

func TestStorageBudgetCommitFailureRollsBackControlAndScalar(t *testing.T) {
	t.Parallel()
	s, _ := testStore(t)
	ctx := context.Background()
	// Pin a single connection so the registered SQLite commit hook applies to
	// the real command transaction, including its receipt and numeric setting.
	s.db.SetMaxOpenConns(1)
	s.db.SetMaxIdleConns(1)
	conn, err := s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Raw(func(raw any) error { raw.(*sqlite3.SQLiteConn).RegisterCommitHook(func() int { return 1 }); return nil }); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	if result, err := applyFixtureStorageBudget(s, "failed-commit", 0, 1<<30); err == nil {
		t.Fatal("failed commit reported success", result)
	}
	conn, err = s.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Raw(func(raw any) error { raw.(*sqlite3.SQLiteConn).RegisterCommitHook(nil); return nil }); err != nil {
		t.Fatal(err)
	}
	conn.Close()
	current, err := s.StorageBudget(ctx)
	if err != nil || current.Control.Generation != 0 || current.Control.LimitBytes != DefaultSoftLimitBytes {
		t.Fatal(current, err)
	}
	if _, err := s.ReadAuthority(ctx, StorageBudgetKey); !errors.Is(err, ErrNotFound) {
		t.Fatal("failed control left a state", err)
	}
	if err := s.Verify(ctx); err != nil {
		t.Fatal(err)
	}
}

// Optional independent executable fixture. Run with PRIFLY_STORAGE_OLD_WRITER
// built from the pre-storage-10 source; the ordinary suite needs no Git build.
func TestStorageBudgetLegacyMigrationFencesOldExecutable(t *testing.T) {
	t.Parallel()
	binary := os.Getenv("PRIFLY_STORAGE_OLD_WRITER")
	if binary == "" {
		t.Skip("set PRIFLY_STORAGE_OLD_WRITER to a storage-9 writer fixture")
	}
	dir := filepath.Join(t.TempDir(), "authority")
	command := exec.Command(binary, dir)
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = command.Process.Kill() })
	scanner := bufio.NewScanner(stdout)
	if !scanner.Scan() || scanner.Text() != "ready" {
		t.Fatal("old writer did not open", scanner.Err(), stderr.String())
	}
	opts := storeTestOptions
	opts.ReadOnly = true
	reader, err := OpenStore(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	ctx := context.Background()
	before, err := reader.Read(ctx, "run:stable", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := reader.StorageBudget(ctx)
	if err != nil || observation.StorageVersion != 9 || observation.Control.Source != "legacy_default" || observation.Control.LimitBytes != DefaultSoftLimitBytes {
		t.Fatal(observation, err)
	}
	// Force a real failed migration commit. DDL and bootstrap metadata roll back
	// together, before trying the successful supported writer open below.
	db, err := sql.Open("sqlite3", filepath.Join(dir, "state.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.Raw(func(raw any) error { raw.(*sqlite3.SQLiteConn).RegisterCommitHook(func() int { return 1 }); return nil }); err != nil {
		t.Fatal(err)
	}
	migrator := Store{softLimitBytes: DefaultSoftLimitBytes, bootstrapVersion: 9}
	if err := migrator.migrateStorageBudget(ctx, conn); err == nil {
		t.Fatal("migration commit failure reported success")
	}
	if err := conn.Raw(func(raw any) error { raw.(*sqlite3.SQLiteConn).RegisterCommitHook(nil); return nil }); err != nil {
		t.Fatal(err)
	}
	var version int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil || version != 9 {
		t.Fatal(version, err)
	}
	conn.Close()
	writer, err := OpenStore(dir, storeTestOptions)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	after, err := writer.Read(ctx, "run:stable", 0, 10)
	if err != nil || !bytes.Equal(before.Snapshot.Data, after.Snapshot.Data) || before.Cut != after.Cut || before.Snapshot.Version != after.Snapshot.Version || before.Snapshot.EventSeq != after.Snapshot.EventSeq {
		t.Fatal("migration rewrote Run history", err)
	}
	observation, err = writer.StorageBudget(ctx)
	if err != nil || observation.StorageVersion != 10 || observation.Control.Generation != 0 || observation.Control.LimitBytes != DefaultSoftLimitBytes || observation.Control.BootstrapStorageVersion != 9 {
		t.Fatal(observation, err)
	}
	if _, err := io.WriteString(stdin, "write\n"); err != nil {
		t.Fatal(err)
	}
	stdin.Close()
	if !scanner.Scan() || scanner.Text() != "fenced" {
		t.Fatal("old writer was not fenced", scanner.Err(), stderr.String())
	}
	if err := command.Wait(); err != nil {
		t.Fatal(err, stderr.String())
	}
	output, err := exec.Command(binary, dir, "open").CombinedOutput()
	if err != nil || strings.TrimSpace(string(output)) != "incompatible" {
		t.Fatal("old opener accepted storage 10", string(output), err)
	}
	if err := writer.Verify(ctx); err != nil {
		t.Fatal(err)
	}
	t.Log("storage 9 read-only observation; failed migration rollback; storage 10 preserved prior Run; already-open and newly opened old executable fenced")
}

func TestStorageBudgetPostWriteHeadroomAndDiskFull(t *testing.T) {
	t.Parallel()
	for _, diskFull := range []bool{false, true} {
		t.Run(fmt.Sprint(diskFull), func(t *testing.T) {
			t.Parallel()
			opts := storeTestOptions
			opts.SoftLimitBytes = 64 << 10
			s, err := OpenStore(filepath.Join(t.TempDir(), "authority"), opts)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			ctx := context.Background()
			if _, err := s.db.Exec(`CREATE TABLE fixture_padding(bytes BLOB); CREATE TRIGGER fixture_budget_allocation AFTER INSERT ON authority_commands WHEN NEW.state_key='storage_budget' AND instr(CAST(NEW.receipt AS TEXT),'"rejection"')=0 BEGIN INSERT INTO fixture_padding VALUES(zeroblob(65536)); END;`); err != nil {
				t.Fatal(err)
			}
			before, err := s.StorageBudget(ctx)
			if err != nil {
				t.Fatal(err)
			}
			target := before.Usage.AllocatedBytes + 1
			if diskFull {
				target = 1 << 20
				if _, err := s.db.Exec(fmt.Sprintf("PRAGMA max_page_count=%d", before.Usage.PageCount)); err != nil {
					t.Fatal(err)
				}
			}
			result, err := applyFixtureStorageBudget(s, "post-write", 0, target)
			if diskFull {
				if err == nil {
					t.Fatal("full physical SQLite returned success", result)
				}
			} else if err != nil || result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "storage_budget_target_exhausted" {
				t.Fatal(result, err)
			}
			after, err := s.StorageBudget(ctx)
			if err != nil || after.Control != before.Control {
				t.Fatal("failed update committed setting", after, err)
			}
			var rows int
			if err := s.db.QueryRow("SELECT count(*) FROM fixture_padding").Scan(&rows); err != nil || rows != 0 {
				t.Fatal("failed update left candidate pages", rows, err)
			}
			if err := s.Verify(ctx); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStorageBudgetAllConsumersUseCommittedRaise(t *testing.T) {
	t.Parallel()
	opts := storeTestOptions
	opts.SoftLimitBytes = 192 << 10
	first, err := OpenStore(filepath.Join(t.TempDir(), "authority"), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	var dir string
	if err := first.db.QueryRow("SELECT state_directory FROM authority").Scan(&dir); err != nil {
		t.Fatal(err)
	}
	stale, err := OpenStore(dir, storeTestOptions)
	if err != nil {
		t.Fatal(err)
	}
	defer stale.Close()
	ctx := context.Background()
	seedQueueTransaction(t, first)
	initial := applyChange(t, first, storeCommand("start", "run:a", 0), storeChange(`{"value":1}`))
	if initial.Receipt.Rejection != nil {
		t.Fatal(initial)
	}
	padding, _ := json.Marshal(map[string]string{"padding": strings.Repeat("p", 256<<10)})
	write := storeChange(string(padding))
	write.RequireStorageBudget = true
	refused := applyChange(t, stale, storeCommand("before-raise", "run:a", 1), write)
	if refused.Receipt.Rejection == nil || refused.Receipt.Rejection.Code != "storage_budget_exhausted" {
		t.Fatal(refused)
	}
	raised, err := applyFixtureStorageBudget(first, "raise", 0, 4<<20)
	if err != nil || raised.Receipt.Rejection != nil {
		t.Fatal(raised, err)
	}
	ordinary := applyChange(t, stale, storeCommand("after-raise", "run:a", 1), write)
	if ordinary.Receipt.Rejection != nil {
		t.Fatal("ordinary writer kept bootstrap quota", ordinary)
	}
	logical := storeCommand("receipt", "run:a", 0)
	logical.Mode = CommandPublication
	logical.ExpectedVersion = nil
	receipt := applyChange(t, stale, logical, Change{ReceiptOnly: true, RequireStorageBudget: true, Result: padding})
	if receipt.Receipt.Rejection != nil {
		t.Fatal("receipt-only kept bootstrap quota", receipt)
	}
	coupled := guardedQueueCommand()
	coupledChange := storeChange(string(padding))
	coupledChange.RequireStorageBudget = true
	result := applyChange(t, stale, coupled, coupledChange)
	if result.Receipt.Rejection != nil {
		t.Fatal("coupled Run/control kept bootstrap quota", result)
	}
	linked, err := stale.CreateLinkedRun(ctx, LinkedRunCommand{ID: "linked", Actor: "owner", SourceRunID: "run:a", NewRunID: "run:linked", ExpectedVersion: 2, Payload: json.RawMessage(`{}`)}, func(Snapshot) (Change, error) { return write, nil })
	if err != nil || linked.Receipt.Rejection != nil {
		t.Fatal("linked creation kept bootstrap quota", linked, err)
	}
	samples := []SampleInput{{ID: "sample:after-raise", Data: json.RawMessage(`{"observed":true}`)}}
	if _, err := stale.AppendSamples(ctx, samples); err != nil {
		t.Fatal("telemetry kept bootstrap quota", err)
	}
	command := storeCommand("command-samples", "run:a", 2)
	command.Samples = func(SampleTimings) []SampleInput {
		return []SampleInput{{ID: "sample:command", Data: json.RawMessage(`{"observed":true}`)}}
	}
	result = applyChange(t, stale, command, storeChange(`{"value":3}`))
	if result.Receipt.Rejection != nil || !result.SamplesRecorded {
		t.Fatal(result)
	}
	// The owner-selected ceiling remains finite on the same open writer.
	huge, _ := json.Marshal(map[string]string{"padding": strings.Repeat("z", 4<<20)})
	overflowChange := storeChange(string(huge))
	overflowChange.RequireStorageBudget = true
	overflow := applyChange(t, stale, storeCommand("overflow-after-raise", "run:a", 3), overflowChange)
	if overflow.Receipt.Rejection == nil || overflow.Receipt.Rejection.Code != "storage_budget_exhausted" || overflow.Receipt.Version != 3 {
		t.Fatal("raise disabled the next overflow guard", overflow)
	}
	overflowChange.RequireStorageBudget = false
	settlement := applyChange(t, stale, storeCommand("settlement-after-raise", "run:a", 3), overflowChange)
	if settlement.Receipt.Rejection != nil {
		t.Fatal("new ceiling blocked mandatory settlement", settlement)
	}
	if _, err := stale.AppendSamples(ctx, []SampleInput{{ID: "sample:reexhausted", Data: json.RawMessage(`{"observed":true}`)}}); !errors.Is(err, ErrSampleLimit) {
		t.Fatal("telemetry crossed the raised-but-exhausted ceiling", err)
	}
	if err := stale.Verify(ctx); err != nil {
		t.Fatal(err)
	}
}

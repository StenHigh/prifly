//go:build cgo

package local

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mattn/go-sqlite3"
)

var storeTestOptions = StoreOptions{EventTypes: []string{"test.updated", "step.publication"}, BusyTimeout: 500 * time.Millisecond}

func testStore(t *testing.T) (*Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "authority")
	s, err := OpenStore(dir, storeTestOptions)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, dir
}

func storeCommand(id, runID string, version int64) Command {
	return Command{ID: id, Actor: "owner", RunID: runID, Payload: json.RawMessage(`{"value":1}`), ExpectedVersion: &version, Mode: CommandCAS}
}

func storeChange(value string) Change {
	return Change{Data: json.RawMessage(value), Events: []EventInput{{Type: "test.updated", Data: json.RawMessage(`{"observed":true}`)}}, Result: json.RawMessage(`{"accepted":true}`)}
}

func applyChange(t *testing.T, s *Store, cmd Command, change Change) ApplyResult {
	t.Helper()
	r, err := s.Apply(context.Background(), cmd, func(Snapshot) (Change, error) { return change, nil })
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAuthorityControlStateIsAtomicAndDeduplicated(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	command := AuthorityCommand{ID: "authority:grant", Actor: "owner", Key: "authority:controls", Payload: json.RawMessage(`{"operation":"grant.issue"}`)}
	calls := 0
	apply := func(AuthoritySnapshot) (AuthorityChange, error) {
		calls++
		return AuthorityChange{Data: json.RawMessage(`{"schema_version":"1","grants":["grant:one"]}`), Result: json.RawMessage(`{"accepted":true}`)}, nil
	}
	first, err := s.ApplyAuthority(ctx, command, apply)
	if err != nil || first.Duplicate || first.Receipt.Rejection != nil || first.Receipt.Version != 1 || calls != 1 {
		t.Fatalf("first authority command: %+v %v calls=%d", first, err, calls)
	}
	duplicate, err := s.ApplyAuthority(ctx, command, apply)
	if err != nil || !duplicate.Duplicate || !reflect.DeepEqual(duplicate.Receipt, first.Receipt) || calls != 1 {
		t.Fatalf("duplicate authority command reapplied: %+v %v calls=%d", duplicate, err, calls)
	}
	view, err := s.ReadAuthority(ctx, command.Key)
	if err != nil || view.Version != 1 || string(view.Data) != `{"schema_version":"1","grants":["grant:one"]}` {
		t.Fatalf("authority state lost: %+v %v", view, err)
	}
	zero := int64(0)
	rejected, err := s.ApplyAuthority(ctx, AuthorityCommand{ID: "authority:stale", Actor: "owner", Key: command.Key, Payload: json.RawMessage(`{"operation":"grant.revoke"}`), ExpectedVersion: &zero}, func(AuthoritySnapshot) (AuthorityChange, error) {
		t.Fatal("stale authority reducer ran")
		return AuthorityChange{}, nil
	})
	if err != nil || rejected.Receipt.Rejection == nil || rejected.Receipt.Rejection.Code != "version_conflict" {
		t.Fatalf("stale authority command was accepted: %+v %v", rejected, err)
	}
	if _, err := s.ApplyAuthority(ctx, AuthorityCommand{ID: command.ID, Actor: command.Actor, Key: command.Key, Payload: json.RawMessage(`{"operation":"other"}`)}, apply); !errors.Is(err, ErrCommandConflict) {
		t.Fatalf("different duplicate request was accepted: %v", err)
	}
	if err := s.Verify(ctx); err != nil {
		t.Fatalf("authority control state does not verify: %v", err)
	}
}

func TestRunCommandRejectsChangedAdditionalAuthorityPin(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	if _, err := s.ApplyAuthority(ctx, AuthorityCommand{ID: "authority:packages-one", Actor: "owner", Key: "packages", Payload: json.RawMessage(`{"operation":"trust"}`)}, func(AuthoritySnapshot) (AuthorityChange, error) {
		return AuthorityChange{Data: json.RawMessage(`{"trusted":true}`)}, nil
	}); err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	cmd := storeCommand("command:pinned", "run:pinned", zero)
	cmd.Pins = []ControlPin{{Key: "packages", Version: 1}}
	accepted := applyChange(t, s, cmd, storeChange(`{"value":1}`))
	if accepted.Receipt.Rejection != nil {
		t.Fatalf("current authority pin rejected: %+v", accepted)
	}
	if _, err := s.ApplyAuthority(ctx, AuthorityCommand{ID: "authority:packages-two", Actor: "owner", Key: "packages", Payload: json.RawMessage(`{"operation":"revoke"}`)}, func(AuthoritySnapshot) (AuthorityChange, error) {
		return AuthorityChange{Data: json.RawMessage(`{"trusted":false}`)}, nil
	}); err != nil {
		t.Fatal(err)
	}
	duplicate, err := s.Apply(ctx, cmd, func(Snapshot) (Change, error) {
		t.Fatal("duplicate command re-entered its reducer")
		return Change{}, nil
	})
	if err != nil || !duplicate.Duplicate || !reflect.DeepEqual(duplicate.Receipt, accepted.Receipt) {
		t.Fatalf("authority change changed an existing receipt: %+v %v", duplicate, err)
	}
	stale := storeCommand("command:stale-pinned", "run:pinned", accepted.Receipt.Version)
	stale.Pins = []ControlPin{{Key: "packages", Version: 1}}
	rejected, err := s.Apply(ctx, stale, func(Snapshot) (Change, error) {
		t.Fatal("stale additional authority pin entered reducer")
		return Change{}, nil
	})
	if err != nil || rejected.Receipt.Rejection == nil || rejected.Receipt.Rejection.Code != "authority_state_conflict" {
		t.Fatalf("stale additional authority pin was accepted: %+v %v", rejected, err)
	}
}

func TestRunCommandCanAtomicallyMutatePinnedAuthorityControl(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	if _, err := s.ApplyAuthority(ctx, AuthorityCommand{ID: "authority:seed", Actor: "owner", Key: "control", Payload: json.RawMessage(`{"operation":"seed"}`)}, func(AuthoritySnapshot) (AuthorityChange, error) {
		return AuthorityChange{Data: json.RawMessage(`{"approvals":["approved"]}`)}, nil
	}); err != nil {
		t.Fatal(err)
	}
	zero := int64(0)
	cmd := storeCommand("run:admit", "run-a", zero)
	cmd.Control = &ControlPin{Key: "control", Version: 1}
	calls := 0
	cmd.ControlMutation = func(state AuthoritySnapshot) (json.RawMessage, error) {
		calls++
		if state.Version != 1 || string(state.Data) != `{"approvals":["approved"]}` {
			t.Fatalf("wrong pinned authority snapshot: %+v", state)
		}
		return json.RawMessage(`{"approvals":["consumed"]}`), nil
	}
	first := applyChange(t, s, cmd, storeChange(`{"value":1}`))
	if first.Receipt.Rejection != nil || calls != 1 {
		t.Fatalf("coupled command rejected or skipped reducer: %+v calls=%d", first, calls)
	}
	control, err := s.ReadAuthority(ctx, "control")
	if err != nil || control.Version != 2 || control.Cut != first.Receipt.Cut || string(control.Data) != `{"approvals":["consumed"]}` {
		t.Fatalf("run and authority were not committed together: %+v %+v %v", first, control, err)
	}
	duplicate, err := s.Apply(ctx, cmd, func(Snapshot) (Change, error) { t.Fatal("duplicate ran Run reducer"); return Change{}, nil })
	if err != nil || !duplicate.Duplicate || calls != 1 {
		t.Fatalf("duplicate repeated control consumption: %+v %v calls=%d", duplicate, err, calls)
	}
	stale := storeCommand("run:stale", "run-a", first.Receipt.Version)
	stale.Control = &ControlPin{Key: "control", Version: 1}
	stale.ControlMutation = func(AuthoritySnapshot) (json.RawMessage, error) {
		t.Fatal("stale control reducer ran")
		return nil, nil
	}
	rejected, err := s.Apply(ctx, stale, func(Snapshot) (Change, error) { t.Fatal("stale Run reducer ran"); return Change{}, nil })
	if err != nil || rejected.Receipt.Rejection == nil || rejected.Receipt.Rejection.Code != "control_conflict" {
		t.Fatalf("stale control pin was admitted: %+v %v", rejected, err)
	}
	if err := s.Verify(ctx); err != nil {
		t.Fatal(err)
	}
	t.Run("receipt failure rolls both records back", func(t *testing.T) {
		s, _ := testStore(t)
		if _, err := s.ApplyAuthority(ctx, AuthorityCommand{ID: "authority:rollback-seed", Actor: "owner", Key: "control", Payload: json.RawMessage(`{"operation":"seed"}`)}, func(AuthoritySnapshot) (AuthorityChange, error) {
			return AuthorityChange{Data: json.RawMessage(`{"approvals":["approved"]}`)}, nil
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`CREATE TRIGGER refuse_coupled_receipt BEFORE INSERT ON commands BEGIN SELECT RAISE(ABORT,'injected receipt persistence failure'); END`); err != nil {
			t.Fatal(err)
		}
		cmd := storeCommand("run:rollback", "run-a", 0)
		cmd.Control = &ControlPin{Key: "control", Version: 1}
		cmd.ControlMutation = func(AuthoritySnapshot) (json.RawMessage, error) {
			return json.RawMessage(`{"approvals":["consumed"]}`), nil
		}
		if _, err := s.Apply(ctx, cmd, func(Snapshot) (Change, error) { return storeChange(`{"value":1}`), nil }); err == nil {
			t.Fatal("failed coupled receipt acknowledged")
		}
		control, err := s.ReadAuthority(ctx, "control")
		if err != nil || control.Version != 1 || string(control.Data) != `{"approvals":["approved"]}` {
			t.Fatalf("failed coupled command changed authority: %+v %v", control, err)
		}
	})
}

// verify chooses its checks by storage version. Reopening after an authority
// command proves that version is known before verification, instead of the
// database being verified under v1 rules and rejecting its own shared cut.
func TestStoreReopensAfterAnAuthorityCommand(t *testing.T) {
	s, dir := testStore(t)
	ctx := context.Background()
	if _, err := s.ApplyAuthority(ctx, AuthorityCommand{ID: "command:control", Actor: "owner", Key: "control", Payload: json.RawMessage(`{"operation":"test"}`)}, func(AuthoritySnapshot) (AuthorityChange, error) {
		return AuthorityChange{Data: json.RawMessage(`{"stops":[]}`)}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(dir, storeTestOptions)
	if err != nil {
		t.Fatalf("a database holding an authority command could not be reopened: %v", err)
	}
	defer reopened.Close()
	if reopened.Info().StorageVersion != StorageVersion {
		t.Fatalf("unexpected storage version: %d", reopened.Info().StorageVersion)
	}
	state, err := reopened.ReadAuthority(ctx, "control")
	if err != nil || string(state.Data) != `{"stops":[]}` {
		t.Fatalf("authority state did not survive the reopen: %v %s", err, state.Data)
	}
}

func TestStoreMigratesV1ForAuthorityControls(t *testing.T) {
	s, dir := testStore(t)
	applyChange(t, s, storeCommand("create", "run-a", 0), storeChange(`{"value":1}`))
	// Rewinding the marker is not enough: a genuine v1 database also lacks the
	// structures later versions added, and leaving them makes the fixture test
	// a migration that never happens in the field.
	if _, err := s.db.Exec("DROP TABLE pinned_bytes; DROP TABLE authority_commands; DROP TABLE authority_states; DROP TABLE slots; DROP TABLE slot_waiters; ALTER TABLE runs DROP COLUMN snapshot_packed; ALTER TABLE events DROP COLUMN state_packed; ALTER TABLE authority DROP COLUMN slot_capacity; ALTER TABLE authority DROP COLUMN admission_seq; ALTER TABLE authority DROP COLUMN verified_cut; PRAGMA user_version=1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := OpenStore(dir, storeTestOptions)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	if upgraded.Info().StorageVersion != StorageVersion {
		t.Fatalf("store was not migrated: %+v", upgraded.Info())
	}
	result, err := upgraded.ApplyAuthority(context.Background(), AuthorityCommand{ID: "authority:migrated", Actor: "owner", Key: "authority:controls", Payload: json.RawMessage(`{"operation":"stop.release"}`)}, func(AuthoritySnapshot) (AuthorityChange, error) {
		return AuthorityChange{Data: json.RawMessage(`{"schema_version":"1"}`)}, nil
	})
	if err != nil || result.Receipt.Rejection != nil || result.Receipt.Version != 1 {
		t.Fatalf("migrated authority controls unavailable: %+v %v", result, err)
	}
}

// Qualification reads every live pool connection rather than trusting the DSN
// or the one connection used by Open. The rejected writes hit the real schema.
func TestStoreQualifiedConnectionsAndForeignKeys(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	first := applyChange(t, s, storeCommand("create", "run-a", 0), storeChange(`{"value":1}`))
	connections := make([]*sql.Conn, 0, 8)
	for i := 0; i < cap(connections); i++ {
		conn, err := s.db.Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = conn.Close() })
		connections = append(connections, conn)
	}
	foreignKeyFailure := func(err error) {
		t.Helper()
		var failure sqlite3.Error
		if !errors.As(err, &failure) || failure.ExtendedCode != sqlite3.ErrConstraintForeignKey {
			t.Fatalf("expected SQLite foreign key rejection, got %v", err)
		}
	}
	for i, conn := range connections {
		var version, journal string
		var synchronous, foreignKeys int
		for _, check := range []struct {
			query string
			value any
		}{
			{"SELECT sqlite_version()", &version},
			{"PRAGMA journal_mode", &journal},
			{"PRAGMA synchronous", &synchronous},
			{"PRAGMA foreign_keys", &foreignKeys},
		} {
			if err := conn.QueryRowContext(ctx, check.query).Scan(check.value); err != nil {
				t.Fatal(err)
			}
		}
		if version != s.Info().SQLiteVersion || !patchedSQLite(version) || journal != "wal" || synchronous != 2 || foreignKeys != 1 {
			t.Fatalf("unqualified connection %d: version=%s journal=%s synchronous=%d fk=%d", i, version, journal, synchronous, foreignKeys)
		}
		_, err := conn.ExecContext(ctx, `INSERT INTO events(run_id,seq,run_version,cut,type,schema_version,actor,command_id,data,digest) VALUES('missing-run',1,1,?,'test.updated',1,'owner','create','{}','invalid')`, first.Receipt.Cut)
		foreignKeyFailure(err)
	}
	conn := connections[0]
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	// The second FK is deferred: an event may precede its command receipt in
	// one transition, but COMMIT must reject a permanently missing receipt.
	if _, err := conn.ExecContext(ctx, `INSERT INTO events(run_id,seq,run_version,cut,type,schema_version,actor,command_id,data,digest) VALUES('run-a',2,2,?,'test.updated',1,'owner','missing-command','{}','invalid')`, first.Receipt.Cut+1000); err != nil {
		_, _ = conn.ExecContext(ctx, "ROLLBACK")
		t.Fatalf("deferred FK was not deferred: %v", err)
	}
	_, commitErr := conn.ExecContext(ctx, "COMMIT")
	_, rollbackErr := conn.ExecContext(ctx, "ROLLBACK")
	foreignKeyFailure(commitErr)
	if rollbackErr != nil {
		t.Fatal(rollbackErr)
	}
	_, duplicateErr := conn.ExecContext(ctx, `INSERT INTO commands SELECT * FROM commands WHERE actor='owner' AND command_id='create'`)
	var duplicate sqlite3.Error
	if !errors.As(duplicateErr, &duplicate) || duplicate.Code != sqlite3.ErrConstraint {
		t.Fatalf("duplicate receipt identity accepted: %v", duplicateErr)
	}
	var events, receipts int
	if err := conn.QueryRowContext(ctx, "SELECT count(*) FROM events").Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRowContext(ctx, "SELECT count(*) FROM commands").Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if events != 1 || receipts != 1 {
		t.Fatalf("failed writes left partial rows: events=%d receipts=%d", events, receipts)
	}
	for _, conn := range connections {
		if err := conn.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Verify(ctx); err != nil {
		t.Fatal(err)
	}
	for version, want := range map[string]bool{"3.44.5": false, "3.44.6": true, "3.50.6": false, "3.50.7": true, "3.51.2": false, "3.51.3": true, "3.53.4": true, "unknown": false} {
		if patchedSQLite(version) != want {
			t.Fatalf("patched SQLite guard classified %s incorrectly", version)
		}
	}
	t.Logf("qualified 8 simultaneous connections: SQLite %s WAL/FULL/FK; immediate and deferred FK, duplicate receipt, rollback", s.Info().SQLiteVersion)
}

func TestStoreReadOnlyQueriesHaveNoMaintenanceWithActiveWriter(t *testing.T) {
	writer, dir := testStore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	first := applyChange(t, writer, storeCommand("first", "run-a", 0), storeChange(`{"value":1}`))
	cut, err := writer.AppendSamples(ctx, []SampleInput{{ID: "sample:first", RunID: "run-a", Data: json.RawMessage(`{"value":7}`)}})
	if err != nil {
		t.Fatal(err)
	}
	opts := storeTestOptions
	opts.ReadOnly = true
	reader, err := OpenStore(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	// Open used one connection. Keep that pool at one so every subsequent
	// read is audited without adding any production introspection API.
	reader.db.SetMaxOpenConns(1)
	reader.db.SetMaxIdleConns(1)
	conn, err := reader.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var forbidden []string
	reads := 0
	err = conn.Raw(func(raw any) error {
		raw.(*sqlite3.SQLiteConn).RegisterAuthorizer(func(action int, a, b, _ string) int {
			mu.Lock()
			defer mu.Unlock()
			switch action {
			case sqlite3.SQLITE_READ:
				reads++
				return sqlite3.SQLITE_OK
			case sqlite3.SQLITE_SELECT, sqlite3.SQLITE_FUNCTION, sqlite3.SQLITE_TRANSACTION:
				return sqlite3.SQLITE_OK
			case sqlite3.SQLITE_PRAGMA:
				if b == "" && (a == "page_count" || a == "page_size" || a == "freelist_count") {
					return sqlite3.SQLITE_OK
				}
			}
			forbidden = append(forbidden, fmt.Sprintf("action=%d %s %s", action, a, b))
			return sqlite3.SQLITE_DENY
		})
		return nil
	})
	if closeErr := conn.Close(); err != nil || closeErr != nil {
		t.Fatalf("install authorizer: %v %v", err, closeErr)
	}
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var releaseOnce sync.Once
	releaseWriter := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseWriter()
	go func() {
		_, err := writer.Apply(ctx, storeCommand("writer", "run-a", first.Receipt.Version), func(Snapshot) (Change, error) {
			// A test-only bounded barrier holds a real BEGIN IMMEDIATE. The
			// read path must stay available without helping/committing it.
			close(entered)
			select {
			case <-release:
				return storeChange(`{"value":2}`), nil
			case <-ctx.Done():
				return Change{}, ctx.Err()
			}
		})
		done <- err
	}()
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("writer did not enter its transaction")
	}
	for i := 0; i < 3; i++ {
		view, err := reader.ReadAt(ctx, "run-a", cut, 0, 100)
		if err != nil || view.Cut != cut || view.Snapshot.Version != 1 || len(view.Events) != 1 {
			t.Fatalf("read-only status/events cut: %+v %v", view, err)
		}
		snapshots, actual, err := reader.ReadAllAt(ctx, cut, 100)
		if err != nil || actual != cut || len(snapshots) != 1 || string(snapshots[0].Data) != `{"value":1}` {
			t.Fatalf("read while writer active lost the fixed snapshot: %+v %v", snapshots, err)
		}
		receipts, actual, err := reader.ReadReceiptsAt(ctx, cut, 100)
		if err != nil || actual != cut || len(receipts) != 1 || receipts[0].ID != "first" {
			t.Fatalf("read-only receipt cut: %+v %v", receipts, err)
		}
		page, err := reader.ReadSamples(ctx, cut, 0, 100)
		if err != nil || page.Cut != cut || len(page.Records) != 1 || page.Records[0].ID != "sample:first" {
			t.Fatalf("read-only sample cut: %+v %v", page, err)
		}
		if _, err := reader.StorageUsage(ctx); err != nil {
			t.Fatal(err)
		}
	}
	releaseWriter()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("reads retained or obstructed the real writer")
	}
	latest, latestCut, err := reader.ReadAll(ctx, 100)
	if err != nil || latestCut != cut+1 || len(latest) != 1 || latest[0].Version != 2 || string(latest[0].Data) != `{"value":2}` {
		t.Fatalf("writer did not commit exactly one transition: %+v cut=%d %v", latest, latestCut, err)
	}
	old, _, err := reader.ReadAllAt(ctx, cut, 100)
	if err != nil || len(old) != 1 || old[0].Version != 1 {
		t.Fatal("concurrent writer changed the historical cut", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if reads == 0 || len(forbidden) != 0 {
		t.Fatalf("read queries attempted a write/maintenance operation: reads=%d forbidden=%v", reads, forbidden)
	}
}

func TestStoreAtomicDedupCASAndDurableRejections(t *testing.T) {
	s, dir := testStore(t)
	cmd := storeCommand("create", "run-a", 0)
	cmd.Payload = json.RawMessage(`{"b":2,"a":1}`)
	change := storeChange(`{"status":"ready"}`)
	change.Events = append(change.Events, EventInput{Type: "test.updated", Data: json.RawMessage(`{"second":true}`)})
	first := applyChange(t, s, cmd, change)
	if first.Receipt.Version != 1 || first.Receipt.EventSeq != 2 || first.Receipt.Rejection != nil {
		t.Fatalf("bad receipt: %+v", first)
	}
	cmd.Payload = json.RawMessage("{\n\"a\": 1.0, \"b\": 2}")
	again, err := s.Apply(context.Background(), cmd, func(Snapshot) (Change, error) { t.Fatal("duplicate ran transform"); return Change{}, nil })
	if err != nil || !again.Duplicate || !reflect.DeepEqual(first.Receipt, again.Receipt) {
		t.Fatalf("duplicate not identical: %+v %v", again, err)
	}
	cmd.Payload = json.RawMessage(`{"a":2,"b":2}`)
	if _, err := s.Apply(context.Background(), cmd, nil); !errors.Is(err, ErrCommandConflict) {
		t.Fatalf("payload substitution: %v", err)
	}
	stale := storeCommand("stale", "run-a", 0)
	rejected := applyChange(t, s, stale, storeChange(`{"bad":true}`))
	if rejected.Receipt.Rejection == nil || rejected.Receipt.Rejection.Code != "version_conflict" {
		t.Fatalf("stale not rejected: %+v", rejected)
	}
	applyChange(t, s, storeCommand("advance", "run-a", 1), storeChange(`{"status":"running"}`))
	repeatRejected := applyChange(t, s, stale, storeChange(`{"bad":true}`))
	if !repeatRejected.Duplicate || !reflect.DeepEqual(rejected.Receipt, repeatRejected.Receipt) {
		t.Fatal("rejected command changed on repeat")
	}
	if err := s.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(dir, storeTestOptions)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	receipt, err := reopened.LookupReceipt(context.Background(), "owner", "create")
	if err != nil || !reflect.DeepEqual(receipt, first.Receipt) {
		t.Fatalf("receipt lost on reopen: %+v, %v", receipt, err)
	}
	view, err := reopened.Read(context.Background(), "run-a", 0, 100)
	if err != nil || view.Snapshot.Version != 2 || view.Snapshot.EventSeq != 3 || len(view.Events) != 3 {
		t.Fatalf("atomic state: %+v, %v", view, err)
	}
}

func TestStorePublicationsSamplesAndFixedCuts(t *testing.T) {
	s, _ := testStore(t)
	first := applyChange(t, s, storeCommand("first", "run-a", 0), storeChange(`{"value":1}`))
	pub := storeCommand("pub", "run-a", 0)
	pub.Mode, pub.ExpectedVersion = CommandPublication, nil
	published := applyChange(t, s, pub, Change{Data: json.RawMessage(`{"value":2}`), Events: []EventInput{{Type: "step.publication", Data: json.RawMessage(`{"hook":"progress"}`)}}})
	if published.Receipt.Version != 1 || published.Receipt.EventSeq != 2 {
		t.Fatalf("publication invalidated run CAS: %+v", published)
	}
	cut, err := s.AppendSamples(context.Background(), []SampleInput{{ID: "sample1", RunID: "run-a", Data: json.RawMessage(`{"cpu":1}`)}})
	if err != nil {
		t.Fatal(err)
	}
	if cut <= published.Receipt.Cut {
		t.Fatal("sample did not have its own cut")
	}
	dupeCut, err := s.AppendSamples(context.Background(), []SampleInput{{ID: "sample1", RunID: "run-a", Data: json.RawMessage(`{"cpu":1}`)}})
	if err != nil || dupeCut != cut {
		t.Fatalf("sample duplicate made a new cut: %d %v", dupeCut, err)
	}
	if _, err := s.AppendSamples(context.Background(), []SampleInput{{ID: "sample1", RunID: "run-a", Data: json.RawMessage(`{"cpu":2}`)}}); !errors.Is(err, ErrCommandConflict) {
		t.Fatalf("sample replacement: %v", err)
	}
	applyChange(t, s, storeCommand("later", "run-a", 1), storeChange(`{"value":3}`))
	old, err := s.ReadAt(context.Background(), "run-a", first.Receipt.Cut, 0, 100)
	if err != nil || string(old.Snapshot.Data) != `{"value":1}` || len(old.Events) != 1 {
		t.Fatalf("future leaked into historical read: %+v %v", old, err)
	}
	history, actualCut, err := s.ReadAllAt(context.Background(), cut, 10)
	if err != nil || actualCut != cut || len(history) != 1 || history[0].Version != 1 || string(history[0].Data) != `{"value":2}` {
		t.Fatalf("population cut: %+v %d %v", history, actualCut, err)
	}
	samples, err := s.ReadSamples(context.Background(), first.Receipt.Cut, 0, 10)
	if err != nil || len(samples.Records) != 0 {
		t.Fatalf("future sample leaked: %+v %v", samples, err)
	}
	samples, err = s.ReadSamples(context.Background(), cut, 0, 10)
	if err != nil || len(samples.Records) != 1 {
		t.Fatalf("sample absent at cut: %+v %v", samples, err)
	}
	page, err := s.Read(context.Background(), "run-a", 0, 1)
	if err != nil || !page.More || len(page.Events) != 1 {
		t.Fatalf("event pagination: %+v %v", page, err)
	}
	if err := s.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestStorePublicationAssignmentAdvancesRunVersion(t *testing.T) {
	s, _ := testStore(t)
	first := applyChange(t, s, storeCommand("create", "run-a", 0), storeChange(`{"value":1}`))
	command := storeCommand("assign", "run-a", 0)
	command.Mode, command.ExpectedVersion = CommandPublication, nil
	change := storeChange(`{"value":2}`)
	change.AdvanceRunVersion = true
	assigned := applyChange(t, s, command, change)
	if assigned.Receipt.Version != first.Receipt.Version+1 || assigned.Receipt.EventSeq != first.Receipt.EventSeq+1 {
		t.Fatalf("publication assignment did not invalidate stale run CAS: %+v", assigned)
	}
	stale := storeCommand("stale", "run-a", first.Receipt.Version)
	result, err := s.Apply(context.Background(), stale, func(Snapshot) (Change, error) { return storeChange(`{"value":3}`), nil })
	if err != nil || result.Receipt.Rejection == nil || result.Receipt.Rejection.Code != "version_conflict" {
		t.Fatalf("stale CAS crossed a publication assignment: %+v %v", result, err)
	}
}

func TestStoreGuardedTransitionAndHistoricalReceipts(t *testing.T) {
	s, _ := testStore(t)
	first := applyChange(t, s, storeCommand("create", "run-a", 0), storeChange(`{"attempt":"a","paused":false}`))
	stop := storeCommand("pause", "run-a", 0)
	stop.Mode = CommandMonotonic
	applyChange(t, s, stop, storeChange(`{"attempt":"a","paused":true}`))
	guarded := storeCommand("observation", "run-a", 0)
	guarded.Mode, guarded.ExpectedVersion = CommandGuarded, nil
	r, err := s.Apply(context.Background(), guarded, func(state Snapshot) (Change, error) {
		if string(state.Data) != `{"attempt":"a","paused":true}` {
			return Change{}, Reject("ownership_changed", "owning attempt changed")
		}
		return storeChange(`{"attempt":"a","paused":true,"observed":true}`), nil
	})
	if err != nil || r.Receipt.Rejection != nil || r.Receipt.Version != 3 {
		t.Fatalf("guarded observation: %+v %v", r, err)
	}
	if r := applyChange(t, s, storeCommand("ordinary-stale", "run-a", 1), storeChange(`{}`)); r.Receipt.Rejection == nil || r.Receipt.Rejection.Code != "version_conflict" {
		t.Fatal("guarded mode weakened ordinary CAS")
	}
	old, cut, err := s.ReadReceiptsAt(context.Background(), first.Receipt.Cut, 10)
	if err != nil || cut != first.Receipt.Cut || len(old) != 1 || old[0].ID != "create" {
		t.Fatalf("historical receipts: %+v %d %v", old, cut, err)
	}
	all, _, err := s.ReadReceiptsAt(context.Background(), -1, 10)
	if err != nil || len(all) != 4 || all[3].Rejection == nil {
		t.Fatalf("rejected command population: %+v %v", all, err)
	}
	if _, _, err := s.ReadReceiptsAt(context.Background(), -1, 1); err == nil {
		t.Fatal("receipt population silently truncated")
	}
	if err := s.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestStoreReceiptOnlyPublication(t *testing.T) {
	s, dir := testStore(t)
	first := applyChange(t, s, storeCommand("create", "run-a", 0), storeChange(`{"value":1}`))
	cmd := storeCommand("event-receipt", "run-a", 0)
	cmd.Mode, cmd.ExpectedVersion = CommandPublication, nil
	change := Change{ReceiptOnly: true, Result: json.RawMessage(`{"publication_id":"original"}`)}
	got := applyChange(t, s, cmd, change)
	if got.Receipt.Rejection != nil || got.Receipt.Cut != first.Receipt.Cut+1 || got.Receipt.Version != first.Receipt.Version || got.Receipt.EventSeq != first.Receipt.EventSeq || string(got.Receipt.Result) != string(change.Result) {
		t.Fatalf("receipt-only publication mutated facts: %+v", got)
	}
	view, err := s.ReadAt(context.Background(), "run-a", got.Receipt.Cut, 0, 100)
	if err != nil || len(view.Events) != 1 || string(view.Snapshot.Data) != `{"value":1}` {
		t.Fatalf("receipt-only wrote a projection/event: %+v %v", view, err)
	}
	for _, invalid := range []struct {
		mode   CommandMode
		change Change
	}{
		{CommandCAS, change}, {CommandGuarded, change},
		{CommandPublication, Change{ReceiptOnly: true}},
		{CommandPublication, Change{ReceiptOnly: true, Result: change.Result, Data: json.RawMessage(`{}`)}},
		{CommandPublication, Change{ReceiptOnly: true, Result: change.Result, AcquireSlot: "a"}},
		{CommandPublication, Change{ReceiptOnly: true, Result: change.Result, Events: storeChange(`{}`).Events}},
		{CommandPublication, Change{ReceiptOnly: true, Result: change.Result, AdvanceRunVersion: true}},
		{CommandCAS, Change{Data: json.RawMessage(`{}`), Events: storeChange(`{}`).Events, AdvanceRunVersion: true}},
	} {
		bad := cmd
		bad.ID = "invalid"
		bad.Mode = invalid.mode
		v := first.Receipt.Version
		if bad.Mode == CommandCAS {
			bad.ExpectedVersion = &v
		}
		if _, err := s.Apply(context.Background(), bad, func(Snapshot) (Change, error) { return invalid.change, nil }); err == nil {
			t.Fatalf("invalid receipt-only mutation accepted: %+v", invalid)
		}
	}
	if _, err := s.LookupReceipt(context.Background(), "owner", "invalid"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("invalid mutation left receipt: %v", err)
	}
	if err := s.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(dir, storeTestOptions)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	again, err := reopened.Apply(context.Background(), cmd, func(Snapshot) (Change, error) { t.Fatal("durable duplicate ran transform"); return Change{}, nil })
	if err != nil || !again.Duplicate || !reflect.DeepEqual(got.Receipt, again.Receipt) {
		t.Fatalf("receipt-only duplicate: %+v %v", again, err)
	}
}

func TestStoreSoftBudgetRollsBackOptionalWorkButPreservesControl(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "authority")
	opts := storeTestOptions
	opts.SoftLimitBytes = 128 << 10
	s, err := OpenStore(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	initial := storeChange(`{"status":"running"}`)
	initial.RequireStorageBudget = true
	initial.AcquireSlot = "attempt:old"
	create := storeCommand("create", "run-a", 0)
	first := applyChange(t, s, create, initial)
	if first.Receipt.Rejection != nil {
		t.Fatal(first.Receipt.Rejection)
	}
	usage, err := s.StorageUsage(context.Background())
	if err != nil || usage.AllocatedBytes != usage.PageCount*usage.PageSize || usage.AllocatedBytes >= opts.SoftLimitBytes {
		t.Fatalf("initial usage: %+v %v", usage, err)
	}
	largeJSON, err := json.Marshal(map[string]string{"data": strings.Repeat("x", 200<<10)})
	if err != nil {
		t.Fatal(err)
	}
	large := storeChange(string(largeJSON))
	large.RequireStorageBudget = true
	large.ReleaseSlot = "attempt:old"
	large.AcquireSlot = "attempt:new"
	quotaCommand := storeCommand("quota", "run-a", 1)
	denied := applyChange(t, s, quotaCommand, large)
	if denied.Receipt.Rejection == nil || denied.Receipt.Rejection.Code != "storage_budget_exhausted" || denied.Receipt.Version != 1 || denied.Receipt.EventSeq != first.Receipt.EventSeq {
		t.Fatalf("oversized optional write accepted or version advanced: %+v", denied)
	}
	view, err := s.Read(context.Background(), "run-a", 0, 100)
	if err != nil || len(view.Events) != 1 || string(view.Snapshot.Data) != string(initial.Data) {
		t.Fatalf("quota rollback left state/event: %+v %v", view, err)
	}
	id, run, err := s.Slot(context.Background())
	if err != nil || id != "attempt:old" || run != "run-a" {
		t.Fatal("quota rollback changed slot ownership")
	}
	logical := storeCommand("large-logical-receipt", "run-a", 0)
	logical.Mode, logical.ExpectedVersion = CommandPublication, nil
	logicalChange := Change{ReceiptOnly: true, RequireStorageBudget: true, Result: largeJSON}
	if r := applyChange(t, s, logical, logicalChange); r.Receipt.Rejection == nil || r.Receipt.Rejection.Code != "storage_budget_exhausted" {
		t.Fatalf("large receipt-only bypassed optional budget: %+v", r)
	}
	control := large
	control.RequireStorageBudget = false
	accepted := applyChange(t, s, storeCommand("settlement", "run-a", 1), control)
	if accepted.Receipt.Rejection != nil || accepted.Receipt.Version != 2 {
		t.Fatalf("soft quota blocked required control: %+v", accepted)
	}
	usage, err = s.StorageUsage(context.Background())
	if err != nil || usage.AllocatedBytes <= usage.SoftLimitBytes {
		t.Fatalf("control did not exercise above-quota path: %+v %v", usage, err)
	}
	newWork := storeChange(`{"new":true}`)
	newWork.RequireStorageBudget = true
	if r := applyChange(t, s, storeCommand("new-work", "run-b", 0), newWork); r.Receipt.Rejection == nil || r.Receipt.Rejection.Code != "storage_budget_exhausted" || r.Receipt.Version != 0 {
		t.Fatalf("new work admitted above soft limit: %+v", r)
	}
	_, before, err := s.ReadAll(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AppendSamples(context.Background(), []SampleInput{{ID: "sample:over-budget", Data: json.RawMessage(`{"value":1}`)}}); !errors.Is(err, ErrSampleLimit) {
		t.Fatalf("samples bypassed budget: %v", err)
	}
	_, after, err := s.ReadAll(context.Background(), 100)
	if err != nil || before != after {
		t.Fatal("rejected sample advanced cut")
	}
	again, err := s.Apply(context.Background(), create, nil)
	if err != nil || !again.Duplicate || !reflect.DeepEqual(again.Receipt, first.Receipt) {
		t.Fatal("exact receipt retry was blocked by quota")
	}
	if err := s.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenStore(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	got, err := reopened.Apply(context.Background(), quotaCommand, nil)
	if err != nil || !got.Duplicate || !reflect.DeepEqual(denied.Receipt, got.Receipt) {
		t.Fatalf("quota rejection did not survive reopening: %+v %v", got, err)
	}
	readonlyOpts := opts
	readonlyOpts.ReadOnly = true
	readonly, err := OpenStore(dir, readonlyOpts)
	if err != nil {
		t.Fatal(err)
	}
	defer readonly.Close()
	if observed, err := readonly.StorageUsage(context.Background()); err != nil || observed.AllocatedBytes <= opts.SoftLimitBytes {
		t.Fatalf("read-only usage unavailable: %+v %v", observed, err)
	}
}

func TestStoreSampleBudgetAfterActualSQLiteAllocation(t *testing.T) {
	opts := storeTestOptions
	opts.SoftLimitBytes = 128 << 10
	s, err := OpenStore(filepath.Join(t.TempDir(), "authority"), opts)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	data, err := json.Marshal(map[string]string{"payload": strings.Repeat("z", 60<<10)})
	if err != nil {
		t.Fatal(err)
	}
	batch := []SampleInput{{ID: "sample:a", Data: data}, {ID: "sample:b", Data: data}}
	if _, err := s.AppendSamples(context.Background(), batch); !errors.Is(err, ErrSampleLimit) {
		t.Fatalf("over-budget batch: %v", err)
	}
	page, err := s.ReadSamples(context.Background(), -1, 0, 100)
	if err != nil || page.Cut != 0 || len(page.Records) != 0 {
		t.Fatal("over-budget sample batch partially persisted")
	}
}

func TestStoreReceiptPopulationIsNotLimitedToOnePage(t *testing.T) {
	s, _ := testStore(t)
	for i := 0; i < 1005; i++ {
		cmd := storeCommand(fmt.Sprintf("reject-%d", i), "run:none", 0)
		result, err := s.Apply(context.Background(), cmd, func(Snapshot) (Change, error) { return Change{}, Reject("policy_denied", "fixture") })
		if err != nil || result.Receipt.Rejection == nil {
			t.Fatal("failed to persist a rejected command")
		}
	}
	all, cut, err := s.ReadReceiptsAt(context.Background(), -1, MaxReceiptRecords)
	if err != nil || len(all) != 1005 || cut != 1005 {
		t.Fatalf("valid history was clipped at a page: %d %d %v", len(all), cut, err)
	}
	if _, _, err := s.ReadReceiptsAt(context.Background(), -1, 1000); err == nil {
		t.Fatal("explicit smaller scan limit silently truncated")
	}
}

func TestStoreGlobalAdmissionSlot(t *testing.T) {
	s, _ := testStore(t)
	first := storeChange(`{"attempt":"attempt-a"}`)
	first.AcquireSlot = "attempt-a"
	if r := applyChange(t, s, storeCommand("admit-a", "run-a", 0), first); r.Receipt.Rejection != nil {
		t.Fatal(r.Receipt.Rejection)
	}
	second := storeChange(`{"attempt":"attempt-b"}`)
	second.AcquireSlot = "attempt-b"
	if r := applyChange(t, s, storeCommand("admit-b", "run-b", 0), second); r.Receipt.Rejection == nil || r.Receipt.Rejection.Code != "capacity_conflict" {
		t.Fatalf("second admission: %+v", r)
	}
	if _, err := s.Read(context.Background(), "run-b", 0, 10); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rejected admission partially created run: %v", err)
	}
	release := storeChange(`{"attempt":null}`)
	release.ReleaseSlot = "attempt-b"
	if r := applyChange(t, s, storeCommand("wrong-release", "run-a", 1), release); r.Receipt.Rejection == nil {
		t.Fatal("wrong owner released slot")
	}
	release.ReleaseSlot = "attempt-a"
	if r := applyChange(t, s, storeCommand("release-a", "run-a", 1), release); r.Receipt.Rejection != nil {
		t.Fatal(r.Receipt.Rejection)
	}
	if r := applyChange(t, s, storeCommand("admit-b-again", "run-b", 0), second); r.Receipt.Rejection != nil {
		t.Fatal(r.Receipt.Rejection)
	}
	id, run, err := s.Slot(context.Background())
	if err != nil || id != "attempt-b" || run != "run-b" {
		t.Fatalf("slot: %s %s %v", id, run, err)
	}
}

func TestStoreConcurrentCAS(t *testing.T) {
	s, dir := testStore(t)
	applyChange(t, s, storeCommand("create", "run-a", 0), storeChange(`{"value":0}`))
	other, err := OpenStore(dir, storeTestOptions)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	const clients = 12
	results := make(chan ApplyResult, clients)
	errorsCh := make(chan error, clients)
	var wg sync.WaitGroup
	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store := s
			if i%2 != 0 {
				store = other
			}
			r, err := store.Apply(context.Background(), storeCommand(fmt.Sprintf("race-%d", i), "run-a", 1), func(Snapshot) (Change, error) { return storeChange(`{"value":1}`), nil })
			results <- r
			errorsCh <- err
		}(i)
	}
	wg.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	accepted, rejected := 0, 0
	for r := range results {
		if r.Receipt.Rejection == nil {
			accepted++
		} else if r.Receipt.Rejection.Code == "version_conflict" {
			rejected++
		}
	}
	if accepted != 1 || rejected != clients-1 {
		t.Fatalf("accepted=%d rejected=%d", accepted, rejected)
	}
	if err := s.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestStoreRollbackBusyAndDiskFull(t *testing.T) {
	t.Run("write-rollback", func(t *testing.T) {
		s, _ := testStore(t)
		if _, err := s.db.Exec(`CREATE TRIGGER refuse_receipt BEFORE INSERT ON commands BEGIN SELECT RAISE(ABORT,'injected receipt persistence failure'); END`); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Apply(context.Background(), storeCommand("fail", "run-a", 0), func(Snapshot) (Change, error) { return storeChange(`{"value":1}`), nil }); err == nil {
			t.Fatal("failed receipt acknowledged")
		}
		if _, err := s.Read(context.Background(), "run-a", 0, 10); !errors.Is(err, ErrNotFound) {
			t.Fatalf("partial state after rollback: %v", err)
		}
		if _, err := s.LookupReceipt(context.Background(), "owner", "fail"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("partial receipt after rollback: %v", err)
		}
		if err := s.Verify(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("busy", func(t *testing.T) {
		s, dir := testStore(t)
		other, err := OpenStore(dir, storeTestOptions)
		if err != nil {
			t.Fatal(err)
		}
		defer other.Close()
		conn, err := s.begin(context.Background(), true)
		if err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		_, err = other.Apply(context.Background(), storeCommand("blocked", "run-a", 0), func(Snapshot) (Change, error) { return storeChange(`{"value":1}`), nil })
		if err == nil || time.Since(started) > 2*time.Second {
			t.Fatalf("busy not bounded: %v, %s", err, time.Since(started))
		}
		if err := rollbackClose(conn); err != nil {
			t.Fatal(err)
		}
		if r := applyChange(t, other, storeCommand("blocked", "run-a", 0), storeChange(`{"value":1}`)); r.Receipt.Rejection != nil {
			t.Fatal("busy became a committed rejection")
		}
	})
	t.Run("sqlite-full", func(t *testing.T) {
		s, _ := testStore(t)
		s.db.SetMaxOpenConns(1)
		var pages int
		if err := s.db.QueryRow("PRAGMA page_count").Scan(&pages); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(fmt.Sprintf("PRAGMA max_page_count=%d", pages)); err != nil {
			t.Fatal(err)
		}
		large := storeChange(`{"value":"` + strings.Repeat("x", 2<<20) + `"}`)
		_, err := s.Apply(context.Background(), storeCommand("full", "run-a", 0), func(Snapshot) (Change, error) { return large, nil })
		if err == nil || !strings.Contains(err.Error(), "full") {
			t.Fatalf("expected real SQLITE_FULL: %v", err)
		}
		if _, err := s.LookupReceipt(context.Background(), "owner", "full"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("disk-full acked: %v", err)
		}
		if _, err := s.Read(context.Background(), "run-a", 0, 10); !errors.Is(err, ErrNotFound) {
			t.Fatalf("disk-full left state: %v", err)
		}
		if err := s.Verify(context.Background()); err != nil {
			t.Fatal(err)
		}
	})
}

func TestStoreSemanticRejectAndInfrastructureFailure(t *testing.T) {
	s, _ := testStore(t)
	r, err := s.Apply(context.Background(), storeCommand("reject", "run-a", 0), func(Snapshot) (Change, error) { return Change{}, Reject("policy_denied", "not allowed") })
	if err != nil || r.Receipt.Rejection == nil || r.Receipt.Rejection.Code != "policy_denied" {
		t.Fatalf("semantic rejection not durable: %+v %v", r, err)
	}
	_, err = s.Apply(context.Background(), storeCommand("infra", "run-a", 0), func(Snapshot) (Change, error) { return Change{}, errors.New("transient preparation failure") })
	if err == nil {
		t.Fatal("infrastructure failure acknowledged")
	}
	if _, err := s.LookupReceipt(context.Background(), "owner", "infra"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("infrastructure receipt persisted: %v", err)
	}
}

func TestStoreRefusesCorruptionAndNewVersions(t *testing.T) {
	for _, mutation := range []struct {
		name, sql string
		want      error
	}{
		{"storage-version", "PRAGMA user_version=99", ErrIncompatible},
		{"foreign-database", "PRAGMA user_version=0; PRAGMA application_id=0", ErrIncompatible},
		{"event-version", "UPDATE events SET schema_version=99", ErrIncompatible},
		{"event-type", "UPDATE events SET type='unknown.event'", ErrIncompatible},
		{"projection", "UPDATE runs SET snapshot='{}'", ErrIntegrity},
		{"event-digest", "UPDATE events SET data='{}'", ErrIntegrity},
		{"receipt-digest", "UPDATE commands SET receipt='{}'", ErrIntegrity},
	} {
		t.Run(mutation.name, func(t *testing.T) {
			s, dir := testStore(t)
			applyChange(t, s, storeCommand("first", "run-a", 0), storeChange(`{"value":1}`))
			if _, err := s.db.Exec(mutation.sql); err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			opened, err := OpenStore(dir, storeTestOptions)
			if opened != nil {
				_ = opened.Close()
			}
			if !errors.Is(err, mutation.want) {
				t.Fatalf("open allowed corrupt or future data: %v", err)
			}
		})
	}
}

func TestStoreReadOnlyAndRelocation(t *testing.T) {
	s, dir := testStore(t)
	applyChange(t, s, storeCommand("first", "run-a", 0), storeChange(`{"value":1}`))
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(t.TempDir(), "restored")
	if err := os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "state.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "state.sqlite3"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if moved, err := OpenStore(destination, storeTestOptions); !errors.Is(err, ErrRecoveryRequired) {
		if moved != nil {
			_ = moved.Close()
		}
		t.Fatalf("moved authority can dispatch: %v", err)
	}
	opts := storeTestOptions
	opts.ReadOnly = true
	ro, err := OpenStore(destination, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer ro.Close()
	if _, err := ro.Read(context.Background(), "run-a", 0, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := ro.Apply(context.Background(), storeCommand("new", "run-a", 1), nil); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("readonly store wrote: %v", err)
	}
	missing := filepath.Join(t.TempDir(), "absent")
	if _, err := OpenStore(missing, opts); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("readonly open created state: %v", err)
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read created a directory")
	}
}

func TestStoreCrashRecovery(t *testing.T) {
	for _, point := range []string{"before-commit", "after-commit"} {
		t.Run(point, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "authority")
			cmd := exec.Command(os.Args[0], "-test.run=^TestStoreCrashHelper$")
			cmd.Env = append(os.Environ(), "PRIFLY_STORE_CRASH_DIR="+dir, "PRIFLY_STORE_CRASH_POINT="+point)
			output, crashErr := cmd.CombinedOutput()
			var exited *exec.ExitError
			wantCode := 91
			if point == "after-commit" {
				wantCode = 92
			}
			if !errors.As(crashErr, &exited) || exited.ExitCode() != wantCode {
				t.Fatalf("helper did not reach the requested crash point: %v, %s", crashErr, output)
			}
			s, err := OpenStore(dir, storeTestOptions)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			r, err := s.Apply(context.Background(), storeCommand("crash", "run-a", 0), func(Snapshot) (Change, error) { return storeChange(`{"value":1}`), nil })
			if err != nil || r.Duplicate != (point == "after-commit") || r.Receipt.Rejection != nil {
				t.Fatalf("crash recovery: %+v %v", r, err)
			}
			if err := s.Verify(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStoreCrashHelper(t *testing.T) {
	dir := os.Getenv("PRIFLY_STORE_CRASH_DIR")
	if dir == "" {
		return
	}
	s, err := OpenStore(dir, storeTestOptions)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Apply(context.Background(), storeCommand("crash", "run-a", 0), func(Snapshot) (Change, error) {
		if os.Getenv("PRIFLY_STORE_CRASH_POINT") == "before-commit" {
			os.Exit(91)
		}
		return storeChange(`{"value":1}`), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	os.Exit(92)
}

// The stage acceptance asks that the scheduler never admit over-limit work.
// Racing admissions must therefore produce one holder of the single slot and
// explainable refusals for the rest, not two runs believing they were admitted.
func TestConcurrentAdmissionsNeverExceedTheSlot(t *testing.T) {
	s, _ := testStore(t)
	const racers = 8
	results := make(chan *Rejection, racers)
	for i := 0; i < racers; i++ {
		go func(i int) {
			runID := fmt.Sprintf("run-%d", i)
			change := storeChange(fmt.Sprintf(`{"attempt":"attempt-%d"}`, i))
			change.AcquireSlot = fmt.Sprintf("attempt-%d", i)
			result, err := s.Apply(context.Background(), storeCommand(fmt.Sprintf("admit-%d", i), runID, 0), func(Snapshot) (Change, error) {
				return change, nil
			})
			if err != nil {
				t.Errorf("a racing admission failed instead of deciding: %v", err)
				results <- &Rejection{Code: "error"}
				return
			}
			results <- result.Receipt.Rejection
		}(i)
	}
	admitted, refused := 0, 0
	for i := 0; i < racers; i++ {
		rejection := <-results
		if rejection == nil {
			admitted++
			continue
		}
		refused++
		if rejection.Code != "capacity_conflict" {
			t.Fatalf("a losing admission was refused without an explainable reason: %s", rejection.Code)
		}
	}
	if admitted != 1 || refused != racers-1 {
		t.Fatalf("racing admissions exceeded the slot: admitted=%d refused=%d", admitted, refused)
	}
	// The refused runs were not partially created.
	created := 0
	for i := 0; i < racers; i++ {
		if _, err := s.Read(context.Background(), fmt.Sprintf("run-%d", i), 0, 10); err == nil {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("a refused admission left a run behind: created=%d", created)
	}
}

// Capacity is a recorded bound, not a constant. Raising it admits exactly that
// many attempts and no more, and the migration from a single slot keeps the
// capacity it had so an upgraded installation admits what it admitted before.
func TestSlotCapacityBoundsConcurrentAdmissions(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	capacity, err := s.SlotCapacity(ctx)
	if err != nil || capacity != 1 {
		t.Fatalf("a new authority did not start at one slot: %d %v", capacity, err)
	}
	if _, err := s.db.Exec("UPDATE authority SET slot_capacity=2 WHERE singleton=1"); err != nil {
		t.Fatal(err)
	}
	admit := func(i int) *Rejection {
		change := storeChange(fmt.Sprintf(`{"attempt":"attempt-%d"}`, i))
		change.AcquireSlot = fmt.Sprintf("attempt-%d", i)
		return applyChange(t, s, storeCommand(fmt.Sprintf("admit-%d", i), fmt.Sprintf("run-%d", i), 0), change).Receipt.Rejection
	}
	if rejection := admit(0); rejection != nil {
		t.Fatalf("the first admission was refused: %+v", rejection)
	}
	if rejection := admit(1); rejection != nil {
		t.Fatalf("the second admission was refused under a capacity of two: %+v", rejection)
	}
	if rejection := admit(2); rejection == nil || rejection.Code != "capacity_conflict" {
		t.Fatalf("a third admission exceeded the recorded capacity: %+v", rejection)
	}
	held, err := s.Slots(ctx)
	if err != nil || len(held) != 2 {
		t.Fatalf("the held set does not match the admissions: %v %+v", err, held)
	}
	// Slot reports a single holder only when exactly one is held.
	if id, _, err := s.Slot(ctx); err != nil || id != "" {
		t.Fatalf("two held slots were reported as one: %q %v", id, err)
	}
	release := storeChange(`{"attempt":null}`)
	release.ReleaseSlot = "attempt-0"
	if r := applyChange(t, s, storeCommand("release-0", "run-0", 1), release); r.Receipt.Rejection != nil {
		t.Fatal(r.Receipt.Rejection)
	}
	// The refused admission already spent its command identity, so the waiting
	// attempt returns as a new command rather than replaying a stored refusal.
	retry := storeChange(`{"attempt":"attempt-2"}`)
	retry.AcquireSlot = "attempt-2"
	if r := applyChange(t, s, storeCommand("admit-2-again", "run-2", 0), retry); r.Receipt.Rejection != nil {
		t.Fatalf("a freed slot did not admit the waiting attempt: %+v", r.Receipt.Rejection)
	}
	// A slot held by another run is refused rather than stolen.
	steal := storeChange(`{"attempt":"attempt-1"}`)
	steal.AcquireSlot = "attempt-1"
	if r := applyChange(t, s, storeCommand("steal", "run-9", 0), steal); r.Receipt.Rejection == nil || r.Receipt.Rejection.Code != "slot_conflict" {
		t.Fatalf("one run took a slot held by another: %+v", r.Receipt)
	}
}

// Capacity and the decision that set it commit together, so the enforced bound
// and the recorded reason can never describe different numbers. A capacity the
// authority is already exceeding is refused rather than applied and repaired.
func TestAdmissionCapacityChangesWithItsOwnDecision(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	setCapacity := func(id string, capacity int64) (AuthorityApplyResult, error) {
		value := capacity
		return s.ApplyAuthority(ctx, AuthorityCommand{ID: id, Actor: "owner", Key: "authority:controls", Payload: json.RawMessage(`{"operation":"admission.capacity"}`)}, func(AuthoritySnapshot) (AuthorityChange, error) {
			return AuthorityChange{Data: json.RawMessage(`{"schema_version":"1"}`), SetCapacity: &value}, nil
		})
	}
	raised, err := setCapacity("authority:raise", 3)
	if err != nil || raised.Receipt.Rejection != nil {
		t.Fatalf("capacity was not raised: %+v %v", raised.Receipt.Rejection, err)
	}
	if capacity, err := s.SlotCapacity(ctx); err != nil || capacity != 3 {
		t.Fatalf("the enforced capacity did not follow its decision: %d %v", capacity, err)
	}

	// Hold two of the three slots.
	for i := 0; i < 2; i++ {
		change := storeChange(fmt.Sprintf(`{"attempt":"attempt-%d"}`, i))
		change.AcquireSlot = fmt.Sprintf("attempt-%d", i)
		if rejection := applyChange(t, s, storeCommand(fmt.Sprintf("admit-%d", i), fmt.Sprintf("run-%d", i), 0), change).Receipt.Rejection; rejection != nil {
			t.Fatalf("an admission within capacity was refused: %+v", rejection)
		}
	}
	lowered, err := setCapacity("authority:lower", 1)
	if err != nil {
		t.Fatal(err)
	}
	if lowered.Receipt.Rejection == nil || lowered.Receipt.Rejection.Code != "capacity_conflict" {
		t.Fatalf("capacity was lowered below the attempts already admitted: %+v", lowered.Receipt.Rejection)
	}
	if capacity, err := s.SlotCapacity(ctx); err != nil || capacity != 3 {
		t.Fatalf("a refused decision still changed the capacity: %d %v", capacity, err)
	}
	// Lowering to exactly what is held is admissible: nothing is evicted.
	exact, err := setCapacity("authority:exact", 2)
	if err != nil || exact.Receipt.Rejection != nil {
		t.Fatalf("lowering to the held count was refused: %+v %v", exact.Receipt.Rejection, err)
	}
	if capacity, err := s.SlotCapacity(ctx); err != nil || capacity != 2 {
		t.Fatalf("capacity did not settle at the held count: %d %v", capacity, err)
	}
	// A third admission is now refused for a named reason, not stalled.
	change := storeChange(`{"attempt":"attempt-2"}`)
	change.AcquireSlot = "attempt-2"
	rejection := applyChange(t, s, storeCommand("admit-2", "run-2", 0), change).Receipt.Rejection
	if rejection == nil || rejection.Code != "capacity_conflict" {
		t.Fatalf("an admission above capacity was not refused by name: %+v", rejection)
	}
	if _, err := setCapacity("authority:zero", 0); err != nil {
		t.Fatal(err)
	} else if capacity, _ := s.SlotCapacity(ctx); capacity != 2 {
		t.Fatalf("a capacity below one was applied: %d", capacity)
	}
}

func admitRun(t *testing.T, s *Store, command, runID, attempt string) *Rejection {
	t.Helper()
	change := storeChange(fmt.Sprintf(`{"attempt":%q}`, attempt))
	change.AcquireSlot = attempt
	return applyChange(t, s, storeCommand(command, runID, 0), change).Receipt.Rejection
}

// A free slot goes to the run that has waited longest, not to the one that
// happens to ask at a luckier moment. Without that rule a busy authority can
// starve a run indefinitely while others keep taking the slot it is waiting for.
func TestFreedSlotServesTheLongestWaitingRun(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	if rejection := admitRun(t, s, "admit-holder", "run-holder", "attempt-holder"); rejection != nil {
		t.Fatalf("the first admission was refused: %+v", rejection)
	}
	// Two runs queue, in this order.
	if rejection := admitRun(t, s, "admit-early", "run-early", "attempt-early"); rejection == nil || rejection.Code != "capacity_conflict" {
		t.Fatalf("a full authority did not refuse by name: %+v", rejection)
	}
	if rejection := admitRun(t, s, "admit-late", "run-late", "attempt-late"); rejection == nil || rejection.Code != "capacity_conflict" {
		t.Fatalf("a full authority did not refuse by name: %+v", rejection)
	}
	waiting, err := s.SlotWaiters(ctx)
	if err != nil || len(waiting) != 2 || waiting["run-early"] >= waiting["run-late"] {
		t.Fatalf("the queue did not record both runs in order: %v %v", waiting, err)
	}

	release := storeChange(`{"attempt":null}`)
	release.ReleaseSlot = "attempt-holder"
	if r := applyChange(t, s, storeCommand("release-holder", "run-holder", 1), release); r.Receipt.Rejection != nil {
		t.Fatal(r.Receipt.Rejection)
	}

	// The later arrival asks first and is deferred, by name, to the earlier one.
	deferred := admitRun(t, s, "admit-late-2", "run-late", "attempt-late")
	if deferred == nil || deferred.Code != "admission_deferred" {
		t.Fatalf("a later arrival overtook the queue: %+v", deferred)
	}
	if !strings.Contains(deferred.Message, "run-early") {
		t.Fatalf("the deferral did not name the run ahead: %s", deferred.Message)
	}
	if rejection := admitRun(t, s, "admit-early-2", "run-early", "attempt-early"); rejection != nil {
		t.Fatalf("the longest waiting run was refused its turn: %+v", rejection)
	}
	// Taking the slot leaves the queue: a holder is not also a waiter.
	waiting, err = s.SlotWaiters(ctx)
	if err != nil || len(waiting) != 1 || waiting["run-late"] == 0 {
		t.Fatalf("the admitted run stayed in the queue: %v %v", waiting, err)
	}
}

// A place in the queue is held by asking again. The store cannot tell a live
// waiter from an abandoned one, so a run that stops asking must stop blocking
// everyone else rather than hold the authority forever.
func TestAnAbandonedWaiterStopsHoldingTheQueue(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	if rejection := admitRun(t, s, "admit-holder", "run-holder", "attempt-holder"); rejection != nil {
		t.Fatal(rejection)
	}
	if rejection := admitRun(t, s, "admit-ghost", "run-ghost", "attempt-ghost"); rejection == nil {
		t.Fatal("a full authority admitted a second attempt")
	}
	release := storeChange(`{"attempt":null}`)
	release.ReleaseSlot = "attempt-holder"
	if r := applyChange(t, s, storeCommand("release-holder", "run-holder", 1), release); r.Receipt.Rejection != nil {
		t.Fatal(r.Receipt.Rejection)
	}
	// While the ghost is fresh it holds its place against a newcomer.
	if deferred := admitRun(t, s, "admit-new", "run-new", "attempt-new"); deferred == nil || deferred.Code != "admission_deferred" {
		t.Fatalf("a fresh waiter did not hold its place: %+v", deferred)
	}
	// Age the ghost past its patience without touching anything else.
	if _, err := s.db.Exec("UPDATE slot_waiters SET seen_seq=seen_seq-? WHERE run_id='run-ghost'", SlotWaiterPatience+1); err != nil {
		t.Fatal(err)
	}
	if rejection := admitRun(t, s, "admit-new-2", "run-new", "attempt-new"); rejection != nil {
		t.Fatalf("an abandoned waiter still held the authority: %+v", rejection)
	}
	waiting, err := s.SlotWaiters(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, present := waiting["run-ghost"]; present {
		t.Fatalf("the abandoned waiter was left in the queue: %v", waiting)
	}
}

// The queue is bounded. Beyond the bound an admission is refused for that
// reason, rather than the queue growing without limit behind a bounded slot set.
func TestAdmissionQueueIsBounded(t *testing.T) {
	s, _ := testStore(t)
	if rejection := admitRun(t, s, "admit-holder", "run-holder", "attempt-holder"); rejection != nil {
		t.Fatal(rejection)
	}
	for i := 0; i < MaxSlotWaiters; i++ {
		rejection := admitRun(t, s, fmt.Sprintf("admit-%d", i), fmt.Sprintf("run-%d", i), fmt.Sprintf("attempt-%d", i))
		if rejection == nil || rejection.Code != "capacity_conflict" {
			t.Fatalf("waiter %d was not queued: %+v", i, rejection)
		}
	}
	overflow := admitRun(t, s, "admit-overflow", "run-overflow", "attempt-overflow")
	if overflow == nil || overflow.Code != "admission_queue_full" {
		t.Fatalf("the queue grew past its bound: %+v", overflow)
	}
	waiting, err := s.SlotWaiters(context.Background())
	if err != nil || len(waiting) != MaxSlotWaiters {
		t.Fatalf("the queue is not at its bound: %d %v", len(waiting), err)
	}
}

// Patience is measured in admission decisions and must exceed the queue size.
// A shorter patience would evict a waiting run after a single round of other
// runs asking, and would make the queue bound unreachable.
func TestQueuePatienceOutlastsAFullRoundOfWaiters(t *testing.T) {
	if SlotWaiterPatience <= MaxSlotWaiters {
		t.Fatalf("patience %d does not outlast a full queue of %d", SlotWaiterPatience, MaxSlotWaiters)
	}
	s, _ := testStore(t)
	if rejection := admitRun(t, s, "admit-holder", "run-holder", "attempt-holder"); rejection != nil {
		t.Fatal(rejection)
	}
	if rejection := admitRun(t, s, "admit-early", "run-early", "attempt-early"); rejection == nil {
		t.Fatal("a full authority admitted a second attempt")
	}
	// A full round of other runs asking must not cost the early run its place.
	for i := 0; i < MaxSlotWaiters-2; i++ {
		admitRun(t, s, fmt.Sprintf("admit-%d", i), fmt.Sprintf("run-%d", i), fmt.Sprintf("attempt-%d", i))
	}
	waiting, err := s.SlotWaiters(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, present := waiting["run-early"]; !present {
		t.Fatalf("a round of other waiters evicted the longest waiting run: %v", waiting)
	}
}

func TestCreateLinkedRunChecksSourceAndPreservesIt(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	zero := int64(0)
	if result, err := s.Apply(ctx, Command{ID: "command:source", Actor: "owner", RunID: "run:source", Payload: json.RawMessage(`{"operation":"start"}`), ExpectedVersion: &zero, Mode: CommandCAS}, func(Snapshot) (Change, error) {
		return storeChange(`{"source":true}`), nil
	}); err != nil || result.Receipt.Rejection != nil {
		t.Fatalf("seed source: %+v %v", result, err)
	}
	if result, err := s.ApplyAuthority(ctx, AuthorityCommand{ID: "authority:packages", Actor: "owner", Key: "authority:packages", Payload: json.RawMessage(`{"operation":"package.trust"}`)}, func(AuthoritySnapshot) (AuthorityChange, error) {
		return AuthorityChange{Data: json.RawMessage(`{"trusted":true}`)}, nil
	}); err != nil || result.Receipt.Rejection != nil {
		t.Fatalf("seed package trust: %+v %v", result, err)
	}
	command := LinkedRunCommand{ID: "command:fork", Actor: "owner", SourceRunID: "run:source", NewRunID: "run:fork", ExpectedVersion: 1, Payload: json.RawMessage(`{"operation":"run.fork"}`), Pins: []ControlPin{{Key: "authority:packages", Version: 1}}}
	called := 0
	result, err := s.CreateLinkedRun(ctx, command, func(source Snapshot) (Change, error) {
		called++
		if source.Version != 1 || string(source.Data) != `{"source":true}` {
			t.Fatalf("wrong source snapshot: %+v", source)
		}
		return Change{Data: json.RawMessage(`{"fork":true}`), Events: []EventInput{{Type: "test.updated", Data: json.RawMessage(`{"linked":true}`)}}, Result: json.RawMessage(`{"new_run_id":"run:fork"}`), RequireStorageBudget: true}, nil
	})
	if err != nil || result.Receipt.Rejection != nil || called != 1 || result.Receipt.RunID != "run:source" || result.Receipt.Version != 1 {
		t.Fatalf("create linked run: %+v %v called=%d", result, err, called)
	}
	source, err := s.Read(ctx, "run:source", 0, 10)
	if err != nil || source.Snapshot.Version != 1 || string(source.Snapshot.Data) != `{"source":true}` {
		t.Fatalf("source history changed: %+v %v", source, err)
	}
	created, err := s.Read(ctx, "run:fork", 0, 10)
	if err != nil || created.Snapshot.Version != 1 || created.Snapshot.EventSeq != 1 || string(created.Snapshot.Data) != `{"fork":true}` {
		t.Fatalf("linked run missing: %+v %v", created, err)
	}
	duplicate, err := s.CreateLinkedRun(ctx, command, func(Snapshot) (Change, error) {
		t.Fatal("duplicate recreated the linked run")
		return Change{}, nil
	})
	if err != nil || !duplicate.Duplicate || called != 1 {
		t.Fatalf("duplicate linked command: %+v %v called=%d", duplicate, err, called)
	}
	stale := command
	stale.ID, stale.NewRunID, stale.ExpectedVersion = "command:stale-fork", "run:stale-fork", 0
	rejected, err := s.CreateLinkedRun(ctx, stale, func(Snapshot) (Change, error) {
		t.Fatal("stale source version entered the reducer")
		return Change{}, nil
	})
	if err != nil || rejected.Receipt.Rejection == nil || rejected.Receipt.Rejection.Code != "version_conflict" {
		t.Fatalf("stale source version created a linked run: %+v %v", rejected, err)
	}
	staleTrust := command
	staleTrust.ID, staleTrust.NewRunID = "command:stale-trust", "run:stale-trust"
	staleTrust.Pins = []ControlPin{{Key: "authority:packages", Version: 0}}
	rejected, err = s.CreateLinkedRun(ctx, staleTrust, func(Snapshot) (Change, error) {
		t.Fatal("stale authority state entered the reducer")
		return Change{}, nil
	})
	if err != nil || rejected.Receipt.Rejection == nil || rejected.Receipt.Rejection.Code != "authority_state_conflict" {
		t.Fatalf("stale trust created a linked run: %+v %v", rejected, err)
	}
	if err := s.Verify(ctx); err != nil {
		t.Fatalf("linked run does not verify: %v", err)
	}
}

// A read-only open does not migrate, so every read has to work on the shape the
// database already has. The packed columns were selected unconditionally, and an
// authority written by an earlier release answered every read with a
// persistence failure instead of its Runs.
func TestStoreReadsAnUnmigratedDatabaseReadOnly(t *testing.T) {
	s, dir := testStore(t)
	ctx := context.Background()
	applyChange(t, s, storeCommand("create", "run-a", 0), storeChange(`{"value":1}`))
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(dir, "state.sqlite3")+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DROP TABLE pinned_bytes; ALTER TABLE runs DROP COLUMN snapshot_packed; ALTER TABLE events DROP COLUMN state_packed; ALTER TABLE authority DROP COLUMN verified_cut; PRAGMA user_version=4"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	options := storeTestOptions
	options.ReadOnly = true
	reader, err := OpenStore(dir, options)
	if err != nil {
		t.Fatalf("a read-only open of an unmigrated database failed: %v", err)
	}
	defer reader.Close()
	if reader.Info().StorageVersion != 4 {
		t.Fatalf("a read-only open migrated the database: %+v", reader.Info())
	}
	view, err := reader.Read(ctx, "run-a", 0, 10)
	if err != nil {
		t.Fatalf("reading a Run failed: %v", err)
	}
	if string(view.Snapshot.Data) != `{"value":1}` {
		t.Fatalf("the Run read back changed: %s", view.Snapshot.Data)
	}
	if _, _, err := reader.ReadAll(ctx, 10); err != nil {
		t.Fatalf("reading every Run failed: %v", err)
	}
	if err := reader.Verify(ctx); err != nil {
		t.Fatalf("verifying an unmigrated database failed: %v", err)
	}
}

// Verification at open used to read the whole database, so a long-lived
// authority paid for its own history every time it was opened. It now records
// how far it has checked and continues from there; a database that predates
// the mark still verifies everything once.
func TestStoreVerifiesIncrementallyFromItsRecordedCut(t *testing.T) {
	s, dir := testStore(t)
	ctx := context.Background()
	applyChange(t, s, storeCommand("create", "run-a", 0), storeChange(`{"value":1}`))
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	// A v4 database has no mark: its first open verifies everything and records
	// one, and the migration itself is a retryable single transaction.
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(dir, "state.sqlite3")+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DROP TABLE pinned_bytes; ALTER TABLE runs DROP COLUMN snapshot_packed; ALTER TABLE events DROP COLUMN state_packed; ALTER TABLE authority DROP COLUMN verified_cut; PRAGMA user_version=4"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := OpenStore(dir, storeTestOptions)
	if err != nil {
		t.Fatal(err)
	}
	if upgraded.Info().StorageVersion != StorageVersion {
		t.Fatalf("store was not migrated: %+v", upgraded.Info())
	}
	var cut, verified int64
	if err := upgraded.db.QueryRow("SELECT cut,verified_cut FROM authority WHERE singleton=1").Scan(&cut, &verified); err != nil {
		t.Fatal(err)
	}
	if verified != cut || cut == 0 {
		t.Fatalf("the first open did not record what it verified: cut=%d verified=%d", cut, verified)
	}
	applyChange(t, upgraded, storeCommand("update", "run-a", 1), storeChange(`{"value":2}`))
	if err := upgraded.Close(); err != nil {
		t.Fatal(err)
	}
	// Reopening advances the mark, and the complete check still passes.
	again, err := OpenStore(dir, storeTestOptions)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if err := again.db.QueryRow("SELECT cut,verified_cut FROM authority WHERE singleton=1").Scan(&cut, &verified); err != nil {
		t.Fatal(err)
	}
	if verified != cut {
		t.Fatalf("reopening did not advance the mark: cut=%d verified=%d", cut, verified)
	}
	if err := again.Verify(ctx); err != nil {
		t.Fatalf("the complete verification no longer passes: %v", err)
	}
	view, err := again.Read(ctx, "run-a", 0, 10)
	if err != nil || string(view.Snapshot.Data) != `{"value":2}` {
		t.Fatalf("history did not survive incremental verification: %v %s", err, view.Snapshot.Data)
	}
}

// Corruption written after the recorded mark is still refused at open.
func TestStoreOpenRefusesCorruptionAfterTheVerifiedCut(t *testing.T) {
	s, dir := testStore(t)
	applyChange(t, s, storeCommand("create", "run-a", 0), storeChange(`{"value":1}`))
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	opened, err := OpenStore(dir, storeTestOptions)
	if err != nil {
		t.Fatal(err)
	}
	applyChange(t, opened, storeCommand("update", "run-a", 1), storeChange(`{"value":2}`))
	if _, err := opened.db.Exec("UPDATE events SET data='{\"tampered\":true}' WHERE seq=(SELECT MAX(seq) FROM events WHERE run_id='run-a')"); err != nil {
		t.Fatal(err)
	}
	if err := opened.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(dir, storeTestOptions); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("a tampered new event was accepted at open: %v", err)
	}
}

// A Run repeats its pinned bytes in every version and in every event that
// records a state. Those strings are stored once and referenced, and what is
// read back is the exact document that was written.
func TestStoreSharesPinnedBytesAcrossSnapshots(t *testing.T) {
	s, dir := testStore(t)
	ctx := context.Background()
	pinned := strings.Repeat("A", 32<<10)
	state := func(version int) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(`{"version":%d,"definitions":[{"bytes":"%s"}],"note":"short"}`, version, pinned))
	}
	for i := range 8 {
		applyChange(t, s, storeCommand(fmt.Sprintf("write-%d", i), "run-a", int64(i)), Change{Data: state(i + 1), Events: []EventInput{{Type: "test.updated", Version: EventVersion, Data: json.RawMessage(`{"n":1}`)}}})
	}
	var snapshots, events, shared int64
	if err := s.db.QueryRow("SELECT sum(length(snapshot)) FROM runs").Scan(&snapshots); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow("SELECT coalesce(sum(length(state_after)),0) FROM events").Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow("SELECT count(*) FROM pinned_bytes").Scan(&shared); err != nil {
		t.Fatal(err)
	}
	if shared != 1 {
		t.Fatalf("the repeated bytes were not shared: %d rows", shared)
	}
	if snapshots > 4<<10 || events > 8<<10 {
		t.Fatalf("snapshots still carry their pinned bytes: runs=%d events=%d", snapshots, events)
	}
	view, err := s.Read(ctx, "run-a", 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if string(view.Snapshot.Data) != string(state(8)) {
		t.Fatalf("the restored snapshot differs from what was written: %s", view.Snapshot.Data)
	}
	for _, event := range view.Events {
		if event.StateAfter != nil && !bytes.Contains(event.StateAfter, []byte(pinned)) {
			t.Fatal("a recorded state was returned without its pinned bytes")
		}
	}
	if err := s.Verify(ctx); err != nil {
		t.Fatalf("verification failed over shared bytes: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	// Losing the shared bytes is an integrity failure, not a silent hole.
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(dir, "state.sqlite3")+"?mode=rw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("DELETE FROM pinned_bytes"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenStore(dir, storeTestOptions); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("a snapshot with missing pinned bytes was accepted: %v", err)
	}
}

// Reading the current population is a read of the runs table, and a historical
// cut uses the partial index instead of scanning every event.
func TestReadAllPlansAvoidFullEventScans(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	applyChange(t, s, storeCommand("create", "run-a", 0), storeChange(`{"value":1}`))
	applyChange(t, s, storeCommand("update", "run-a", 1), storeChange(`{"value":2}`))
	plan := func(query string, arguments ...any) string {
		rows, err := s.db.QueryContext(ctx, "EXPLAIN QUERY PLAN "+query, arguments...)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		text := ""
		for rows.Next() {
			var id, parent, notUsed int
			var detail string
			if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
				t.Fatal(err)
			}
			text += detail + "\n"
		}
		return text
	}
	current := plan("SELECT run_id,version,event_seq,snapshot,snapshot_digest,snapshot_packed FROM runs ORDER BY run_id LIMIT ?", 10)
	if strings.Contains(current, "SCAN events") {
		t.Fatalf("the current population still reads the journal: %s", current)
	}
	historical := plan(`SELECT e.run_id,e.run_version,e.seq,e.state_after,e.state_digest,e.state_packed
FROM events e WHERE e.state_after IS NOT NULL AND e.cut<=? AND NOT EXISTS
(SELECT 1 FROM events n WHERE n.run_id=e.run_id AND n.state_after IS NOT NULL AND n.seq>e.seq AND n.cut<=?)
ORDER BY e.run_id LIMIT ?`, 1, 1, 10)
	if !strings.Contains(historical, "events_state") {
		t.Fatalf("a historical cut does not use the recorded-state index: %s", historical)
	}
	// Both answers still agree with what the store returns.
	runs, _, err := s.ReadAll(ctx, 10)
	if err != nil || len(runs) != 1 || string(runs[0].Data) != `{"value":2}` {
		t.Fatalf("current population read: %v %+v", err, runs)
	}
	historicalRuns, _, err := s.ReadAllAt(ctx, 1, 10)
	if err != nil || len(historicalRuns) != 1 || string(historicalRuns[0].Data) != `{"value":1}` {
		t.Fatalf("historical population read: %v %+v", err, historicalRuns)
	}
}

// A refusal code, an event type and an authority key are read by other programs
// and printed to people, so each has a shape. Every type this build declares
// has to satisfy the one it belongs to.
func TestRecordedNamesHaveAGrammar(t *testing.T) {
	for _, code := range []string{"not_found", "capacity_conflict", "unsupported_storage_version", "a"} {
		if !validCode(code) {
			t.Fatalf("a documented refusal code was refused: %s", code)
		}
	}
	for _, code := range []string{"", "Not_Found", "not found", "attempt.settled", "a" + strings.Repeat("b", 64), "9lives"} {
		if validCode(code) {
			t.Fatalf("a code outside the grammar was accepted: %q", code)
		}
	}
	for _, typ := range []string{"run.created", "attempt.settled", "state.changed", "diagnostic.recorded"} {
		if !validEventType(typ) {
			t.Fatalf("a recorded event type was refused: %s", typ)
		}
	}
	for _, typ := range []string{"", "Run.Created", "run..created", "run.", ".created", "run created"} {
		if validEventType(typ) {
			t.Fatalf("an event type outside the grammar was accepted: %q", typ)
		}
	}
	for _, key := range []string{"control", "packages", "authority:controls"} {
		if !validAuthorityKey(key) {
			t.Fatalf("an authority key was refused: %s", key)
		}
	}
	for _, key := range []string{"", "Control", "authority::controls", "authority:", "authority controls"} {
		if validAuthorityKey(key) {
			t.Fatalf("an authority key outside the grammar was accepted: %q", key)
		}
	}
}

package local

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
	_ "github.com/mattn/go-sqlite3"
)

const (
	StorageVersion = 4
	// MaxSlotWaiters bounds the admission queue. A queue without a bound is a
	// second unbounded resource hiding behind a bounded one.
	MaxSlotWaiters = 64
	// SlotWaiterPatience is how many admission decisions a queued run may sit
	// out before it loses its place. The unit is admission decisions, not cuts:
	// a waiter must not be evicted by unrelated authority traffic, only by the
	// queue moving on without it. The store cannot tell a live waiter from an
	// abandoned one by looking at the row, so asking again is what holds a place.
	//
	// Patience must exceed the queue size. A full queue asking once each already
	// costs MaxSlotWaiters decisions, so a shorter patience would evict a
	// waiting run after a single round of other runs asking, and would make the
	// queue bound unreachable rather than merely generous.
	SlotWaiterPatience          = 2 * MaxSlotWaiters
	EventVersion                = 1
	applicationID               = 0x5052464c
	MaxCommandBytes             = 1 << 20
	MaxSnapshotBytes            = 16 << 20
	MaxReadRecords              = 1000
	MaxReceiptRecords           = 100000
	MaxSampleRecords            = 100000
	DefaultSoftLimitBytes int64 = 512 << 20
)

var (
	ErrCommandConflict = errors.New("command ID already has a different request")
	ErrIncompatible    = errors.New("unsupported storage or event version")
	// errQueueFull is internal: it is turned into a recorded rejection rather
	// than returned to a caller, so a full queue is a decision, not a failure.
	errQueueFull        = errors.New("admission queue is full")
	ErrNotFound         = errors.New("record not found")
	ErrReadOnly         = errors.New("store is read-only")
	ErrRecoveryRequired = errors.New("authority directory changed; read-only recovery required")
	ErrSampleLimit      = errors.New("diagnostic storage limit reached")
)

type CommandMode string

const (
	CommandCAS         CommandMode = "cas"
	CommandMonotonic   CommandMode = "monotonic"
	CommandPublication CommandMode = "publication"
	CommandGuarded     CommandMode = "guarded"
)

type StoreOptions struct {
	EventTypes     []string
	BusyTimeout    time.Duration
	ReadOnly       bool
	SoftLimitBytes int64
}

type StoreInfo struct {
	AuthorityID    string `json:"authority_id"`
	Epoch          int64  `json:"epoch"`
	StorageVersion int    `json:"storage_version"`
	SQLiteVersion  string `json:"sqlite_version"`
	JournalMode    string `json:"journal_mode"`
	Synchronous    int    `json:"synchronous"`
	ForeignKeys    bool   `json:"foreign_keys"`
	ReadOnly       bool   `json:"read_only"`
}

type Store struct {
	db             *sql.DB
	info           StoreInfo
	eventTypes     map[string]bool
	softLimitBytes int64
}

type Snapshot struct {
	RunID    string          `json:"run_id"`
	Version  int64           `json:"run_version"`
	EventSeq int64           `json:"event_seq"`
	Data     json.RawMessage `json:"data"`
}

// AuthoritySnapshot is control-plane state that does not belong to a Run.
// It exists so installation/project controls never need a synthetic Run.
type AuthoritySnapshot struct {
	Key     string          `json:"key"`
	Version int64           `json:"version"`
	Cut     int64           `json:"cut"`
	Data    json.RawMessage `json:"data"`
}

type AuthorityCommand struct {
	ID              string
	Actor           string
	Key             string
	Payload         json.RawMessage
	ExpectedVersion *int64
}

type AuthorityChange struct {
	Data   json.RawMessage
	Result json.RawMessage
	// SetCapacity changes how many attempts this authority admits at once, in
	// the same transaction that records the decision, so the recorded decision
	// and the capacity it describes can never disagree.
	SetCapacity *int64
}

type AuthorityReceipt struct {
	ID        string          `json:"command_id"`
	Actor     string          `json:"actor"`
	Key       string          `json:"key"`
	Digest    string          `json:"request_digest"`
	Version   int64           `json:"version"`
	Cut       int64           `json:"cut"`
	Result    json.RawMessage `json:"result"`
	Rejection *Rejection      `json:"rejection,omitempty"`
}

type AuthorityApplyResult struct {
	Receipt             AuthorityReceipt
	Duplicate           bool
	LockWait            time.Duration
	TransactionDuration time.Duration
}

// Actor is an authenticated principal, never a worker-supplied identity. Runtime
// must check current receipt read access before every call, including duplicates.
// Publication bypasses run CAS only: its pure transform must enforce own scoped
// CAS, owner, generations and terminal rules. Monotonic is only for restriction.
// Guarded is internal: runtime validates the owning Attempt or exact control
// epoch in the transform; a caller must never select this mode through the wire.
type Command struct {
	ID              string
	Actor           string
	RunID           string
	Payload         json.RawMessage
	ExpectedVersion *int64
	Mode            CommandMode
	// Control binds this command to the exact authority control version its
	// caller evaluated. It is nil for commands that grant no new work.
	Control *ControlPin
	// Pins keep additional authority decisions current at the same commit. They
	// are distinct from Control because they are read-only guards, not authority
	// state a command may mutate.
	Pins []ControlPin
	// ControlMutation changes the pinned authority state in the same SQLite
	// transaction as this Run command. It is runtime-owned code, not wire data;
	// it must be pure just like the Run transform. Receipt-only retries never
	// invoke it, so a consumed decision cannot be spent twice.
	ControlMutation func(AuthoritySnapshot) (json.RawMessage, error)
	// Samples supplies this command's own telemetry. The store calls it once,
	// immediately before commit, with the timings it measured, so recording a
	// command costs no second write transaction. Telemetry never fails a
	// command: a batch that no longer fits the diagnostic allowance is dropped.
	Samples CommandTelemetry
}

// CommandTelemetry builds the samples for one applied command from the timings
// measured inside its transaction.
type CommandTelemetry func(SampleTimings) []SampleInput

// SampleTimings are the facts only the store holds when a command commits.
// TransactionDuration is measured up to the sample write, so it excludes the
// commit itself; that is deliberate, since the samples are part of it.
type SampleTimings struct {
	LockWait            time.Duration
	TransactionDuration time.Duration
	AllocatedBytes      int64
	Version             int64
	Rejected            bool
}

// LinkedRunCommand creates a new Run while checking the exact version of the
// source Run in the same transaction. It is intentionally narrower than a
// general multi-Run reducer: a linked Run never rewrites its source history.
type LinkedRunCommand struct {
	ID              string
	Actor           string
	SourceRunID     string
	NewRunID        string
	Payload         json.RawMessage
	ExpectedVersion int64
	Pins            []ControlPin
}

// ControlPin makes a stop and an admission prepared before it mutually
// exclusive. The admission is evaluated outside the transaction, so without the
// pin a control command committing in between would be silently overtaken.
type ControlPin struct {
	Key     string `json:"key"`
	Version int64  `json:"version"`
}

type EventInput struct {
	Type    string          `json:"type"`
	Version int             `json:"schema_version"`
	Data    json.RawMessage `json:"data"`
}

type Event struct {
	EventInput
	RunID      string `json:"run_id"`
	Seq        int64  `json:"seq"`
	RunVersion int64  `json:"run_version"`
	Cut        int64  `json:"cut"`
	CommandID  string `json:"command_id"`
	Actor      string `json:"actor"`
	Digest     string `json:"digest"`
	// A final event of each command carries its complete projection for replay.
	// The journal remains authoritative; runs is a checked materialized view.
	StateAfter json.RawMessage `json:"state_after,omitempty"`
}

type Change struct {
	Data        json.RawMessage
	Events      []EventInput
	Result      json.RawMessage
	AcquireSlot string
	ReleaseSlot string
	// AdvanceRunVersion distinguishes a publication that also commits a
	// workflow assignment from a scoped hook update. Only publication mode may
	// request it; ordinary CAS mutations advance unconditionally.
	AdvanceRunVersion bool
	// ReceiptOnly acknowledges an already accepted logical publication through
	// a new command identity. It cannot mutate state, advance time or own slots.
	ReceiptOnly bool
	// Set by trusted runtime for new work and optional publications, including
	// logical receipt-only retries. Control/settlement retain priority above the
	// soft logical-page budget; this is not an OS free-space guarantee.
	RequireStorageBudget bool
}

type Rejection struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (r *Rejection) Error() string      { return r.Code + ": " + r.Message }
func Reject(code, message string) error { return &Rejection{Code: code, Message: message} }

type Receipt struct {
	ID        string          `json:"command_id"`
	Actor     string          `json:"actor"`
	RunID     string          `json:"run_id"`
	Digest    string          `json:"request_digest"`
	Version   int64           `json:"run_version"`
	EventSeq  int64           `json:"event_seq"`
	Cut       int64           `json:"cut"`
	Result    json.RawMessage `json:"result"`
	Rejection *Rejection      `json:"rejection,omitempty"`
}

type ApplyResult struct {
	Receipt             Receipt
	Duplicate           bool
	LockWait            time.Duration
	TransactionDuration time.Duration
	// SamplesRecorded reports that this command wrote its own telemetry in its
	// own transaction. A caller only falls back to a separate write for the
	// paths that never reached one, such as an exact repeat.
	SamplesRecorded bool
}

type ReadView struct {
	Snapshot Snapshot `json:"snapshot"`
	Cut      int64    `json:"cut"`
	Events   []Event  `json:"events"`
	More     bool     `json:"more"`
}

// OpenStore never initializes over a foreign database. A moved authority is
// available only for reads; importing a backup must not resume its dispatches.
// SQLite options are verified against the actual linked library at open.
func OpenStore(dir string, opts StoreOptions) (*Store, error) {
	if opts.SoftLimitBytes == 0 {
		opts.SoftLimitBytes = DefaultSoftLimitBytes
	}
	if opts.SoftLimitBytes < 64<<10 || opts.SoftLimitBytes > 64<<30 {
		return nil, errors.New("SQLite soft budget must be within 64 KiB..64 GiB")
	}
	if opts.BusyTimeout == 0 {
		// A concurrent writer holds the write lock for as long as its own
		// transaction runs. Waiting is cheaper than failing an observation that
		// has nowhere else to be recorded.
		opts.BusyTimeout = 3 * time.Second
	}
	if opts.BusyTimeout < time.Millisecond || opts.BusyTimeout > 5*time.Second || len(opts.EventTypes) == 0 {
		return nil, errors.New("event types and a busy timeout within 1ms..5s are required")
	}
	var root *os.Root
	var err error
	if opts.ReadOnly {
		root, err = existingRoot(dir)
	} else {
		root, err = privateRoot(dir)
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	canonical, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return nil, err
	}
	const filename = "state.sqlite3"
	if st, err := root.Lstat(filename); err == nil {
		if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
			return nil, fmt.Errorf("authority database is not a private regular file: %w", ErrUnsafePath)
		}
	} else if errors.Is(err, os.ErrNotExist) && !opts.ReadOnly {
		f, err := root.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
		if err != nil && !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if f != nil {
			err = f.Sync()
			_ = f.Close()
			if err != nil {
				return nil, err
			}
		}
		if err := syncRoot(root, "."); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	// Refuse malicious sidecars before SQLite can follow them.
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if st, err := root.Lstat(filename + suffix); err == nil && !st.Mode().IsRegular() {
			return nil, ErrUnsafePath
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	u := url.URL{Scheme: "file", Path: filepath.Join(canonical, filename)}
	q := u.Query()
	q.Set("mode", "rw")
	if opts.ReadOnly {
		q.Set("mode", "ro")
	}
	q.Set("_busy_timeout", strconv.FormatInt(opts.BusyTimeout.Milliseconds(), 10))
	q.Set("_foreign_keys", "on")
	q.Set("_synchronous", "FULL")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite3", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(8)
	s := &Store{db: db, eventTypes: make(map[string]bool), softLimitBytes: opts.SoftLimitBytes}
	for _, typ := range opts.EventTypes {
		if !validIdentity(typ) {
			_ = db.Close()
			return nil, errors.New("invalid supported event type")
		}
		s.eventTypes[typ] = true
	}
	if err := s.open(canonical, opts.ReadOnly); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error    { return s.db.Close() }
func (s *Store) Info() StoreInfo { return s.info }

func (s *Store) open(dir string, readOnly bool) error {
	ctx := context.Background()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	var version, appID, count int
	if err := conn.QueryRowContext(ctx, "SELECT sqlite_version()").Scan(&s.info.SQLiteVersion); err != nil {
		return err
	}
	if !patchedSQLite(s.info.SQLiteVersion) {
		return fmt.Errorf("%w: SQLite WAL build %s is not qualified", ErrIncompatible, s.info.SQLiteVersion)
	}
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if err := conn.QueryRowContext(ctx, "PRAGMA application_id").Scan(&appID); err != nil {
		return err
	}
	if version == 0 && appID == 0 {
		if readOnly {
			return ErrIncompatible
		}
		if err := conn.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'").Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("%w: refusing a foreign database", ErrIncompatible)
		}
		if err := s.initialize(ctx, conn, dir); err != nil {
			return err
		}
		version, appID = StorageVersion, applicationID
	}
	if appID != applicationID || version < 1 || version > StorageVersion {
		return ErrIncompatible
	}
	if version < StorageVersion && !readOnly {
		if err := s.migrate(ctx, conn, version); err != nil {
			return err
		}
		version = StorageVersion
	}
	var savedDir string
	if err := conn.QueryRowContext(ctx, "SELECT id,epoch,state_directory FROM authority WHERE singleton=1").Scan(&s.info.AuthorityID, &s.info.Epoch, &savedDir); err != nil {
		return err
	}
	if savedDir != dir && !readOnly {
		return ErrRecoveryRequired
	}
	// verify selects its checks by storage version, so the version must be known
	// before it runs. Otherwise a v2 database is verified under v1 rules and its
	// authority commands are excluded from the shared cut.
	s.info.StorageVersion = version
	if err := s.verify(ctx, conn); err != nil {
		return err
	}
	if !readOnly {
		if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode=WAL").Scan(&s.info.JournalMode); err != nil {
			return err
		}
	} else if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&s.info.JournalMode); err != nil {
		return err
	}
	var fk int
	if err := conn.QueryRowContext(ctx, "PRAGMA synchronous").Scan(&s.info.Synchronous); err != nil {
		return err
	}
	if err := conn.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil {
		return err
	}
	if s.info.JournalMode != "wal" || s.info.Synchronous != 2 || fk != 1 {
		return errors.New("SQLite requires verified WAL, synchronous=FULL and foreign_keys=ON")
	}
	s.info.ForeignKeys, s.info.StorageVersion, s.info.ReadOnly = true, version, readOnly
	return nil
}

func (s *Store) initialize(ctx context.Context, conn *sql.Conn, dir string) error {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	// Another opener may have completed initialization while we waited.
	var version int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version == StorageVersion {
		return nil
	}
	if version != 0 {
		return ErrIncompatible
	}
	const schema = `
CREATE TABLE authority(singleton INTEGER PRIMARY KEY CHECK(singleton=1),id TEXT NOT NULL UNIQUE,epoch INTEGER NOT NULL CHECK(epoch>0),state_directory TEXT NOT NULL,cut INTEGER NOT NULL CHECK(cut>=0),slot_id TEXT NOT NULL DEFAULT '',slot_run TEXT NOT NULL DEFAULT '',slot_capacity INTEGER NOT NULL DEFAULT 1 CHECK(slot_capacity>=1),admission_seq INTEGER NOT NULL DEFAULT 0);
CREATE TABLE slots(slot_id TEXT PRIMARY KEY,run_id TEXT NOT NULL);
CREATE TABLE slot_waiters(run_id TEXT PRIMARY KEY,since_seq INTEGER NOT NULL,seen_seq INTEGER NOT NULL);
CREATE TABLE runs(run_id TEXT PRIMARY KEY,version INTEGER NOT NULL CHECK(version>0),event_seq INTEGER NOT NULL CHECK(event_seq>0),snapshot BLOB NOT NULL,snapshot_digest TEXT NOT NULL);
CREATE TABLE commands(actor TEXT NOT NULL,command_id TEXT NOT NULL,run_id TEXT NOT NULL,digest TEXT NOT NULL,cut INTEGER NOT NULL UNIQUE,receipt BLOB NOT NULL,receipt_digest TEXT NOT NULL,PRIMARY KEY(actor,command_id));
CREATE TABLE events(run_id TEXT NOT NULL REFERENCES runs(run_id),seq INTEGER NOT NULL CHECK(seq>0),run_version INTEGER NOT NULL CHECK(run_version>0),cut INTEGER NOT NULL REFERENCES commands(cut) DEFERRABLE INITIALLY DEFERRED,type TEXT NOT NULL,schema_version INTEGER NOT NULL,actor TEXT NOT NULL,command_id TEXT NOT NULL,data BLOB NOT NULL,digest TEXT NOT NULL,state_after BLOB,state_digest TEXT,PRIMARY KEY(run_id,seq));
CREATE INDEX events_cut ON events(cut);
CREATE TABLE samples(seq INTEGER PRIMARY KEY AUTOINCREMENT,id TEXT NOT NULL UNIQUE,run_id TEXT NOT NULL,cut INTEGER NOT NULL,data BLOB NOT NULL,digest TEXT NOT NULL);
CREATE INDEX samples_cut ON samples(cut,seq);
CREATE TABLE authority_states(state_key TEXT PRIMARY KEY,version INTEGER NOT NULL CHECK(version>0),cut INTEGER NOT NULL UNIQUE,data BLOB NOT NULL,digest TEXT NOT NULL);
CREATE TABLE authority_commands(actor TEXT NOT NULL,command_id TEXT NOT NULL,state_key TEXT NOT NULL,digest TEXT NOT NULL,cut INTEGER NOT NULL UNIQUE,receipt BLOB NOT NULL,receipt_digest TEXT NOT NULL,PRIMARY KEY(actor,command_id));
PRAGMA application_id=1347569228;
PRAGMA user_version=4;`
	if _, err := conn.ExecContext(ctx, schema); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "INSERT INTO authority(singleton,id,epoch,state_directory,cut) VALUES(1,?,1,?,0)", "authority-"+hex.EncodeToString(id[:]), dir); err != nil {
		return err
	}
	_, err := conn.ExecContext(ctx, "COMMIT")
	return err
}

func (s *Store) migrate(ctx context.Context, conn *sql.Conn, version int) error {
	if version == 3 {
		return s.migrateWaiters(ctx, conn)
	}
	if version == 2 {
		return s.migrateSlots(ctx, conn)
	}
	if version != 1 {
		return ErrIncompatible
	}
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	var current int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return err
	}
	if current == StorageVersion {
		return nil
	}
	if current != version {
		return ErrIncompatible
	}
	const schema = `
CREATE TABLE authority_states(state_key TEXT PRIMARY KEY,version INTEGER NOT NULL CHECK(version>0),cut INTEGER NOT NULL UNIQUE,data BLOB NOT NULL,digest TEXT NOT NULL);
CREATE TABLE authority_commands(actor TEXT NOT NULL,command_id TEXT NOT NULL,state_key TEXT NOT NULL,digest TEXT NOT NULL,cut INTEGER NOT NULL UNIQUE,receipt BLOB NOT NULL,receipt_digest TEXT NOT NULL,PRIMARY KEY(actor,command_id));
PRAGMA user_version=2;`
	if _, err := conn.ExecContext(ctx, schema); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	return s.migrateSlots(ctx, conn)
}

// migrateSlots turns the single admission slot into a bounded set. The existing
// occupant becomes the one held slot and the capacity stays one, so an upgraded
// installation admits exactly what it admitted before.
func (s *Store) migrateSlots(ctx context.Context, conn *sql.Conn) error {
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	var current int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return err
	}
	if current == StorageVersion {
		return nil
	}
	if current != 2 {
		return ErrIncompatible
	}
	if _, err := conn.ExecContext(ctx, `
CREATE TABLE slots(slot_id TEXT PRIMARY KEY,run_id TEXT NOT NULL);
ALTER TABLE authority ADD COLUMN slot_capacity INTEGER NOT NULL DEFAULT 1;`); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "INSERT INTO slots(slot_id,run_id) SELECT slot_id,slot_run FROM authority WHERE singleton=1 AND slot_id<>''"); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA user_version=3"); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	return s.migrateWaiters(ctx, conn)
}

// migrateWaiters adds the admission queue. An upgraded installation starts with
// an empty queue: nothing was waiting before, because there was nowhere to wait.
func (s *Store) migrateWaiters(ctx context.Context, conn *sql.Conn) error {
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer conn.ExecContext(context.Background(), "ROLLBACK")
	var current int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return err
	}
	if current == StorageVersion {
		return nil
	}
	if current != 3 {
		return ErrIncompatible
	}
	if _, err := conn.ExecContext(ctx, `
CREATE TABLE slot_waiters(run_id TEXT PRIMARY KEY,since_seq INTEGER NOT NULL,seen_seq INTEGER NOT NULL);
ALTER TABLE authority ADD COLUMN admission_seq INTEGER NOT NULL DEFAULT 0;`); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA user_version=4"); err != nil {
		return err
	}
	_, err := conn.ExecContext(ctx, "COMMIT")
	return err
}

// admissionTurn decides whether this run may take a free slot now. The declared
// policy is longest waiting first, with the run identity breaking ties so the
// order is reproducible: a run that has been waiting longer cannot be overtaken
// by one that merely asked at a luckier moment, which is what keeps a busy
// authority from starving anyone.
//
// The store cannot tell a live waiter from an abandoned one by looking at its
// row, so a place in the queue is held by asking again: a waiter that has not
// asked within SlotWaiterPatience cuts stops counting and is removed. Returning
// nil means the caller may acquire; anything else is the refusal to record.
func (s *Store) admissionTurn(ctx context.Context, conn *sql.Conn, runID string, full bool) (*Rejection, error) {
	var seq int64
	if err := conn.QueryRowContext(ctx, "SELECT admission_seq FROM authority WHERE singleton=1").Scan(&seq); err != nil {
		return nil, err
	}
	seq++
	if _, err := conn.ExecContext(ctx, "UPDATE authority SET admission_seq=? WHERE singleton=1", seq); err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, "DELETE FROM slot_waiters WHERE seen_seq < ?", seq-SlotWaiterPatience); err != nil {
		return nil, err
	}
	var since int64
	err := conn.QueryRowContext(ctx, "SELECT since_seq FROM slot_waiters WHERE run_id=?", runID).Scan(&since)
	queued := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	enqueue := func() error {
		if !queued {
			var waiting int64
			if err := conn.QueryRowContext(ctx, "SELECT count(*) FROM slot_waiters").Scan(&waiting); err != nil {
				return err
			}
			if waiting >= MaxSlotWaiters {
				return errQueueFull
			}
		}
		_, err := conn.ExecContext(ctx, "INSERT INTO slot_waiters(run_id,since_seq,seen_seq) VALUES(?,?,?) ON CONFLICT(run_id) DO UPDATE SET seen_seq=excluded.seen_seq", runID, seq, seq)
		return err
	}
	if full {
		if err := enqueue(); err != nil {
			if errors.Is(err, errQueueFull) {
				return &Rejection{Code: "admission_queue_full", Message: "the admission queue is full"}, nil
			}
			return nil, err
		}
		return &Rejection{Code: "capacity_conflict", Message: "the authority has no free admission slot"}, nil
	}
	var aheadRun string
	var aheadSince int64
	err = conn.QueryRowContext(ctx, "SELECT run_id,since_seq FROM slot_waiters WHERE run_id<>? ORDER BY since_seq,run_id LIMIT 1", runID).Scan(&aheadRun, &aheadSince)
	if errors.Is(err, sql.ErrNoRows) {
		_, err := conn.ExecContext(ctx, "DELETE FROM slot_waiters WHERE run_id=?", runID)
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	// A run already in the queue keeps its own place; one that never queued is
	// behind everyone who did, so it cannot jump a free slot.
	mine := seq
	if queued {
		mine = since
	}
	if aheadSince < mine || aheadSince == mine && aheadRun < runID {
		if err := enqueue(); err != nil {
			if errors.Is(err, errQueueFull) {
				return &Rejection{Code: "admission_queue_full", Message: "the admission queue is full"}, nil
			}
			return nil, err
		}
		return &Rejection{Code: "admission_deferred", Message: "a run that has waited longer holds the next free slot: " + aheadRun}, nil
	}
	_, err = conn.ExecContext(ctx, "DELETE FROM slot_waiters WHERE run_id=?", runID)
	return nil, err
}

// Apply serializes command decisions using BEGIN IMMEDIATE. transform must be
// pure: no filesystem, network, process operations or unbounded work. An accepted
// mutation and its complete receipt are committed together. A typed Rejection is
// committed without state changes; other errors roll back and receive no ack.
func (s *Store) Apply(ctx context.Context, cmd Command, transform func(Snapshot) (Change, error)) (out ApplyResult, err error) {
	if s.info.ReadOnly {
		return out, ErrReadOnly
	}
	digest, err := commandDigest(cmd)
	if err != nil {
		return out, err
	}
	started := time.Now()
	conn, err := s.begin(ctx, true)
	out.LockWait = time.Since(started)
	if err != nil {
		return out, err
	}
	txStarted := time.Now()
	defer func() { out.TransactionDuration = time.Since(txStarted); _ = rollbackClose(conn) }()
	var oldDigest, receiptDigest string
	var saved []byte
	err = conn.QueryRowContext(ctx, "SELECT digest,receipt,receipt_digest FROM commands WHERE actor=? AND command_id=?", cmd.Actor, cmd.ID).Scan(&oldDigest, &saved, &receiptDigest)
	if err == nil {
		if oldDigest != digest {
			return out, ErrCommandConflict
		}
		if digestBytes(saved) != receiptDigest || json.Unmarshal(saved, &out.Receipt) != nil {
			return out, ErrIntegrity
		}
		out.Duplicate = true
		return out, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	state, err := loadSnapshot(ctx, conn, cmd.RunID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return out, err
	}
	out.Receipt = Receipt{ID: cmd.ID, Actor: cmd.Actor, RunID: cmd.RunID, Digest: digest, Version: state.Version, EventSeq: state.EventSeq, Result: json.RawMessage("null")}
	var rejection *Rejection
	if cmd.Mode == CommandCAS && *cmd.ExpectedVersion != state.Version {
		rejection = &Rejection{Code: "version_conflict", Message: "expected run version differs from current version"}
	} else if cmd.Mode != CommandCAS && state.Version == 0 {
		rejection = &Rejection{Code: "not_found", Message: "run does not exist"}
	}
	var control AuthoritySnapshot
	if rejection == nil && cmd.Control != nil {
		control, err = loadAuthoritySnapshot(ctx, conn, cmd.Control.Key)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return out, err
		}
		if control.Version != cmd.Control.Version {
			rejection = &Rejection{Code: "control_conflict", Message: "authority control state changed after this admission was evaluated"}
		}
	}
	if rejection == nil {
		pins, err := commandPins(cmd.Pins, cmd.Control)
		if err != nil {
			return out, err
		}
		for _, pin := range pins {
			authority, err := loadAuthoritySnapshot(ctx, conn, pin.Key)
			if err != nil && !errors.Is(err, ErrNotFound) {
				return out, err
			}
			if authority.Version != pin.Version {
				rejection = &Rejection{Code: "authority_state_conflict", Message: "authority state changed after this admission was evaluated"}
				break
			}
		}
	}
	var change Change
	if rejection == nil {
		if transform == nil {
			return out, errors.New("command transform is required")
		}
		change, err = applyTransform(transform, state)
		if err != nil && !errors.As(err, &rejection) {
			return out, err
		}
	}
	var controlData json.RawMessage
	if rejection == nil && cmd.ControlMutation != nil && !change.ReceiptOnly {
		if cmd.Control == nil {
			return out, errors.New("control mutation requires an authority control pin")
		}
		controlData, err = applyControlMutation(cmd.ControlMutation, control)
		if err != nil && !errors.As(err, &rejection) {
			return out, err
		}
		if rejection == nil && (len(controlData) == 0 || len(controlData) > MaxSnapshotBytes || !json.Valid(controlData)) {
			return out, errors.New("control mutation requires bounded valid authority state")
		}
	}
	var cut, capacity int64
	if err := conn.QueryRowContext(ctx, "SELECT cut,slot_capacity FROM authority WHERE singleton=1").Scan(&cut, &capacity); err != nil {
		return out, err
	}
	release, acquire := "", ""
	if rejection == nil {
		if err := s.validateChange(change, cmd.Mode); err != nil {
			return out, err
		}
		if change.ReleaseSlot != "" {
			var owner string
			err := conn.QueryRowContext(ctx, "SELECT run_id FROM slots WHERE slot_id=?", change.ReleaseSlot).Scan(&owner)
			if errors.Is(err, sql.ErrNoRows) || err == nil && owner != cmd.RunID {
				rejection = &Rejection{Code: "slot_conflict", Message: "release does not own the occupied slot"}
			} else if err != nil {
				return out, err
			} else {
				release = change.ReleaseSlot
			}
		}
		if rejection == nil && change.AcquireSlot != "" {
			var owner string
			err := conn.QueryRowContext(ctx, "SELECT run_id FROM slots WHERE slot_id=?", change.AcquireSlot).Scan(&owner)
			switch {
			case err == nil && owner != cmd.RunID:
				rejection = &Rejection{Code: "slot_conflict", Message: "another run already holds this slot"}
			case err == nil:
				// An exact re-acquire by the same run is inert, not a second hold.
			case errors.Is(err, sql.ErrNoRows):
				var held int64
				if err := conn.QueryRowContext(ctx, "SELECT count(*) FROM slots WHERE slot_id<>?", release).Scan(&held); err != nil {
					return out, err
				}
				decision, err := s.admissionTurn(ctx, conn, cmd.RunID, held >= capacity)
				if err != nil {
					return out, err
				}
				if decision != nil {
					rejection = decision
				} else {
					acquire = change.AcquireSlot
				}
			default:
				return out, err
			}
		}
	}
	if rejection != nil && (!validIdentity(rejection.Code) || len(rejection.Message) > 4096) {
		return out, errors.New("invalid rejection contract")
	}
	limited := rejection == nil && change.RequireStorageBudget
	if limited {
		usage, err := storageUsage(ctx, conn, s.softLimitBytes)
		if err != nil {
			return out, err
		}
		if usage.AllocatedBytes >= s.softLimitBytes {
			rejection = storageBudgetRejection()
			limited = false
		} else if _, err := conn.ExecContext(ctx, "SAVEPOINT storage_budget"); err != nil {
			return out, err
		}
	}
	cut++
	out.Receipt.Cut, out.Receipt.Rejection = cut, rejection
	if rejection == nil && change.Result != nil {
		out.Receipt.Result = change.Result
	}
	if rejection == nil && !change.ReceiptOnly {
		if cmd.Mode != CommandPublication || change.AdvanceRunVersion {
			out.Receipt.Version++
		}
		out.Receipt.EventSeq += int64(len(change.Events))
		if _, err := conn.ExecContext(ctx, `INSERT INTO runs(run_id,version,event_seq,snapshot,snapshot_digest) VALUES(?,?,?,?,?)
ON CONFLICT(run_id) DO UPDATE SET version=excluded.version,event_seq=excluded.event_seq,snapshot=excluded.snapshot,snapshot_digest=excluded.snapshot_digest`, cmd.RunID, out.Receipt.Version, out.Receipt.EventSeq, []byte(change.Data), digestBytes(change.Data)); err != nil {
			return out, err
		}
		if len(change.Events) > 0 {
			// One command commonly writes several events; preparing the insert
			// once keeps the parse out of the loop under the writer lock.
			insert, err := conn.PrepareContext(ctx, "INSERT INTO events(run_id,seq,run_version,cut,type,schema_version,actor,command_id,data,digest,state_after,state_digest) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)")
			if err != nil {
				return out, err
			}
			defer insert.Close()
			for i, event := range change.Events {
				if event.Version == 0 {
					event.Version = EventVersion
				}
				var after any
				var afterDigest any
				if i == len(change.Events)-1 {
					after = []byte(change.Data)
					afterDigest = digestBytes(change.Data)
				}
				if _, err := insert.ExecContext(ctx, cmd.RunID, state.EventSeq+int64(i)+1, out.Receipt.Version, cut, event.Type, event.Version, cmd.Actor, cmd.ID, []byte(event.Data), digestBytes(event.Data), after, afterDigest); err != nil {
					return out, err
				}
			}
		}
		if release != "" {
			if _, err := conn.ExecContext(ctx, "DELETE FROM slots WHERE slot_id=?", release); err != nil {
				return out, err
			}
		}
		if acquire != "" {
			if _, err := conn.ExecContext(ctx, "INSERT INTO slots(slot_id,run_id) VALUES(?,?)", acquire, cmd.RunID); err != nil {
				return out, err
			}
		}
		if controlData != nil {
			if _, err := conn.ExecContext(ctx, `INSERT INTO authority_states(state_key,version,cut,data,digest) VALUES(?,?,?,?,?) ON CONFLICT(state_key) DO UPDATE SET version=excluded.version,cut=excluded.cut,data=excluded.data,digest=excluded.digest`, control.Key, control.Version+1, cut, []byte(controlData), digestBytes(controlData)); err != nil {
				return out, err
			}
		}
	}
	if err := insertReceipt(ctx, conn, out.Receipt); err != nil {
		return out, err
	}
	if _, err := conn.ExecContext(ctx, "UPDATE authority SET cut=? WHERE singleton=1", cut); err != nil {
		return out, err
	}
	if limited {
		usage, err := storageUsage(ctx, conn, s.softLimitBytes)
		if err != nil {
			return out, err
		}
		if usage.AllocatedBytes > s.softLimitBytes {
			if _, err := conn.ExecContext(ctx, "ROLLBACK TO storage_budget"); err != nil {
				return out, err
			}
			out.Receipt.Version, out.Receipt.EventSeq = state.Version, state.EventSeq
			out.Receipt.Result = json.RawMessage("null")
			out.Receipt.Rejection = storageBudgetRejection()
			if err := insertReceipt(ctx, conn, out.Receipt); err != nil {
				return out, err
			}
			if _, err := conn.ExecContext(ctx, "UPDATE authority SET cut=? WHERE singleton=1", cut); err != nil {
				return out, err
			}
		}
		if _, err := conn.ExecContext(ctx, "RELEASE storage_budget"); err != nil {
			return out, err
		}
	}
	if err := recordCommandSamples(ctx, conn, s.softLimitBytes, cut, cmd.Samples, SampleTimings{
		LockWait: out.LockWait, TransactionDuration: time.Since(txStarted),
		Version: out.Receipt.Version, Rejected: out.Receipt.Rejection != nil,
	}); err != nil {
		return out, err
	} else if cmd.Samples != nil {
		out.SamplesRecorded = true
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return out, err
	}
	return out, nil
}

// recordCommandSamples writes a command's own telemetry at its own cut. The
// samples ride the command's transaction, so a failure to fit the diagnostic
// allowance drops them rather than losing the command they describe.
func recordCommandSamples(ctx context.Context, conn *sql.Conn, softLimitBytes, cut int64, telemetry CommandTelemetry, timings SampleTimings) error {
	if telemetry == nil {
		return nil
	}
	usage, err := storageUsage(ctx, conn, softLimitBytes)
	if err != nil {
		return err
	}
	timings.AllocatedBytes = usage.AllocatedBytes
	batch := telemetry(timings)
	if len(batch) == 0 {
		return nil
	}
	if err := validSampleBatch(batch); err != nil {
		return err
	}
	if _, err := insertSamples(ctx, conn, softLimitBytes, cut, batch); err != nil && !errors.Is(err, ErrSampleLimit) && !errors.Is(err, ErrCommandConflict) {
		return err
	}
	return nil
}

// CreateLinkedRun atomically checks a source Run and creates a distinct new
// Run. The source snapshot is read only: semantic rework must preserve its
// history instead of changing old inputs, approvals or effects in place.
func (s *Store) CreateLinkedRun(ctx context.Context, cmd LinkedRunCommand, transform func(Snapshot) (Change, error)) (out ApplyResult, err error) {
	if s.info.ReadOnly {
		return out, ErrReadOnly
	}
	digest, err := linkedRunCommandDigest(cmd)
	if err != nil {
		return out, err
	}
	started := time.Now()
	conn, err := s.begin(ctx, true)
	out.LockWait = time.Since(started)
	if err != nil {
		return out, err
	}
	txStarted := time.Now()
	defer func() { out.TransactionDuration = time.Since(txStarted); _ = rollbackClose(conn) }()
	var oldDigest, receiptDigest string
	var saved []byte
	err = conn.QueryRowContext(ctx, "SELECT digest,receipt,receipt_digest FROM commands WHERE actor=? AND command_id=?", cmd.Actor, cmd.ID).Scan(&oldDigest, &saved, &receiptDigest)
	if err == nil {
		if oldDigest != digest {
			return out, ErrCommandConflict
		}
		if digestBytes(saved) != receiptDigest || json.Unmarshal(saved, &out.Receipt) != nil {
			return out, ErrIntegrity
		}
		out.Duplicate = true
		return out, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	source, err := loadSnapshot(ctx, conn, cmd.SourceRunID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return out, ErrNotFound
		}
		return out, err
	}
	out.Receipt = Receipt{ID: cmd.ID, Actor: cmd.Actor, RunID: cmd.SourceRunID, Digest: digest, Version: source.Version, EventSeq: source.EventSeq, Result: json.RawMessage("null")}
	var rejection *Rejection
	if source.Version != cmd.ExpectedVersion {
		rejection = &Rejection{Code: "version_conflict", Message: "expected source run version differs from current version"}
	}
	for _, pin := range cmd.Pins {
		if rejection != nil {
			break
		}
		control, err := loadAuthoritySnapshot(ctx, conn, pin.Key)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return out, err
		}
		if control.Version != pin.Version {
			rejection = &Rejection{Code: "authority_state_conflict", Message: "authority state changed after this linked run was evaluated"}
		}
	}
	if rejection == nil {
		if _, err := loadSnapshot(ctx, conn, cmd.NewRunID); err == nil {
			rejection = &Rejection{Code: "run_exists", Message: "linked run already exists"}
		} else if !errors.Is(err, ErrNotFound) {
			return out, err
		}
	}
	var change Change
	if rejection == nil {
		if transform == nil {
			return out, errors.New("linked run transform is required")
		}
		change, err = applyTransform(transform, source)
		if err != nil && !errors.As(err, &rejection) {
			return out, err
		}
	}
	if rejection == nil {
		if err := s.validateChange(change, CommandCAS); err != nil {
			return out, err
		}
		if change.AdvanceRunVersion || change.ReceiptOnly || change.AcquireSlot != "" || change.ReleaseSlot != "" {
			return out, errors.New("linked run creation cannot alter an existing run or admission slots")
		}
	}
	if rejection != nil && (!validIdentity(rejection.Code) || len(rejection.Message) > 4096) {
		return out, errors.New("invalid rejection contract")
	}
	var cut int64
	if err := conn.QueryRowContext(ctx, "SELECT cut FROM authority WHERE singleton=1").Scan(&cut); err != nil {
		return out, err
	}
	limited := rejection == nil && change.RequireStorageBudget
	if limited {
		usage, err := storageUsage(ctx, conn, s.softLimitBytes)
		if err != nil {
			return out, err
		}
		if usage.AllocatedBytes >= s.softLimitBytes {
			rejection = storageBudgetRejection()
			limited = false
		} else if _, err := conn.ExecContext(ctx, "SAVEPOINT storage_budget"); err != nil {
			return out, err
		}
	}
	cut++
	out.Receipt.Cut, out.Receipt.Rejection = cut, rejection
	if rejection == nil && change.Result != nil {
		out.Receipt.Result = change.Result
	}
	if rejection == nil {
		version, sequence := int64(1), int64(len(change.Events))
		if _, err := conn.ExecContext(ctx, "INSERT INTO runs(run_id,version,event_seq,snapshot,snapshot_digest) VALUES(?,?,?,?,?)", cmd.NewRunID, version, sequence, []byte(change.Data), digestBytes(change.Data)); err != nil {
			return out, err
		}
		for i, event := range change.Events {
			if event.Version == 0 {
				event.Version = EventVersion
			}
			var after any = []byte(change.Data)
			var afterDigest any = digestBytes(change.Data)
			if _, err := conn.ExecContext(ctx, "INSERT INTO events(run_id,seq,run_version,cut,type,schema_version,actor,command_id,data,digest,state_after,state_digest) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)", cmd.NewRunID, int64(i)+1, version, cut, event.Type, event.Version, cmd.Actor, cmd.ID, []byte(event.Data), digestBytes(event.Data), after, afterDigest); err != nil {
				return out, err
			}
		}
	}
	if err := insertReceipt(ctx, conn, out.Receipt); err != nil {
		return out, err
	}
	if _, err := conn.ExecContext(ctx, "UPDATE authority SET cut=? WHERE singleton=1", cut); err != nil {
		return out, err
	}
	if limited {
		usage, err := storageUsage(ctx, conn, s.softLimitBytes)
		if err != nil {
			return out, err
		}
		if usage.AllocatedBytes > s.softLimitBytes {
			if _, err := conn.ExecContext(ctx, "ROLLBACK TO storage_budget"); err != nil {
				return out, err
			}
			out.Receipt.Result = json.RawMessage("null")
			out.Receipt.Rejection = storageBudgetRejection()
			if err := insertReceipt(ctx, conn, out.Receipt); err != nil {
				return out, err
			}
			if _, err := conn.ExecContext(ctx, "UPDATE authority SET cut=? WHERE singleton=1", cut); err != nil {
				return out, err
			}
		}
		if _, err := conn.ExecContext(ctx, "RELEASE storage_budget"); err != nil {
			return out, err
		}
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return out, err
	}
	return out, nil
}

// ApplyAuthority serializes a control-plane command beside ordinary Run
// commands. Its reducer is subject to the same no-I/O transaction rule.
func (s *Store) ApplyAuthority(ctx context.Context, cmd AuthorityCommand, transform func(AuthoritySnapshot) (AuthorityChange, error)) (out AuthorityApplyResult, err error) {
	if s.info.ReadOnly {
		return out, ErrReadOnly
	}
	digest, err := authorityCommandDigest(cmd)
	if err != nil {
		return out, err
	}
	started := time.Now()
	conn, err := s.begin(ctx, true)
	out.LockWait = time.Since(started)
	if err != nil {
		return out, err
	}
	txStarted := time.Now()
	defer func() { out.TransactionDuration = time.Since(txStarted); _ = rollbackClose(conn) }()
	var saved, receiptDigest, savedDigest []byte
	err = conn.QueryRowContext(ctx, "SELECT digest,receipt,receipt_digest FROM authority_commands WHERE actor=? AND command_id=?", cmd.Actor, cmd.ID).Scan(&savedDigest, &saved, &receiptDigest)
	if err == nil {
		var receipt AuthorityReceipt
		if string(savedDigest) != digest {
			return out, ErrCommandConflict
		}
		if digestBytes(saved) != string(receiptDigest) || json.Unmarshal(saved, &receipt) != nil {
			return out, ErrIntegrity
		}
		out.Receipt, out.Duplicate = receipt, true
		return out, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return out, err
	}
	state, err := loadAuthoritySnapshot(ctx, conn, cmd.Key)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return out, err
	}
	out.Receipt = AuthorityReceipt{ID: cmd.ID, Actor: cmd.Actor, Key: cmd.Key, Digest: digest, Version: state.Version, Result: json.RawMessage("null")}
	var rejection *Rejection
	if cmd.ExpectedVersion != nil && *cmd.ExpectedVersion != state.Version {
		rejection = &Rejection{Code: "version_conflict", Message: "expected authority control version differs from current version"}
	}
	var change AuthorityChange
	if rejection == nil {
		if transform == nil {
			return out, errors.New("authority transform is required")
		}
		change, err = applyAuthorityTransform(transform, state)
		if err != nil && !errors.As(err, &rejection) {
			return out, err
		}
	}
	if rejection == nil && (len(change.Data) == 0 || len(change.Data) > MaxSnapshotBytes || !json.Valid(change.Data) || change.Result != nil && (len(change.Result) > MaxCommandBytes || !json.Valid(change.Result))) {
		return out, errors.New("authority mutation requires bounded valid state and result")
	}
	var cut int64
	if err := conn.QueryRowContext(ctx, "SELECT cut FROM authority WHERE singleton=1").Scan(&cut); err != nil {
		return out, err
	}
	if rejection == nil && change.SetCapacity != nil {
		var held int64
		if err := conn.QueryRowContext(ctx, "SELECT count(*) FROM slots").Scan(&held); err != nil {
			return out, err
		}
		// Lowering capacity below what is already admitted would either evict
		// live work or leave the authority above its own limit. Refuse instead.
		if *change.SetCapacity < 1 {
			rejection = &Rejection{Code: "invalid_capacity", Message: "an authority admits at least one attempt at a time"}
		} else if *change.SetCapacity < held {
			rejection = &Rejection{Code: "capacity_conflict", Message: "capacity is below the attempts already admitted"}
		}
	}
	cut++
	out.Receipt.Cut, out.Receipt.Rejection = cut, rejection
	if rejection == nil {
		out.Receipt.Version++
		if change.Result != nil {
			out.Receipt.Result = change.Result
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO authority_states(state_key,version,cut,data,digest) VALUES(?,?,?,?,?) ON CONFLICT(state_key) DO UPDATE SET version=excluded.version,cut=excluded.cut,data=excluded.data,digest=excluded.digest`, cmd.Key, out.Receipt.Version, cut, []byte(change.Data), digestBytes(change.Data)); err != nil {
			return out, err
		}
		if change.SetCapacity != nil {
			if _, err := conn.ExecContext(ctx, "UPDATE authority SET slot_capacity=? WHERE singleton=1", *change.SetCapacity); err != nil {
				return out, err
			}
		}
	}
	encoded, err := json.Marshal(out.Receipt)
	if err != nil {
		return out, err
	}
	if _, err := conn.ExecContext(ctx, "INSERT INTO authority_commands(actor,command_id,state_key,digest,cut,receipt,receipt_digest) VALUES(?,?,?,?,?,?,?)", cmd.Actor, cmd.ID, cmd.Key, digest, cut, encoded, digestBytes(encoded)); err != nil {
		return out, err
	}
	if _, err := conn.ExecContext(ctx, "UPDATE authority SET cut=? WHERE singleton=1", cut); err != nil {
		return out, err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return out, err
	}
	return out, nil
}

func (s *Store) ReadAuthority(ctx context.Context, key string) (AuthoritySnapshot, error) {
	if !validIdentity(key) {
		return AuthoritySnapshot{}, errors.New("invalid authority state key")
	}
	conn, err := s.begin(ctx, false)
	if err != nil {
		return AuthoritySnapshot{}, err
	}
	defer rollbackClose(conn)
	return loadAuthoritySnapshot(ctx, conn, key)
}

func insertReceipt(ctx context.Context, conn *sql.Conn, receipt Receipt) error {
	encoded, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, "INSERT INTO commands(actor,command_id,run_id,digest,cut,receipt,receipt_digest) VALUES(?,?,?,?,?,?,?)", receipt.Actor, receipt.ID, receipt.RunID, receipt.Digest, receipt.Cut, encoded, digestBytes(encoded))
	return err
}

func commandDigest(cmd Command) (string, error) {
	if !validIdentity(cmd.ID) || !validIdentity(cmd.Actor) || !validIdentity(cmd.RunID) || len(cmd.Payload) > MaxCommandBytes || !json.Valid(cmd.Payload) {
		return "", errors.New("invalid command envelope")
	}
	if cmd.Mode != CommandCAS && cmd.Mode != CommandMonotonic && cmd.Mode != CommandPublication && cmd.Mode != CommandGuarded {
		return "", errors.New("unsupported command mode")
	}
	if cmd.ExpectedVersion != nil && (*cmd.ExpectedVersion < 0 || *cmd.ExpectedVersion > (1<<53)-1) {
		return "", errors.New("expected version is outside exact JSON integer range")
	}
	if cmd.Mode == CommandCAS && cmd.ExpectedVersion == nil {
		return "", errors.New("CAS requires a nonnegative expected version")
	}
	if cmd.Control != nil && (!validIdentity(cmd.Control.Key) || cmd.Control.Version < 0 || cmd.Control.Version > (1<<53)-1) {
		return "", errors.New("invalid authority control pin")
	}
	if _, err := commandPins(cmd.Pins, cmd.Control); err != nil {
		return "", err
	}
	// Hash all protected request fields, not merely the business payload. Extra
	// authority pins are current commit guards, not caller-requested semantics:
	// excluding them preserves receipt retries made after an authority change.
	envelope, err := json.Marshal(struct {
		RunID           string          `json:"run_id"`
		Mode            CommandMode     `json:"mode"`
		ExpectedVersion *int64          `json:"expected_version"`
		Payload         json.RawMessage `json:"payload"`
		Control         *ControlPin     `json:"control,omitempty"`
	}{cmd.RunID, cmd.Mode, cmd.ExpectedVersion, cmd.Payload, cmd.Control})
	if err != nil {
		return "", err
	}
	canonical, err := jsoncanonicalizer.Transform(envelope)
	if err != nil {
		return "", errors.New("command contains non-canonicalizable JSON")
	}
	return digestBytes(canonical), nil
}

func commandPins(pins []ControlPin, control *ControlPin) ([]ControlPin, error) {
	ordered := append([]ControlPin(nil), pins...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Key < ordered[j].Key })
	for i, pin := range ordered {
		if !validIdentity(pin.Key) || pin.Version < 0 || pin.Version > (1<<53)-1 || i != 0 && ordered[i-1].Key == pin.Key || control != nil && pin.Key == control.Key {
			return nil, errors.New("invalid command authority pins")
		}
	}
	return ordered, nil
}

func linkedRunCommandDigest(cmd LinkedRunCommand) (string, error) {
	pins := append([]ControlPin(nil), cmd.Pins...)
	sort.Slice(pins, func(i, j int) bool { return pins[i].Key < pins[j].Key })
	for i, pin := range pins {
		if !validIdentity(pin.Key) || pin.Version < 0 || pin.Version > (1<<53)-1 || i != 0 && pins[i-1].Key == pin.Key {
			return "", errors.New("invalid linked run authority pins")
		}
	}
	if !validIdentity(cmd.ID) || !validIdentity(cmd.Actor) || !validIdentity(cmd.SourceRunID) || !validIdentity(cmd.NewRunID) || cmd.ExpectedVersion < 0 || cmd.ExpectedVersion > (1<<53)-1 || len(cmd.Payload) > MaxCommandBytes || !json.Valid(cmd.Payload) {
		return "", errors.New("invalid linked run command envelope")
	}
	b, err := json.Marshal(struct {
		SourceRunID     string          `json:"source_run_id"`
		NewRunID        string          `json:"new_run_id"`
		ExpectedVersion int64           `json:"expected_version"`
		Payload         json.RawMessage `json:"payload"`
		Pins            []ControlPin    `json:"authority_pins,omitempty"`
	}{cmd.SourceRunID, cmd.NewRunID, cmd.ExpectedVersion, cmd.Payload, pins})
	if err != nil {
		return "", err
	}
	b, err = jsoncanonicalizer.Transform(b)
	if err != nil {
		return "", errors.New("linked run command contains non-canonicalizable JSON")
	}
	return digestBytes(b), nil
}

func authorityCommandDigest(cmd AuthorityCommand) (string, error) {
	if !validIdentity(cmd.ID) || !validIdentity(cmd.Actor) || !validIdentity(cmd.Key) || len(cmd.Payload) > MaxCommandBytes || !json.Valid(cmd.Payload) || cmd.ExpectedVersion != nil && (*cmd.ExpectedVersion < 0 || *cmd.ExpectedVersion > 1<<53-1) {
		return "", errors.New("invalid authority command envelope")
	}
	b, err := json.Marshal(struct {
		Key             string          `json:"key"`
		ExpectedVersion *int64          `json:"expected_version"`
		Payload         json.RawMessage `json:"payload"`
	}{cmd.Key, cmd.ExpectedVersion, cmd.Payload})
	if err != nil {
		return "", err
	}
	b, err = jsoncanonicalizer.Transform(b)
	if err != nil {
		return "", errors.New("authority command contains non-canonicalizable JSON")
	}
	return digestBytes(b), nil
}

func (s *Store) validateChange(change Change, mode CommandMode) error {
	if change.ReceiptOnly {
		if mode != CommandPublication || change.AdvanceRunVersion || len(change.Data) != 0 || len(change.Events) != 0 || change.AcquireSlot != "" || change.ReleaseSlot != "" || len(change.Result) == 0 || len(change.Result) > MaxCommandBytes || !json.Valid(change.Result) {
			return errors.New("receipt-only publication requires only a bounded result")
		}
		return nil
	}
	if change.AdvanceRunVersion && mode != CommandPublication {
		return errors.New("only a publication may explicitly advance run version")
	}
	if len(change.Data) == 0 || len(change.Data) > MaxSnapshotBytes || !json.Valid(change.Data) || len(change.Events) == 0 || len(change.Events) > 100 {
		return errors.New("mutation requires bounded valid snapshot and 1..100 events")
	}
	if change.Result != nil && (len(change.Result) > MaxCommandBytes || !json.Valid(change.Result)) {
		return errors.New("invalid command result")
	}
	for _, event := range change.Events {
		if !s.eventTypes[event.Type] || (event.Version != 0 && event.Version != EventVersion) {
			return ErrIncompatible
		}
		if len(event.Data) > MaxCommandBytes || !json.Valid(event.Data) {
			return errors.New("invalid event payload")
		}
	}
	if change.AcquireSlot != "" && !validIdentity(change.AcquireSlot) || change.ReleaseSlot != "" && !validIdentity(change.ReleaseSlot) {
		return errors.New("invalid slot identity")
	}
	return nil
}

func (s *Store) begin(ctx context.Context, write bool) (*sql.Conn, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	statement := "BEGIN"
	if write {
		statement = "BEGIN IMMEDIATE"
	}
	if _, err := conn.ExecContext(ctx, statement); err != nil {
		_ = conn.Close()
		return nil, err
	}
	if write {
		if err := storageHeader(ctx, conn); err != nil {
			_ = rollbackClose(conn)
			return nil, err
		}
	}
	return conn, nil
}

func storageHeader(ctx context.Context, conn *sql.Conn) error {
	var version, id int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if err := conn.QueryRowContext(ctx, "PRAGMA application_id").Scan(&id); err != nil {
		return err
	}
	if id != applicationID || version < 1 || version > StorageVersion {
		return ErrIncompatible
	}
	return nil
}

func rollbackClose(conn *sql.Conn) error {
	_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
	return conn.Close()
}

func loadSnapshot(ctx context.Context, conn *sql.Conn, runID string) (Snapshot, error) {
	state := Snapshot{RunID: runID}
	var digest string
	err := conn.QueryRowContext(ctx, "SELECT version,event_seq,snapshot,snapshot_digest FROM runs WHERE run_id=?", runID).Scan(&state.Version, &state.EventSeq, scanJSON{&state.Data}, &digest)
	if errors.Is(err, sql.ErrNoRows) {
		return state, ErrNotFound
	}
	if err != nil {
		return state, err
	}
	if digestBytes(state.Data) != digest {
		return state, ErrIntegrity
	}
	return state, nil
}

func loadAuthoritySnapshot(ctx context.Context, conn *sql.Conn, key string) (AuthoritySnapshot, error) {
	state := AuthoritySnapshot{Key: key}
	var digest string
	err := conn.QueryRowContext(ctx, "SELECT version,cut,data,digest FROM authority_states WHERE state_key=?", key).Scan(&state.Version, &state.Cut, scanJSON{&state.Data}, &digest)
	if errors.Is(err, sql.ErrNoRows) {
		return state, ErrNotFound
	}
	if err != nil {
		return state, err
	}
	if digestBytes(state.Data) != digest {
		return state, ErrIntegrity
	}
	return state, nil
}

func validIdentity(value string) bool {
	return len(value) > 0 && len(value) <= 256 && !strings.ContainsAny(value, "\x00\r\n")
}

// RawMessage is a defined byte slice (an alias of jsontext.Value in Go 1.27),
// which database/sql cannot scan NULL into directly. Keep one explicit copying
// scanner for BLOB/TEXT/NULL so nullable StateAfter and corrupted rows are safe.
type scanJSON struct{ target *json.RawMessage }

func (s scanJSON) Scan(value any) error {
	switch v := value.(type) {
	case nil:
		*s.target = nil
	case []byte:
		*s.target = append((*s.target)[:0], v...)
	case string:
		*s.target = append((*s.target)[:0], v...)
	default:
		return ErrIntegrity
	}
	return nil
}

func patchedSQLite(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		return false
	}
	var v [3]int
	for i := range v {
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return false
		}
		v[i] = n
	}
	return slices.Compare(v[:], []int{3, 51, 3}) >= 0 || (v[0] == 3 && v[1] == 50 && v[2] >= 7) || (v[0] == 3 && v[1] == 44 && v[2] >= 6)
}

package local

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
)

const (
	benchRuns        = 1200
	benchFillerBytes = 8 << 10
	// benchMinimumBytes is what the fixture must actually have allocated
	// before either benchmark starts its timer. Both benchmarks are named for
	// ten megabytes of history; this is the floor that makes the name a fact.
	benchMinimumBytes = 9 << 20
)

// fillBenchHistory writes benchRuns real Runs and returns what SQLite ended up
// holding. Every command is CAS at version 0, which is what creates a Run.
//
// The first version of these fixtures used CommandGuarded without creating the
// Run first, so all 1200 commands were rejected with "run does not exist": the
// database both benchmarks opened held 1200 rejection receipts, no Runs and no
// events at all, while their names and comments said ten megabytes of history.
// A benchmark that measures the wrong database reports a number, and the number
// looks exactly like a measurement — hence the assertions below, which run
// before the timer and fail the benchmark rather than letting it publish.
func fillBenchHistory(b *testing.B, store *Store) {
	b.Helper()
	payload := make([]byte, benchFillerBytes)
	for i := range payload {
		payload[i] = 'x'
	}
	body := json.RawMessage(`{"filler":"` + string(payload) + `"}`)
	ctx := context.Background()
	for i := range benchRuns {
		version := int64(0)
		result, err := store.Apply(ctx, Command{
			ID: fmt.Sprintf("command:bench-%d", i), Actor: "bench",
			RunID: fmt.Sprintf("run:bench-%d", i), Payload: json.RawMessage(`{}`),
			Mode: CommandCAS, ExpectedVersion: &version,
		}, func(Snapshot) (Change, error) {
			return Change{Data: body, Events: []EventInput{{Type: "run.created", Version: EventVersion, Data: body}}}, nil
		})
		if err != nil {
			b.Fatal(err)
		}
		if result.Receipt.Rejection != nil {
			b.Fatalf("the fixture recorded no history: command %d was rejected with %s", i, result.Receipt.Rejection.Code)
		}
	}
	conn, err := store.begin(ctx, false)
	if err != nil {
		b.Fatal(err)
	}
	defer rollbackClose(conn)
	var runs, events int64
	if err := conn.QueryRowContext(ctx, "SELECT count(*) FROM runs").Scan(&runs); err != nil {
		b.Fatal(err)
	}
	if err := conn.QueryRowContext(ctx, "SELECT count(*) FROM events").Scan(&events); err != nil {
		b.Fatal(err)
	}
	usage, err := storageUsage(ctx, conn, 1<<30)
	if err != nil {
		b.Fatal(err)
	}
	if runs != benchRuns || events != benchRuns {
		b.Fatalf("the fixture holds %d Runs and %d events, not %d of each", runs, events, benchRuns)
	}
	if usage.AllocatedBytes < benchMinimumBytes {
		b.Fatalf("the fixture allocated %d bytes, below the %d this benchmark is named for", usage.AllocatedBytes, benchMinimumBytes)
	}
}

// A large authority used to be read in full every time it was opened, so the
// cost of opening grew with everything it had ever recorded. This measures the
// open of a database with roughly ten megabytes of history.
func BenchmarkOpenStore10MB(b *testing.B) {
	dir := filepath.Join(b.TempDir(), "authority")
	options := StoreOptions{EventTypes: []string{"run.created"}, SoftLimitBytes: 1 << 30}
	store, err := OpenStore(dir, options)
	if err != nil {
		b.Fatal(err)
	}
	fillBenchHistory(b, store)
	if err := store.Close(); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for b.Loop() {
		opened, err := OpenStore(dir, options)
		if err != nil {
			b.Fatal(err)
		}
		if err := opened.Close(); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkVerifyStore10MB is what opening that same database used to cost:
// the complete scan, which is now reserved for doctor.
func BenchmarkVerifyStore10MB(b *testing.B) {
	dir := filepath.Join(b.TempDir(), "authority")
	options := StoreOptions{EventTypes: []string{"run.created"}, SoftLimitBytes: 1 << 30}
	store, err := OpenStore(dir, options)
	if err != nil {
		b.Fatal(err)
	}
	defer store.Close()
	fillBenchHistory(b, store)
	b.ResetTimer()
	for b.Loop() {
		if err := store.Verify(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}

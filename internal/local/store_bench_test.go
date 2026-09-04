package local

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
)

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
	payload := make([]byte, 8<<10)
	for i := range payload {
		payload[i] = 'x'
	}
	body := json.RawMessage(`{"filler":"` + string(payload) + `"}`)
	for i := range 1200 {
		runID := fmt.Sprintf("run:bench-%d", i)
		if _, err := store.Apply(context.Background(), Command{ID: fmt.Sprintf("command:bench-%d", i), Actor: "bench", RunID: runID, Payload: json.RawMessage(`{}`), Mode: CommandGuarded}, func(Snapshot) (Change, error) {
			return Change{Data: body, Events: []EventInput{{Type: "run.created", Version: EventVersion, Data: body}}}, nil
		}); err != nil {
			b.Fatal(err)
		}
	}
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
	payload := make([]byte, 8<<10)
	for i := range payload {
		payload[i] = 'x'
	}
	body := json.RawMessage(`{"filler":"` + string(payload) + `"}`)
	for i := range 1200 {
		runID := fmt.Sprintf("run:bench-%d", i)
		if _, err := store.Apply(context.Background(), Command{ID: fmt.Sprintf("command:bench-%d", i), Actor: "bench", RunID: runID, Payload: json.RawMessage(`{}`), Mode: CommandGuarded}, func(Snapshot) (Change, error) {
			return Change{Data: body, Events: []EventInput{{Type: "run.created", Version: EventVersion, Data: body}}}, nil
		}); err != nil {
			b.Fatal(err)
		}
	}
	b.ResetTimer()
	for b.Loop() {
		if err := store.Verify(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}

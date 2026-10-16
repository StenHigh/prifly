//go:build cgo

package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func scanDocument(size int, packed bool) json.RawMessage {
	prefix, suffix := `{"padding":"`, `"}`
	if !packed {
		suffix = `","$pinned":null}`
	}
	return json.RawMessage(prefix + strings.Repeat("a", size-len(prefix)-len(suffix)) + suffix)
}

func TestSnapshotScanPreservesAggregateReadBoundaries(t *testing.T) {
	t.Parallel()
	for _, packed := range []bool{true, false} {
		t.Run(fmt.Sprintf("packed-%t", packed), func(t *testing.T) {
			t.Parallel()
			s, _ := testStore(t)
			for i := range 8 {
				applyChange(t, s, storeCommand(fmt.Sprintf("seed-%d", i), fmt.Sprintf("run-%d", i), 0), storeChange(string(scanDocument(8<<20, packed))))
			}
			for i, delta := range []int{-1, 0, 1} {
				applyChange(t, s, storeCommand(fmt.Sprintf("boundary-%d", i), "run-7", int64(i+1)), storeChange(string(scanDocument((8<<20)+delta, packed))))
				expected := int64(64<<20) + int64(delta)
				_, _, err := s.ReadAll(context.Background(), 1000)
				var exhausted *ScanLimitError
				if delta <= 0 && err != nil {
					t.Fatal(err)
				}
				if delta > 0 && (!errors.As(err, &exhausted) || exhausted.RestoredBytes != expected || exhausted.Records != 8 || exhausted.ByteLimit != 64<<20 || exhausted.Reason != "bytes") {
					t.Fatalf("wrong bounded refusal: %+v %v", exhausted, err)
				}
				seen := 0
				scan, err := s.ScanSnapshots(context.Background(), func(snapshot Snapshot) error {
					seen++
					if !json.Valid(snapshot.Data) {
						t.Fatal("invalid restored JSON")
					}
					return nil
				})
				if err != nil || !scan.Complete || scan.Records != 8 || scan.RestoredBytes != expected || seen != 8 {
					t.Fatalf("scan: %+v %v seen=%d", scan, err, seen)
				}
			}
			var packedRows int
			if err := s.db.QueryRow("SELECT sum(snapshot_packed) FROM runs").Scan(&packedRows); err != nil {
				t.Fatal(err)
			}
			if packed && packedRows != 8 || !packed && packedRows != 0 {
				t.Fatalf("fixture uses wrong representation: %d", packedRows)
			}
		})
	}
}

func TestSnapshotScanBeyondRecordLimit(t *testing.T) {
	t.Parallel()
	s, _ := testStore(t)
	for i := range 1001 {
		applyChange(t, s, storeCommand(fmt.Sprintf("seed-%04d", i), fmt.Sprintf("run-%04d", i), 0), storeChange(`{"valid":true}`))
	}
	_, _, err := s.ReadAll(context.Background(), 1000)
	var exhausted *ScanLimitError
	if !errors.As(err, &exhausted) || exhausted.Records != 1001 || exhausted.RecordLimit != 1000 || exhausted.Reason != "count" {
		t.Fatalf("missing record limit: %+v %v", exhausted, err)
	}
	scan, err := s.ScanSnapshots(context.Background(), nil)
	if err != nil || !scan.Complete || scan.Records != 1001 {
		t.Fatalf("scan: %+v %v", scan, err)
	}
}

func TestSnapshotScanKeepsOneReadCut(t *testing.T) {
	t.Parallel()
	s, dir := testStore(t)
	writer, err := OpenStore(dir, storeTestOptions)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	applyChange(t, s, storeCommand("first", "run-a", 0), storeChange(`{"value":1}`))
	applyChange(t, s, storeCommand("second", "run-b", 0), storeChange(`{"value":1}`))
	signal, done := make(chan struct{}), make(chan error, 1)
	go func() {
		<-signal
		_, err := writer.Apply(context.Background(), storeCommand("update", "run-b", 1), func(Snapshot) (Change, error) { return storeChange(`{"value":2}`), nil })
		done <- err
	}()
	seen := 0
	scan, err := s.ScanSnapshots(context.Background(), func(snapshot Snapshot) error {
		seen++
		if seen == 1 {
			close(signal)
			if err := <-done; err != nil {
				return err
			}
		}
		if snapshot.Version != 1 || string(snapshot.Data) != `{"value":1}` {
			t.Fatalf("mixed snapshots: %+v", snapshot)
		}
		return nil
	})
	if err != nil || !scan.Complete || seen != 2 {
		t.Fatalf("scan: %+v %v", scan, err)
	}
	latest, _, err := s.ReadAll(context.Background(), 100)
	if err != nil || latest[1].Version != 2 {
		t.Fatalf("writer not visible after scan: %+v %v", latest, err)
	}
}

func TestSnapshotScanNeverProvesAbsenceAfterReadFailure(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"cancel", "callback", "digest", "missing-pin", "oversized-pin"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			s, _ := testStore(t)
			applyChange(t, s, storeCommand("first", "run-a", 0), storeChange(`{"value":1}`))
			applyChange(t, s, storeCommand("second", "run-b", 0), storeChange(string(scanDocument(1024, true))))
			switch kind {
			case "digest":
				_, _ = s.db.Exec("UPDATE runs SET snapshot_digest='broken' WHERE run_id='run-b'")
			case "missing-pin":
				_, _ = s.db.Exec("DELETE FROM pinned_bytes")
			case "oversized-pin":
				_, _ = s.db.Exec("UPDATE pinned_bytes SET bytes=zeroblob(?)", MaxSnapshotBytes+1)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stopped := errors.New("callback stopped")
			scan, err := s.ScanSnapshots(ctx, func(snapshot Snapshot) error {
				if kind == "cancel" {
					cancel()
				}
				if kind == "callback" {
					return stopped
				}
				return nil
			})
			if err == nil || scan.Complete {
				t.Fatalf("incomplete scan claimed success: %+v %v", scan, err)
			}
			if kind == "callback" && !errors.Is(err, stopped) {
				t.Fatal(err)
			}
			if kind == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			if kind != "callback" && kind != "cancel" && !errors.Is(err, ErrIntegrity) {
				t.Fatal(err)
			}
		})
	}
}

func TestOwnershipCutGuardsBothCommitPathsAndKeepsRetryIdentity(t *testing.T) {
	t.Parallel()
	for _, authority := range []bool{true, false} {
		t.Run(fmt.Sprintf("authority-%t", authority), func(t *testing.T) {
			t.Parallel()
			s, _ := testStore(t)
			applyChange(t, s, storeCommand("seed", "run-a", 0), storeChange(`{"value":1}`))
			scan, err := s.ScanSnapshots(context.Background(), nil)
			if err != nil {
				t.Fatal(err)
			}
			// Change an existing Run: population count and claims version are unchanged.
			applyChange(t, s, storeCommand("changed-holder", "run-a", 1), storeChange(`{"value":2}`))
			called := false
			if authority {
				cmd := AuthorityCommand{ID: "guarded", Actor: "owner", Key: "claims", Payload: json.RawMessage(`{}`), ExpectedCut: &scan.Cut}
				reducer := func(AuthoritySnapshot) (AuthorityChange, error) {
					called = true
					return AuthorityChange{Data: json.RawMessage(`{}`)}, nil
				}
				refused, err := s.ApplyAuthority(context.Background(), cmd, reducer)
				if err != nil || refused.Receipt.Rejection == nil || refused.Receipt.Rejection.Code != "claim_ownership_conflict" || called {
					t.Fatalf("stale: %+v %v", refused, err)
				}
				fresh, err := s.ScanSnapshots(context.Background(), nil)
				if err != nil {
					t.Fatal(err)
				}
				cmd.ID = "fresh"
				cmd.ExpectedCut = &fresh.Cut
				accepted, err := s.ApplyAuthority(context.Background(), cmd, reducer)
				if err != nil || accepted.Receipt.Rejection != nil || !called {
					t.Fatalf("fresh: %+v %v", accepted, err)
				}
				called = false
				cmd.ExpectedCut = &scan.Cut
				retry, err := s.ApplyAuthority(context.Background(), cmd, reducer)
				if err != nil || !retry.Duplicate || called {
					t.Fatalf("retry: %+v %v", retry, err)
				}
				plain := cmd
				plain.ExpectedCut = nil
				a, _ := authorityCommandDigest(cmd)
				b, _ := authorityCommandDigest(plain)
				if a != b {
					t.Fatal("proof changed retry digest")
				}
			} else {
				cmd := storeCommand("guarded", "run-a", 2)
				cmd.ExpectedCut = &scan.Cut
				reducer := func(Snapshot) (Change, error) { called = true; return storeChange(`{"value":3}`), nil }
				refused, err := s.Apply(context.Background(), cmd, reducer)
				if err != nil || refused.Receipt.Rejection == nil || refused.Receipt.Rejection.Code != "claim_ownership_conflict" || called {
					t.Fatalf("stale: %+v %v", refused, err)
				}
				fresh, err := s.ScanSnapshots(context.Background(), nil)
				if err != nil {
					t.Fatal(err)
				}
				cmd.ID = "fresh"
				cmd.ExpectedCut = &fresh.Cut
				accepted, err := s.Apply(context.Background(), cmd, reducer)
				if err != nil || accepted.Receipt.Rejection != nil || !called {
					t.Fatalf("fresh: %+v %v", accepted, err)
				}
				called = false
				cmd.ExpectedCut = &scan.Cut
				retry, err := s.Apply(context.Background(), cmd, reducer)
				if err != nil || !retry.Duplicate || called {
					t.Fatalf("retry: %+v %v", retry, err)
				}
				plain := cmd
				plain.ExpectedCut = nil
				a, _ := commandDigest(cmd)
				b, _ := commandDigest(plain)
				if a != b {
					t.Fatal("proof changed retry digest")
				}
			}
		})
	}
}

//go:build cgo

package local

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestMonitorRevisionPages(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	for _, cursor := range []string{"bad\n cursor", strings.Repeat("x", 257)} {
		if _, _, err := s.RevisionPage(ctx, cursor, 100); err == nil {
			t.Fatal("accepted invalid cursor")
		}
	}
	for i := 0; i < 205; i++ {
		applyChange(t, s, storeCommand(fmt.Sprintf("cmd:%03d", i), fmt.Sprintf("run:%03d", i), 0), storeChange(`{"n":1}`))
	}
	cursor := ""
	count := 0
	for {
		page, next, err := s.RevisionPage(ctx, cursor, 37)
		if err != nil {
			t.Fatal(err)
		}
		for _, row := range page {
			if row.RunID <= cursor || row.Version != 1 || row.EventSeq != 1 || len(row.Data) != 0 {
				t.Fatalf("invalid revision: %+v", row)
			}
			cursor = row.RunID
			count++
		}
		if next == "" {
			break
		}
		if next != cursor {
			t.Fatal("cursor does not follow last row")
		}
	}
	if count != 205 {
		t.Fatalf("read %d runs", count)
	}
}

func TestMonitorPageKeepsSnapshotIntegrity(t *testing.T) {
	s, _ := testStore(t)
	applyChange(t, s, storeCommand("first", "run:one", 0), storeChange(`{"n":1}`))
	if _, err := s.db.Exec("UPDATE events SET state_after='{}'"); err != nil {
		t.Fatal(err)
	}
	page, _, err := s.RevisionPage(context.Background(), "", 10)
	if err != nil || len(page) != 1 {
		t.Fatal(page, err)
	}
	if _, err := s.Read(context.Background(), page[0].RunID, 0, 1); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("corrupt snapshot was served: %v", err)
	}
}

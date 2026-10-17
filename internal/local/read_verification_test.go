//go:build cgo

package local

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLightVerificationIsReadOnlyAndChecksNewRecords(t *testing.T) {
	t.Parallel()
	s, dir := testStore(t)
	ctx := context.Background()
	applyChange(t, s, storeCommand("command:verified-prefix", "run:prefix", 0), storeChange(`{}`))
	options := storeTestOptions
	options.ReadOnly = true
	first, err := OpenStore(dir, options)
	if err != nil {
		t.Fatal(err)
	}
	proof := first.ReadVerification()
	if proof == nil || proof.cut < 1 {
		t.Fatal("missing verified prefix")
	}
	_ = first.Close()
	options.ReadVerification = proof
	second, err := OpenStore(dir, options)
	if err != nil {
		t.Fatal(err)
	}
	if second.ReadVerification().cut != proof.cut {
		t.Fatal("unchanged prefix moved")
	}
	_ = second.Close()
	applyChange(t, s, storeCommand("command:unverified-suffix", "run:suffix", 0), storeChange(`{}`))
	if _, err := s.db.Exec("UPDATE events SET digest='corrupt' WHERE run_id='run:suffix'"); err != nil {
		t.Fatal(err)
	}
	if opened, err := OpenStore(dir, options); err == nil {
		opened.Close()
		t.Fatal("cached prefix bypassed new corrupt event")
	}
	if _, err := s.ReadAuthority(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}
func TestLightVerificationCannotBeTransferredToAnotherDatabase(t *testing.T) {
	t.Parallel()
	s, dir := testStore(t)
	applyChange(t, s, storeCommand("command:prefix", "run:prefix", 0), storeChange(`{}`))
	options := storeTestOptions
	options.ReadOnly = true
	first, err := OpenStore(dir, options)
	if err != nil {
		t.Fatal(err)
	}
	proof := first.ReadVerification()
	_ = first.Close()
	copyDir := t.TempDir()
	otherInfo := s.Info()

	if proof.through(copyDir, otherInfo, proof.file, proof.cut) != 0 {
		t.Fatal("proof crossed directory")
	}
	other, otherDir := testStore(t)
	_ = other
	info, err := os.Stat(filepath.Join(otherDir, "state.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	if proof.through(dir, otherInfo, info, proof.cut) != 0 {
		t.Fatal("proof crossed inode")
	}
}

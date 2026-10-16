package runtime

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/stenhigh/prifly/internal/local"
)

// ClaimOwnershipDiagnostics is a read-only ownership assessment. SQLite page
// usage is measured separately from the restored snapshot population.
type ClaimOwnershipDiagnostics struct {
	SchemaVersion string              `json:"schema_version"`
	AuthorityID   string              `json:"authority_id"`
	ClaimID       string              `json:"claim_id"`
	Generation    int64               `json:"generation"`
	Status        string              `json:"status"`
	RunID         string              `json:"run_id"`
	HolderRunID   string              `json:"holder_run_id"`
	Assessment    string              `json:"assessment"`
	Reason        string              `json:"reason"`
	Scan          ClaimOwnershipScan  `json:"scan"`
	Storage       *local.StorageUsage `json:"storage"`
}
type ClaimOwnershipScan struct {
	Cut           int64 `json:"cut"`
	Complete      bool  `json:"complete"`
	Records       int64 `json:"records"`
	RestoredBytes int64 `json:"restored_bytes"`
	RecordLimit   int64 `json:"bounded_read_record_limit"`
	ByteLimit     int64 `json:"bounded_read_byte_limit"`
}

//go:embed claim-ownership.schema.json
var ClaimOwnershipSchema []byte

// DiagnoseClaim reports whether release ownership can be proved. A refusal
// still returns the partial view; neither this method nor its CLI changes state.
func (e *Engine) DiagnoseClaim(ctx context.Context, id string) (view ClaimOwnershipDiagnostics, diagnosisErr error) {
	view = ClaimOwnershipDiagnostics{SchemaVersion: "claim-ownership-diagnostics/1", AuthorityID: e.Installation.ID, ClaimID: id, Assessment: "unknown", Scan: ClaimOwnershipScan{RecordLimit: local.MaxReadRecords, ByteLimit: local.MaxScanBytes}}
	usage, usageErr := e.Store.StorageUsage(ctx)
	if usageErr == nil {
		view.Storage = &usage
	}
	record, claimsVersion, err := e.readClaims(ctx)
	if err != nil {
		view.Reason = "claims_unreadable"
		return view, err
	}
	// Claims and Run evidence are read separately. Refuse a mixed observation
	// if the ledger moved while the snapshot scan/lookup was in progress.
	defer func() {
		if err := e.confirmClaimDiagnosis(ctx, claimsVersion, &view); err != nil {
			diagnosisErr = err
		}
	}()
	var selected *WorktreeClaim
	for _, c := range record.Claims {
		if c.ID == id {
			copy := c
			selected = &copy
		}
	}
	if selected == nil {
		view.Reason = "claim_missing"
		return view, fault("claim_missing", "the requested claim does not exist")
	}
	view.Generation, view.Status, view.RunID = selected.Generation, selected.Status, selected.RunID
	if selected.RunID == "" {
		scan, scanErr := e.scanClaimHolders(ctx, id, true)
		view.Scan.Cut, view.Scan.Complete, view.Scan.Records, view.Scan.RestoredBytes = scan.Cut, scan.Complete, scan.Records, scan.RestoredBytes
		err = scanErr
	} else {
		run, snapshot, loadErr := e.load(ctx, selected.RunID)
		err = loadErr
		if err == nil && run.ID != selected.RunID {
			err = local.ErrIntegrity
		}
		if err == nil {
			view.Scan.Cut = snapshot.Cut
			view.Scan.Records = 1
			view.Scan.RestoredBytes = int64(len(snapshot.Snapshot.Data))
			view.Scan.Complete = true
			if !claimRunFinished(run) {
				err = &claimHolderError{RunID: run.ID}
			}
		}
	}
	if err != nil {
		view.Reason = "ownership_unproven"
		var holder *claimHolderError
		if errors.As(err, &holder) {
			view.Assessment = "blocked"
			view.HolderRunID = holder.RunID
			view.Reason = "run_holds_claim"
		}
		return view, err
	}
	view.Assessment, view.Reason = "releasable", "ownership_proven"
	if selected.Status == "released" {
		view.Assessment, view.Reason = "released", "claim_already_released"
	}
	if usageErr != nil {
		view.Assessment, view.Reason = "unknown", "storage_usage_unreadable"
		return view, usageErr
	}
	return view, nil
}

func (e *Engine) confirmClaimDiagnosis(ctx context.Context, expected int64, view *ClaimOwnershipDiagnostics) error {
	_, current, err := e.readClaims(ctx)
	if err != nil {
		view.Assessment, view.Reason = "unknown", "claims_unreadable"
		return err
	}
	if current != expected {
		view.Assessment, view.Reason = "unknown", "claims_changed"
		return fault("claim_ownership_conflict", "claims changed during diagnosis; inspect the current generation and diagnose again")
	}
	return nil
}

type claimHolderError struct{ RunID string }

func (e *claimHolderError) Error() string {
	return fmt.Sprintf("Run %s still holds the claim", e.RunID)
}
func (e *claimHolderError) Unwrap() error { return fault("claim_owner_unproven", e.Error()) }

// StartOutcome reconciles a possibly lost start response using the command's
// deterministic Run identity and durable receipt. Absence is reported only
// after both reads succeeded (or explicitly returned not found).
func (e *Engine) StartOutcome(ctx context.Context, command string) (runID string, err error) {
	id := startRunID(e.owner, command)
	run, _, readErr := e.load(ctx, id)
	if readErr == nil {
		if run.ID != id {
			return "", local.ErrIntegrity
		}
		return run.ID, nil
	}
	if !errors.Is(readErr, local.ErrNotFound) {
		return "", readErr
	}
	receipt, receiptErr := e.Store.LookupReceipt(ctx, e.owner, command)
	if errors.Is(receiptErr, local.ErrNotFound) {
		return "", nil
	}
	if receiptErr != nil {
		return "", receiptErr
	}
	if receipt.Rejection != nil {
		return "", nil
	}
	return "", local.ErrIntegrity // accepted receipt with no readable Run
}

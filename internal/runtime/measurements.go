package runtime

import (
	"context"
	"time"

	"github.com/stenhigh/prifly/internal/local"
)

// Diagnostic sampling is bounded and cannot undo a committed control command.
// A crash/disk-full can lose samples; reports must never claim complete coverage
// for this stream. Mandatory state, results and receipts live in the transaction.
func (e *Engine) recordCommand(commandID, runID string, started time.Time, result local.ApplyResult, applyErr error) {
	if result.Receipt.Version == 0 {
		runID = ""
	}
	if len(commandID) > 128 {
		commandID = ""
	}
	now := e.clock.now()
	requestID := newID("measurement")
	values := map[string]float64{"core.command_requests": 1, "core.command_duration": float64(time.Since(started)) / float64(time.Millisecond)}
	if result.Duplicate {
		values["core.command_duplicates"] = 1
	}
	if persistenceFailure(applyErr) {
		values["core.persistence_failures"] = 1
	}
	if result.TransactionDuration > 0 {
		values["core.lock_wait"] = float64(result.LockWait) / float64(time.Millisecond)
		values["core.transaction_duration"] = float64(result.TransactionDuration) / float64(time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if usage, err := e.Store.StorageUsage(ctx); err == nil {
		values["core.storage_bytes"] = float64(usage.AllocatedBytes)
	}
	batch := make([]local.SampleInput, 0, len(values))
	for metric, value := range values {
		unit, method := "ms", "runtime_observation"
		if metric == "core.command_requests" || metric == "core.command_duplicates" || metric == "core.persistence_failures" {
			unit, method = "count", "command_intake"
		}
		sampleRunID, sampleCommandID := runID, commandID
		if metric == "core.storage_bytes" {
			unit, method = "bytes", "sqlite_page_accounting"
			sampleRunID, sampleCommandID = "", ""
		}
		data, err := canonical(TelemetrySampleData{SchemaVersion: "telemetry-sample/1", Metric: metric, Value: &value, Unit: unit, Method: method, Quality: "measured", Coverage: "sample", Observed: now, CommandID: sampleCommandID})
		if err != nil {
			continue
		}
		batch = append(batch, local.SampleInput{ID: derivedID("sample", requestID, metric), RunID: sampleRunID, Data: data})
	}
	if len(batch) == 0 {
		return
	}
	_, _ = e.Store.AppendSamples(ctx, batch)
}

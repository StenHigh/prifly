package runtime

import (
	"context"
	"testing"
)

func TestStorageBudgetStatusUnknownCarriesReason(t *testing.T) {
	t.Parallel()
	e := artifactEngine(t)
	if err := e.Store.Close(); err != nil {
		t.Fatal(err)
	}
	view, err := e.StorageBudgetStatus(context.Background())
	if err == nil || view.Measurement != "unknown" || view.Reason == "" || view.Usage != nil || view.Control != nil {
		t.Fatalf("unknown observation must carry a reason without invented measurements: view=%+v err=%v", view, err)
	}
}

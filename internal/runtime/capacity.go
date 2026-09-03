package runtime

import (
	"context"
	"errors"
	"fmt"

	"encoding/json"

	"github.com/stenhigh/prifly/internal/local"
)

// MaxAdmissionCapacity is the qualified local profile's ceiling on attempts
// admitted at once. It is a qualification statement, not a performance target:
// a higher number would describe concurrency this build has not been qualified
// to run, whatever the hardware could manage.
const MaxAdmissionCapacity = 4

// CapacityRequest changes how many attempts this authority admits at once.
type CapacityRequest struct {
	CommandID string
	Capacity  int64
	Reason    string
}

// SetAdmissionCapacity records the decision and applies it in one transaction.
// The effective capacity is the one the slot table enforces; nothing keeps a
// second copy of it that could drift from the decision that set it.
func (e *Engine) SetAdmissionCapacity(ctx context.Context, c CapacityRequest) (local.AuthorityApplyResult, error) {
	if c.CommandID == "" || c.Reason == "" || len(c.Reason) > 4096 {
		return local.AuthorityApplyResult{}, errors.New("explicit command and reason required")
	}
	if c.Capacity < 1 || c.Capacity > MaxAdmissionCapacity {
		return local.AuthorityApplyResult{}, local.Reject("unqualified_capacity", fmt.Sprintf("this build is qualified for 1 to %d attempts at once", MaxAdmissionCapacity))
	}
	control, _, err := e.ensureControl(ctx)
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationAdmit) {
		return local.AuthorityApplyResult{}, local.Reject("object_access_denied", "the session principal cannot change admission capacity")
	}
	payload, err := canonical(map[string]any{"operation": "admission.capacity", "command_id": c.CommandID, "capacity": c.Capacity, "reason": c.Reason})
	if err != nil {
		return local.AuthorityApplyResult{}, err
	}
	capacity := c.Capacity
	return e.applyControlCapacity(ctx, c.CommandID, payload, &capacity, func(control *AuthorityControl, obs Observation) (json.RawMessage, error) {
		control.ControlEpoch++
		return canonical(map[string]any{"admission_capacity": capacity})
	})
}

// AdmissionCapacity reports the enforced capacity and what currently holds it.
func (e *Engine) AdmissionCapacity(ctx context.Context) (int64, map[string]string, error) {
	capacity, err := e.Store.SlotCapacity(ctx)
	if err != nil {
		return 0, nil, err
	}
	held, err := e.Store.Slots(ctx)
	return capacity, held, err
}

// AdmissionQueue reports which runs are waiting for a slot and since which
// admission decision. A run holds its place by asking again, so this is what
// the authority currently believes is waiting, not a promise that each entry
// is still live.
func (e *Engine) AdmissionQueue(ctx context.Context) (map[string]int64, error) {
	return e.Store.SlotWaiters(ctx)
}

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"time"

	// A copy of the zone database is carried in the binary so that a host with
	// no zoneinfo installed can still resolve a schedule's timezone: a schedule
	// that cannot name its own zone is not a schedule. It is a fallback, not a
	// pin - the host's database is preferred when it has one - so this build
	// does not claim a fixed tzdata version, and no versioned policy governs a
	// change to it. What that costs is stated where it lands: a future local
	// time may be read differently after the database changes. A slot already
	// decided keeps its recorded instant and is never reinterpreted.
	_ "time/tzdata"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// This build owns no timer and starts no daemon. Nothing here wakes up: due
// slots are computed and fired only when the authority is asked, by an explicit
// call. That is a deliberate position, not an omission waiting for a scheduler -
// promising automatic wakeup without an owner for the timers would be a promise
// nobody keeps. A slot that came due while nothing was asking is a missed slot,
// and the misfire policy is what decides what to do about it.

// localOccurrences resolves one nominal wall time in a zone. A wall time is
// normally one instant, is two on the fold day and is none inside the spring
// gap. The candidates are that wall time read at the offsets in force either
// side of a possible transition; no other offset in the zone can produce it.
func localOccurrences(loc *time.Location, y int, mo time.Month, d, h, mi int) (valid, candidates []time.Time) {
	naive := time.Date(y, mo, d, h, mi, 0, 0, time.UTC)
	for _, probe := range []time.Duration{-zoneProbeWindow, zoneProbeWindow} {
		_, offset := naive.Add(probe).In(loc).Zone()
		instant := naive.Add(-time.Duration(offset) * time.Second).UTC()
		if slices.ContainsFunc(candidates, instant.Equal) {
			continue
		}
		candidates = append(candidates, instant)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].Before(candidates[j]) })
	for _, instant := range candidates {
		wall := instant.In(loc)
		if wall.Year() == y && wall.Month() == mo && wall.Day() == d && wall.Hour() == h && wall.Minute() == mi {
			valid = append(valid, instant)
		}
	}
	return valid, candidates
}

// daySlots expands one local calendar day into logical slots under the
// schedule's declared gap and fold policies.
func daySlots(s Schedule, calendar ScheduleCalendar, loc *time.Location, day time.Time) ([]slot, error) {
	y, mo, d := day.Date()
	slots := []slot{}
	for _, entry := range calendar.DailyLocalTimes {
		h, mi, err := parseLocalTime(entry)
		if err != nil {
			return nil, err
		}
		// A logical slot is a date as well as a time. Nine o'clock on Tuesday
		// and nine o'clock on Wednesday are different slots, so the identity
		// carries the day it belongs to.
		value := fmt.Sprintf("%04d-%02d-%02dT%s", y, int(mo), d, entry)
		valid, candidates := localOccurrences(loc, y, mo, d, h, mi)
		switch {
		case len(valid) == 0:
			if s.DSTGap == "skip" {
				// The wall clock never shows this time on this day, and the
				// schedule said not to invent a moment when it would have.
				continue
			}
			// next_valid is the instant a clock still running at the offset in
			// force before the jump would have reached. That is the later
			// candidate, and it is the first instant whose local time is past
			// the one the calendar asked for.
			slots = append(slots, slot{value, candidates[len(candidates)-1], 0})
		case len(valid) == 1:
			slots = append(slots, slot{value, valid[0], 0})
		default:
			// The local time happens twice. Which of them the calendar meant is
			// the schedule's declared answer, not a library default.
			switch s.DSTFold {
			case "first":
				slots = append(slots, slot{value, valid[0], 0})
			case "second":
				slots = append(slots, slot{value, valid[1], 1})
			default:
				slots = append(slots, slot{value, valid[0], 0}, slot{value, valid[1], 1})
			}
		}
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].instant.Before(slots[j].instant) })
	return slots, nil
}

// dueSlots lists the logical slots strictly after the watermark and not after
// the observed instant, oldest first. The watermark is exclusive because a slot
// at or before it already has a decision, whatever that decision was.
func dueSlots(s Schedule, calendar ScheduleCalendar, after, now time.Time) ([]slot, error) {
	loc, err := time.LoadLocation(s.Timezone)
	if err != nil {
		return nil, fmt.Errorf("unknown_timezone: %s is not a zone this build can resolve", s.Timezone)
	}
	if now.Before(after) {
		// A watermark ahead of the clock is a rollback, not a backlog. Deciding
		// slots again on the strength of a clock that went backwards is exactly
		// the duplicate this watermark exists to prevent.
		return []slot{}, nil
	}
	// One extra day either side covers a slot whose instant falls outside its
	// own local day once the zone offset is applied.
	const windowMargin = 3
	days := int(now.Sub(after)/(24*time.Hour)) + windowMargin
	if days*len(calendar.DailyLocalTimes) > MaxDueSlots {
		return nil, local.Reject("schedule_backlog", fmt.Sprintf("this schedule is further behind than one call may decide (%d slots)", MaxDueSlots))
	}
	start := after.In(loc)
	// Noon, so that adding whole days never lands the cursor itself in a gap.
	cursor := time.Date(start.Year(), start.Month(), start.Day(), 12, 0, 0, 0, loc).AddDate(0, 0, -1)
	end := now.In(loc).AddDate(0, 0, 1)
	due := []slot{}
	for ; !cursor.After(end); cursor = cursor.AddDate(0, 0, 1) {
		slots, err := daySlots(s, calendar, loc, cursor)
		if err != nil {
			return nil, err
		}
		for _, candidate := range slots {
			if !candidate.instant.After(after) || candidate.instant.After(now) {
				continue
			}
			due = append(due, candidate)
		}
	}
	sort.Slice(due, func(i, j int) bool { return due[i].instant.Before(due[j].instant) })
	if len(due) > MaxDueSlots {
		return nil, local.Reject("schedule_backlog", fmt.Sprintf("this schedule is further behind than one call may decide (%d slots)", MaxDueSlots))
	}
	return due, nil
}

// slotDisposition says what happens to the oldest due slot given how many slots
// are still due behind it. With no timer, the newest due slot is the current
// one; everything older than it was missed, and the misfire policy owns those.
func slotDisposition(s Schedule, remaining int) (disposition, reason string) {
	if remaining <= 1 {
		return "firing", ""
	}
	// The budget counts missed slots only, measured back from the newest.
	budget := 0
	if s.Misfire == "bounded_catch_up" {
		budget = s.MaxCatchUp
	}
	if remaining-1 <= budget {
		return "firing", ""
	}
	if s.Misfire == "coalesce" {
		return "coalesced", "superseded_by_later_slot"
	}
	if s.Misfire == "skip" {
		return "skipped", "misfire_skipped"
	}
	return "skipped", "catch_up_exhausted"
}

func (e *Engine) readSchedules(ctx context.Context) (ScheduleRecord, int64, error) {
	empty := ScheduleRecord{SchemaVersion: AuthoritySchedulesVersion, AuthorityID: e.Installation.ID, Schedules: []ScheduleEntry{}}
	snapshot, err := e.Store.ReadAuthority(ctx, AuthoritySchedulesKey)
	if errors.Is(err, local.ErrNotFound) {
		return empty, 0, nil
	}
	if err != nil {
		return ScheduleRecord{}, 0, err
	}
	var record ScheduleRecord
	if err := decode(snapshot.Data, &record); err != nil {
		return ScheduleRecord{}, 0, err
	}
	if record.SchemaVersion != AuthoritySchedulesVersion || record.AuthorityID != e.Installation.ID {
		return ScheduleRecord{}, 0, errors.New("unsupported or foreign schedule record")
	}
	return record, snapshot.Version, nil
}

// Schedules lists what this installation holds, including the slot decisions
// each schedule has already taken.
func (e *Engine) Schedules(ctx context.Context) (ScheduleRecord, error) {
	record, _, err := e.readSchedules(ctx)
	return record, err
}

// scheduleResource reads one registered document by its exact reference. The
// inventory already checked that the bytes hash to the reference, so a document
// that has been edited since the schedule was created is not silently accepted.
func (e *Engine) scheduleResource(ref flow.Ref, into any) error {
	_, registry, _, err := e.inventoryResources()
	if err != nil {
		return err
	}
	data, present := registry[ref]
	if !present {
		return local.Reject("definition_absent", "no registered definition matches "+ref.String())
	}
	return decode(data, into)
}

// definitionPath finds where a registered definition lives, so that a schedule
// can start its workflow through the ordinary Start path rather than a private
// one that would skip its checks.
func (e *Engine) definitionPath(ref flow.Ref) (string, error) {
	file, err := e.localRegistry()
	if err != nil {
		return "", err
	}
	for _, entry := range append(append([]Definition{}, file.Entries...), e.packageEntries()...) {
		if entry.Ref == ref {
			return entry.Path, nil
		}
	}
	return "", local.Reject("definition_absent", "no registered definition matches "+ref.String())
}

// scheduleDocuments resolves the pair of pinned documents a schedule points at:
// the calendar it runs on and the brief plus inputs its Runs are started with.
func (e *Engine) scheduleDocuments(s Schedule) (ScheduleCalendar, ScheduleInputs, error) {
	var calendar ScheduleCalendar
	var inputs ScheduleInputs
	if err := e.scheduleResource(s.CalendarRef, &calendar); err != nil {
		return calendar, inputs, err
	}
	if err := validateCalendar(calendar); err != nil {
		return calendar, inputs, err
	}
	if err := e.scheduleResource(s.InputMapperRef, &inputs); err != nil {
		return calendar, inputs, err
	}
	if inputs.SchemaVersion != ScheduleInputsVersion {
		return calendar, inputs, errors.New("unsupported schedule input mapper contract version")
	}
	briefBytes, err := canonical(inputs.Brief)
	if err != nil {
		return calendar, inputs, err
	}
	if err := flow.ValidateProtocol("RunBrief", briefBytes); err != nil {
		return calendar, inputs, err
	}
	// The owner confirms once, here, when the schedule is created. A fired Run
	// does not manufacture a confirmation nobody gave at the moment it fires.
	if inputs.Brief.Confirmation != "explicit" {
		return calendar, inputs, local.Reject("confirmation_required", "a schedule's brief carries the owner's explicit confirmation")
	}
	return calendar, inputs, nil
}

type ScheduleRequest struct {
	CommandID string
	Schedule  Schedule
}

// CreateSchedule records a schedule. Creating one is not running one: the new
// schedule's watermark starts at the moment it was created, so nothing in its
// calendar's past is due, and this call fires nothing.
func (e *Engine) CreateSchedule(ctx context.Context, request ScheduleRequest) (Schedule, error) {
	if e.ReadOnly {
		return Schedule{}, local.ErrReadOnly
	}
	if request.CommandID == "" {
		return Schedule{}, errors.New("explicit command_id is required")
	}
	if err := validateSchedule(request.Schedule); err != nil {
		return Schedule{}, err
	}
	if _, _, err := e.scheduleDocuments(request.Schedule); err != nil {
		return Schedule{}, err
	}
	if _, err := e.definitionPath(request.Schedule.WorkflowRef); err != nil {
		return Schedule{}, err
	}
	control, _, err := e.ensureControl(ctx)
	if err != nil {
		return Schedule{}, err
	}
	// A schedule is a standing instruction to admit Runs, so the access it needs
	// is the access to admit them.
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationAdmit) {
		return Schedule{}, local.Reject("object_access_denied", "the session principal cannot create a schedule in this project")
	}
	for _, id := range request.Schedule.GrantRefs {
		if control.grant(id) == nil {
			return Schedule{}, local.Reject("grant_absent", "this schedule names a grant that does not exist: "+id)
		}
	}
	payload, err := canonical(map[string]any{"operation": "schedule.create", "command_id": request.CommandID, "schedule": request.Schedule})
	if err != nil {
		return Schedule{}, err
	}
	applied, err := e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: request.CommandID, Actor: e.owner, Key: AuthoritySchedulesKey, Payload: payload}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
		record, err := scheduleRecordFrom(e, s)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		if record.entry(request.Schedule.ID) != nil {
			// Replacing a schedule would move a new calendar under an existing
			// watermark, which would make already decided slots mean something
			// else. A different calendar is a different schedule.
			return local.AuthorityChange{}, local.Reject("schedule_present", "a schedule of that identity already exists")
		}
		if len(record.Schedules) >= MaxSchedules {
			return local.AuthorityChange{}, local.Reject("schedules_exhausted", fmt.Sprintf("this installation already holds its limit of %d schedules", MaxSchedules))
		}
		obs := e.clock.now()
		record.Schedules = append(record.Schedules, ScheduleEntry{Schedule: request.Schedule, Created: obs, DecidedThrough: obs.UTC, Slots: []SlotDecision{}})
		sort.Slice(record.Schedules, func(i, j int) bool { return record.Schedules[i].Schedule.ID < record.Schedules[j].Schedule.ID })
		data, err := canonicalState(record)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		return local.AuthorityChange{Data: data, Result: json.RawMessage(`{"created":true}`)}, nil
	})
	return request.Schedule, authorityRefusal(applied, err)
}

// SetScheduleEnabled turns a schedule on or off. Disabling does not move the
// watermark: slots that come due while it is off stay undecided, and the
// misfire policy decides what they mean when it is turned back on.
func (e *Engine) SetScheduleEnabled(ctx context.Context, commandID, id string, enabled bool) error {
	if e.ReadOnly {
		return local.ErrReadOnly
	}
	if commandID == "" {
		return errors.New("explicit command_id is required")
	}
	control, _, err := e.ensureControl(ctx)
	if err != nil {
		return err
	}
	if !control.allows(e.owner, "project", e.Config.ID, ControlOperationAdmit) {
		return local.Reject("object_access_denied", "the session principal cannot change a schedule in this project")
	}
	payload, err := canonical(map[string]any{"operation": "schedule.enabled", "command_id": commandID, "schedule_id": id, "enabled": enabled})
	if err != nil {
		return err
	}
	applied, err := e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: commandID, Actor: e.owner, Key: AuthoritySchedulesKey, Payload: payload}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
		record, err := scheduleRecordFrom(e, s)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		entry := record.entry(id)
		if entry == nil {
			return local.AuthorityChange{}, local.Reject("not_found", "no schedule of that identity exists")
		}
		entry.Schedule.Enabled = enabled
		data, err := canonicalState(record)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		return local.AuthorityChange{Data: data, Result: json.RawMessage(`{"enabled":` + strconvBool(enabled) + `}`)}, nil
	})
	return authorityRefusal(applied, err)
}

func strconvBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

// authorityRefusal turns a receipt's recorded refusal back into an error. The
// store keeps a refusal as a receipt rather than an error, which is right for
// the journal and wrong for a caller that must not treat it as success.
func authorityRefusal(result local.AuthorityApplyResult, err error) error {
	if err != nil {
		return err
	}
	if refused := result.Receipt.Rejection; refused != nil {
		return local.Reject(refused.Code, refused.Message)
	}
	return nil
}

func scheduleRecordFrom(e *Engine, s local.AuthoritySnapshot) (ScheduleRecord, error) {
	record := ScheduleRecord{SchemaVersion: AuthoritySchedulesVersion, AuthorityID: e.Installation.ID, Schedules: []ScheduleEntry{}}
	if s.Version == 0 {
		return record, nil
	}
	if err := decode(s.Data, &record); err != nil {
		return ScheduleRecord{}, err
	}
	if record.SchemaVersion != AuthoritySchedulesVersion || record.AuthorityID != e.Installation.ID {
		return ScheduleRecord{}, errors.New("unsupported or foreign schedule record")
	}
	return record, nil
}

// liveScheduleRuns counts the Runs this schedule started that have not settled.
// It reads the Runs themselves rather than trusting the ledger's word for it:
// what "still running" means is the Run's answer, not the schedule's.
func (e *Engine) liveScheduleRuns(ctx context.Context, entry ScheduleEntry) (map[string]bool, error) {
	live := map[string]bool{}
	for _, decision := range entry.Slots {
		if decision.RunID == "" || live[decision.RunID] {
			continue
		}
		r, _, err := e.load(ctx, decision.RunID)
		if errors.Is(err, local.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !r.terminal() {
			live[decision.RunID] = true
		}
	}
	return live, nil
}

// trimLedger keeps the ledger bounded without ever dropping a decision that is
// still answerable: a live Run's slot and an unfinished firing stay. Overlap is
// capped at 100 by the contract, so what must be kept always fits.
func trimLedger(entry *ScheduleEntry, live map[string]bool) {
	for i := 0; len(entry.Slots) > MaxSlotLedger && i < len(entry.Slots); {
		decision := entry.Slots[i]
		if decision.Disposition == "firing" || live[decision.RunID] {
			i++
			continue
		}
		entry.Slots = append(entry.Slots[:i], entry.Slots[i+1:]...)
	}
}

// recordSlotDecision writes one slot's decision and moves the watermark past it
// in the same transaction. Those two facts cannot be separated: a decision that
// did not move the watermark would be taken again, and a watermark that moved
// without a decision would hide what happened.
func (e *Engine) recordSlotDecision(ctx context.Context, version int64, id string, decision SlotDecision, live map[string]bool) error {
	payload, err := canonical(map[string]any{"operation": "schedule.slot_decided", "schedule_id": id, "slot_id": decision.SlotID,
		"instant_utc": decision.InstantUTC, "disposition": decision.Disposition, "reason": decision.Reason, "run_id": decision.RunID})
	if err != nil {
		return err
	}
	commandID := derivedID("command", "schedule.slot", id, decision.SlotID, decision.Disposition)
	applied, err := e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: commandID, Actor: e.owner, Key: AuthoritySchedulesKey, Payload: payload, ExpectedVersion: &version}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
		record, err := scheduleRecordFrom(e, s)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		entry := record.entry(id)
		if entry == nil {
			return local.AuthorityChange{}, local.Reject("not_found", "no schedule of that identity exists")
		}
		if existing := entry.decision(decision.SlotID); existing != nil {
			return local.AuthorityChange{}, local.Reject("slot_decided", "this logical slot already has its one decision")
		}
		watermark, err := time.Parse(time.RFC3339Nano, entry.DecidedThrough)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		instant, err := time.Parse(time.RFC3339Nano, decision.InstantUTC)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		if !instant.After(watermark) {
			return local.AuthorityChange{}, local.Reject("slot_decided", "this logical slot is already behind the watermark")
		}
		entry.Slots = append(entry.Slots, decision)
		entry.DecidedThrough = decision.InstantUTC
		trimLedger(entry, live)
		data, err := canonicalState(record)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		return local.AuthorityChange{Data: data, Result: json.RawMessage(`{"disposition":"` + decision.Disposition + `"}`)}, nil
	})
	return authorityRefusal(applied, err)
}

// confirmSlotRun records that the reserved Run now exists. Until it does, the
// slot reads as firing, which is the honest word for a Run that was promised
// and whose creation has not come back yet.
func (e *Engine) confirmSlotRun(ctx context.Context, version int64, id, slot, runID string) error {
	payload, err := canonical(map[string]any{"operation": "schedule.slot_fired", "schedule_id": id, "slot_id": slot, "run_id": runID})
	if err != nil {
		return err
	}
	commandID := derivedID("command", "schedule.fired", id, slot)
	applied, err := e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: commandID, Actor: e.owner, Key: AuthoritySchedulesKey, Payload: payload, ExpectedVersion: &version}, func(s local.AuthoritySnapshot) (local.AuthorityChange, error) {
		record, err := scheduleRecordFrom(e, s)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		entry := record.entry(id)
		if entry == nil {
			return local.AuthorityChange{}, local.Reject("not_found", "no schedule of that identity exists")
		}
		decision := entry.decision(slot)
		if decision == nil || decision.Disposition != "firing" || decision.RunID != runID {
			return local.AuthorityChange{}, local.Reject("slot_conflict", "this slot is not the one waiting for its run")
		}
		decision.Disposition = "fired"
		data, err := canonicalState(record)
		if err != nil {
			return local.AuthorityChange{}, err
		}
		return local.AuthorityChange{Data: data, Result: json.RawMessage(`{"run_id":"` + runID + `"}`)}, nil
	})
	return authorityRefusal(applied, err)
}

// startSlotRun creates the ordinary scoped Run for one slot. It goes through the
// same Start path as a hand-typed run, so the same admission, control stop,
// capacity and pinning apply. A schedule is a reason to start work, never a way
// around the checks that decide whether the work may start.
func (e *Engine) startSlotRun(ctx context.Context, s Schedule, inputs ScheduleInputs, commandID string) error {
	path, err := e.definitionPath(s.WorkflowRef)
	if err != nil {
		return err
	}
	brief, err := canonical(inputs.Brief)
	if err != nil {
		return err
	}
	values := map[string]json.RawMessage{}
	for name, value := range inputs.Inputs {
		encoded, err := canonical(value)
		if err != nil {
			return err
		}
		values[name] = encoded
	}
	_, err = e.Start(ctx, StartOptions{CommandID: commandID, WorkflowFile: path, Brief: brief, InputValues: values})
	return err
}

// FireDueSlots decides every logical slot this schedule has reached and not yet
// decided, oldest first, and returns what it decided. Nothing fires by itself:
// this call is the only thing that makes a schedule act.
func (e *Engine) FireDueSlots(ctx context.Context, id string) ([]SlotDecision, error) {
	if e.ReadOnly {
		return nil, local.ErrReadOnly
	}
	record, _, err := e.readSchedules(ctx)
	if err != nil {
		return nil, err
	}
	entry := record.entry(id)
	if entry == nil {
		return nil, local.Reject("not_found", "no schedule of that identity exists")
	}
	if !entry.Schedule.Enabled {
		return nil, local.Reject("schedule_disabled", "a disabled schedule decides nothing")
	}
	calendar, inputs, err := e.scheduleDocuments(entry.Schedule)
	if err != nil {
		return nil, err
	}
	decided := []SlotDecision{}
	// One iteration decides at most one slot, so the whole backlog a single call
	// may take on is bounded by the same number that bounds the backlog itself.
	for range MaxDueSlots + 1 {
		record, version, err := e.readSchedules(ctx)
		if err != nil {
			return decided, err
		}
		entry := record.entry(id)
		if entry == nil {
			return decided, local.Reject("not_found", "the schedule disappeared under this call")
		}
		live, err := e.liveScheduleRuns(ctx, *entry)
		if err != nil {
			return decided, err
		}
		if pending := entry.pending(); pending != nil {
			// A slot was reserved and its Run was not confirmed. Finishing it
			// uses the same derived command, so the authority either creates the
			// Run or hands back the receipt for the one it already created.
			if err := e.startSlotRun(ctx, entry.Schedule, inputs, pending.CommandID); err != nil {
				return decided, err
			}
			finished := *pending
			finished.Disposition = "fired"
			if err := e.confirmSlotRun(ctx, version, id, pending.SlotID, pending.RunID); err != nil {
				return decided, err
			}
			// The caller is told what this call finished, not only what it
			// started: a slot recovered here is a run that exists because of it.
			decided = append(decided, finished)
			continue
		}
		watermark, err := time.Parse(time.RFC3339Nano, entry.DecidedThrough)
		if err != nil {
			return decided, err
		}
		obs := e.clock.now()
		now, err := time.Parse(time.RFC3339Nano, obs.UTC)
		if err != nil {
			return decided, err
		}
		due, err := dueSlots(entry.Schedule, calendar, watermark, now)
		if err != nil {
			return decided, err
		}
		if len(due) == 0 {
			return decided, nil
		}
		next := due[0]
		disposition, reason := slotDisposition(entry.Schedule, len(due))
		decision := SlotDecision{SlotID: slotID(id, next.localTime, next.fold), LocalTime: next.localTime,
			InstantUTC: next.instant.Format(time.RFC3339Nano), Fold: next.fold, Disposition: disposition, Reason: reason, Decided: obs}
		if disposition == "firing" && len(live) >= entry.Schedule.MaxOverlap {
			// The schedule is already holding as many live Runs as it declared
			// it would. The slot is decided rather than deferred: deferring it
			// would fire it later, which is the duplicate in time the whole
			// watermark exists to prevent.
			decision.Disposition, decision.Reason = "refused", "max_overlap"
		}
		if decision.Disposition == "firing" {
			decision.CommandID = scheduleCommandID(id, decision.SlotID)
			decision.RunID = startRunID(e.owner, decision.CommandID)
		}
		if err := e.recordSlotDecision(ctx, version, id, decision, live); err != nil {
			return decided, err
		}
		decided = append(decided, decision)
		if decision.Disposition != "firing" {
			continue
		}
		if err := e.startSlotRun(ctx, entry.Schedule, inputs, decision.CommandID); err != nil {
			return decided, err
		}
		if err := e.confirmSlotRun(ctx, version+1, id, decision.SlotID, decision.RunID); err != nil {
			return decided, err
		}
		decided[len(decided)-1].Disposition = "fired"
	}
	return decided, local.Reject("schedule_backlog", fmt.Sprintf("this schedule is further behind than one call may decide (%d slots)", MaxDueSlots))
}

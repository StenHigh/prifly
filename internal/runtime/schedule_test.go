package runtime

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

// registerScheduleDocument writes one document and registers it under its exact
// digest, which is how a schedule pins what it points at: an edited calendar is
// a different reference, not a quietly different meaning for the same one.
func registerScheduleDocument(t *testing.T, e *Engine, id, kind, path string, value any) flow.Ref {
	t.Helper()
	body, err := canonical(value)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := flow.Digest(body)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(filepath.Join(e.Root, path)), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.Root, path), body, 0600); err != nil {
		t.Fatal(err)
	}
	return registerScheduleEntry(t, e, flow.Ref{ID: id, Version: "1.0.0", Digest: digest}, kind, path)
}

func registerScheduleEntry(t *testing.T, e *Engine, ref flow.Ref, kind, path string) flow.Ref {
	t.Helper()
	registryBytes, err := os.ReadFile(filepath.Join(e.Root, e.Config.Configuration.RegistryFile))
	var registry RegistryFile
	if err != nil || json.Unmarshal(registryBytes, &registry) != nil {
		t.Fatal("read registry", err)
	}
	registry.Entries = append(registry.Entries, Definition{Ref: ref, Kind: kind, Path: path})
	writeRuntimeJSON(t, filepath.Join(e.Root, e.Config.Configuration.RegistryFile), registry)
	return ref
}

// scheduleFixture registers the empty workflow, a calendar and the pinned brief
// plus inputs, then hands back a schedule that has not been created yet.
func scheduleFixture(t *testing.T, times []string, bend func(*Schedule)) (*Engine, Schedule) {
	t.Helper()
	e, options := emptyRuntime(t)
	ctx := context.Background()
	plan, _, _, _, err := e.compileFile(options.WorkflowFile)
	if err != nil {
		t.Fatal(err)
	}
	workflowRef := registerScheduleEntry(t, e, planRef(plan), "workflow", options.WorkflowFile)
	calendarRef := registerScheduleDocument(t, e, "test:calendar/daily", "resource", "schedules/daily.json",
		ScheduleCalendar{SchemaVersion: ScheduleCalendarVersion, DailyLocalTimes: times})
	briefBytes, err := os.ReadFile(filepath.Join(e.Root, options.BriefFile))
	if err != nil {
		t.Fatal(err)
	}
	var brief Brief
	if err := decode(briefBytes, &brief); err != nil {
		t.Fatal(err)
	}
	inputsRef := registerScheduleDocument(t, e, "test:schedule-inputs/empty", "resource", "schedules/inputs.json",
		ScheduleInputs{SchemaVersion: ScheduleInputsVersion, Brief: brief, Inputs: map[string]any{}})
	grant := issuedGrant(t, e, ctx, 1)
	s := Schedule{SchemaVersion: ScheduleVersion, ID: "test:schedule/nightly", WorkflowRef: workflowRef,
		InputMapperRef: inputsRef, Timezone: "America/New_York", CalendarRef: calendarRef,
		DSTGap: "next_valid", DSTFold: "first", Misfire: "bounded_catch_up", MaxCatchUp: 1000,
		MaxOverlap: 100, GrantRefs: []string{grant.Grant.ID}, Enabled: true}
	if bend != nil {
		bend(&s)
	}
	return e, s
}

func createdSchedule(t *testing.T, times []string, bend func(*Schedule)) (*Engine, Schedule) {
	t.Helper()
	e, s := scheduleFixture(t, times, bend)
	if _, err := e.CreateSchedule(context.Background(), ScheduleRequest{CommandID: "command:schedule", Schedule: s}); err != nil {
		t.Fatal(err)
	}
	return e, s
}

// backdateSchedule moves the watermark into the past. For a build that owns no
// timer, the watermark is exactly what "time has passed without anyone asking"
// means, so moving it is how a missed backlog is reproduced.
func backdateSchedule(t *testing.T, e *Engine, id string, through time.Time) {
	t.Helper()
	ctx := context.Background()
	record, version, err := e.readSchedules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	entry := record.entry(id)
	if entry == nil {
		t.Fatal("the fixture schedule is absent")
	}
	entry.DecidedThrough = through.UTC().Format(time.RFC3339Nano)
	data, err := canonicalState(record)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := canonical(map[string]any{"operation": "test.backdate", "schedule_id": id})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Store.ApplyAuthority(ctx, local.AuthorityCommand{ID: newID("command"), Actor: e.owner, Key: AuthoritySchedulesKey, Payload: payload, ExpectedVersion: &version}, func(local.AuthoritySnapshot) (local.AuthorityChange, error) {
		return local.AuthorityChange{Data: data}, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func countDispositions(decisions []SlotDecision) map[string]int {
	counts := map[string]int{}
	for _, decision := range decisions {
		counts[decision.Disposition]++
	}
	return counts
}

// AC-054. A logical slot fires at most once, and how many times the process
// that decides it has started is not one of the inputs.
func TestScheduleSlotFiresOnceAcrossRestarts(t *testing.T) {
	e, s := createdSchedule(t, []string{"09:00", "21:00"}, nil)
	ctx := context.Background()
	backdateSchedule(t, e, s.ID, time.Now().Add(-72*time.Hour))
	first, err := e.FireDueSlots(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	fired := countDispositions(first)["fired"]
	if fired < 2 {
		t.Fatalf("three days of a twice-daily calendar produced %d runs: %+v", fired, first)
	}
	identities, runs := map[string]bool{}, map[string]bool{}
	for _, decision := range first {
		if identities[decision.SlotID] {
			t.Fatalf("the same logical slot was decided twice: %s", decision.SlotID)
		}
		identities[decision.SlotID] = true
		if decision.RunID != "" {
			if runs[decision.RunID] {
				t.Fatalf("two slots claim the same run: %s", decision.RunID)
			}
			runs[decision.RunID] = true
		}
	}
	if len(runs) != fired {
		t.Fatalf("%d fired slots produced %d runs", fired, len(runs))
	}
	// Asking again changes nothing: every slot up to now is already decided.
	again, err := e.FireDueSlots(ctx, s.ID)
	if err != nil || len(again) != 0 {
		t.Fatalf("a second call decided slots again: %v %+v", err, again)
	}
	// A restart re-derives the same logical slots and finds them behind the
	// watermark. This is the whole of AC-054's restart requirement.
	restarted, err := Open(e.Root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	afterRestart, err := restarted.FireDueSlots(ctx, s.ID)
	if err != nil || len(afterRestart) != 0 {
		t.Fatalf("a restart decided already decided slots: %v %+v", err, afterRestart)
	}
	record, err := restarted.Schedules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	entry := record.entry(s.ID)
	if entry == nil {
		t.Fatal("the schedule did not survive the restart")
	}
	for _, decision := range entry.Slots {
		if decision.Disposition == "firing" {
			t.Fatalf("a slot is still claiming a run that was never created: %+v", decision)
		}
	}
}

// Creating a schedule is not running one. Its watermark starts where it was
// created, so nothing already in the calendar's past is due.
func TestScheduleCreationStartsNothing(t *testing.T) {
	e, s := createdSchedule(t, []string{"00:00", "12:00"}, nil)
	ctx := context.Background()
	decisions, err := e.FireDueSlots(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(decisions) != 0 {
		t.Fatalf("creating a schedule fired its calendar's past: %+v", decisions)
	}
	snapshots, _, err := e.Store.ReadAll(ctx, 64)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 0 {
		t.Fatalf("creating a schedule created %d runs", len(snapshots))
	}
}

func TestDisabledScheduleDecidesNothingAndKeepsItsBacklog(t *testing.T) {
	e, s := createdSchedule(t, []string{"09:00"}, func(s *Schedule) { s.Enabled = false })
	ctx := context.Background()
	backdateSchedule(t, e, s.ID, time.Now().Add(-48*time.Hour))
	_, err := e.FireDueSlots(ctx, s.ID)
	rejectionCode(t, err, "schedule_disabled")
	if err := e.SetScheduleEnabled(ctx, "command:enable", s.ID, true); err != nil {
		t.Fatal(err)
	}
	// Disabling did not move the watermark, so the backlog is still there and
	// the declared misfire policy is what decides it.
	decisions, err := e.FireDueSlots(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if countDispositions(decisions)["fired"] == 0 {
		t.Fatalf("re-enabling lost the backlog the schedule kept: %+v", decisions)
	}
	if err := e.SetScheduleEnabled(ctx, "command:disable", s.ID, false); err != nil {
		t.Fatal(err)
	}
	record, err := e.Schedules(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if record.entry(s.ID).Schedule.Enabled {
		t.Fatal("the schedule stayed enabled after being disabled")
	}
}

// Coalesce runs the backlog once. Every missed slot still gets its own recorded
// decision, because "we chose not to run it" is a fact about that slot.
func TestScheduleCoalesceStartsOneRunForTheBacklog(t *testing.T) {
	e, s := createdSchedule(t, []string{"09:00", "21:00"}, func(s *Schedule) { s.Misfire, s.MaxCatchUp = "coalesce", 0 })
	ctx := context.Background()
	backdateSchedule(t, e, s.ID, time.Now().Add(-72*time.Hour))
	decisions, err := e.FireDueSlots(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	counts := countDispositions(decisions)
	if counts["fired"] != 1 || counts["coalesced"] < 2 {
		t.Fatalf("coalesce did not collapse the backlog into one run: %+v", counts)
	}
	if decisions[len(decisions)-1].Disposition != "fired" {
		t.Fatalf("coalesce ran an older slot than the current one: %+v", decisions[len(decisions)-1])
	}
}

func TestScheduleSkipDropsMissedSlots(t *testing.T) {
	e, s := createdSchedule(t, []string{"09:00", "21:00"}, func(s *Schedule) { s.Misfire, s.MaxCatchUp = "skip", 0 })
	ctx := context.Background()
	backdateSchedule(t, e, s.ID, time.Now().Add(-72*time.Hour))
	decisions, err := e.FireDueSlots(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	counts := countDispositions(decisions)
	if counts["fired"] != 1 || counts["skipped"] < 2 {
		t.Fatalf("skip did not drop the missed slots: %+v", counts)
	}
	for _, decision := range decisions[:len(decisions)-1] {
		if decision.Reason != "misfire_skipped" {
			t.Fatalf("a dropped slot was recorded without saying why: %+v", decision)
		}
	}
}

// The catch-up budget counts missed slots only, so a bounded schedule starts one
// run for the current slot plus at most max_catch_up for what it missed.
func TestScheduleBoundedCatchUpStopsAtItsBudget(t *testing.T) {
	e, s := createdSchedule(t, []string{"09:00", "21:00"}, func(s *Schedule) { s.MaxCatchUp = 2 })
	ctx := context.Background()
	backdateSchedule(t, e, s.ID, time.Now().Add(-96*time.Hour))
	decisions, err := e.FireDueSlots(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	counts := countDispositions(decisions)
	if counts["fired"] != 3 {
		t.Fatalf("a budget of two missed slots produced %d runs: %+v", counts["fired"], counts)
	}
	for _, decision := range decisions[:len(decisions)-3] {
		if decision.Disposition != "skipped" || decision.Reason != "catch_up_exhausted" {
			t.Fatalf("an over-budget slot was not recorded as such: %+v", decision)
		}
	}
}

// Overlap is refused, not deferred: a deferred slot would fire later, which is
// the duplicate in time the watermark exists to prevent.
func TestScheduleMaxOverlapRefusesRatherThanDefers(t *testing.T) {
	e, s := createdSchedule(t, []string{"09:00", "21:00"}, func(s *Schedule) { s.MaxOverlap = 1 })
	ctx := context.Background()
	backdateSchedule(t, e, s.ID, time.Now().Add(-72*time.Hour))
	decisions, err := e.FireDueSlots(ctx, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	counts := countDispositions(decisions)
	if counts["fired"] != 1 || counts["refused"] == 0 {
		t.Fatalf("one live run did not close the overlap budget: %+v", counts)
	}
	for _, decision := range decisions {
		if decision.Disposition == "refused" && decision.Reason != "max_overlap" {
			t.Fatalf("a refusal did not say what refused it: %+v", decision)
		}
	}
	// A refused slot is decided, so asking again does not run it after all.
	again, err := e.FireDueSlots(ctx, s.ID)
	if err != nil || len(again) != 0 {
		t.Fatalf("a refused slot came back: %v %+v", err, again)
	}
}

// The two days a year a wall time is not one instant. The declared policy is
// what answers it; nothing here falls back to a library default.
func TestScheduleResolvesDaylightGapAndFold(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	gapDay := time.Date(2026, time.March, 8, 12, 0, 0, 0, loc)
	foldDay := time.Date(2026, time.November, 1, 12, 0, 0, 0, loc)
	base := Schedule{DSTGap: "next_valid", DSTFold: "both"}
	gapCalendar := ScheduleCalendar{SchemaVersion: ScheduleCalendarVersion, DailyLocalTimes: []string{"02:30"}}
	foldCalendar := ScheduleCalendar{SchemaVersion: ScheduleCalendarVersion, DailyLocalTimes: []string{"01:30"}}

	skipped := base
	skipped.DSTGap = "skip"
	slots, err := daySlots(skipped, gapCalendar, loc, gapDay)
	if err != nil || len(slots) != 0 {
		t.Fatalf("skip invented an instant for a wall time that never happened: %v %+v", err, slots)
	}
	slots, err = daySlots(base, gapCalendar, loc, gapDay)
	if err != nil || len(slots) != 1 || slots[0].instant.UTC().Format(time.RFC3339) != "2026-03-08T07:30:00Z" {
		t.Fatalf("next_valid did not resolve the gap to the instant a running clock would reach: %v %+v", err, slots)
	}

	slots, err = daySlots(base, foldCalendar, loc, foldDay)
	if err != nil || len(slots) != 2 || slots[0].fold != 0 || slots[1].fold != 1 {
		t.Fatalf("fold both did not produce both occurrences: %v %+v", err, slots)
	}
	if slots[0].instant.UTC().Format(time.RFC3339) != "2026-11-01T05:30:00Z" || slots[1].instant.UTC().Format(time.RFC3339) != "2026-11-01T06:30:00Z" {
		t.Fatalf("the two occurrences are not the two real instants: %+v", slots)
	}
	if slotID("s", slots[0].localTime, slots[0].fold) == slotID("s", slots[1].localTime, slots[1].fold) {
		t.Fatal("the two occurrences of one wall time share a logical slot identity")
	}
	for policy, want := range map[string]string{"first": "2026-11-01T05:30:00Z", "second": "2026-11-01T06:30:00Z"} {
		chosen := base
		chosen.DSTFold = policy
		slots, err := daySlots(chosen, foldCalendar, loc, foldDay)
		if err != nil || len(slots) != 1 || slots[0].instant.UTC().Format(time.RFC3339) != want {
			t.Fatalf("dst_fold %s chose %v %+v", policy, err, slots)
		}
	}
}

func TestScheduleRefusesContractsItCannotKeep(t *testing.T) {
	e, s := scheduleFixture(t, []string{"09:00"}, nil)
	ctx := context.Background()
	for name, bend := range map[string]func(*Schedule){
		"unknown gap policy":  func(s *Schedule) { s.DSTGap = "nearest" },
		"unknown fold policy": func(s *Schedule) { s.DSTFold = "either" },
		"unknown misfire":     func(s *Schedule) { s.Misfire = "queue" },
		"overlap below one":   func(s *Schedule) { s.MaxOverlap = 0 },
		"catch-up over cap":   func(s *Schedule) { s.MaxCatchUp = 1001 },
		"unknown timezone":    func(s *Schedule) { s.Timezone = "Mars/Olympus" },
		"no grants":           func(s *Schedule) { s.GrantRefs = []string{} },
		"unknown grant":       func(s *Schedule) { s.GrantRefs = []string{"grant:absent"} },
	} {
		candidate := s
		bend(&candidate)
		if _, err := e.CreateSchedule(ctx, ScheduleRequest{CommandID: "command:" + name, Schedule: candidate}); err == nil {
			t.Fatalf("a schedule with an %s was accepted", name)
		}
	}
	if _, err := e.CreateSchedule(ctx, ScheduleRequest{CommandID: "command:schedule", Schedule: s}); err != nil {
		t.Fatal(err)
	}
	// A second schedule of the same identity would move a new calendar under an
	// existing watermark, which would change what already decided slots meant.
	_, err := e.CreateSchedule(ctx, ScheduleRequest{CommandID: "command:again", Schedule: s})
	rejectionCode(t, err, "schedule_present")
}

func TestScheduleCapabilityIsReported(t *testing.T) {
	manifest := Capabilities()
	core := manifest.Profiles[1]
	found := false
	for _, capability := range core.Capabilities {
		if capability == "schedule" {
			found = true
		}
	}
	for _, capability := range manifest.Unsupported {
		if capability == "schedule" {
			t.Fatal("the manifest calls an implemented schedule contract unsupported")
		}
	}
	if !found {
		t.Fatal("the manifest omits the implemented schedule contract")
	}
}

// A watermark ahead of the clock is a clock that went backwards, not a backlog.
func TestScheduleClockRollbackDecidesNothing(t *testing.T) {
	e, s := createdSchedule(t, []string{"09:00"}, nil)
	ctx := context.Background()
	backdateSchedule(t, e, s.ID, time.Now().Add(48*time.Hour))
	decisions, err := e.FireDueSlots(ctx, s.ID)
	if err != nil || len(decisions) != 0 {
		t.Fatalf("a rolled back clock re-decided slots: %v %+v", err, decisions)
	}
}

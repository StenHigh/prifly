package runtime

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
)

const (
	AuthoritySchedulesKey     = "schedules"
	AuthoritySchedulesVersion = "authority-schedules/1"
	// ScheduleVersion is the published Schedule contract this build writes. The
	// shape is not this slice's to choose: it was delivered in the protocol
	// schema and is implemented as it stands.
	ScheduleVersion = "1"
	// ScheduleCalendarVersion and ScheduleInputsVersion pin the two documents a
	// schedule points at. They are ordinary registered resources, so a changed
	// calendar is a changed digest and therefore a different schedule input.
	ScheduleCalendarVersion = "schedule-calendar/1"
	ScheduleInputsVersion   = "schedule-inputs/1"

	// MaxSchedules is what one installation may hold. A schedule is a standing
	// permission to create Runs, so the number of them is stated rather than
	// discovered when the authority state stops fitting.
	MaxSchedules = 64
	// MaxCalendarTimes bounds one calendar's daily wall-clock times. Sixty-four
	// is far past any human schedule and keeps a day's expansion small enough
	// that the backlog bound below is the one that actually binds.
	MaxCalendarTimes = 64
	// MaxDueSlots bounds one call's backlog. The contract caps max_catch_up at
	// 1000, so 1001 is the largest backlog any legal policy could act on; a
	// larger one is refused rather than silently truncated, because dropping
	// slots to fit a bound would be a decision nobody recorded.
	MaxDueSlots = 1001
	// MaxSlotLedger is how many decisions one schedule keeps as evidence. It is
	// not what makes a slot fire once - the watermark does that - so trimming it
	// costs history, never dedup.
	MaxSlotLedger = 256
	// zoneProbeWindow brackets a wall time with the offsets in force a day
	// either side of it. No tzdata zone changes its offset twice within
	// forty-eight hours, so those two offsets are the only candidates.
	zoneProbeWindow = 24 * time.Hour
)

// Schedule is the published contract exactly. It is a project-level record, not
// part of any Run: it says which workflow to start, with which pinned inputs, on
// which calendar, in which timezone, and under which limits.
type Schedule struct {
	SchemaVersion  string   `json:"schema_version"`
	ID             string   `json:"id"`
	WorkflowRef    flow.Ref `json:"workflow_ref"`
	InputMapperRef flow.Ref `json:"input_mapper_ref"`
	Timezone       string   `json:"timezone"`
	CalendarRef    flow.Ref `json:"calendar_ref"`
	// DSTGap and DSTFold say what a local wall time means on the two days a year
	// it is not a single instant. Leaving that to a library default would make
	// the answer depend on which library, which is exactly what pinning avoids.
	DSTGap  string `json:"dst_gap"`
	DSTFold string `json:"dst_fold"`
	Misfire string `json:"misfire"`
	// MaxCatchUp counts missed slots only. The current slot is not catch-up, so
	// a schedule with max_catch_up 0 still fires the slot that has just come due.
	MaxCatchUp int `json:"max_catch_up"`
	MaxOverlap int `json:"max_overlap"`
	// GrantRefs name the grants this schedule was created under. This build's
	// Start path binds no grants to a Run, so they are checked to exist and
	// recorded; they do not narrow what the created Run may do. A Run created
	// here is scoped by the ordinary Start admission, nothing weaker.
	GrantRefs []string `json:"grant_refs"`
	Enabled   bool     `json:"enabled"`
}

// ScheduleCalendar is the pinned calendar semantics: a set of daily wall-clock
// times in the schedule's timezone. There is no cron parser, because a parser
// dialect is a semantics choice that then has to be pinned as well.
type ScheduleCalendar struct {
	SchemaVersion string `json:"schema_version"`
	// DailyLocalTimes are exact "HH:MM" 24-hour local times, every day.
	DailyLocalTimes []string `json:"daily_local_times"`
}

// ScheduleInputs is what a fired Run is started with. The brief lives here
// rather than being synthesised at fire time: the owner confirms once, when the
// schedule is created, and that confirmation is pinned by digest with the rest.
type ScheduleInputs struct {
	SchemaVersion string         `json:"schema_version"`
	Brief         Brief          `json:"brief"`
	Inputs        map[string]any `json:"inputs"`
}

// SlotDecision is what became of one logical slot. Every slot this schedule has
// reached has exactly one of these, including the ones that did not run: "we
// never reached it" and "we reached it and would not run it" are different facts.
type SlotDecision struct {
	// SlotID is derived from the schedule and the slot's own local date, time
	// and fold, so the same logical slot has the same identity in every process
	// that ever computes it. Nothing about a daemon's lifetime enters it.
	SlotID string `json:"slot_id"`
	// LocalTime is the full local wall time this slot stands for, not just the
	// calendar's time of day: nine on Tuesday is not nine on Wednesday.
	LocalTime  string `json:"local_time"`
	InstantUTC string `json:"instant_utc"`
	// Fold is 0 for a normal or first occurrence and 1 for the repeat of an
	// ambiguous local time under dst_fold both.
	Fold int `json:"fold"`
	// Disposition is one of firing, fired, skipped, coalesced or refused.
	Disposition string      `json:"disposition"`
	Reason      string      `json:"reason,omitempty"`
	RunID       string      `json:"run_id,omitempty"`
	CommandID   string      `json:"command_id,omitempty"`
	Decided     Observation `json:"decided"`
}

// ScheduleEntry is one schedule plus what the authority knows about it.
type ScheduleEntry struct {
	Schedule Schedule    `json:"schedule"`
	Created  Observation `json:"created"`
	// DecidedThrough is the watermark that makes a logical slot fire at most
	// once, ever. Every slot at or before it already has a decision, and it only
	// moves forward. A restart re-derives the same slots and finds them already
	// behind the watermark, so how many times a process started is not an input
	// to whether a slot runs.
	DecidedThrough string         `json:"decided_through"`
	Slots          []SlotDecision `json:"slots"`
}

type ScheduleRecord struct {
	SchemaVersion string          `json:"schema_version"`
	AuthorityID   string          `json:"authority_id"`
	Schedules     []ScheduleEntry `json:"schedules"`
}

func (r *ScheduleRecord) entry(id string) *ScheduleEntry {
	for i := range r.Schedules {
		if r.Schedules[i].Schedule.ID == id {
			return &r.Schedules[i]
		}
	}
	return nil
}

// pending is the slot whose Run was reserved but whose creation was not yet
// confirmed. It exists so that a crash between reserving a slot and creating its
// Run is finished on the next call rather than losing the slot in silence.
func (s *ScheduleEntry) pending() *SlotDecision {
	for i := range s.Slots {
		if s.Slots[i].Disposition == "firing" {
			return &s.Slots[i]
		}
	}
	return nil
}

func (s *ScheduleEntry) decision(slotID string) *SlotDecision {
	for i := range s.Slots {
		if s.Slots[i].SlotID == slotID {
			return &s.Slots[i]
		}
	}
	return nil
}

func slotID(scheduleID, localTime string, fold int) string {
	return derivedID("slot", scheduleID, localTime, strconv.Itoa(fold))
}

// scheduleCommandID is derived from the logical slot, so retrying an
// interrupted firing presents the authority the same command it already saw.
// That is what turns "at most once" into "exactly once once asked again".
func scheduleCommandID(scheduleID, slot string) string {
	return derivedID("command", "schedule.fire", scheduleID, slot)
}

// slot is one computed logical slot before any policy is applied to it.
type slot struct {
	localTime string
	instant   time.Time
	fold      int
}

// validateSchedule checks the record against its published contract and the
// parts of it a JSON schema cannot state: that the timezone loads, that the
// enumerated policies are the ones this build implements.
func validateSchedule(s Schedule) error {
	b, err := canonical(s)
	if err != nil {
		return err
	}
	if err := flow.ValidateProtocol("Schedule", b); err != nil {
		return err
	}
	if s.SchemaVersion != ScheduleVersion {
		return errors.New("unsupported schedule contract version")
	}
	if _, err := time.LoadLocation(s.Timezone); err != nil {
		return faultf("unknown_timezone", "%s is not a zone this build can resolve", s.Timezone)
	}
	return nil
}

func validateCalendar(c ScheduleCalendar) error {
	if c.SchemaVersion != ScheduleCalendarVersion {
		return errors.New("unsupported schedule calendar contract version")
	}
	if len(c.DailyLocalTimes) == 0 || len(c.DailyLocalTimes) > MaxCalendarTimes {
		return faultf("invalid_calendar", "a calendar states between 1 and %d daily local times", MaxCalendarTimes)
	}
	seen := map[string]bool{}
	for _, value := range c.DailyLocalTimes {
		if _, _, err := parseLocalTime(value); err != nil {
			return err
		}
		if seen[value] {
			return fault("invalid_calendar", "the same daily local time is stated twice")
		}
		seen[value] = true
	}
	return nil
}

// parseLocalTime accepts exactly "HH:MM". The narrow form is the pinned parser
// semantics: anything looser would leave the meaning of a calendar depending on
// which parser read it.
func parseLocalTime(value string) (hour, minute int, err error) {
	fields := strings.Split(value, ":")
	if len(fields) != 2 || len(fields[0]) != 2 || len(fields[1]) != 2 {
		return 0, 0, faultf("invalid_calendar", "%q is not an exact HH:MM local time", value)
	}
	hour, herr := strconv.Atoi(fields[0])
	minute, merr := strconv.Atoi(fields[1])
	if herr != nil || merr != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, faultf("invalid_calendar", "%q is not an exact HH:MM local time", value)
	}
	return hour, minute, nil
}

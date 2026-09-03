package runtime

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
	"github.com/stenhigh/prifly/internal/local"
)

const (
	TelemetryQueryVersion                 = "telemetry-query/1"
	TelemetryReportVersion                = "telemetry-report/1"
	TelemetryCalculatorRevision           = "foundation-telemetry/1"
	TelemetryCalculatorRevisionCore       = "core-telemetry/1"
	TelemetryCalculatorRevisionInvocation = "core-telemetry/2"
	TelemetryCalculatorRevisionContext    = "core-telemetry/3"
	TelemetryMaxRuns                      = 1000
	TelemetryMaxRecords                   = 100000
	TelemetryMaxGroups                    = 1000
	TelemetryMaxPage                      = 1000
	TelemetryMaxBytes                     = 64 << 20
	TelemetryMaxResponseBytes             = 8 << 20
	telemetryMaxNumber                    = float64(1<<53 - 1)
)

// TelemetrySampleData is an internal core observation, not a worker wire DTO.
// A saved measured sample is not a claim of complete interval coverage.
type TelemetrySampleData struct {
	SchemaVersion string      `json:"schema_version"`
	Metric        string      `json:"metric"`
	Value         *float64    `json:"value"`
	Unit          string      `json:"unit"`
	Method        string      `json:"method"`
	Quality       string      `json:"quality"`
	Coverage      string      `json:"coverage"`
	Reason        string      `json:"reason,omitempty"`
	Observed      Observation `json:"observed"`
	CommandID     string      `json:"command_id,omitempty"`
	AttemptID     string      `json:"attempt_id,omitempty"`
	Status        string      `json:"status,omitempty"`
	Code          string      `json:"code,omitempty"`
}

// RunStatus/RunOutcome select the Run cohort. All other filters select records
// inside that cohort; they never silently change Run reliability denominators.
type TelemetryFilters struct {
	RunStatus  []string                     `json:"run_status,omitempty"`
	RunOutcome []string                     `json:"run_outcome,omitempty"`
	Status     []string                     `json:"status,omitempty"`
	Verdict    []string                     `json:"verdict,omitempty"`
	Severity   []string                     `json:"severity,omitempty"`
	Origin     []string                     `json:"origin,omitempty"`
	Code       []string                     `json:"code,omitempty"`
	Scope      []string                     `json:"scope,omitempty"`
	WorkflowID []string                     `json:"workflow_id,omitempty"`
	StepID     []string                     `json:"step_id,omitempty"`
	Dimensions map[string][]json.RawMessage `json:"dimensions,omitempty"`
}

// CompletedFrom/Before is an explicit alternative to the creation cohort.
// EventFrom/Before filters timestamped observations, not Run creation or ratio
// denominators. Receipts have no invented timestamp; an event window excludes
// them with an explicit coverage count.
type TelemetryQuery struct {
	SchemaVersion   string           `json:"schema_version"`
	Mode            string           `json:"mode"`
	RunIDs          []string         `json:"run_ids,omitempty"`
	CreatedFrom     string           `json:"created_from,omitempty"`
	CreatedBefore   string           `json:"created_before,omitempty"`
	CompletedFrom   string           `json:"completed_from,omitempty"`
	CompletedBefore string           `json:"completed_before,omitempty"`
	EventFrom       string           `json:"event_from,omitempty"`
	EventBefore     string           `json:"event_before,omitempty"`
	Filters         TelemetryFilters `json:"filters,omitempty"`
	GroupBy         []string         `json:"group_by,omitempty"`
	Metrics         []string         `json:"metrics,omitempty"`
	Aggregations    []string         `json:"aggregations,omitempty"`
	Cut             *int64           `json:"cut,omitempty"`
	Cursor          string           `json:"cursor,omitempty"`
	Limit           int              `json:"limit,omitempty"`
}

func (q *TelemetryQuery) UnmarshalJSON(data []byte) error {
	if len(data) > 32<<10 {
		return errors.New("telemetry query exceeds 32 KiB")
	}
	b, err := flow.Canonical(data)
	if err != nil {
		return err
	}
	type query TelemetryQuery
	var value query
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&value); err != nil {
		return fmt.Errorf("invalid closed telemetry query: %w", err)
	}
	// encoding/json otherwise treats null slices/structs as their zero value,
	// silently broadening a malformed filter into an unfiltered query.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return err
	}
	if fields == nil {
		return telemetryProblem("invalid_query", "query must be an object")
	}
	for name, raw := range fields {
		if string(raw) == "null" && name != "cut" {
			return telemetryProblem("invalid_query", "null is not a value for "+name)
		}
	}
	if _, present := fields["limit"]; present && value.Limit == 0 {
		return telemetryProblem("invalid_query", "an explicit limit must be positive")
	}
	var filters map[string]json.RawMessage
	if raw, present := fields["filters"]; present {
		if err := json.Unmarshal(raw, &filters); err != nil {
			return err
		}
		for name, value := range filters {
			if string(value) == "null" {
				return telemetryProblem("invalid_filter", "null is not a value for "+name)
			}
		}
	}
	*q = TelemetryQuery(value)
	return nil
}

type TelemetryDescriptor struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Revision      string    `json:"revision"`
	Kind          string    `json:"kind"`
	Unit          string    `json:"unit"`
	Scope         string    `json:"scope"`
	Origin        string    `json:"origin"`
	Method        string    `json:"method"`
	Temporality   string    `json:"temporality"`
	Availability  string    `json:"availability"`
	DefinitionRef *flow.Ref `json:"definition_ref,omitempty"`
	Dimensions    []string  `json:"dimensions"`
	Aggregations  []string  `json:"aggregations"`
}

type TelemetrySubject struct {
	Kind           string `json:"kind"`
	ID             string `json:"id"`
	RunID          string `json:"run_id,omitempty"`
	StepInstanceID string `json:"step_instance_id,omitempty"`
	AttemptID      string `json:"attempt_id,omitempty"`
}

type TelemetryCoverage struct {
	Observed         int  `json:"observed"`
	Expected         *int `json:"expected"`
	ExpectedUnknown  bool `json:"expected_unknown"`
	Measured         int  `json:"measured"`
	Partial          int  `json:"partial"`
	Unavailable      int  `json:"unavailable"`
	Estimated        int  `json:"estimated"`
	NotApplicable    int  `json:"not_applicable"`
	Open             int  `json:"open"`
	MissingTimestamp int  `json:"missing_timestamp"`
	LossUnknown      bool `json:"loss_unknown"`
}

// Record is a safe projection. It never carries tokens, source files, raw
// stdout/stderr, result summaries, diagnostic text, or arbitrary hook payloads.
type TelemetryRecord struct {
	ID           string            `json:"id"`
	DescriptorID string            `json:"descriptor_id"`
	Metric       string            `json:"metric"`
	Subject      TelemetrySubject  `json:"subject"`
	Origin       string            `json:"origin"`
	Method       string            `json:"method"`
	Unit         string            `json:"unit"`
	Generation   string            `json:"generation"`
	Observed     *Observation      `json:"observed"`
	Value        *float64          `json:"value"`
	Integer      *int64            `json:"integer"`
	Quality      string            `json:"quality"`
	Coverage     string            `json:"coverage"`
	IsOpen       bool              `json:"is_open"`
	Reasons      []string          `json:"reasons"`
	Duration     *Duration         `json:"duration,omitempty"`
	Dimensions   map[string]string `json:"dimensions"`
	Evidence     []string          `json:"evidence"`
	Numerator    *int64            `json:"numerator,omitempty"`
	Denominator  *int64            `json:"denominator,omitempty"`
	// Order is authority acceptance order within a subject, never worker UTC.
	Order int64 `json:"order"`
}

type TelemetryRatio struct {
	Numerator   int64    `json:"numerator"`
	Denominator int64    `json:"denominator"`
	Value       *float64 `json:"value"`
	Quality     string   `json:"quality"`
}

type TelemetryPopulation struct {
	Basis                  string                    `json:"basis"`
	Matched                int                       `json:"matched"`
	Terminal               int                       `json:"terminal"`
	Open                   int                       `json:"open"`
	Uncertain              int                       `json:"uncertain"`
	Cancelled              int                       `json:"cancelled"`
	UnresolvedEffects      int                       `json:"unresolved_effects"`
	Invocations            int                       `json:"invocations"`
	Activations            int                       `json:"activations"`
	Steps                  int                       `json:"steps"`
	Attempts               int                       `json:"attempts"`
	StartedAttempts        int                       `json:"started_attempts"`
	SettledAttempts        int                       `json:"settled_attempts"`
	FirstAttemptsOpen      int                       `json:"first_attempts_open"`
	FullWarningCoverage    int                       `json:"full_warning_coverage"`
	UnknownWarningCoverage int                       `json:"unknown_warning_coverage"`
	WarnedIncomplete       int                       `json:"warned_incomplete"`
	RunStatus              map[string]int            `json:"run_status"`
	RunOutcome             map[string]int            `json:"run_outcome"`
	StepVerdict            map[string]int            `json:"step_verdict"`
	AttemptStatus          map[string]int            `json:"attempt_status"`
	Ratios                 map[string]TelemetryRatio `json:"ratios"`
}

type TelemetryAggregate struct {
	DescriptorID   string            `json:"descriptor_id"`
	Metric         string            `json:"metric"`
	Scope          string            `json:"scope"`
	Method         string            `json:"method"`
	Unit           string            `json:"unit"`
	Dimensions     map[string]string `json:"dimensions"`
	Quality        string            `json:"quality"`
	Coverage       TelemetryCoverage `json:"coverage"`
	N              int               `json:"n"`
	Sum            *float64          `json:"sum"`
	Min            *float64          `json:"min"`
	Max            *float64          `json:"max"`
	Mean           *float64          `json:"mean"`
	P50            *float64          `json:"p50"`
	P95            *float64          `json:"p95"`
	Last           *float64          `json:"last"`
	Total          *int64            `json:"total"`
	Delta          *int64            `json:"delta"`
	Ratio          *TelemetryRatio   `json:"ratio,omitempty"`
	QuantileMethod string            `json:"quantile_method,omitempty"`
	Reasons        []string          `json:"reasons"`
	Evidence       []string          `json:"evidence"`
}

type TelemetryAsOf struct {
	RunID         string      `json:"run_id"`
	RunVersion    int64       `json:"run_version"`
	EventSequence int64       `json:"event_sequence"`
	Observed      Observation `json:"observed"`
}

type TelemetryResponse struct {
	SchemaVersion      string                `json:"schema_version"`
	QueryVersion       string                `json:"query_version"`
	QueryDigest        string                `json:"query_digest"`
	AuthorityID        string                `json:"authority_id"`
	Principal          string                `json:"principal"`
	AccessScope        string                `json:"access_scope"`
	Cut                int64                 `json:"cut"`
	AsOf               []TelemetryAsOf       `json:"as_of"`
	AsOfBasis          string                `json:"as_of_basis"`
	CalculatorRevision string                `json:"calculator_revision"`
	TimingRevision     string                `json:"timing_revision"`
	CollectionProfile  string                `json:"collection_profile"`
	Retention          string                `json:"retention"`
	Population         TelemetryPopulation   `json:"population"`
	Descriptors        []TelemetryDescriptor `json:"descriptors"`
	Records            []TelemetryRecord     `json:"records"`
	Aggregates         []TelemetryAggregate  `json:"aggregates"`
	RecordCount        int                   `json:"record_count"`
	Coverage           TelemetryCoverage     `json:"coverage"`
	NextCursor         string                `json:"next_cursor,omitempty"`
	Warnings           []string              `json:"warnings"`
	Limits             map[string]int        `json:"limits"`
}

func telemetryProblem(code, message string) error {
	return &flow.Problem{Code: code, Path: "/telemetry", Message: message}
}
func telemetryIn(value string, values []string) bool {
	return len(values) == 0 || slices.Contains(values, value)
}
func telemetryPtr[T any](value T) *T { return &value }

func normalizeTelemetryQuery(q TelemetryQuery) (TelemetryQuery, error) {
	if q.SchemaVersion != TelemetryQueryVersion {
		return q, telemetryProblem("unsupported_version", "expected "+TelemetryQueryVersion)
	}
	if !slices.Contains([]string{"catalog", "records", "aggregate"}, q.Mode) {
		return q, telemetryProblem("unsupported", "only catalog, records and aggregate are supported")
	}
	if q.Limit == 0 {
		q.Limit = 100
		if q.Mode != "records" {
			q.Limit = TelemetryMaxPage
		}
	}
	if q.Limit < 1 || q.Limit > TelemetryMaxPage {
		return q, telemetryProblem("query_limit", "limit must be 1..1000")
	}
	if q.Cut != nil && (*q.Cut < 0 || *q.Cut > 1<<53-1) {
		return q, telemetryProblem("invalid_cut", "cut must be an exact nonnegative integer")
	}
	if len(q.Cursor) > 4096 || q.Cursor != "" && q.Mode != "records" {
		return q, telemetryProblem("invalid_cursor", "cursor is bounded and only supported for records")
	}
	if q.CreatedFrom != "" || q.CreatedBefore != "" {
		if q.CompletedFrom != "" || q.CompletedBefore != "" {
			return q, telemetryProblem("incompatible_windows", "creation and completion cohorts are alternatives")
		}
	}
	for _, window := range [][2]*string{{&q.CreatedFrom, &q.CreatedBefore}, {&q.CompletedFrom, &q.CompletedBefore}, {&q.EventFrom, &q.EventBefore}} {
		var times [2]time.Time
		for i, p := range window {
			if *p != "" {
				v, err := time.Parse(time.RFC3339Nano, *p)
				if err != nil {
					return q, telemetryProblem("invalid_window", "window boundary must be RFC3339")
				}
				times[i] = v
				*p = v.UTC().Format(time.RFC3339Nano)
			}
		}
		if !times[0].IsZero() && !times[1].IsZero() && !times[0].Before(times[1]) {
			return q, telemetryProblem("invalid_window", "window is a nonempty [from,before) interval")
		}
	}
	lists := []*[]string{&q.RunIDs, &q.GroupBy, &q.Metrics, &q.Aggregations, &q.Filters.RunStatus, &q.Filters.RunOutcome, &q.Filters.Status, &q.Filters.Verdict, &q.Filters.Severity, &q.Filters.Origin, &q.Filters.Code, &q.Filters.Scope, &q.Filters.WorkflowID, &q.Filters.StepID}
	for _, list := range lists {
		if len(*list) > 64 {
			return q, telemetryProblem("query_limit", "a selector may have at most 64 values")
		}
		v := slices.Clone(*list)
		sort.Strings(v)
		for i, item := range v {
			if item == "" || len(item) > 256 || strings.ContainsAny(item, "\x00\r\n") || i > 0 && v[i-1] == item {
				return q, telemetryProblem("invalid_selector", "selectors require unique bounded nonempty strings")
			}
		}
		*list = v
	}
	for _, status := range q.Filters.RunStatus {
		if !slices.Contains([]string{"created", "ready", "running", "waiting", "stopping", "uncertain", "completed", "failed", "cancelled"}, status) {
			return q, telemetryProblem("invalid_filter", "unknown Run status")
		}
	}
	for _, status := range q.Filters.Status {
		if !slices.Contains([]string{"created", "ready", "pending", "dispatching", "running", "waiting", "verifying", "stopping", "uncertain", "completed", "failed", "cancelled", "accepted", "rejected"}, status) {
			return q, telemetryProblem("invalid_filter", "unknown entity or receipt status")
		}
	}
	for _, verdict := range q.Filters.Verdict {
		if !slices.Contains([]string{"pass", "fail", "needs_revision", "no_work", "unknown"}, verdict) {
			return q, telemetryProblem("invalid_filter", "unknown verdict")
		}
	}
	for _, severity := range q.Filters.Severity {
		if !slices.Contains([]string{"trace", "debug", "info", "warn", "error", "fatal", "unknown"}, severity) {
			return q, telemetryProblem("invalid_filter", "unknown severity")
		}
	}
	for _, scope := range q.Filters.Scope {
		if !slices.Contains([]string{"run", "workflow_invocation", "stage_activation", "step_instance", "attempt", "check_execution", "command", "authority"}, scope) {
			return q, telemetryProblem("invalid_filter", "unsupported scope")
		}
	}
	for _, origin := range q.Filters.Origin {
		if !slices.Contains([]string{"core", "os", "worker-reported"}, origin) {
			return q, telemetryProblem("invalid_filter", "unsupported origin")
		}
	}
	if len(q.Filters.Dimensions) > 8 || len(q.GroupBy) > 8 {
		return q, telemetryProblem("query_limit", "at most 8 custom filters/grouping dimensions")
	}
	dims := map[string][]json.RawMessage{}
	for key, values := range q.Filters.Dimensions {
		if key == "" || len(key) > 64 || len(values) == 0 || len(values) > 32 {
			return q, telemetryProblem("invalid_filter", "custom dimension requires a name and 1..32 scalar values")
		}
		v := make([]json.RawMessage, 0, len(values))
		for _, item := range values {
			b, err := flow.Canonical(item)
			if err != nil || len(b) > 128 || len(b) == 0 || b[0] == '{' || b[0] == '[' || string(b) == "null" {
				return q, telemetryProblem("invalid_filter", "custom filters require bounded string, boolean or numeric scalars")
			}
			v = append(v, b)
		}
		sort.Slice(v, func(i, j int) bool { return string(v[i]) < string(v[j]) })
		for i := 1; i < len(v); i++ {
			if bytes.Equal(v[i-1], v[i]) {
				return q, telemetryProblem("invalid_filter", "duplicate dimension value")
			}
		}
		dims[key] = v
	}
	q.Filters.Dimensions = dims
	for _, group := range q.GroupBy {
		if !slices.Contains(telemetryDimensions, group) && !slices.Contains(telemetryCheckDimensions, group) && !strings.HasPrefix(group, "custom.") {
			return q, telemetryProblem("invalid_grouping", "unsupported grouping dimension: "+group)
		}
	}
	for _, aggregation := range q.Aggregations {
		if !slices.Contains([]string{"count", "sum", "min", "max", "mean", "p50", "p95", "last", "total", "delta", "ratio"}, aggregation) {
			return q, telemetryProblem("invalid_aggregation", "unsupported aggregation: "+aggregation)
		}
	}
	if q.Mode != "aggregate" && (len(q.GroupBy) > 0 || len(q.Aggregations) > 0) {
		return q, telemetryProblem("invalid_query", "grouping and aggregation are only for aggregate mode")
	}
	if q.Mode == "catalog" && (telemetryHasRecordFilters(q.Filters) || q.EventFrom != "" || q.EventBefore != "") {
		return q, telemetryProblem("invalid_query", "catalog does not accept observation filters or event windows")
	}
	return q, nil
}

var telemetryDimensions = []string{"run_status", "run_outcome", "status", "verdict", "severity", "origin", "code", "workflow_id", "workflow_revision", "step_id", "step_revision", "stage_id", "core_build", "profile", "executor_id", "scope", "resource_scope"}

func telemetryHasRecordFilters(f TelemetryFilters) bool {
	return len(f.Status)+len(f.Verdict)+len(f.Severity)+len(f.Origin)+len(f.Code)+len(f.Scope)+len(f.StepID)+len(f.Dimensions) > 0
}
func telemetryWindow(utc, from, before string) bool {
	if from == "" && before == "" {
		return true
	}
	v, err := time.Parse(time.RFC3339Nano, utc)
	if err != nil {
		return false
	}
	if from != "" {
		a, _ := time.Parse(time.RFC3339Nano, from)
		if v.Before(a) {
			return false
		}
	}
	if before != "" {
		b, _ := time.Parse(time.RFC3339Nano, before)
		if !v.Before(b) {
			return false
		}
	}
	return true
}

func telemetryDescriptor(name, kind, unit, scope, origin, method, temporality, availability string) TelemetryDescriptor {
	d := TelemetryDescriptor{ID: "core:" + name + "/1", Name: name, Revision: "1", Kind: kind, Unit: unit, Scope: scope, Origin: origin, Method: method, Temporality: temporality, Availability: availability, Dimensions: slices.Clone(telemetryDimensions), Aggregations: []string{}}
	switch kind {
	case "distribution":
		d.Aggregations = []string{"count", "sum", "min", "max", "mean", "p50", "p95"}
	case "gauge":
		d.Aggregations = []string{"count", "last", "min", "max"}
	case "counter":
		d.Aggregations = []string{"count", "total", "delta"}
	case "occurrence":
		d.Aggregations = []string{"count", "total"}
	case "ratio":
		d.Aggregations = []string{"ratio"}
	}
	return d
}
func telemetryCoreDescriptors() map[string]TelemetryDescriptor {
	descriptors := map[string]TelemetryDescriptor{}
	add := func(d TelemetryDescriptor) { descriptors[d.ID] = d }
	for _, name := range []string{"entities_created", "entities_admitted", "entities_started", "entities_settled", "diagnostics", "commands", "command_rejections"} {
		add(telemetryDescriptor("core."+name, "occurrence", "count", "entity", "core", "durable_journal", "delta", "supported"))
	}
	for _, name := range []string{"failed_run_fraction", "succeeded_run_fraction", "warning_run_fraction", "first_attempt_pass_fraction"} {
		add(telemetryDescriptor("core."+name, "ratio", "fraction", "cohort", "core", "cohort_predicate/1", "ratio", "supported"))
	}
	for _, name := range TimingMetricNames() {
		add(telemetryDescriptor("timing."+name, "distribution", "ms", "entity", "core", "foundation-timing/1", "observation", "supported"))
	}
	for _, name := range []string{"cpu_user", "cpu_system", "cpu_total"} {
		add(telemetryDescriptor("os."+name, "distribution", "s", "attempt", "os", "os_process_wait_rusage", "per_waited_child", "supported_when_observed"))
	}
	add(telemetryDescriptor("os.process_exit", "occurrence", "count", "attempt", "os", "os_process_wait", "delta", "supported_when_observed"))
	for _, name := range []string{"command_requests", "command_duplicates", "persistence_failures"} {
		add(telemetryDescriptor("core."+name, "occurrence", "count", "command", "core", "command_intake", "delta", "sampled"))
	}
	for _, name := range []string{"command_duration", "lock_wait", "transaction_duration", "validation_duration", "dispatch_duration"} {
		add(telemetryDescriptor("core."+name, "distribution", "ms", "command", "core", "runtime_observation", "observation", "sampled"))
	}
	add(telemetryDescriptor("core.storage_bytes", "gauge", "bytes", "authority", "core", "sqlite_page_accounting", "instantaneous", "supported_when_observed"))
	for _, item := range []struct{ name, kind, unit string }{{"os.rss", "gauge", "bytes"}, {"os.io_read_bytes", "counter", "bytes"}, {"os.io_write_bytes", "counter", "bytes"}, {"provider.tokens", "counter", "tokens"}, {"provider.requests", "counter", "count"}} {
		origin := "os"
		if strings.HasPrefix(item.name, "provider.") {
			origin = "provider"
		}
		add(telemetryDescriptor(item.name, item.kind, item.unit, "attempt", origin, "unsupported_meter", "none", "unsupported"))
	}
	return descriptors
}

type telemetryCursor struct {
	Version   string `json:"version"`
	Authority string `json:"authority"`
	Principal string `json:"principal"`
	Access    string `json:"access"`
	Digest    string `json:"digest"`
	Cut       int64  `json:"cut"`
	Offset    int    `json:"offset"`
	Expires   int64  `json:"expires"`
}

func telemetryCursorKey(e *Engine) ([]byte, error) {
	// Recheck current owner and private installation on every page. A cursor is
	// a position, not permission. No read path creates or rotates this key.
	if e.owner != fmt.Sprintf("local:uid:%d", os.Geteuid()) || e.Installation.OwnerUID != os.Geteuid() {
		return nil, telemetryProblem("access_denied", "current local owner is required")
	}
	b, err := readLocal(e.Root, ".prifly/installation.json", 32<<10)
	if err != nil {
		return nil, err
	}
	var installation Installation
	if err := decode(b, &installation); err != nil {
		return nil, err
	}
	if installation.OwnerUID != os.Geteuid() || installation.ID != e.Installation.ID || installation.ID != e.Store.Info().AuthorityID {
		return nil, telemetryProblem("access_denied", "authority or current owner changed")
	}
	key, err := hex.DecodeString(installation.TelemetryCursorKey)
	if err != nil || len(key) != 32 {
		return nil, telemetryProblem("invalid_installation", "private telemetry cursor key unavailable")
	}
	return key, nil
}
func telemetryReadCursor(encoded string, key []byte) (telemetryCursor, error) {
	var c telemetryCursor
	parts := strings.Split(encoded, ".")
	if len(parts) != 2 {
		return c, telemetryProblem("invalid_cursor", "invalid framing")
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return c, telemetryProblem("invalid_cursor", "invalid encoding")
	}
	mac, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return c, telemetryProblem("invalid_cursor", "invalid signature")
	}
	h := hmac.New(sha256.New, key)
	_, _ = h.Write(b)
	if !hmac.Equal(mac, h.Sum(nil)) {
		return c, telemetryProblem("invalid_cursor", "cursor signature does not match")
	}
	if err := decode(b, &c); err != nil {
		return c, telemetryProblem("invalid_cursor", "invalid cursor contract")
	}
	if c.Version != "telemetry-cursor/1" || c.Cut < 0 || c.Offset < 0 || c.Offset > TelemetryMaxRecords || c.Expires < time.Now().Unix() {
		return c, telemetryProblem("expired_cursor", "cursor is invalid or expired")
	}
	return c, nil
}
func telemetryWriteCursor(c telemetryCursor, key []byte) (string, error) {
	b, err := canonical(c)
	if err != nil {
		return "", err
	}
	h := hmac.New(sha256.New, key)
	_, _ = h.Write(b)
	return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(h.Sum(nil)), nil
}

type telemetryCollector struct {
	ctx              context.Context
	query            TelemetryQuery
	descriptors      map[string]TelemetryDescriptor
	records          []TelemetryRecord
	bytes            int
	seen             map[string]string
	missingTimestamp int
}

func (c *telemetryCollector) add(record TelemetryRecord) error {
	if err := c.ctx.Err(); err != nil {
		return err
	}
	if c.query.Mode == "catalog" {
		return nil
	}
	if record.Reasons == nil {
		record.Reasons = []string{}
	}
	if record.Evidence == nil {
		record.Evidence = []string{}
	}
	if record.Dimensions == nil {
		record.Dimensions = map[string]string{}
	}
	if record.Origin == "" {
		record.Origin = c.descriptors[record.DescriptorID].Origin
	}
	record.Dimensions["origin"] = record.Origin
	record.Dimensions["scope"] = record.Subject.Kind
	b, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if existing, ok := c.seen[record.ID]; ok {
		if existing != rawDigest(b) {
			return local.ErrIntegrity
		}
		return nil
	}
	c.seen[record.ID] = rawDigest(b)
	c.bytes += len(b)
	if c.bytes > TelemetryMaxBytes || len(c.records) >= TelemetryMaxRecords {
		return telemetryProblem("query_limit", "complete record scan exceeds 100000 records or 64 MiB")
	}
	c.records = append(c.records, record)
	return nil
}
func telemetryBase(d TelemetryDescriptor, id string, subject TelemetrySubject, obs *Observation, dimensions map[string]string) TelemetryRecord {
	labels := map[string]string{}
	for k, v := range dimensions {
		labels[k] = v
	}
	return TelemetryRecord{ID: id, DescriptorID: d.ID, Metric: d.Name, Subject: subject, Origin: d.Origin, Method: d.Method, Unit: d.Unit, Observed: obs, Quality: "measured", Coverage: "complete", Reasons: []string{}, Dimensions: labels, Evidence: []string{subject.ID}}
}
func (c *telemetryCollector) core(name, id string, subject TelemetrySubject, obs *Observation, dimensions map[string]string) TelemetryRecord {
	return telemetryBase(c.descriptors["core:"+name+"/1"], id, subject, obs, dimensions)
}

// Telemetry reads immutable history at a single global logical cut. ReadAllAt
// fixes membership first; receipts and samples are then read at that exact cut.
// ponytail: the finite local scan is capped at 1000 Runs/100000 projected records;
// add indexed analytical projections only when this explicit ceiling is reached.
func (e *Engine) Telemetry(ctx context.Context, query TelemetryQuery) (TelemetryResponse, error) {
	var response TelemetryResponse
	q, err := normalizeTelemetryQuery(query)
	if err != nil {
		return response, err
	}
	// Access is computed before any record is selected: permission filtering
	// precedes the cohort, so a denied reader never influences a count.
	scope, err := e.readAccess(ctx)
	if err != nil {
		return response, err
	}
	key, err := telemetryCursorKey(e)
	if err != nil {
		return response, err
	}
	digestQuery := q
	digestQuery.Cursor = ""
	digestQuery.Cut = nil
	encoded, err := canonical(digestQuery)
	if err != nil {
		return response, err
	}
	digest := rawDigest(encoded)
	cut := int64(-1)
	if q.Cut != nil {
		cut = *q.Cut
	}
	offset := 0
	expiry := time.Now().Add(30 * time.Minute).Unix()
	if q.Cursor != "" {
		cursor, err := telemetryReadCursor(q.Cursor, key)
		if err != nil {
			return response, err
		}
		if cursor.Authority != e.Installation.ID || cursor.Principal != e.owner || cursor.Access != scope || cursor.Digest != digest || q.Cut != nil && cursor.Cut != cut {
			return response, telemetryProblem("cursor_mismatch", "cursor is bound to principal, authority, query and cut")
		}
		cut, offset, expiry = cursor.Cut, cursor.Offset, cursor.Expires
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	snapshots, cut, err := e.Store.ReadAllAt(ctx, cut, TelemetryMaxRuns)
	if err != nil {
		return response, err
	}
	response = TelemetryResponse{SchemaVersion: TelemetryReportVersion, QueryVersion: TelemetryQueryVersion, QueryDigest: digest, AuthorityID: e.Installation.ID, Principal: e.owner, AccessScope: scope, Cut: cut, AsOf: []TelemetryAsOf{}, AsOfBasis: "persisted_last_observation_per_run_at_cut", CalculatorRevision: TelemetryCalculatorRevision, TimingRevision: "foundation-timing/1", CollectionProfile: "standard/1", Retention: "retained_immutable_history_no_automatic_deletion/1", Descriptors: []TelemetryDescriptor{}, Records: []TelemetryRecord{}, Aggregates: []TelemetryAggregate{}, Warnings: []string{"worker_reports_are_cooperative_not_os_or_provider_measurements", "sample_loss_unknown_no_historical_reconstruction", "population_is_run_cohort_before_record_filters"}, Limits: map[string]int{"runs": TelemetryMaxRuns, "receipts": local.MaxReceiptRecords, "projected_records": TelemetryMaxRecords, "groups": TelemetryMaxGroups, "scan_bytes": TelemetryMaxBytes, "response_bytes": TelemetryMaxResponseBytes, "page": TelemetryMaxPage, "cooperative_deadline_ms": 5000}}
	response.Population = TelemetryPopulation{Basis: "created_at_cohort", RunStatus: map[string]int{}, RunOutcome: map[string]int{}, StepVerdict: map[string]int{}, AttemptStatus: map[string]int{}, Ratios: map[string]TelemetryRatio{}}
	if q.CompletedFrom != "" || q.CompletedBefore != "" {
		response.Population.Basis = "completed_within_cohort"
	}
	c := telemetryCollector{ctx: ctx, query: q, descriptors: telemetryCoreDescriptors(), records: []TelemetryRecord{}, seen: map[string]string{}}
	selected := map[string]Run{}
	foundIDs := map[string]bool{}
	type planKey struct {
		Ref     flow.Ref
		Profile string
	}
	plans := map[planKey]*flow.Plan{}
	for _, snapshot := range snapshots {
		if err := ctx.Err(); err != nil {
			return TelemetryResponse{}, err
		}
		c.bytes += len(snapshot.Data)
		if c.bytes > TelemetryMaxBytes {
			return TelemetryResponse{}, telemetryProblem("query_limit", "snapshot scan exceeds 64 MiB")
		}
		var r Run
		if err := decodeState(snapshot.Data, &r); err != nil {
			return TelemetryResponse{}, err
		}
		if !supportedRun(r) || r.ID != snapshot.RunID || r.AuthorityID != e.Installation.ID || r.ProjectID != e.Config.ID {
			return TelemetryResponse{}, local.ErrIntegrity
		}
		foundIDs[r.ID] = true
		if !telemetryIn(r.ID, q.RunIDs) || !telemetryIn(r.Status, q.Filters.RunStatus) || !telemetryIn(telemetryOutcome(r.Outcome), q.Filters.RunOutcome) || !telemetryIn(r.WorkflowRef.ID, q.Filters.WorkflowID) {
			continue
		}
		if !telemetryWindow(r.Created.UTC, q.CreatedFrom, q.CreatedBefore) {
			continue
		}
		if (q.CompletedFrom != "" || q.CompletedBefore != "") && (!r.terminal() || r.Settled == nil || !telemetryWindow(r.Settled.UTC, q.CompletedFrom, q.CompletedBefore)) {
			continue
		}
		// Compiled workflow plans may be shared; each Run's executor/resource
		// snapshot and lock still require their own integrity proof.
		if isContextState(r.SchemaVersion) {
			if err := contextPinnedInvariant(r); err != nil {
				return TelemetryResponse{}, err
			}
		}
		key := planKey{r.WorkflowRef, r.Profile}
		plan := plans[key]
		if plan == nil {
			plan, err = r.plan()
			if err != nil {
				return TelemetryResponse{}, fmt.Errorf("pinned telemetry plan %s: %w", r.ID, err)
			}
			plans[key] = plan
		}
		selected[r.ID] = r
		if isContextState(r.SchemaVersion) {
			if response.CalculatorRevision != TelemetryCalculatorRevisionContext {
				response.Warnings = append(response.Warnings, "population_counts_steps_and_attempts_only_check_population_is_in_check_records")
			}
			response.CalculatorRevision = TelemetryCalculatorRevisionContext
			response.TimingRevision = TimingCalculatorRevisionContext
		} else if isInvocationState(r.SchemaVersion) && response.CalculatorRevision != TelemetryCalculatorRevisionContext {
			response.CalculatorRevision = TelemetryCalculatorRevisionInvocation
			response.TimingRevision = TimingCalculatorRevisionCore
		} else if r.Profile == flow.CoreProfile && response.CalculatorRevision != TelemetryCalculatorRevisionInvocation && response.CalculatorRevision != TelemetryCalculatorRevisionContext {
			response.CalculatorRevision = TelemetryCalculatorRevisionCore
		}
		response.AsOf = append(response.AsOf, TelemetryAsOf{r.ID, snapshot.Version, snapshot.EventSeq, r.LastObserved})
		if err := c.collectRun(r, plan, &response.Population); err != nil {
			return TelemetryResponse{}, err
		}
	}
	for _, id := range q.RunIDs {
		if !foundIDs[id] {
			return TelemetryResponse{}, telemetryProblem("not_found_at_cut", "requested Run is absent at the selected cut")
		}
	}
	if err := c.validateSelections(); err != nil {
		return TelemetryResponse{}, err
	}
	if q.Mode != "catalog" {
		if err := c.collectReceipts(e, cut, selected); err != nil {
			return TelemetryResponse{}, err
		}
		if err := c.collectSamples(e, cut, selected); err != nil {
			return TelemetryResponse{}, err
		}
	}
	for _, d := range c.descriptors {
		if telemetryIn(d.Name, q.Metrics) {
			response.Descriptors = append(response.Descriptors, d)
		}
	}
	sort.Slice(response.Descriptors, func(i, j int) bool { return response.Descriptors[i].ID < response.Descriptors[j].ID })
	if len(response.Descriptors) > TelemetryMaxGroups {
		return TelemetryResponse{}, telemetryProblem("query_limit", "descriptor catalog exceeds 1000 entries")
	}
	sort.Slice(response.AsOf, func(i, j int) bool { return response.AsOf[i].RunID < response.AsOf[j].RunID })
	telemetryFinalizePopulation(&response.Population)
	if q.Mode != "catalog" {
		records := c.filtered()
		response.RecordCount = len(records)
		response.Coverage = telemetryCoverage(records)
		response.Coverage.MissingTimestamp = c.missingTimestamp
		response.Coverage.LossUnknown = true
		if q.Mode == "records" {
			sort.Slice(records, func(i, j int) bool { return telemetryRecordLess(records[i], records[j]) })
			if offset > len(records) {
				return TelemetryResponse{}, telemetryProblem("expired_cursor", "historical page membership is unavailable")
			}
			end := min(offset+q.Limit, len(records))
			response.Records = records[offset:end]
			if end < len(records) {
				response.NextCursor, err = telemetryWriteCursor(telemetryCursor{"telemetry-cursor/1", e.Installation.ID, e.owner, "local-owner/1", digest, cut, end, expiry}, key)
				if err != nil {
					return TelemetryResponse{}, err
				}
			}
		} else {
			response.Aggregates, err = c.aggregate(records)
			if err != nil {
				return TelemetryResponse{}, err
			}
		}
	}
	if q.EventFrom != "" || q.EventBefore != "" {
		response.Warnings = append(response.Warnings, "event_window_filters_observation_receipt_utc_not_intervals_or_cohort_ratios", "untimestamped_receipts_and_cohort_ratios_excluded_from_record_window")
	}
	b, err := json.Marshal(response)
	if err != nil {
		return TelemetryResponse{}, err
	}
	if len(b) > TelemetryMaxResponseBytes {
		return TelemetryResponse{}, telemetryProblem("query_limit", "complete response exceeds 8 MiB; request fewer metrics or a smaller records page")
	}
	if err := ctx.Err(); err != nil {
		return TelemetryResponse{}, err
	}
	return response, nil
}

func telemetryOutcome(outcome *string) string {
	if outcome == nil {
		return "unknown"
	}
	return *outcome
}

func telemetryRunLabels(r Run) map[string]string {
	return map[string]string{"workflow_id": r.WorkflowRef.ID, "workflow_revision": r.WorkflowRef.String(), "run_status": r.Status, "run_outcome": telemetryOutcome(r.Outcome), "core_build": r.CoreBuild, "profile": r.Profile}
}
func telemetryAttemptClosed(a *Attempt) bool {
	return a != nil && a.Settled != nil && slices.Contains([]string{"completed", "failed", "cancelled"}, a.Status)
}
func telemetryChannelClosed(a *Attempt) bool {
	// This qualifies the cooperative declared publication transport, not hidden
	// warnings/calls inside the executable and never stdout/stderr completeness.
	return telemetryAttemptClosed(a) && a.Status == "completed" && a.Accepted != nil && a.ProcessOutcome != nil && a.ProcessOutcome.WaitReturned && a.ProcessOutcome.GroupEmpty && !a.ProcessOutcome.Uncertain && a.ProcessOutcome.ResultError == ""
}

// Index the already compiled closure once. Pinned child plans are reused across
// invocations; their instances and publications remain separately identified.
func telemetryPlanIndex(r Run, root *flow.Plan) (map[flow.Ref]*flow.Plan, error) {
	plans := map[flow.Ref]*flow.Plan{}
	for _, p := range workflowPlans(root) {
		plans[planRef(p)] = p
	}
	if isInvocationState(r.SchemaVersion) {
		if err := invocationInvariant(r); err != nil {
			return nil, local.ErrIntegrity
		}
		for id, inv := range r.Invocations {
			if plans[inv.WorkflowRef] == nil {
				return nil, local.ErrIntegrity
			}
			if id == r.RootInvocationID {
				if inv.WorkflowRef != r.WorkflowRef {
					return nil, local.ErrIntegrity
				}
				continue
			}
			parent := r.Invocations[inv.ParentInvocationID]
			caller := r.Activations[inv.CallerActivationID]
			if parent == nil || !r.childMatchesCaller(inv, caller) || plans[parent.WorkflowRef] == nil {
				return nil, local.ErrIntegrity
			}
			child := plans[parent.WorkflowRef].BodyPlan(caller.StageID)
			if child == nil || inv.WorkflowRef != planRef(child) {
				return nil, local.ErrIntegrity
			}
		}
	}
	return plans, nil
}

func telemetryActivationPlan(r Run, plans map[flow.Ref]*flow.Plan, activation *Activation) (*flow.Plan, error) {
	if activation == nil {
		return nil, local.ErrIntegrity
	}
	ref := r.WorkflowRef
	if isInvocationState(r.SchemaVersion) {
		inv := r.Invocations[activation.InvocationID]
		if inv == nil || inv.ID != activation.InvocationID || inv.RunID != r.ID {
			return nil, local.ErrIntegrity
		}
		ref = inv.WorkflowRef
	}
	p := plans[ref]
	if p == nil {
		return nil, local.ErrIntegrity
	}
	return p, nil
}

func telemetryWarningCoverage(r Run, plans map[flow.Ref]*flow.Plan) bool {
	if !r.terminal() || len(r.Gaps) != 0 || len(r.Attempts) == 0 {
		return false
	}
	for _, a := range r.Attempts {
		if !telemetryChannelClosed(a) {
			return false
		}
		activation := r.Activations[a.ActivationID]
		if activation == nil {
			return false
		}
		p, err := telemetryActivationPlan(r, plans, activation)
		if err != nil {
			return false
		}
		definition := p.Steps[activation.StageID]
		declared := false
		for _, m := range definition.Telemetry {
			if m.Kind == "diagnostic" && m.Severity == "warn" {
				declared = true
			}
		}
		if !declared {
			return false
		}
	}
	return true
}
func telemetryRatio(numerator, denominator int64) TelemetryRatio {
	r := TelemetryRatio{Numerator: numerator, Denominator: denominator, Quality: "unavailable"}
	if denominator > 0 {
		r.Value = telemetryPtr(float64(numerator) / float64(denominator))
		r.Quality = "measured"
	}
	return r
}
func telemetryFinalizePopulation(p *TelemetryPopulation) {
	for _, name := range []string{"core.failed_run_fraction", "core.succeeded_run_fraction", "core.warning_run_fraction", "core.first_attempt_pass_fraction"} {
		if _, ok := p.Ratios[name]; !ok {
			p.Ratios[name] = TelemetryRatio{}
		}
	}
	for name, r := range p.Ratios {
		p.Ratios[name] = telemetryRatio(r.Numerator, r.Denominator)
	}
}
func telemetryPopulationRatio(p *TelemetryPopulation, name string, n, d int64) {
	r := p.Ratios[name]
	r.Numerator += n
	r.Denominator += d
	p.Ratios[name] = r
}

func (c *telemetryCollector) collectRun(r Run, p *flow.Plan, pop *TelemetryPopulation) error {
	plans, err := telemetryPlanIndex(r, p)
	if err != nil {
		return err
	}
	if isContextState(r.SchemaVersion) {
		if err := c.collectChecks(r, plans); err != nil {
			return err
		}
	} else if isInvocationState(r.SchemaVersion) {
		for _, name := range TimingMetricNames() {
			d := telemetryDescriptor("timing."+name, "distribution", "ms", "entity", "core", TimingCalculatorRevisionCore, "observation", "supported")
			d.ID, d.Revision = "core:timing."+name+"/2", "2"
			c.descriptors[d.ID] = d
		}
	}
	labels := telemetryRunLabels(r)
	pop.Matched++
	pop.RunStatus[r.Status]++
	pop.RunOutcome[telemetryOutcome(r.Outcome)]++
	if r.terminal() {
		pop.Terminal++
	} else {
		pop.Open++
	}
	if r.Status == "uncertain" {
		pop.Uncertain++
	}
	if r.Status == "cancelled" {
		pop.Cancelled++
	}
	if r.HasUnresolvedEffects {
		pop.UnresolvedEffects++
	}
	if isInvocationState(r.SchemaVersion) {
		pop.Invocations += len(r.Invocations)
	} else if r.RootInvocationID != "" {
		pop.Invocations++
	}
	pop.Activations += len(r.Activations)
	pop.Steps += len(r.Steps)
	pop.Attempts += len(r.Attempts)
	for _, s := range r.Steps {
		if s == nil {
			return local.ErrIntegrity
		}
		pop.StepVerdict[telemetryVerdict(s.Verdict)]++
	}
	for _, a := range r.Attempts {
		if a == nil {
			return local.ErrIntegrity
		}
		pop.AttemptStatus[a.Status]++
		if a.Started != nil {
			pop.StartedAttempts++
		}
		if telemetryAttemptClosed(a) {
			pop.SettledAttempts++
		}
	}
	warned, err := c.collectPublications(r, plans)
	if err != nil {
		return err
	}
	for _, diagnostic := range r.Diagnostics {
		if diagnostic.ID == "" || diagnostic.RunID != r.ID {
			return local.ErrIntegrity
		}
		subject := TelemetrySubject{Kind: "run", ID: r.ID, RunID: r.ID}
		activationID := diagnostic.ActivationID
		if diagnostic.AttemptID != "" {
			a := r.Attempts[diagnostic.AttemptID]
			if a == nil {
				return local.ErrIntegrity
			}
			subject = TelemetrySubject{Kind: "attempt", ID: a.ID, RunID: r.ID, StepInstanceID: a.StepID, AttemptID: a.ID}
			if r.Profile == flow.CoreProfile {
				if a.ActivationID == "" || activationID != "" && activationID != a.ActivationID {
					return local.ErrIntegrity
				}
				activationID = a.ActivationID
			}
		}
		record := c.core("core.diagnostics", derivedID("telemetry-diagnostic", r.ID, diagnostic.ID), subject, &diagnostic.Observed, labels)
		if r.Profile == flow.CoreProfile && activationID != "" {
			activation := r.Activations[activationID]
			if activation == nil || activation.ID != activationID {
				return local.ErrIntegrity
			}
			if activation.Kind == "step" {
				step := r.Steps[activation.StepID]
				if step == nil || step.ID != activation.StepID || step.ActivationID != activation.ID || subject.AttemptID != "" && subject.StepInstanceID != step.ID {
					return local.ErrIntegrity
				}
				if subject.AttemptID == "" {
					record.Subject = TelemetrySubject{Kind: "step_instance", ID: step.ID, RunID: r.ID, StepInstanceID: step.ID}
				}
				record.Dimensions["step_id"] = step.Ref.ID
				record.Dimensions["step_revision"] = step.Ref.String()
			} else {
				if activation.StepID != "" || subject.AttemptID != "" {
					return local.ErrIntegrity
				}
				record.Subject = TelemetrySubject{Kind: "stage_activation", ID: activation.ID, RunID: r.ID}
			}
			record.Dimensions["stage_id"] = activation.StageID
		}
		record.Origin = diagnostic.Origin
		record.Integer = telemetryPtr(int64(1))
		record.Dimensions["severity"] = diagnostic.Severity
		record.Dimensions["code"] = diagnostic.Code
		record.Generation = diagnostic.Observed.Session
		record.Evidence = []string{diagnostic.ID}
		if diagnostic.PublicationID != "" {
			record.Evidence = append(record.Evidence, diagnostic.PublicationID)
		}
		if err := c.add(record); err != nil {
			return err
		}
		if diagnostic.Severity == "warn" {
			warned = true
		}
	}
	fullWarning := telemetryWarningCoverage(r, plans)
	if fullWarning {
		pop.FullWarningCoverage++
	} else {
		pop.UnknownWarningCoverage++
		if warned {
			pop.WarnedIncomplete++
		}
	}
	runSubject := TelemetrySubject{Kind: "run", ID: r.ID, RunID: r.ID}
	terminal := int64(0)
	if r.terminal() {
		terminal = 1
	}
	for _, item := range []struct {
		name string
		n, d int64
	}{
		{"core.failed_run_fraction", telemetryBool(r.Status == "failed"), terminal},
		{"core.succeeded_run_fraction", telemetryBool(r.Status == "completed" && telemetryOutcome(r.Outcome) == "succeeded"), terminal},
		{"core.warning_run_fraction", telemetryBool(fullWarning && warned), telemetryBool(fullWarning)},
	} {
		telemetryPopulationRatio(pop, item.name, item.n, item.d)
		record := c.core(item.name, derivedID("telemetry-ratio", r.ID, item.name), runSubject, nil, labels)
		record.Numerator = telemetryPtr(item.n)
		record.Denominator = telemetryPtr(item.d)
		record.Value = telemetryRatio(item.n, item.d).Value
		if item.d == 0 {
			record.Quality = "unavailable"
			record.Reasons = append(record.Reasons, "outside_known_denominator")
		}
		if err := c.add(record); err != nil {
			return err
		}
	}
	for _, step := range r.Steps {
		if len(step.AttemptIDs) == 0 {
			continue
		}
		a := r.Attempts[step.AttemptIDs[0]]
		if a == nil || a.StepID != step.ID {
			return local.ErrIntegrity
		}
		d := telemetryBool(telemetryAttemptClosed(a))
		if isContextState(r.SchemaVersion) && a.Status == "completed" && a.Accepted == nil && step.Settled == nil {
			d = 0 // Proven process exit does not resolve pending result acceptance.
		}
		n := telemetryBool(d == 1 && a.Status == "completed" && a.Accepted != nil && a.Accepted.Verdict == "pass")
		if d == 0 {
			pop.FirstAttemptsOpen++
		}
		telemetryPopulationRatio(pop, "core.first_attempt_pass_fraction", n, d)
		record := c.core("core.first_attempt_pass_fraction", derivedID("telemetry-first", r.ID, step.ID), TelemetrySubject{Kind: "step_instance", ID: step.ID, RunID: r.ID, StepInstanceID: step.ID, AttemptID: a.ID}, nil, labels)
		record.Numerator = &n
		record.Denominator = &d
		record.Value = telemetryRatio(n, d).Value
		record.Dimensions["step_id"] = step.Ref.ID
		record.Dimensions["step_revision"] = step.Ref.String()
		if d == 0 {
			record.Quality = "unavailable"
			record.Reasons = append(record.Reasons, "first_attempt_open_or_uncertain")
		}
		if err := c.add(record); err != nil {
			return err
		}
	}
	if c.query.Mode == "catalog" {
		return nil
	}
	// No live liveness probe or wall-clock extension enters historical reports.
	tree := Timing(r, r.LastObserved, false)
	var visit func(TimingNode) error
	visit = func(node TimingNode) error {
		subject := TelemetrySubject{Kind: node.Kind, ID: node.ID, RunID: r.ID}
		created, settled := &r.Created, r.Settled
		dimensions := telemetryRunLabels(r)
		dimensions["status"] = node.Status
		dimensions["verdict"] = telemetryVerdict(node.Verdict)
		if node.StageID != "" {
			dimensions["stage_id"] = node.StageID
		}
		var step *Step
		var attempt *Attempt
		var check *CheckExecution
		switch node.Kind {
		case "workflow_invocation":
			if isInvocationState(r.SchemaVersion) {
				inv := r.Invocations[node.ID]
				if inv == nil {
					return local.ErrIntegrity
				}
				created, settled = &inv.Created, inv.Settled
			}
		case "stage_activation":
			a := r.Activations[node.ID]
			if a == nil {
				return local.ErrIntegrity
			}
			created, settled = &a.Created, a.Settled
			step = r.Steps[a.StepID]
		case "step_instance":
			step = r.Steps[node.ID]
			if step == nil {
				return local.ErrIntegrity
			}
			created, settled = &step.Created, step.Settled
		case "attempt":
			attempt = r.Attempts[node.ID]
			if attempt == nil {
				return local.ErrIntegrity
			}
			created, settled = &attempt.Admitted, attempt.Settled
			subject.AttemptID = attempt.ID
			step = r.Steps[attempt.StepID]
			if attempt.Accepted != nil {
				dimensions["verdict"] = attempt.Accepted.Verdict
			}
		case "check_execution":
			check = r.CheckExecutions[node.ID]
			var err error
			dimensions, err = telemetryCheckLabels(r, plans, check)
			if err != nil {
				return err
			}
			created, settled = &check.Admitted, check.Settled
		}
		if step != nil {
			subject.StepInstanceID = step.ID
			dimensions["step_id"] = step.Ref.ID
			dimensions["step_revision"] = step.Ref.String()
			if activation := r.Activations[step.ActivationID]; activation != nil {
				dimensions["stage_id"] = activation.StageID
				scoped, err := telemetryActivationPlan(r, plans, activation)
				if err != nil {
					return err
				}
				if definition, ok := scoped.Steps[activation.StageID]; ok {
					dimensions["executor_id"] = definition.Executor.AdapterRef.String()
				}
			}
		}
		addFact := func(name string, obs *Observation) error {
			record := c.core(name, derivedID("telemetry-fact", r.ID, node.ID, name), subject, obs, dimensions)
			record.Integer = telemetryPtr(int64(1))
			if obs != nil {
				record.Generation = obs.Session
			}
			return c.add(record)
		}
		if check == nil {
			if err := addFact("core.entities_created", created); err != nil {
				return err
			}
			if settled != nil && node.Status != "uncertain" {
				if err := addFact("core.entities_settled", settled); err != nil {
					return err
				}
			}
		}
		if attempt != nil {
			if err := addFact("core.entities_admitted", created); err != nil {
				return err
			}
			if attempt.Started != nil {
				if err := addFact("core.entities_started", attempt.Started); err != nil {
					return err
				}
			}
			if err := c.collectAttemptResources(r, attempt, subject, dimensions); err != nil {
				return err
			}
		}
		metricNames := TimingMetricNames()
		if isContextState(r.SchemaVersion) {
			metricNames = ContextTimingMetricNames()
		}
		for _, name := range metricNames {
			duration, ok := node.Metrics[name]
			if !ok {
				continue
			}
			obs := settled
			if isContextState(r.SchemaVersion) && attempt != nil && step != nil && name == "result_to_acceptance" {
				// Candidate acceptance may be known after the producer process
				// has settled. Its latency belongs to that later boundary.
				obs = step.Settled
			}
			if obs == nil {
				obs = &r.LastObserved
			}
			descriptorID := "core:timing." + name + "/1"
			if isContextState(r.SchemaVersion) {
				descriptorID = "core:timing." + name + "/3"
			} else if isInvocationState(r.SchemaVersion) {
				descriptorID = "core:timing." + name + "/2"
			}
			record := telemetryBase(c.descriptors[descriptorID], derivedID("telemetry-timing", r.ID, node.ID, name), subject, obs, dimensions)
			record.Quality = duration.Quality
			record.IsOpen = duration.IsOpen
			record.Duration = &duration
			record.Reasons = slices.Clone(duration.Reasons)
			record.Generation = created.Session
			record.Method = tree.CalculatorRevision + "/" + telemetryTimingBasis(node, name)
			if duration.ValueMS != nil {
				record.Value = telemetryPtr(float64(*duration.ValueMS))
			}
			if duration.Quality == "partial" {
				record.Coverage = "partial"
			}
			if err := c.add(record); err != nil {
				return err
			}
		}
		for _, child := range node.Children {
			if err := visit(child); err != nil {
				return err
			}
		}
		return nil
	}
	return visit(tree.Root)
}
func telemetryBool(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
func telemetryVerdict(v string) string {
	if v == "" {
		return "unknown"
	}
	return v
}
func telemetryTimingBasis(node TimingNode, name string) string {
	basis := []string{}
	for _, interval := range node.Intervals {
		if interval.Metric == name && interval.SuspendBasis != "" && !slices.Contains(basis, interval.SuspendBasis) {
			basis = append(basis, interval.SuspendBasis)
		}
	}
	sort.Strings(basis)
	if len(basis) == 0 {
		return "persisted_composition"
	}
	return strings.Join(basis, "+")
}

func (c *telemetryCollector) collectAttemptResources(r Run, a *Attempt, subject TelemetrySubject, dimensions map[string]string) error {
	obs := a.Settled
	if obs == nil {
		obs = a.ExecutorEnd
	}
	if obs == nil {
		obs = &r.LastObserved
	}
	for _, name := range []string{"os.cpu_user", "os.cpu_system", "os.cpu_total"} {
		record := c.core(name, derivedID("telemetry-resource", r.ID, a.ID, name), subject, obs, dimensions)
		record.Generation = a.ID
		record.Quality = "unavailable"
		record.Reasons = []string{"os_accounting_unavailable"}
		record.Dimensions["resource_scope"] = "waited_child_os_accounting"
		if a.ProcessOutcome != nil && a.ProcessOutcome.CPU != nil {
			cpu := a.ProcessOutcome.CPU
			if cpu.UserNS < 0 || cpu.SystemNS < 0 || float64(cpu.UserNS) > telemetryMaxNumber || float64(cpu.SystemNS) > telemetryMaxNumber || float64(cpu.UserNS)+float64(cpu.SystemNS) > telemetryMaxNumber {
				return telemetryProblem("invalid_measurement", "CPU accounting is outside exact bounded integer range")
			}
			ns := cpu.UserNS
			if name == "os.cpu_system" {
				ns = cpu.SystemNS
			}
			if name == "os.cpu_total" {
				ns += cpu.SystemNS
			}
			record.Value = telemetryPtr(float64(ns) / 1e9)
			record.Quality = "measured"
			record.Method = cpu.Method
			record.Dimensions["resource_scope"] = cpu.Scope
			record.Coverage = "scope_only"
			record.Reasons = []string{cpu.Coverage}
		}
		if err := c.add(record); err != nil {
			return err
		}
	}
	if a.ProcessOutcome != nil && a.ProcessOutcome.WaitReturned && a.ProcessOutcome.ExitCode != nil {
		record := c.core("os.process_exit", derivedID("telemetry-exit", r.ID, a.ID), subject, obs, dimensions)
		record.Integer = telemetryPtr(int64(1))
		record.Dimensions["code"] = "exit:" + strconv.Itoa(*a.ProcessOutcome.ExitCode)
		record.Generation = a.ID
		if err := c.add(record); err != nil {
			return err
		}
	}
	for _, d := range c.descriptors {
		if d.Availability != "unsupported" || len(c.query.Metrics) == 0 || !slices.Contains(c.query.Metrics, d.Name) {
			continue
		}
		record := telemetryBase(d, derivedID("telemetry-unavailable", r.ID, a.ID, d.ID), subject, obs, dimensions)
		record.Quality = "unavailable"
		record.Coverage = "unknown"
		record.Generation = a.ID
		record.Reasons = []string{"unsupported_meter"}
		if err := c.add(record); err != nil {
			return err
		}
	}
	return nil
}

func telemetryMappingDescriptor(ref flow.Ref, m flow.Mapping) TelemetryDescriptor {
	kind := m.Kind
	unit := m.Unit
	if kind == "diagnostic" {
		kind = "occurrence"
		unit = "count"
	}
	d := telemetryDescriptor("step."+m.Name, kind, unit, "attempt", "worker-reported", "declared_publication/1", m.Aggregation, "declared")
	d.ID = derivedID("descriptor", ref.String(), m.Name, m.Revision)
	d.Revision = m.Revision
	d.DefinitionRef = &ref
	for name := range m.Dimensions {
		d.Dimensions = append(d.Dimensions, "custom."+name)
	}
	sort.Strings(d.Dimensions)
	return d
}

func (c *telemetryCollector) collectPublications(r Run, plans map[flow.Ref]*flow.Plan) (bool, error) {
	for _, p := range plans {
		for stage, definition := range p.Steps {
			ref := p.Workflow.Definition.Stages[stage].StepRef
			for _, mapping := range definition.Telemetry {
				d := telemetryMappingDescriptor(ref, mapping)
				c.descriptors[d.ID] = d
			}
		}
	}
	warned := false
	seen := map[string]string{}
	observed := map[string]bool{}
	for i, publication := range r.Publications {
		a := r.Attempts[publication.AttemptID]
		s := r.Steps[publication.StepID]
		if a == nil || s == nil || a.StepID != s.ID || publication.ID == "" {
			return false, local.ErrIntegrity
		}
		activation := r.Activations[a.ActivationID]
		if activation == nil {
			return false, local.ErrIntegrity
		}
		p, err := telemetryActivationPlan(r, plans, activation)
		if err != nil {
			return false, err
		}
		definition, ok := p.Steps[activation.StageID]
		if !ok {
			return false, local.ErrIntegrity
		}
		if _, err := p.ValidatePublication(activation.StageID, publication.Hook, publication.Kind, publication.Value); err != nil {
			return false, err
		}
		identity := publication.AttemptID + "/" + publication.Hook + "/" + publication.Kind + "/" + strconv.FormatInt(publication.Version, 10)
		if publication.Kind == "event" {
			identity = publication.AttemptID + "/" + publication.Hook + "/event/" + publication.EventKey
		}
		b, err := flow.Canonical(publication.Value)
		if err != nil {
			return false, err
		}
		digest := rawDigest(b)
		if old, ok := seen[identity]; ok {
			if old != digest {
				return false, local.ErrIntegrity
			}
			continue
		}
		seen[identity] = digest
		value, err := flow.Parse(b, "json")
		if err != nil {
			return false, err
		}
		for _, mapping := range definition.Telemetry {
			if mapping.Hook != publication.Hook {
				continue
			}
			d := telemetryMappingDescriptor(s.Ref, mapping)
			if _, ok := c.descriptors[d.ID]; !ok {
				return false, local.ErrIntegrity
			}
			observed[a.ID+"/"+d.ID] = true
			subject := TelemetrySubject{Kind: "attempt", ID: a.ID, RunID: r.ID, StepInstanceID: s.ID, AttemptID: a.ID}
			record := telemetryBase(d, derivedID("telemetry-publication", r.ID, identity, d.ID), subject, &publication.Received, telemetryRunLabels(r))
			record.Generation = a.ID
			record.Order = int64(i + 1)
			record.Evidence = []string{publication.ID}
			record.Dimensions["step_id"] = s.Ref.ID
			record.Dimensions["step_revision"] = s.Ref.String()
			record.Dimensions["stage_id"] = activation.StageID
			record.Dimensions["status"] = a.Status
			record.Dimensions["verdict"] = telemetryVerdict(s.Verdict)
			for name, pointer := range mapping.Dimensions {
				v, exists := flow.JSONPointer(value, pointer)
				if !exists {
					record.Dimensions["custom."+name] = "null"
				} else {
					encoded, err := json.Marshal(v)
					if err != nil {
						return false, err
					}
					record.Dimensions["custom."+name] = string(encoded)
				}
			}
			if mapping.Kind == "diagnostic" {
				record.Integer = telemetryPtr(int64(1))
				record.Evidence = []string{derivedID("diagnostic", publication.ID, mapping.Name, mapping.Revision), publication.ID}
				record.Reasons = append(record.Reasons, "same_occurrence_as_core_diagnostic_projection")
				record.Dimensions["severity"] = mapping.Severity
				record.Dimensions["code"] = mapping.Code
				if mapping.Severity == "warn" {
					warned = true
				}
			} else {
				number, exists := flow.JSONPointer(value, mapping.Field)
				if !exists {
					record.Quality = "unavailable"
					record.Reasons = []string{"absent_current_value"}
				} else {
					raw, ok := number.(json.Number)
					if !ok {
						return false, local.ErrIntegrity
					}
					v, err := raw.Float64()
					if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > telemetryMaxNumber {
						return false, telemetryProblem("invalid_measurement", "numeric observation is nonfinite or outside exact bounds")
					}
					if mapping.Kind == "counter" {
						if v < 0 || math.Trunc(v) != v {
							return false, telemetryProblem("invalid_measurement", "counter must be an exact nonnegative integer")
						}
						record.Integer = telemetryPtr(int64(v))
					} else {
						record.Value = &v
					}
				}
			}
			if err := c.add(record); err != nil {
				return false, err
			}
		}
	}
	for _, a := range r.Attempts {
		if a == nil {
			return false, local.ErrIntegrity
		}
		s := r.Steps[a.StepID]
		activation := r.Activations[a.ActivationID]
		if s == nil || activation == nil {
			return false, local.ErrIntegrity
		}
		p, err := telemetryActivationPlan(r, plans, activation)
		if err != nil {
			return false, err
		}
		definition := p.Steps[activation.StageID]
		for _, mapping := range definition.Telemetry {
			d := telemetryMappingDescriptor(s.Ref, mapping)
			if observed[a.ID+"/"+d.ID] {
				continue
			}
			record := telemetryBase(d, derivedID("telemetry-missing-publication", r.ID, a.ID, d.ID), TelemetrySubject{Kind: "attempt", ID: a.ID, RunID: r.ID, StepInstanceID: s.ID, AttemptID: a.ID}, &r.LastObserved, telemetryRunLabels(r))
			record.Generation = a.ID
			record.Quality = "unavailable"
			record.Coverage = "unknown"
			record.Reasons = []string{"declared_meter_not_reported"}
			record.Dimensions["step_id"] = s.Ref.ID
			record.Dimensions["step_revision"] = s.Ref.String()
			record.Dimensions["stage_id"] = activation.StageID
			record.Dimensions["status"] = a.Status
			record.Dimensions["verdict"] = telemetryVerdict(s.Verdict)
			if mapping.Kind == "diagnostic" {
				record.Dimensions["severity"] = mapping.Severity
				record.Dimensions["code"] = mapping.Code
				if telemetryChannelClosed(a) && len(r.Gaps) == 0 {
					record.Quality = "measured"
					record.Coverage = "complete_declared_channel"
					record.Integer = telemetryPtr(int64(0))
					record.Reasons = []string{"accepted_occurrences_only_hidden_worker_messages_unknown"}
				}
			}
			if err := c.add(record); err != nil {
				return false, err
			}
		}
	}
	return warned, nil
}

func (c *telemetryCollector) validateSelections() error {
	known := map[string]bool{}
	dimensions := map[string]bool{}
	for _, d := range c.descriptors {
		known[d.Name] = true
		if !telemetryIn(d.Name, c.query.Metrics) || d.Availability == "unsupported" && len(c.query.Metrics) == 0 {
			continue
		}
		for _, dimension := range d.Dimensions {
			dimensions[dimension] = true
		}
		for _, aggregation := range c.query.Aggregations {
			if !slices.Contains(d.Aggregations, aggregation) {
				return telemetryProblem("invalid_aggregation", aggregation+" is incompatible with "+d.Name+" ("+d.Kind+")")
			}
		}
	}
	for _, name := range c.query.Metrics {
		if !known[name] {
			return telemetryProblem("unknown_metric", "metric is not defined by core or a pinned definition in this cohort: "+name)
		}
	}
	for _, dimension := range c.query.GroupBy {
		if strings.HasPrefix(dimension, "custom.") && !dimensions[dimension] {
			return telemetryProblem("unknown_dimension", "custom grouping is not declared by a selected descriptor")
		}
		if slices.Contains(telemetryCheckDimensions, dimension) && !dimensions[dimension] {
			return telemetryProblem("unknown_dimension", "check grouping is not declared by a selected descriptor")
		}
	}
	for dimension := range c.query.Filters.Dimensions {
		if !dimensions["custom."+dimension] {
			return telemetryProblem("unknown_dimension", "custom filter is not declared by a selected descriptor")
		}
	}
	return nil
}
func telemetryAuthorityScope(q TelemetryQuery) bool {
	return len(q.RunIDs) == 0 && q.CreatedFrom == "" && q.CreatedBefore == "" && q.CompletedFrom == "" && q.CompletedBefore == "" && len(q.Filters.RunStatus)+len(q.Filters.RunOutcome)+len(q.Filters.WorkflowID) == 0
}
func (c *telemetryCollector) collectReceipts(e *Engine, cut int64, runs map[string]Run) error {
	if len(c.query.Metrics) > 0 && !slices.Contains(c.query.Metrics, "core.commands") && !slices.Contains(c.query.Metrics, "core.command_rejections") {
		return nil
	}
	receipts, actual, err := e.Store.ReadReceiptsAt(c.ctx, cut, local.MaxReceiptRecords)
	if err != nil {
		return err
	}
	if actual != cut {
		return local.ErrIntegrity
	}
	for _, receipt := range receipts {
		receiptBytes, err := json.Marshal(receipt)
		if err != nil {
			return err
		}
		c.bytes += len(receiptBytes)
		if c.bytes > TelemetryMaxBytes {
			return telemetryProblem("query_limit", "complete snapshot, receipt and record scan exceeds 64 MiB")
		}
		r, selected := runs[receipt.RunID]
		if !selected && !telemetryAuthorityScope(c.query) {
			continue
		}
		labels := map[string]string{}
		subject := TelemetrySubject{Kind: "command", ID: receipt.ID}
		if selected {
			labels = telemetryRunLabels(r)
			subject.RunID = r.ID
		}
		labels["status"] = "accepted"
		if receipt.Rejection != nil {
			labels["status"] = "rejected"
			labels["code"] = receipt.Rejection.Code
		}
		names := []string{"core.commands"}
		if receipt.Rejection != nil {
			names = append(names, "core.command_rejections")
		}
		for _, name := range names {
			record := c.core(name, derivedID("telemetry-receipt", receipt.Actor, receipt.ID, name), subject, nil, labels)
			record.Method = "durable_receipt"
			record.Integer = telemetryPtr(int64(1))
			record.Generation = e.Installation.ID
			record.Order = receipt.Cut
			record.Evidence = []string{"receipt:" + receipt.Actor + ":" + receipt.ID}
			if err := c.add(record); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *telemetryCollector) collectSamples(e *Engine, cut int64, runs map[string]Run) error {
	sampleNames := []string{"core.command_requests", "core.command_duplicates", "core.persistence_failures", "core.command_duration", "core.lock_wait", "core.transaction_duration", "core.validation_duration", "core.dispatch_duration", "core.storage_bytes"}
	needed := len(c.query.Metrics) == 0
	for _, name := range c.query.Metrics {
		if slices.Contains(sampleNames, name) {
			needed = true
		}
	}
	if !needed {
		return nil
	}
	var after int64
	count := 0
	for {
		page, err := e.Store.ReadSamples(c.ctx, cut, after, 1000)
		if err != nil {
			return err
		}
		if page.Cut != cut {
			return local.ErrIntegrity
		}
		for _, sample := range page.Records {
			after = sample.Seq
			count++
			c.bytes += len(sample.Data)
			if count > TelemetryMaxRecords || c.bytes > TelemetryMaxBytes {
				return telemetryProblem("query_limit", "complete sample scan exceeds 100000 records or 64 MiB")
			}
			var data TelemetrySampleData
			if err := decode(sample.Data, &data); err != nil {
				return err
			}
			if data.SchemaVersion != "telemetry-sample/1" || !slices.Contains(sampleNames, data.Metric) {
				return telemetryProblem("unsupported_version", "unknown core sample schema or instrument")
			}
			d := c.descriptors["core:"+data.Metric+"/1"]
			if data.Unit != d.Unit || data.Method != d.Method || !slices.Contains([]string{"measured", "partial", "unavailable", "estimated"}, data.Quality) || !slices.Contains([]string{"sample", "complete", "partial", "unknown"}, data.Coverage) || data.Observed.Session == "" || data.Observed.Source == "" {
				return telemetryProblem("invalid_measurement", "sample unit, method, quality, coverage or generation is invalid")
			}
			if _, err := time.Parse(time.RFC3339Nano, data.Observed.UTC); err != nil {
				return telemetryProblem("invalid_measurement", "sample observation UTC is invalid")
			}
			if data.Quality == "measured" && data.Value == nil {
				return telemetryProblem("invalid_measurement", "measured sample has no value")
			}
			if data.Value != nil && (math.IsNaN(*data.Value) || math.IsInf(*data.Value, 0) || *data.Value < 0 || *data.Value > telemetryMaxNumber || d.Kind == "occurrence" && math.Trunc(*data.Value) != *data.Value) {
				return telemetryProblem("invalid_measurement", "sample is nonfinite, negative, fractional counter or overflowing")
			}
			r, selected := runs[sample.RunID]
			if !selected && (sample.RunID != "" || !telemetryAuthorityScope(c.query)) {
				continue
			}
			labels := map[string]string{}
			subject := TelemetrySubject{Kind: "authority", ID: e.Installation.ID}
			if selected {
				labels = telemetryRunLabels(r)
				subject.RunID = r.ID
			}
			if data.CommandID != "" {
				subject.Kind = "command"
				subject.ID = data.CommandID
			}
			if data.AttemptID != "" {
				a := r.Attempts[data.AttemptID]
				if !selected || a == nil {
					return telemetryProblem("invalid_measurement", "sample references absent Attempt at cut")
				}
				subject.AttemptID = a.ID
				subject.StepInstanceID = a.StepID
			}
			if data.Metric == "core.storage_bytes" && sample.RunID != "" {
				return telemetryProblem("invalid_measurement", "shared storage meter cannot be attributed to a Run")
			}
			record := telemetryBase(d, sample.ID, subject, &data.Observed, labels)
			record.Quality = data.Quality
			record.Coverage = "sample"
			record.Generation = data.Observed.Session
			record.Order = sample.Seq
			record.Evidence = []string{sample.ID}
			record.Reasons = []string{"stream_expected_count_unknown", "sample_loss_unknown"}
			if data.Reason != "" {
				record.Reasons = append(record.Reasons, data.Reason)
			}
			if data.Status != "" {
				record.Dimensions["status"] = data.Status
			}
			if data.Code != "" {
				record.Dimensions["code"] = data.Code
			}
			if d.Kind == "occurrence" && data.Value != nil {
				record.Integer = telemetryPtr(int64(*data.Value))
			} else {
				record.Value = data.Value
			}
			if err := c.add(record); err != nil {
				return err
			}
		}
		if !page.More {
			return nil
		}
	}
}

func (c *telemetryCollector) filtered() []TelemetryRecord {
	result := []TelemetryRecord{}
	f := c.query.Filters
	for _, record := range c.records {
		d := c.descriptors[record.DescriptorID]
		if !telemetryIn(record.Metric, c.query.Metrics) || len(c.query.Metrics) == 0 && d.Availability == "unsupported" {
			continue
		}
		if !telemetryIn(record.Dimensions["status"], f.Status) || !telemetryIn(record.Dimensions["verdict"], f.Verdict) || !telemetryIn(record.Dimensions["severity"], f.Severity) || !telemetryIn(record.Origin, f.Origin) || !telemetryIn(record.Dimensions["code"], f.Code) || !telemetryIn(record.Subject.Kind, f.Scope) || !telemetryIn(record.Dimensions["step_id"], f.StepID) {
			continue
		}
		matches := true
		for dimension, values := range f.Dimensions {
			v, exists := record.Dimensions["custom."+dimension]
			match := false
			for _, value := range values {
				if string(value) == v {
					match = true
				}
			}
			if !exists || !match {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}
		if c.query.EventFrom != "" || c.query.EventBefore != "" {
			if record.Observed == nil {
				c.missingTimestamp++
				continue
			}
			if !telemetryWindow(record.Observed.UTC, c.query.EventFrom, c.query.EventBefore) {
				continue
			}
		}
		result = append(result, record)
	}
	return result
}
func telemetryRecordLess(a, b TelemetryRecord) bool {
	for _, pair := range [][2]string{{a.Subject.RunID, b.Subject.RunID}, {a.Subject.Kind, b.Subject.Kind}, {a.Subject.ID, b.Subject.ID}, {a.DescriptorID, b.DescriptorID}} {
		if pair[0] != pair[1] {
			return pair[0] < pair[1]
		}
	}
	if a.Order != b.Order {
		return a.Order < b.Order
	}
	return a.ID < b.ID
}
func telemetryCoverage(records []TelemetryRecord) TelemetryCoverage {
	coverage := TelemetryCoverage{Observed: len(records)}
	expected := len(records)
	coverage.Expected = &expected
	for _, record := range records {
		switch record.Quality {
		case "measured":
			coverage.Measured++
		case "partial":
			coverage.Partial++
		case "unavailable":
			coverage.Unavailable++
		case "estimated":
			coverage.Estimated++
		case "not_applicable":
			coverage.NotApplicable++
		}
		if record.IsOpen {
			coverage.Open++
		}
		if record.Coverage == "sample" || record.Coverage == "unknown" {
			coverage.ExpectedUnknown = true
			coverage.Expected = nil
		}
		if record.Coverage == "sample" {
			coverage.LossUnknown = true
		}
	}
	return coverage
}

func (c *telemetryCollector) aggregate(records []TelemetryRecord) ([]TelemetryAggregate, error) {
	groups := map[string][]TelemetryRecord{}
	labels := map[string]map[string]string{}
	// A state counter is owned by one Attempt generation. Changing a descriptive
	// label neither resets it nor creates a second independently additive source.
	// Group the whole selected source history using its latest selected labels.
	latestCounter := map[string]TelemetryRecord{}
	for _, record := range records {
		if c.descriptors[record.DescriptorID].Kind == "counter" {
			key := telemetryCounterSource(record)
			old, ok := latestCounter[key]
			if !ok || record.Order > old.Order {
				latestCounter[key] = record
			}
		}
	}
	for _, record := range records {
		if err := c.ctx.Err(); err != nil {
			return nil, err
		}
		d := c.descriptors[record.DescriptorID]
		dimensions := map[string]string{}
		groupingRecord := record
		if d.Kind == "counter" {
			groupingRecord = latestCounter[telemetryCounterSource(record)]
		}
		for _, key := range c.query.GroupBy {
			value, ok := groupingRecord.Dimensions[key]
			if !ok {
				value = "unknown"
			}
			dimensions[key] = value
		}
		// Instrument revision, actual scope and method are never silently mixed.
		if scope := record.Dimensions["resource_scope"]; scope != "" {
			dimensions["resource_scope"] = scope
		}
		if d.Kind == "gauge" {
			dimensions["source_identity"] = record.Subject.ID
			dimensions["source_generation"] = record.Generation
		}
		encoded, err := canonical(dimensions)
		if err != nil {
			return nil, err
		}
		key := record.DescriptorID + "\x00" + record.Subject.Kind + "\x00" + record.Method + "\x00" + string(encoded)
		if _, ok := groups[key]; !ok && len(groups) >= min(TelemetryMaxGroups, c.query.Limit) {
			return nil, telemetryProblem("query_limit", "complete aggregate exceeds requested group limit; no partial aggregate returned")
		}
		groups[key] = append(groups[key], record)
		labels[key] = dimensions
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := []TelemetryAggregate{}
	matched := map[string]bool{}
	for _, key := range keys {
		group := groups[key]
		d := c.descriptors[group[0].DescriptorID]
		aggregate, err := telemetrySummarize(d, group, labels[key])
		if err != nil {
			return nil, err
		}
		telemetrySelectAggregations(&aggregate, c.query.Aggregations)
		result = append(result, aggregate)
		matched[d.ID] = true
	}
	if len(c.query.Metrics) > 0 {
		ids := []string{}
		for id, d := range c.descriptors {
			if telemetryIn(d.Name, c.query.Metrics) && !matched[id] {
				ids = append(ids, id)
			}
		}
		sort.Strings(ids)
		for _, id := range ids {
			if len(result) >= min(TelemetryMaxGroups, c.query.Limit) {
				return nil, telemetryProblem("query_limit", "complete aggregate exceeds requested group limit")
			}
			d := c.descriptors[id]
			aggregate := TelemetryAggregate{DescriptorID: d.ID, Metric: d.Name, Scope: d.Scope, Method: d.Method, Unit: d.Unit, Dimensions: map[string]string{}, Quality: "unavailable", Coverage: TelemetryCoverage{ExpectedUnknown: true}, Reasons: []string{"no_matching_observation"}, Evidence: []string{}}
			if d.Availability == "unsupported" {
				aggregate.Reasons = []string{"unsupported_meter"}
			} else if d.Kind == "occurrence" && d.Method == "durable_journal" {
				aggregate.Quality = "measured"
				aggregate.Total = telemetryPtr(int64(0))
				aggregate.Coverage = TelemetryCoverage{Expected: telemetryPtr(0)}
				aggregate.Reasons = []string{"accepted_occurrences_only"}
			}
			telemetrySelectAggregations(&aggregate, c.query.Aggregations)
			result = append(result, aggregate)
		}
	}
	return result, nil
}

func telemetrySummarize(d TelemetryDescriptor, records []TelemetryRecord, dimensions map[string]string) (TelemetryAggregate, error) {
	sort.SliceStable(records, func(i, j int) bool { return telemetryRecordLess(records[i], records[j]) })
	g := TelemetryAggregate{DescriptorID: d.ID, Metric: d.Name, Scope: records[0].Subject.Kind, Method: records[0].Method, Unit: d.Unit, Dimensions: dimensions, Quality: "unavailable", Coverage: telemetryCoverage(records), Reasons: []string{}, Evidence: []string{}}
	for _, record := range records {
		if len(g.Evidence) < 5 {
			g.Evidence = append(g.Evidence, record.ID)
		}
	}
	if d.Kind == "ratio" {
		var n, den int64
		for _, record := range records {
			if record.Numerator == nil || record.Denominator == nil {
				return g, local.ErrIntegrity
			}
			n += *record.Numerator
			den += *record.Denominator
		}
		ratio := telemetryRatio(n, den)
		g.Ratio = &ratio
		g.Quality = ratio.Quality
		g.N = int(den)
		if den == 0 {
			g.Reasons = append(g.Reasons, "empty_known_denominator")
		}
		if g.Coverage.Unavailable > 0 {
			g.Reasons = append(g.Reasons, "ineligible_or_open_subjects_excluded_from_denominator")
		}
		return g, nil
	}
	if d.Kind == "counter" {
		return telemetryCumulative(g, records)
	}
	if d.Kind == "occurrence" {
		var total int64
		for _, record := range records {
			if record.Quality != "measured" || record.Integer == nil || record.IsOpen {
				continue
			}
			if *record.Integer < 0 || *record.Integer > int64(telemetryMaxNumber)-total {
				return g, telemetryProblem("measurement_overflow", "exact occurrence total overflows")
			}
			total += *record.Integer
			g.N++
		}
		if g.N > 0 {
			g.Total = &total
		}
		telemetryAggregateQuality(&g)
		return g, nil
	}
	values := []float64{}
	for _, record := range records {
		if record.Quality == "measured" && record.Value != nil && !record.IsOpen {
			if math.IsNaN(*record.Value) || math.IsInf(*record.Value, 0) || math.Abs(*record.Value) > telemetryMaxNumber {
				return g, telemetryProblem("invalid_measurement", "nonfinite or overflowing numeric record")
			}
			values = append(values, *record.Value)
		}
	}
	g.N = len(values)
	if g.N == 0 {
		telemetryAggregateQuality(&g)
		return g, nil
	}
	ordered := slices.Clone(values)
	sort.Float64s(ordered)
	g.Min = telemetryPtr(ordered[0])
	g.Max = telemetryPtr(ordered[len(ordered)-1])
	if d.Kind == "gauge" {
		// A full replacement without the field invalidates the current value.
		latest := records[0]
		for _, record := range records[1:] {
			if record.Order > latest.Order {
				latest = record
			}
		}
		if latest.Quality == "measured" && latest.Value != nil {
			g.Last = telemetryPtr(*latest.Value)
		} else {
			g.Reasons = append(g.Reasons, "latest_gauge_value_unavailable")
		}
	} else {
		// Compensated addition; never average group means or percentiles.
		sum, compensation := 0.0, 0.0
		for _, value := range values {
			corrected := value - compensation
			next := sum + corrected
			compensation = (next - sum) - corrected
			sum = next
			if math.IsInf(sum, 0) || math.IsNaN(sum) || math.Abs(sum) > telemetryMaxNumber {
				return g, telemetryProblem("measurement_overflow", "distribution sum exceeds finite bound")
			}
		}
		g.Sum = &sum
		g.Mean = telemetryPtr(sum / float64(len(values)))
		g.P50 = telemetryPtr(ordered[int(math.Ceil(.5*float64(len(ordered))))-1])
		g.P95 = telemetryPtr(ordered[int(math.Ceil(.95*float64(len(ordered))))-1])
		g.QuantileMethod = "exact_nearest_rank"
		if g.N < 20 {
			g.Reasons = append(g.Reasons, "small_sample_n_less_than_20")
		}
	}
	telemetryAggregateQuality(&g)
	return g, nil
}
func telemetryAggregateQuality(g *TelemetryAggregate) {
	if g.N == 0 {
		g.Quality = "unavailable"
		if g.Coverage.NotApplicable == g.Coverage.Observed && g.Coverage.Observed > 0 {
			g.Quality = "not_applicable"
		}
		g.Reasons = append(g.Reasons, "no_complete_closed_measurements")
		return
	}
	g.Quality = "measured"
	if g.Coverage.Unavailable+g.Coverage.Partial+g.Coverage.Estimated+g.Coverage.Open > 0 || g.Coverage.ExpectedUnknown {
		g.Quality = "partial"
		g.Reasons = append(g.Reasons, "complete_values_only_population_not_fully_measured")
	}
	if g.Coverage.LossUnknown {
		g.Reasons = append(g.Reasons, "sample_loss_unknown")
	}
}
func telemetryCumulative(g TelemetryAggregate, records []TelemetryRecord) (TelemetryAggregate, error) {
	series := map[string][]TelemetryRecord{}
	for _, record := range records {
		key := telemetryCounterSource(record)
		series[key] = append(series[key], record)
	}
	var total, delta int64
	complete, hasBaseline := true, true
	for _, values := range series {
		sort.SliceStable(values, func(i, j int) bool { return values[i].Order < values[j].Order })
		var first, last *int64
		observations := 0
		for _, value := range values {
			if value.Quality != "measured" || value.Integer == nil {
				continue
			}
			if *value.Integer < 0 || *value.Integer > int64(telemetryMaxNumber) {
				return g, telemetryProblem("invalid_measurement", "cumulative counter is outside exact bounds")
			}
			if last != nil && *value.Integer < *last {
				return g, telemetryProblem("counter_decreased", "cumulative counter decreased within one source generation")
			}
			if first == nil {
				first = value.Integer
			}
			last = value.Integer
			observations++
		}
		latest := values[len(values)-1]
		if latest.Integer == nil || latest.Quality != "measured" || latest.IsOpen {
			complete = false
			continue
		}
		if first == nil || last == nil {
			complete = false
			continue
		}
		if *last > int64(telemetryMaxNumber)-total || *last-*first > int64(telemetryMaxNumber)-delta {
			return g, telemetryProblem("measurement_overflow", "exact cumulative aggregation overflows")
		}
		total += *last
		delta += *last - *first
		if observations < 2 {
			hasBaseline = false
		}
		g.N++
	}
	if complete && g.N > 0 {
		g.Total = &total
		if hasBaseline {
			g.Delta = &delta
		}
	}
	telemetryAggregateQuality(&g)
	if g.N > 0 {
		g.Quality = "partial"
		g.Reasons = append(g.Reasons, "delta_is_first_to_last_observation_not_full_window_increment", "baseline_before_first_observation_unknown", "counter_groups_use_latest_selected_source_dimensions")
		if !hasBaseline {
			g.Reasons = append(g.Reasons, "at_least_two_observations_required_for_delta")
		}
	}
	if !complete {
		g.Reasons = append(g.Reasons, "latest_counter_value_unavailable")
	}
	return g, nil
}
func telemetryCounterSource(record TelemetryRecord) string {
	return record.DescriptorID + "\x00" + record.Subject.RunID + "\x00" + record.Subject.ID + "\x00" + record.Generation
}
func telemetrySelectAggregations(g *TelemetryAggregate, requested []string) {
	if len(requested) == 0 {
		return
	}
	if !slices.Contains(requested, "sum") {
		g.Sum = nil
	}
	if !slices.Contains(requested, "min") {
		g.Min = nil
	}
	if !slices.Contains(requested, "max") {
		g.Max = nil
	}
	if !slices.Contains(requested, "mean") {
		g.Mean = nil
	}
	if !slices.Contains(requested, "p50") {
		g.P50 = nil
	}
	if !slices.Contains(requested, "p95") {
		g.P95 = nil
	}
	if !slices.Contains(requested, "last") {
		g.Last = nil
	}
	if !slices.Contains(requested, "total") {
		g.Total = nil
	}
	if !slices.Contains(requested, "delta") {
		g.Delta = nil
	}
	if !slices.Contains(requested, "ratio") {
		g.Ratio = nil
	}
	if g.P50 == nil && g.P95 == nil {
		g.QuantileMethod = ""
	}
}

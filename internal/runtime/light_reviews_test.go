package runtime

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stenhigh/prifly/internal/flow"
)

type lightReviewFixture struct {
	Engine     *Engine
	Run        Run
	Invocation *Invocation
	Mapping    LightReviewMapping
	Projection *LightProjection
	Cache      *LightCache
	Reader     *Attempt
}

func newLightReviewFixture(t *testing.T, gateStatus, result, checkStatus string, optional bool, change func(map[string]any)) lightReviewFixture {
	t.Helper()
	e, _ := emptyRuntime(t)
	reg := flow.Registry{}
	refs := map[string]flow.Ref{}
	definitions := []PinnedDefinition{}
	for _, name := range []string{"request", "report", "gate", "candidate"} {
		body, err := os.ReadFile(filepath.Join("../../test/fixtures/light/consumer/aif-review-evidence/schemas", name+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		value, err := flow.Parse(body, "yaml")
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		digest, err := flow.Digest(raw)
		if err != nil {
			t.Fatal(err)
		}
		ref := flow.Ref{ID: value.(map[string]any)["id"].(string), Version: "1.0.0", Digest: digest}
		reg[ref] = raw
		refs[name] = ref
		definitions = append(definitions, PinnedDefinition{Ref: ref, Kind: "schema", Bytes: raw})
	}
	put := func(name string, value any) ArtifactRef {
		t.Helper()
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		ref := refs[name]
		a, err := e.putArtifact(body, "json", &ref, newID("artifact"), map[string]any{"kind": "authority", "authority_id": e.Installation.ID, "command_id": "command:light-fixture", "port": name}, nil, reg)
		if err != nil {
			t.Fatal(err)
		}
		return a.Ref()
	}
	candidate := map[string]any{"algorithm": "git-candidate/1", "base_commit": strings.Repeat("a", 40), "head_commit": strings.Repeat("b", 40), "index_digest": "sha256:" + strings.Repeat("1", 64), "files_digest": "sha256:" + strings.Repeat("2", 64), "digest": "sha256:" + strings.Repeat("3", 64)}
	request := map[string]any{"base_commit": strings.Repeat("a", 40), "round": 1, "checks": []any{map[string]any{"id": "security", "description": "typed fixture", "method": "code", "required": !optional, "enabled": true, "applicable": true}}, "optional_policy": "ignore", "require_separate_session": false}
	candidateRef, requestRef := put("candidate", candidate), put("request", request)
	report := map[string]any{"round": 1, "request_digest": requestRef.Digest, "candidate": candidate, "session": "unavailable", "model_profile": "unavailable", "model": nil, "checks": []any{map[string]any{"id": "security", "status": checkStatus, "result": result, "reason": "Recorded typed finding", "evidence": []any{}}}}
	if change != nil {
		change(report)
	}
	reportRef := put("report", report)
	gateRef := put("gate", map[string]any{"gate": "review", "status": gateStatus, "blocking": gateStatus == "blocked" || gateStatus == "failed", "blocking_owner_only": gateStatus == "blocked", "findings": []string{"Typed gate evidence"}, "suggested_next": "continue"})
	inv := &Invocation{ID: "invocation:review", Inputs: map[string]ArtifactRef{"request": requestRef}, WorkflowRef: flow.Ref{ID: "fixture:workflow/review", Version: "1.0.0", Digest: "sha256:" + strings.Repeat("4", 64)}}
	reader := &Attempt{ID: "attempt:reader", ActivationID: "activation:reader", Admitted: Observation{UTC: "2026-10-17T10:00:00Z"}, Context: ContextManifest{Inputs: map[string]LocalPort{"request": {Ref: requestRef}, "candidate": {Ref: candidateRef}}}, Accepted: &Result{Verdict: "pass", Outputs: map[string]ArtifactRef{"report": reportRef}}}
	gate := &Attempt{ID: "attempt:gate", ActivationID: "activation:gate", Accepted: &Result{Verdict: "pass", Outputs: map[string]ArtifactRef{"gate": gateRef}}, Context: ContextManifest{Inputs: map[string]LocalPort{"report": {Ref: reportRef}, "candidate": {Ref: candidateRef}}}}
	r := Run{ID: "run:typed-fixture", RootInvocationID: inv.ID, Definitions: definitions, Invocations: map[string]*Invocation{inv.ID: inv}, Activations: map[string]*Activation{reader.ActivationID: {ID: reader.ActivationID, InvocationID: inv.ID, StageID: "review"}, gate.ActivationID: {ID: gate.ActivationID, InvocationID: inv.ID, StageID: "evaluate"}}, Attempts: map[string]*Attempt{reader.ID: reader, gate.ID: gate}}
	m := LightReviewMapping{Lens: "Security", ReportStage: "review", ReportPort: "report", ReportSchema: refs["report"], GateStage: "evaluate", GatePort: "gate", GateSchema: refs["gate"], GateScope: "lens", GateReportInputs: []string{"report"}, RequestPort: "request", CandidatePort: "candidate", RoundPointer: "/round", CandidatePointer: "/candidate", RequestPointer: "/request_digest", ChecksPointer: "/checks", GateStatusPointer: "/status", GateBlockingPointer: "/blocking", GateOwnerPointer: "/blocking_owner_only", FindingsPointer: "/findings", DisabledState: "disabled"}
	projection := &LightProjection{SchemaVersion: LightProjectionVersion, Digest: "sha256:" + strings.Repeat("5", 64), Workflows: []LightWorkflowMapping{{WorkflowRef: inv.WorkflowRef, Phases: []LightPhaseMapping{}, Reviews: []LightReviewMapping{m}}}}
	cache := NewLightCache()
	cache.budget = 16 << 20
	return lightReviewFixture{e, r, inv, m, projection, cache, reader}
}
func TestLightTypedReportAndGateCannotInventPass(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		Name, Gate, Result, Status, Expected string
		Optional                             bool
		Change                               func(map[string]any)
	}{
		{Name: "passed", Gate: "passed", Result: "passed", Status: "executed", Expected: "passed"},
		{Name: "usable report blocked gate", Gate: "blocked", Result: "passed", Status: "executed", Expected: "unavailable"},
		{Name: "defect", Gate: "failed", Result: "failed", Status: "executed", Expected: "defect"},
		{Name: "required unavailable", Gate: "blocked", Result: "not_run", Status: "unavailable", Expected: "unavailable"},
		{Name: "optional excluded", Gate: "passed", Result: "not_run", Status: "excluded", Expected: "warnings", Optional: true},
		{Name: "wrong round", Gate: "passed", Result: "passed", Status: "executed", Expected: "unknown", Change: func(r map[string]any) { r["round"] = 2 }},
		{Name: "wrong candidate", Gate: "passed", Result: "passed", Status: "executed", Expected: "unknown", Change: func(r map[string]any) {
			c := r["candidate"].(map[string]any)
			copy := map[string]any{}
			for k, v := range c {
				copy[k] = v
			}
			copy["digest"] = "sha256:" + strings.Repeat("e", 64)
			r["candidate"] = copy
		}},
		{Name: "wrong request", Gate: "passed", Result: "passed", Status: "executed", Expected: "unknown", Change: func(r map[string]any) { r["request_digest"] = "sha256:" + strings.Repeat("f", 64) }},
		{Name: "report warnings", Gate: "warnings", Result: "warnings", Status: "executed", Expected: "warnings"},
	} {
		t.Run(tc.Name, func(t *testing.T) {
			t.Parallel()
			f := newLightReviewFixture(t, tc.Gate, tc.Result, tc.Status, tc.Optional, tc.Change)
			row := f.Engine.lightReview(f.Run, f.Cache, f.Projection, f.Invocation, f.Mapping, f.Reader, 12)
			if row.State != tc.Expected {
				t.Fatalf("false acceptance: %+v", row)
			}
			if row.Report == nil || row.Request == nil || row.Candidate == nil || row.Cut != 12 {
				t.Fatal("missing provenance")
			}
			if tc.Expected == "passed" && row.Gate == nil {
				t.Fatal("passed without gate")
			}
			if err := validatePublic(t, "LightReviews", LightReviewsPage{SchemaVersion: "prifly-light-reviews/1", Source: LightSourceInfo{Freshness: "fresh", Coverage: "complete"}, Reviews: []LightReview{row}, Page: LightPage{Limit: 10, Complete: true}}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestLightRepairMissingReportAndParallelScopes(t *testing.T) {
	t.Parallel()
	f := newLightReviewFixture(t, "passed", "passed", "executed", false, nil)
	f.Mapping.RepairStages = []string{"repair"}
	f.Run.Activations["activation:repair"] = &Activation{ID: "activation:repair", InvocationID: f.Invocation.ID, StageID: "repair", Created: Observation{UTC: "2026-10-17T10:01:00Z"}}
	row := f.Engine.lightReview(f.Run, f.Cache, f.Projection, f.Invocation, f.Mapping, f.Reader, 13)
	if row.State == "passed" || row.Freshness != "stale" {
		t.Fatalf("repair retained pass: %+v", row)
	}
	delete(f.Run.Activations, "activation:repair")
	f.Reader.Accepted = nil
	row = f.Engine.lightReview(f.Run, f.Cache, f.Projection, f.Invocation, f.Mapping, f.Reader, 14)
	if row.State != "waiting" {
		t.Fatalf("pending report: %+v", row)
	}
	f.Reader.Settled = &Observation{UTC: "2026-10-17T10:02:00Z"}
	row = f.Engine.lightReview(f.Run, f.Cache, f.Projection, f.Invocation, f.Mapping, f.Reader, 15)
	if row.State != "unknown" {
		t.Fatalf("missing report became acceptance: %+v", row)
	}
}

func TestLightFreshnessHandlesFractionalAndUnknownTimes(t *testing.T) {
	t.Parallel()
	if !lightAtOrAfter(Observation{UTC: "2026-10-17T10:00:00.1Z"}, Observation{UTC: "2026-10-17T10:00:00Z"}) {
		t.Fatal("later fractional repair retained pass")
	}
	if !lightAtOrAfter(Observation{}, Observation{UTC: "2026-10-17T10:00:00Z"}) {
		t.Fatal("unknown repair ordering retained pass")
	}
}

func TestLightApplicabilityIsThreeValuedAndBounded(t *testing.T) {
	t.Parallel()
	f := newLightReviewFixture(t, "passed", "passed", "executed", false, nil)
	scalar := LightCondition{Input: "request", Pointer: "/round", Equals: json.RawMessage(`1`)}
	missing := LightCondition{Input: "missing", Pointer: "/round", Equals: json.RawMessage(`1`)}
	for _, test := range []struct {
		Condition    LightCondition
		Value, Known bool
	}{
		{scalar, true, true},
		{LightCondition{All: []LightCondition{scalar, missing}}, false, false},
		{LightCondition{Any: []LightCondition{scalar, missing}}, true, true},
		{LightCondition{Input: "request", Pointer: "/checks", Equals: json.RawMessage(`true`)}, false, false},
	} {
		value, known := f.Engine.lightCondition(f.Run, f.Cache, f.Invocation, test.Condition)
		if value != test.Value || known != test.Known {
			t.Fatalf("invented applicability: %v/%v", value, known)
		}
	}
	nested := scalar
	for range 6 {
		nested = LightCondition{All: []LightCondition{nested}}
	}
	nodes := 0
	if lightConditionValid(nested, 0, &nodes) {
		t.Fatal("unbounded predicate accepted")
	}
}

func TestLightConsumerAggregateGateAndBundleProof(t *testing.T) {
	t.Parallel()
	f := newLightReviewFixture(t, "passed", "passed", "executed", false, nil)
	body, err := os.ReadFile("../../test/fixtures/light/consumer/aif-feature/schemas/feature-gate.yaml")
	if err != nil {
		t.Fatal(err)
	}
	value, err := flow.Parse(body, "yaml")
	if err != nil {
		t.Fatal(err)
	}
	schema, _ := json.Marshal(value)
	digest, _ := flow.Digest(schema)
	ref := flow.Ref{ID: "aif-feature:schema/feature-gate", Version: "1.0.0", Digest: digest}
	f.Run.Definitions = append(f.Run.Definitions, PinnedDefinition{Ref: ref, Kind: "schema", Bytes: schema})
	f.Mapping.GateSchema = ref
	f.Mapping.GateScope = "aggregate"
	raw := []byte(`{"gate":"feature","status":"blocked","blocking":true,"blocking_owner_only":true,"findings":["Owner decision required"],"suggested_next":"owner"}`)
	gate, err := f.Engine.putArtifact(raw, "json", &ref, newID("artifact"), map[string]any{"kind": "authority", "authority_id": f.Engine.Installation.ID, "command_id": "command:aggregate-fixture", "port": "gate"}, nil, f.Run.registry())
	if err != nil {
		t.Fatal(err)
	}
	f.Run.Attempts["attempt:gate"].Accepted.Outputs["gate"] = gate.Ref()
	row := f.Engine.lightReview(f.Run, f.Cache, f.Projection, f.Invocation, f.Mapping, f.Reader, 20)
	if row.CheckStatus != "passed" || row.State != "unavailable" || !row.OwnerOnly || row.GateScope != "aggregate" {
		t.Fatalf("W5 aggregate fabricated acceptance: %+v", row)
	}
	report := f.Reader.Accepted.Outputs["report"]
	_, bytes, err := f.Engine.Artifact(report)
	if err != nil {
		t.Fatal(err)
	}
	refValue := map[string]any{"artifact_id": report.ArtifactID, "revision": float64(report.Revision), "digest": report.Digest}
	entry := map[string]any{"branch": "security", "ref": refValue, "raw_base64": base64.StdEncoding.EncodeToString(bytes)}
	bundle := map[string]any{"status": "ready", "reports": []any{entry}}
	if !lightBundleContains(bundle, report) {
		t.Fatal("exact W5 bundle was not correlated")
	}
	entry["raw_base64"] = base64.StdEncoding.EncodeToString([]byte(`{"invented":"pass"}`))
	if lightBundleContains(bundle, report) {
		t.Fatal("bundle ref hid different raw bytes")
	}
}
func TestLightUIUXLensesKeepTheirSharedGateScope(t *testing.T) {
	t.Parallel()
	f := newLightReviewFixture(t, "blocked", "passed", "executed", false, nil)
	other := *f.Reader
	other.ID = "attempt:ux"
	other.ActivationID = "activation:ux"
	f.Run.Attempts[other.ID] = &other
	f.Run.Activations[other.ActivationID] = &Activation{ID: other.ActivationID, InvocationID: f.Invocation.ID, StageID: "ux"}
	ui, ux := f.Mapping, f.Mapping
	ui.Lens = "UI"
	ui.ReportStage = "review"
	ui.GateScope = "aggregate"
	ui.GateReportInputs = []string{"ui_report"}
	ux.Lens = "UX"
	ux.ReportStage = "ux"
	ux.GateScope = "aggregate"
	ux.GateReportInputs = []string{"ux_report"}
	gate := f.Run.Attempts["attempt:gate"]
	gate.Context.Inputs["ui_report"] = LocalPort{Ref: f.Reader.Accepted.Outputs["report"]}
	gate.Context.Inputs["ux_report"] = LocalPort{Ref: other.Accepted.Outputs["report"]}
	f.Projection.Workflows[0].Reviews = []LightReviewMapping{ui, ux}
	rows := f.Engine.lightReviews(f.Run, f.Cache, f.Projection, 21)
	if len(rows) != 2 || rows[0].ID == rows[1].ID {
		t.Fatalf("lost UI/UX identities: %+v", rows)
	}
	for _, row := range rows {
		if row.CheckStatus != "passed" || row.GateScope != "aggregate" || row.State != "unavailable" {
			t.Fatalf("shared gate lost scope: %+v", row)
		}
	}
	// No producer invocations for an optional workflow means no invented lens.
	f.Projection.Workflows[0].WorkflowRef.ID = "fixture:workflow/not-entered"
	if rows := f.Engine.lightReviews(f.Run, f.Cache, f.Projection, 22); len(rows) != 0 {
		t.Fatalf("invented optional reviewers: %+v", rows)
	}
}

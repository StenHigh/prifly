package runtime

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"time"

	"github.com/stenhigh/prifly/internal/flow"
)

func (e *Engine) lightArtifact(r Run, c *LightCache, ref ArtifactRef, schema *flow.Ref) (lightArtifact, error) {
	if item, ok := c.Artifacts[ref]; ok {
		if schema != nil && (item.Meta.SchemaRef == nil || *item.Meta.SchemaRef != *schema) {
			return item, lightIntegrity("artifact schema differs from mapping")
		}
		return item, nil
	}
	meta, err := e.artifactMetadata(ref)
	if err != nil {
		return lightArtifact{}, err
	}
	if schema != nil && (meta.SchemaRef == nil || *meta.SchemaRef != *schema) {
		return lightArtifact{}, lightIntegrity("artifact schema differs from mapping")
	}
	if meta.SizeBytes > 4<<20 || meta.SizeBytes > c.budget {
		return lightArtifact{}, fault("light_report_unavailable", "artifact exceeds projection byte budget")
	}
	_, body, err := e.Artifact(ref)
	if err != nil {
		return lightArtifact{}, err
	}
	c.budget -= int64(len(body))
	c.Reads++
	// Metadata alone does not establish typed content. Validate against the
	// exact pinned schema once before retaining a content-addressed value.
	if meta.SchemaRef != nil {
		if err := flow.ValidateSchema(r.registry(), *meta.SchemaRef, body); err != nil {
			return lightArtifact{}, err
		}
	}
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return lightArtifact{}, err
	}
	item := lightArtifact{meta, value}
	if c.artifactBytes+meta.SizeBytes > 32<<20 || len(c.Artifacts) >= 8192 {
		c.Artifacts = map[ArtifactRef]lightArtifact{}
		c.artifactBytes = 0
	}
	c.Artifacts[ref] = item
	c.artifactBytes += meta.SizeBytes
	return item, nil
}
func lightAttempts(r Run, inv, stage string) []*Attempt {
	list := []*Attempt{}
	for _, a := range r.Attempts {
		if a == nil {
			continue
		}
		act := r.Activations[a.ActivationID]
		if act != nil && act.InvocationID == inv && act.StageID == stage {
			list = append(list, a)
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].AdmittedVersion != list[j].AdmittedVersion {
			return list[i].AdmittedVersion < list[j].AdmittedVersion
		}
		left, leftErr := time.Parse(time.RFC3339Nano, list[i].Admitted.UTC)
		right, rightErr := time.Parse(time.RFC3339Nano, list[j].Admitted.UTC)
		if leftErr == nil && rightErr == nil && !left.Equal(right) {
			return left.Before(right)
		}
		return list[i].ID < list[j].ID
	})
	return list
}
func lightLatestOutput(r Run, inv, stage, port string) (ArtifactRef, bool) {
	attempts := lightAttempts(r, inv, stage)
	for i := len(attempts) - 1; i >= 0; i-- {
		if attempts[i].Accepted != nil {
			ref, ok := attempts[i].Accepted.Outputs[port]
			if ok {
				return ref, true
			}
		}
	}
	return ArtifactRef{}, false
}
func (e *Engine) lightReviews(r Run, c *LightCache, p *LightProjection, cut int64) []LightReview {
	result := []LightReview{}
	if p == nil {
		return result
	}
	invocations := map[string]*Invocation{}
	for id, inv := range r.Invocations {
		invocations[id] = inv
	}
	if len(invocations) == 0 {
		invocations[r.RootInvocationID] = &Invocation{ID: r.RootInvocationID, WorkflowRef: r.WorkflowRef, Inputs: r.Inputs, Outputs: r.Outputs}
	}
	ids := []string{}
	for id := range invocations {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		inv := invocations[id]
		if inv == nil {
			continue
		}
		for _, w := range p.Workflows {
			if w.WorkflowRef != inv.WorkflowRef {
				continue
			}
			for _, m := range w.Reviews {
				attempts := lightAttempts(r, id, m.ReportStage)
				if len(attempts) == 0 {
					pending := slices.Contains(r.readyFor(id), m.ReportStage) || m.Enabled != nil
					for _, activation := range r.Activations {
						if activation != nil && activation.InvocationID == id && activation.StageID == m.ReportStage && activation.Settled == nil {
							pending = true
						}
					}
					if !pending {
						continue
					}
					attempts = []*Attempt{nil}
				}
				for _, a := range attempts {
					row := e.lightReview(r, c, p, inv, m, a, cut)
					iteration := inv
					seen := map[string]bool{}
					for iteration.Iteration == nil && iteration.ParentInvocationID != "" && !seen[iteration.ID] {
						seen[iteration.ID] = true
						parent := invocations[iteration.ParentInvocationID]
						if parent == nil {
							break
						}
						iteration = parent
					}
					for _, sibling := range invocations {
						if sibling != nil && iteration.Iteration != nil && sibling.Iteration != nil && sibling.ParentInvocationID == iteration.ParentInvocationID && sibling.CallerActivationID == iteration.CallerActivationID && *sibling.Iteration > *iteration.Iteration {
							row.Freshness = "stale"
							row.State = "unknown"
							row.Reason = "Предыдущая итерация; нужна приёмка текущего candidate"
						}
					}
					result = append(result, row)
				}
			}
		}
	}
	return result
}
func (e *Engine) lightReview(r Run, c *LightCache, p *LightProjection, inv *Invocation, m LightReviewMapping, a *Attempt, cut int64) LightReview {
	row := LightReview{ID: inv.ID + "/" + m.Lens, RunID: r.ID, InvocationID: inv.ID, Lens: m.Lens, Applicability: "unknown", ReportSchema: m.ReportSchema, GateSchema: m.GateSchema, GateScope: m.GateScope, CheckStatus: "unknown", GateStatus: "unknown", Freshness: "unknown", State: "unknown", Findings: []string{}, Reason: "Нет данных", MappingDigest: p.Digest, Cut: cut}
	if inv.Iteration != nil {
		row.IterationID = fmt.Sprintf("%s/%d", inv.CallerActivationID, *inv.Iteration)
	}
	if m.Enabled != nil {
		enabled, known := e.lightCondition(r, c, inv, *m.Enabled)
		if !known {
			row.Reason = "Applicability не установлена"
			return row
		}
		if !enabled {
			row.Applicability = m.DisabledState
			row.State = m.DisabledState
			row.Reason = "Не включено записанной конфигурацией"
			return row
		}
	}

	if a == nil {
		row.Reason = "Выполнение reader ещё не записано"
		return row
	}
	row.ID += "/" + a.ID
	row.AttemptID = a.ID
	row.ActivationID = a.ActivationID
	request, ok := a.Context.Inputs[m.RequestPort]
	if !ok {
		row.Reason = "Нет закреплённого request"
		return row
	}
	row.Request = &request.Ref
	req, err := e.lightArtifact(r, c, request.Ref, nil)
	if err != nil {
		row.Reason = err.Error()
		return row
	}
	row.RequestDigest = &request.Ref.Digest
	candidate, ok := a.Context.Inputs[m.CandidatePort]
	if !ok {
		row.Reason = "Нет закреплённого candidate"
		return row
	}
	row.Candidate = &candidate.Ref
	current, err := e.lightArtifact(r, c, candidate.Ref, nil)
	if err != nil {
		row.Reason = err.Error()
		return row
	}
	basisInvocation := inv.ID
	for range m.GateAncestor {
		at := r.Invocations[basisInvocation]
		if at == nil || at.ParentInvocationID == "" {
			row.Reason = "Basis invocation отсутствует"
			return row
		}
		basisInvocation = at.ParentInvocationID
	}
	if m.CandidateStage != "" {
		if latest, ok := lightLatestOutput(r, basisInvocation, m.CandidateStage, m.CandidateOutput); !ok || latest != candidate.Ref {
			row.Freshness = "stale"
			row.Reason = "Candidate изменился или не подтверждён"
			return row
		}
	}
	repairInvocation := inv.ID
	for range max(m.GateAncestor, m.RepairAncestor) {
		at := r.Invocations[repairInvocation]
		if at == nil || at.ParentInvocationID == "" {
			row.Reason = "Repair scope не установлен"
			return row
		}
		repairInvocation = at.ParentInvocationID
	}
	for _, stage := range m.RepairStages {
		for _, act := range r.Activations {
			if act != nil && act.InvocationID == repairInvocation && act.StageID == stage && lightAtOrAfter(act.Created, a.Admitted) {
				row.Freshness = "stale"
				row.Reason = "Записан repair; прежняя приёмка недействительна"
				return row
			}
		}
	}
	if value, ok := lightPointer(current.Value, "/digest"); ok {
		if s, ok := value.(string); ok {
			row.CandidateDigest = &s
		}
	}
	if round, ok := lightPointer(req.Value, m.RoundPointer); ok {
		if n, ok := round.(float64); ok && n >= 0 && n == float64(int64(n)) {
			v := int64(n)
			row.Round = &v
		}
	}
	applicable, reason := lightRequestApplicability(req.Value, m.ChecksPointer)
	row.Applicability = applicable
	if applicable == "disabled" || applicable == "not_applicable" {
		row.State = applicable
		row.Reason = reason
		return row
	}
	if applicable != "enabled" {
		row.Reason = reason
		return row
	}
	if a.Accepted == nil {
		if a.Settled == nil {
			row.State = "waiting"
			row.Reason = "Ожидает recorded report"
		} else {
			row.Reason = "Reader завершён без принятого отчёта"
		}
		return row
	}
	row.ProducerVerdict = a.Accepted.Verdict
	reportRef, ok := a.Accepted.Outputs[m.ReportPort]
	if !ok {
		row.Reason = "Reader завершён без report port"
		return row
	}
	row.Report = &reportRef
	report, err := e.lightArtifact(r, c, reportRef, &m.ReportSchema)
	if err != nil {
		row.Freshness = "invalid"
		row.Reason = err.Error()
		return row
	}
	basis, ok := lightPointer(report.Value, m.CandidatePointer)
	reportRound, roundOK := lightPointer(report.Value, m.RoundPointer)
	reportRequest, requestOK := lightPointer(report.Value, m.RequestPointer)
	if !ok || !reflect.DeepEqual(basis, current.Value) || !roundOK || row.Round == nil || reportRound != float64(*row.Round) || !requestOK || reportRequest != request.Ref.Digest {
		row.Freshness = "stale"
		row.Reason = "Report не соответствует request/candidate/round"
		return row
	}
	row.Freshness = "fresh"
	row.CheckStatus, row.Findings = lightChecks(req.Value, report.Value, m.ChecksPointer)
	row.State = row.CheckStatus
	row.Reason = "Приёмка gate ещё не подтверждена"
	gateInv := inv.ID
	for range m.GateAncestor {
		at := r.Invocations[gateInv]
		if at == nil || at.ParentInvocationID == "" {
			row.State = "unknown"
			row.Reason = "Gate invocation отсутствует"
			return row
		}
		gateInv = at.ParentInvocationID
	}
	gates := lightAttempts(r, gateInv, m.GateStage)
	for _, stage := range m.GateAlternatives {
		gates = append(gates, lightAttempts(r, gateInv, stage)...)
	}
	found := false
	for _, g := range gates {
		if g.Accepted == nil {
			continue
		}
		gateRef, ok := g.Accepted.Outputs[m.GatePort]
		if !ok {
			continue
		}
		correlated := false
		for _, port := range m.GateReportInputs {
			input, ok := g.Context.Inputs[port]
			if !ok {
				continue
			}
			if input.Ref == reportRef {
				correlated = true
				break
			}
			bundle, err := e.lightArtifact(r, c, input.Ref, nil)
			if err == nil && lightBundleContains(bundle.Value, reportRef) {
				correlated = true
				break
			}
		}
		if !correlated {
			continue
		}
		if input, exists := g.Context.Inputs[m.CandidatePort]; exists && input.Ref != candidate.Ref {
			continue
		}
		if found {
			row.State = "unknown"
			row.Freshness = "invalid"
			row.Reason = "Несколько gate для одного report"
			return row
		}
		found = true
		row.Gate = &gateRef
		gate, err := e.lightArtifact(r, c, gateRef, &m.GateSchema)
		if err != nil {
			row.State = "unknown"
			row.Freshness = "invalid"
			row.Reason = err.Error()
			return row
		}
		status, _ := lightPointer(gate.Value, m.GateStatusPointer)
		blocking, _ := lightPointer(gate.Value, m.GateBlockingPointer)
		owner, _ := lightPointer(gate.Value, m.GateOwnerPointer)
		var valid bool
		row.GateStatus, valid = status.(string)
		if !valid || !slices.Contains([]string{"passed", "warnings", "failed", "blocked"}, row.GateStatus) {
			row.State = "unknown"
			row.Freshness = "invalid"
			row.Reason = "Неизвестный gate status"
			return row
		}
		row.Blocking, valid = blocking.(bool)
		if !valid {
			row.State = "unknown"
			row.Freshness = "invalid"
			row.Reason = "Нет gate blocking"
			return row
		}
		row.OwnerOnly, valid = owner.(bool)
		if !valid {
			row.State = "unknown"
			row.Freshness = "invalid"
			row.Reason = "Нет gate owner scope"
			return row
		}
		if findings, ok := lightPointer(gate.Value, m.FindingsPointer); ok {
			if list, ok := findings.([]any); ok {
				for _, item := range list {
					if s, ok := item.(string); ok {
						row.Findings = append(row.Findings, s)
					}
				}
			}
		}
	}
	if !found {
		if row.State == "passed" || row.State == "warnings" {
			row.State = "waiting"
		}
		return lightFindings(row)
	}
	row.Reason = "Recorded checks и gate"
	switch {
	case row.GateStatus == "passed" && (row.Blocking || row.OwnerOnly):
		row.State = "unknown"
		row.Freshness = "invalid"
		row.Reason = "Противоречивый gate contract"
	case row.CheckStatus == "defect":
		row.State = "defect"
	case row.GateStatus == "blocked" || row.Blocking || row.OwnerOnly:
		row.State = "unavailable"
	case row.GateStatus == "failed":
		row.State = "defect"
	case row.CheckStatus == "unknown":
		row.State = "unknown"
	case row.CheckStatus == "unavailable":
		row.State = "unavailable"
	case row.CheckStatus == "warnings" || row.GateStatus == "warnings":
		row.State = "warnings"
	case row.CheckStatus == "passed" && row.GateStatus == "passed":
		row.State = "passed"
	default:
		row.State = "unknown"
	}
	return lightFindings(row)
}
func lightFindings(row LightReview) LightReview {
	row.FindingCount = len(row.Findings)
	if len(row.Findings) > 3 {
		row.Findings = row.Findings[:3]
		row.Truncated = true
	}
	for i, s := range row.Findings {
		preview := lightPreview(s, 160)
		row.Truncated = row.Truncated || preview != s
		row.Findings[i] = preview
	}
	return row
}
func lightBundleContains(value any, ref ArtifactRef) bool {
	object, ok := value.(map[string]any)
	if !ok || object["status"] != "ready" {
		return false
	}
	reports, ok := object["reports"].([]any)
	if !ok || len(reports) > 16 {
		return false
	}
	for _, item := range reports {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		b, _ := json.Marshal(entry["ref"])
		var got ArtifactRef
		if json.Unmarshal(b, &got) == nil && got == ref {
			encoded, ok := entry["raw_base64"].(string)
			if !ok || len(encoded) > 5592408 {
				continue
			}
			body, err := base64.StdEncoding.DecodeString(encoded)
			if err == nil && rawDigest(body) == ref.Digest {
				return true
			}
		}
	}
	return false
}
func lightRequestApplicability(request any, pointer string) (string, string) {
	value, ok := lightPointer(request, pointer)
	checks, valid := value.([]any)
	if !ok || !valid || len(checks) == 0 || len(checks) > 80 {
		return "unknown", "Нет declared check composition"
	}
	enabled, applicable := false, false
	for _, raw := range checks {
		check, ok := raw.(map[string]any)
		if !ok {
			return "unknown", "Invalid check composition"
		}
		on, onOK := check["enabled"].(bool)
		ap, apOK := check["applicable"].(bool)
		if !onOK || !apOK {
			return "unknown", "Unknown check applicability"
		}
		enabled = enabled || on
		applicable = applicable || on && ap
	}
	if !enabled {
		return "disabled", "Все checks явно выключены"
	}
	if !applicable {
		return "not_applicable", "Checks явно не применимы"
	}
	return "enabled", ""
}
func lightChecks(request, report any, pointer string) (string, []string) {
	req, _ := lightPointer(request, pointer)
	rep, _ := lightPointer(report, pointer)
	declared, ok := req.([]any)
	actual, valid := rep.([]any)
	findings := []string{}
	if !ok || !valid || len(actual) > 80 {
		return "unknown", findings
	}
	checks := map[string]map[string]any{}
	for _, raw := range actual {
		check, ok := raw.(map[string]any)
		if !ok {
			return "unknown", findings
		}
		id, ok := check["id"].(string)
		if !ok || checks[id] != nil {
			return "unknown", findings
		}
		checks[id] = check
	}
	state := "passed"
	seen := map[string]bool{}
	executed := false
	for _, raw := range declared {
		d, ok := raw.(map[string]any)
		if !ok {
			return "unknown", findings
		}
		id, ok := d["id"].(string)
		if !ok || seen[id] {
			return "unknown", findings
		}
		seen[id] = true
		if d["enabled"] != true || d["applicable"] != true {
			continue
		}
		check := checks[id]
		if check == nil {
			return "unknown", findings
		}
		required, ok := d["required"].(bool)
		if !ok {
			return "unknown", findings
		}
		status, result := check["status"], check["result"]
		if reason, ok := check["reason"].(string); ok && reason != "" {
			findings = append(findings, reason)
		}
		if status != "executed" || result == "not_run" {
			if required {
				if state != "defect" {
					state = "unavailable"
				}
			} else if state == "passed" {
				state = "warnings"
			}
			continue
		}
		executed = true
		switch result {
		case "failed":
			state = "defect"
		case "warnings":
			if state == "passed" {
				state = "warnings"
			}
		case "passed":
		default:
			return "unknown", findings
		}
	}
	for id := range checks {
		if !seen[id] {
			return "unknown", findings
		}
	}
	if !executed && state == "passed" {
		state = "unknown"
	}
	return state, findings
}

func (e *Engine) lightCondition(r Run, c *LightCache, inv *Invocation, condition LightCondition) (bool, bool) {
	if len(condition.All) > 0 || len(condition.Any) > 0 {
		children := condition.All
		all := len(children) > 0
		if !all {
			children = condition.Any
		}
		unknown := false
		for _, child := range children {
			value, known := e.lightCondition(r, c, inv, child)
			if !known {
				unknown = true
				continue
			}
			if all && !value {
				return false, true
			}
			if !all && value {
				return true, true
			}
		}
		if unknown {
			return false, false
		}
		return all, true
	}
	ref, ok := inv.Inputs[condition.Input]
	if !ok {
		return false, false
	}
	artifact, err := e.lightArtifact(r, c, ref, nil)
	if err != nil {
		return false, false
	}
	value, ok := lightPointer(artifact.Value, condition.Pointer)
	if !ok {
		return false, false
	}
	switch value.(type) {
	case nil, string, bool, float64:
	default:
		return false, false
	}

	values := condition.AnyOf
	if len(condition.Equals) > 0 {
		values = []json.RawMessage{condition.Equals}
	}
	for _, raw := range values {
		var expected any
		if json.Unmarshal(raw, &expected) != nil {
			return false, false
		}
		if reflect.DeepEqual(value, expected) {
			return true, true
		}
	}
	return false, true
}

func lightAtOrAfter(left, right Observation) bool {
	l, le := time.Parse(time.RFC3339Nano, left.UTC)
	r, re := time.Parse(time.RFC3339Nano, right.UTC)
	// Unknown ordering cannot keep a previous PASS alive after recorded repair.
	return le != nil || re != nil || !l.Before(r)
}

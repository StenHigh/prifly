package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	prifly "github.com/stenhigh/prifly/internal/runtime"
)

// The host here is scripted. This proves the public protocol and real native
// workers, not a human's identity or Codex/Claude's question presentation.
func TestCLIProjectMixedDecisionResumesWithoutGit(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("the native CSV workers require Node")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	resolved, err := exec.CommandContext(ctx, node, "-p", "process.execPath").Output()
	cancel()
	if err != nil || !filepath.IsAbs(strings.TrimSpace(string(resolved))) {
		t.Fatalf("resolve actual Node binary: %q %v", resolved, err)
	}
	node = strings.TrimSpace(string(resolved))
	t.Setenv("PATH", t.TempDir())
	root, authority := t.TempDir(), filepath.Join(t.TempDir(), "authority")
	command := func(args ...string) string {
		t.Helper()
		code, out, stderr := runCLI(t, args...)
		if code != 0 {
			t.Fatalf("%v: exit=%d %s", args, code, stderr)
		}
		return out
	}
	decode := func(text string, value any) {
		t.Helper()
		if err := json.Unmarshal([]byte(text), value); err != nil {
			t.Fatal(err)
		}
	}
	writeJSON := func(value any) string {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "request.json")
		writeFixtureFile(t, filepath.Dir(path), filepath.Base(path), string(data))
		return path
	}
	command("project", "init", "--repository", root, "--state-root", authority, "--host", "codex-app")
	folder := ".prifly/workflows/mixed"
	// Reuse the shipped native parse/report workers and their data contract.
	// Only the middle host operation and its decision are fixture-specific.
	for _, name := range []string{"steps/parse.yaml", "steps/report.yaml", "schemas/rows.yaml", "files/worker.mjs", "sample.csv"} {
		data, err := os.ReadFile(filepath.Join("../../examples/workflows/csv-report", name))
		if err != nil {
			t.Fatal(err)
		}
		writeFixtureFile(t, root, folder+"/"+name, string(data))
	}
	writeFixtureFile(t, root, ".prifly/project.yaml", `schema_version: prifly-project-profile/3
hosts: {codex-app: .agents/skills}
packages: {mixed: {source: .prifly/workflows/mixed}}
launches:
  mixed:
    title: Scale CSV rows
    description: Native parse, scripted host decision, native report.
    kind: workflow
    workflow: .prifly/workflows/mixed/workflow.yaml
`)
	writeFixtureFile(t, root, folder+"/workflow.yaml", mixedDecisionWorkflow)
	writeFixtureFile(t, root, folder+"/steps/scale.yaml", `authoring: prifly-step/1
id: test:step/scale
version: 1.0.0
kind: worker
inputs:
  rows: {schema_ref: "{{schema_rows}}"}
outputs:
  rows: {schema_ref: "{{schema_rows}}", required_for: [pass]}
executor: {adapter_ref: "{{assisted}}", operation: session}
instructions_ref: "{{context_scale}}"
effects: {class: none, retry_class: never}
result_schema_ref: "{{result}}"
`)
	writeFixtureFile(t, root, folder+"/contexts/scale.yaml", `id: test:context/scale
version: 1.0.0
media_type: text/markdown; charset=utf-8
text: Request the declared multiplier decision, then multiply each amount by the accepted value. Write only the rows output slot.
`)
	writeFixtureFile(t, root, folder+"/decisions/multiplier.yaml", `authoring: prifly-run-decision/1
id: multiplier
title: Choose the amount multiplier
phase: runtime
choices: [{id: once, title: Keep amounts, value: 1}, {id: twice, title: Double amounts, value: 2}]
sensitivity: ordinary
destination: {kind: session_context, name: multiplier}
`)
	command("project", "local", "set", "--repository", root, "--allow-executable", "node="+node)
	start := []string{"project", "start", "--repository", root, "--launch", "mixed", "--input", "csv=" + filepath.Join(root, folder, "sample.csv"), "--allow-execution"}
	// A declared assisted step needs an explicit host, but not a Git workspace.
	if code, _, stderr := runCLI(t, start...); code == 0 || !strings.Contains(stderr, "project_start_host_required") {
		t.Fatalf("missing host did not fail before dispatch: %d %s", code, stderr)
	}
	var started projectStartResult
	decode(command(append(start, "--host", "codex-app")...), &started)
	runID := started.Run.Run.ID
	if runID == "" || started.Workspace != nil || started.Run.Run.Brief != (prifly.ArtifactRef{}) {
		t.Fatal("mixed start lost its identity or manufactured a task/repository claim")
	}
	var task prifly.SessionTask
	decode(command("--project", authority, "session", "task", "--run", runID), &task)
	if !task.DecisionBridge || task.ClaimID != "" || task.RepositoryWorkspace != "" || len(task.SkillRefs) != 1 {
		t.Fatalf("wrong real assisted handoff: %+v", task)
	}
	readSnapshot := func() (prifly.Run, []byte) {
		t.Helper()
		// No mutable engine survives any CLI call. This extra read checks exact
		// durable bytes and claims without relaxing public-view redaction.
		engine, err := prifly.Open(authority, true)
		if err != nil {
			t.Fatal(err)
		}
		defer engine.Close()
		claims, err := engine.Claims(context.Background())
		if err != nil || len(claims.Claims) != 0 {
			t.Fatalf("effects:none acquired a repository claim: %+v %v", claims, err)
		}
		saved, err := engine.Store.Read(context.Background(), runID, 0, 1)
		if err != nil {
			t.Fatal(err)
		}
		var run prifly.Run
		decode(string(saved.Snapshot.Data), &run)
		return run, saved.Snapshot.Data
	}
	assertNotFinal := func() {
		t.Helper()
		run, _ := readSnapshot()
		if len(run.Attempts) != 2 || len(run.Outputs) != 0 {
			t.Fatal("final managed command was admitted before the assisted result")
		}
		for _, activation := range run.Activations {
			if activation.StageID == "report" {
				t.Fatal("final stage became active before the assisted result")
			}
		}
	}
	assertNotFinal()
	rowsPath := filepath.Join(task.Workspace, task.Context.Inputs["rows"].Path)
	rowsBefore, err := os.ReadFile(rowsPath)
	if err != nil || !bytes.Contains(rowsBefore, []byte(`"amount":12`)) {
		t.Fatalf("native parse did not feed the host: %q %v", rowsBefore, err)
	}
	run, _ := readSnapshot()
	definition := run.DecisionCatalog.Decisions[0]
	digest, err := prifly.DecisionDefinitionDigest(definition)
	if err != nil {
		t.Fatal(err)
	}
	request := prifly.DecisionRequest{SchemaVersion: prifly.DecisionRequestVersion, RunID: runID, AttemptID: task.AttemptID, EnvelopeDigest: task.EnvelopeDigest, DecisionID: definition.ID, DefinitionDigest: digest, ExpectedRunVersion: task.RunVersion}
	// The decision variant omits result entirely; JSON null is not a result
	// and must not be sent alongside a decision request.
	command("--project", authority, "session", "submit", "--file", writeJSON(map[string]any{"schema_version": task.SchemaVersion, "run_id": runID, "attempt_id": task.AttemptID, "envelope_digest": task.EnvelopeDigest, "decision_request": request}))
	var next prifly.NextView
	decode(command("--project", authority, "run", "next", runID), &next)
	if next.Action != "waiting_decision" {
		t.Fatalf("typed request did not survive reopen: %s", next.Action)
	}
	if code, _, stderr := runCLI(t, "--project", authority, "session", "task", "--run", runID); code == 0 || !strings.Contains(stderr, "no_active_handoff") {
		t.Fatalf("waiting delivery remained executable: %d %s", code, stderr)
	}
	var ledger struct {
		RunID         string                  `json:"run_id"`
		RunVersion    int64                   `json:"run_version"`
		Pending       *prifly.DecisionRequest `json:"pending"`
		RequestDigest string                  `json:"pending_request_digest"`
	}
	decode(command("--project", authority, "run", "decisions", runID), &ledger)
	if ledger.RunID != runID || ledger.Pending == nil || ledger.Pending.AttemptID != task.AttemptID || ledger.RequestDigest == "" {
		t.Fatal("reopened pending decision changed identity")
	}
	answer := []string{"--project", authority, "run", "decision", runID, "answer", "--decision", definition.ID, "--request-digest", ledger.RequestDigest, "--expected-run-version", strconv.FormatInt(ledger.RunVersion, 10), "--value"}
	_, beforeInvalid := readSnapshot()
	if code, _, stderr := runCLI(t, append(append([]string{}, answer...), `"double"`)...); code == 0 || !strings.Contains(stderr, "invalid_decision_answer") {
		t.Fatalf("wrong typed answer was accepted: %d %s", code, stderr)
	}
	_, afterInvalid := readSnapshot()
	if !bytes.Equal(beforeInvalid, afterInvalid) {
		t.Fatal("invalid answer changed durable Run state")
	}
	command("--project", authority, "run", "drive", runID)
	assertNotFinal()
	// Drive may record observations; use the current generation of the same
	// pending request, not an invented receipt digest or a new Run.
	decode(command("--project", authority, "run", "decisions", runID), &ledger)
	answer[len(answer)-2] = strconv.FormatInt(ledger.RunVersion, 10)
	command(append(answer, "2")...)
	var resumed prifly.SessionTask
	decode(command("--project", authority, "session", "task", "--run", runID), &resumed)
	if resumed.AttemptID != task.AttemptID || resumed.RunID != runID || resumed.EnvelopeDigest == task.EnvelopeDigest || string(resumed.DecisionContext["multiplier"]) != "2" {
		t.Fatal("accepted answer did not redeliver the same assisted attempt")
	}
	command("--project", authority, "run", "drive", runID)
	assertNotFinal()
	var data struct {
		Rows []struct {
			Name   string `json:"name"`
			Amount int    `json:"amount"`
		} `json:"rows"`
	}
	decode(string(rowsBefore), &data)
	for i := range data.Rows {
		data.Rows[i].Amount *= 2
	}
	rowsAfter, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	slot := resumed.Context.Outputs["rows"]
	writeFixtureFile(t, resumed.Workspace, slot.Path, string(rowsAfter))
	result, err := json.Marshal(prifly.Result{SchemaVersion: "1", RunID: runID, StepInstanceID: resumed.StepInstanceID, AttemptID: resumed.AttemptID, EnvelopeDigest: resumed.EnvelopeDigest, Verdict: "pass", Outputs: map[string]prifly.ArtifactRef{"rows": {ArtifactID: slot.ArtifactID, Revision: slot.Revision, Digest: fmt.Sprintf("sha256:%x", sha256.Sum256(rowsAfter))}}, EvidenceRefs: []any{}, EffectReceiptRefs: []any{}, Summary: "Scripted host applied the accepted multiplier"})
	if err != nil {
		t.Fatal(err)
	}
	command("--project", authority, "session", "submit", "--file", writeJSON(prifly.SessionSubmission{SchemaVersion: resumed.SchemaVersion, RunID: runID, AttemptID: resumed.AttemptID, EnvelopeDigest: resumed.EnvelopeDigest, Result: result}))
	var final prifly.RunView
	for i := 0; i < 3; i++ {
		decode(command("--project", authority, "run", "drive", runID), &final)
		if final.Run.Status == "completed" {
			break
		}
	}
	if final.Run.Status != "completed" || final.Run.Outcome == nil || *final.Run.Outcome != "succeeded" || len(final.Run.Attempts) != 3 || final.Run.PendingDecision != nil {
		t.Fatalf("mixed Run did not complete: status=%s diagnostics=%+v", final.Run.Status, final.Run.Diagnostics)
	}
	if len(final.Run.DecisionLedger) != 1 || final.Run.DecisionLedger[0].Status != "answered" || final.Run.DecisionLedger[0].Source != "actor" || string(final.Run.DecisionLedger[0].Value) != "2" {
		t.Fatal("terminal ledger lost the accepted typed answer and source")
	}
	exported := filepath.Join(t.TempDir(), "report.txt")
	command("--project", authority, "artifact", "export", "--ref", writeJSON(final.Run.Outputs["report"]), "--output", exported)
	output, err := os.ReadFile(exported)
	if err != nil || string(output) != "Pri-Fly CSV report\nRows: 3\nTotal: 48\n" {
		t.Fatalf("final native command did not consume the host's changed rows: %q %v", output, err)
	}
	run, _ = readSnapshot()
	if run.ID != runID || run.Brief != (prifly.ArtifactRef{}) {
		t.Fatal("final reopen changed Run identity or absent brief")
	}
	if _, err := os.Stat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
		t.Fatal("mixed Run created Git state", err)
	}
}

const mixedDecisionWorkflow = `authoring: prifly-project-workflow/1
package:
  id: test:package/mixed
  version: 1.0.0
  description: Native commands surrounding an assisted typed decision.
  requires_core_protocol: "1"
  references:
    adapter: core:adapter/local-process@2.0.0
    assisted: core:adapter/assisted-session@1.0.0
    policy: core:policy/local@3.0.0
    result: core:schema/step-result@1.0.0
id: test:workflow/mixed
version: 1.0.0
inputs:
  csv: {format: blob, media_types: [text/csv]}
outputs:
  report: {format: blob, media_types: [text/plain], required_for: [succeeded]}
decision_catalog: [.prifly/workflows/mixed/decisions/multiplier.yaml]
entry: parse
policy_ref: "{{policy}}"
limits: {max_step_instances: 3, max_control_transitions: 8}
stages:
  parse:
    kind: step
    step_ref: "{{step_parse}}"
    input_bindings: {csv: $inputs.csv}
    on: {pass: scale, fail: rejected, needs_revision: rejected, no_work: rejected}
    on_error: rejected
  scale:
    kind: step
    step_ref: "{{step_scale}}"
    input_bindings: {rows: $stages.parse.rows}
    on: {pass: report, fail: rejected, needs_revision: rejected, no_work: rejected}
    on_error: rejected
  report:
    kind: step
    step_ref: "{{step_report}}"
    input_bindings: {rows: $stages.scale.rows}
    on: {pass: done, fail: rejected, needs_revision: rejected, no_work: rejected}
    on_error: rejected
  done: {kind: finish, outcome: succeeded, output_bindings: {report: $stages.report.report}}
  rejected: {kind: finish, outcome: rejected}
execution_bindings:
  steps:
    example:step/parse:
      executable: node
      args: [worker.mjs, parse]
      files: {worker.mjs: files/worker.mjs}
      timeout_ms: 10000
      grace_ms: 100
      max_output_bytes: 1048576
    example:step/report:
      executable: node
      args: [worker.mjs, report]
      files: {worker.mjs: files/worker.mjs}
      timeout_ms: 10000
      grace_ms: 100
      max_output_bytes: 1048576
`

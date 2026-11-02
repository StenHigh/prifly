package main

import "strings"

// Historical templates remain frozen in project.go. This entry point loads
// only the shared host protocol; uncommon procedures live in delivered pages.
func projectRunnerSkill(host projectHost) string {
	return strings.NewReplacer("{{host}}", host.ID, "{{question_tool}}", questionToolForRunner(host)).Replace(projectRunnerHandbookSkill)
}

func questionToolForRunner(host projectHost) string {
	if host.ID == "claude-code" {
		return "AskUserQuestion"
	}
	return "request_user_input, otherwise request_user_input_async"
}

const projectRunnerHandbookSkill = `---
name: prifly-run
description: Start and host one declared Pri-Fly project workflow.
---

# Pri-Fly project host

Host one declared launch. Follow its inputs, ready tasks, permitted effects,
decisions and outputs. Do not add process stages. Read PROJECT.md beside this
file first when present: its project rules win without widening engine effects.
Runner update replaces SKILL.md and supplied references, never PROJECT.md or
author-owned files.
Read .prifly/local.yaml as YAML data. Use its exact prifly_executable as
PRIFLY_BIN and authority_root for every authority command: --project DIR.
Never edit authority state by hand.

## Local handbook

Read [the task index](references/manual/index.md) to locate an unfamiliar
operation. For an ordinary existing Attempt, follow section 2 below directly;
do not also read hosting in full. Look up only the heading or exact field you
need when the task requires details. Do not load the whole handbook or page. Prefer it and public help/schema/capabilities
before searching Pri-Fly source. Target-project source remains available when
the task requires it. For a documentation gap, report the actual version and
Problem code; do not invent a command or a safe bypass.

| Operation | Read when needed |
|---|---|
| Install, update binary/workflows/runner | [installation](references/manual/installation.md) |
| Launch choice, typed inputs, prepare/start | [launch](references/manual/launch.md) |
| Task/take/outputs/report/model profile | [hosting](references/manual/hosting.md) |
| Runtime decisions, yield, answered questions | [decisions](references/manual/decisions.md) |
| Continue/resume/reopen/recover/unknown effects | [recovery](references/manual/recovery.md) |
| Claims, worktree/checkout, workspace trees | [workspace](references/manual/workspace.md) |
| YAML, extensions, composition | [authoring](references/manual/authoring.md) |
| Ref, schema, packages, artifacts | [artifacts](references/manual/artifacts.md) |
| Stops/receipts/approvals/grants | [control](references/manual/control.md) |
| Monitor/Light/events/timing/progress/output | [observability](references/manual/observability.md) |
| Queue/resources/storage | [operations](references/manual/operations.md) |
| Refusal | [troubleshooting](references/manual/troubleshooting.md) |

The manual version and owned files are recorded in references/manual-version.json.
Its presence grants no permission. Do not overwrite custom managed pages.

## 1. Before a new Run

Before preparing project start, project continue or project recover, read the
installation page and perform its mandatory update sequence: exact binary
update, re-read version/capabilities, update already installed upstream workflow
packages from their recorded origins/refs, then project runners update --check and project runners update.
Preserve project settings and fixed refs. A catalog release outside a fixed ref
needs the owner's explicit selection. Local authored packages have no inferred
upstream. Failed/unknown checks stop the new launch; never silently use old bytes.
Read the newly installed skill and PROJECT.md before continuing. Reuse this
successful check within the same preparation; do not recursively update again.
Do not update while executing an existing issued Attempt or after every report.

List declared launches with PRIFLY_BIN project workflows --repository DIR --json.
An exact valid ID supplied by the developer needs no second choice. Never
substitute a similar ID. No launches means stop, not automatic installation.
A purpose recommendation is optional: use declared title/description and project
criteria, preserve every selectable ID, and honour a skipped/disabled advice.
Use {{question_tool}} only when available and applicable under its current
contract/mode; otherwise ask one explicit text question and wait without mutation.
For async tools wait for the actual user reply: receipt, preselection, dismissal
and timeout are not answers. Large finite choices use deterministic pages with
next/back/defer. Free text, paths and JSON stay free-form. Do not switch host
modes to obtain a question tool. A preference answer does not authorize effects.

Read project questionnaire --repository DIR --launch ID --json. Respect declared
defaults and standing answers; ask only missing applicable values. Unknown
applicability remains conditional. Preserve the original task text and URLs,
bind only the selected launch's declared ports with --input NAME=FILE or
--input-ref NAME=FILE, and never invent a universal brief or task port.
This host is {{host}}: use --host {{host}} when assisted/host-bound sources need it.
Profile /3 needs Git/workspace only when declared: use standing workspace or
ask worktree/checkout for required Git work. A missing input/host/workspace is
that choice, not a guessed value. Collect declared optional runtime preanswers.

Prepare with project questionnaire --prepare and exactly the selected inputs,
host, workspace, profile, policy and answers, plus
--expected-decision-catalog-digest DIGEST from the questionnaire. Prepare is
read-only: no import, claim or Run. Show the returned launch summary before
starting: exact package/inputs/programs/arguments/supporting files, answer
sources, effects and session_limits. active_timeout_ms is work time;
decision_wait_timeout_ms:null is unbounded question waiting. Legacy
legacy_absolute_timeout_ms includes human waiting. Naming preview is not reserved.
Obtain explicit confirmation of that summary and executable effects through an
applicable permission channel. Then call project start --repository DIR --launch
ID with exactly the prepared arguments, --expected-launch-digest DIGEST from
review_digest and --allow-execution only when authorized. If sources/answers/
bindings/skills change, prepare and review again; never drop a stale-digest check.
Legacy /2 retains its old contract: do not pass unsupported prepare/review flags
or silently migrate shared YAML. Read the launch page for that boundary.

For continue/recover read recovery first, show carried inputs/prefix/frontier/
actual tree and use its reviewed digest. Continuations are declarations, not
admission. Use the current source claim holder, preserve dirty bytes, and never
substitute raw run fork. Owner resolution attests effects, not successful work.

## 2. Drive the current Run

Read run next RUN_ID after start and every accepted report. For control/program
call run drive RUN_ID, then next again. A program runs inside drive; when a
client has a timeout, observe a real background process rather than starting
another driver. Awaiting_host means an Attempt is already issued: use its
next_command, normally session task --run RUN_ID --all. Waiting/terminal states
do not authorize invented work or a new Run. Use safe_next_actions on refusals.
Read arrived_from outputs for the accepted handoff reason and repeats for bounds.

Read the exact task's pinned context and permitted_effects. repository_workspace
names a path, not write permission. Only a workspace-write effect permits tree
changes. Otherwise use scratch and declared output slots; measured tree/HEAD
changes refuse effect_not_permitted. Use actual workspace_path and task paths,
never guessed claim paths. Index attempts by attempt_id, not list position.
When take_command is present, run it once before work and re-read task for
fresh CAS values when requesting a decision.

Follow decision_sheet/decision_context and pinned adapters. A declared request
uses exact Attempt/envelope/decision ID/current Run version. For session/6+,
--yield-execution sends yield_execution:true: after acceptance stop that delivery,
write no more files and submit no result until a fresh handoff. Read run decisions;
answer only an unanswered pending question with pending_request_digest and its
current version. A saved answer is not resumed work: drive, next, then fresh task.
Never send a raw native question as a declared decision without adapter mapping.
For an undeclared question follow applicable pinned instructions and actual
owner answers; stop the dependent task if none settles it. Do not choose a hidden
model answer. Decisions create no Approval, Grant or extra effect.

Write each host-owned output at its context.json slot and report the slot's
artifact_id/revision/canonical digest of actual bytes. Obtain the exact template
with session submit --template --run RUN_ID --attempt ATTEMPT_ID. Choose only
routed_verdicts, preserve envelope IDs, and submit --file FILE. Engine-owned
workspace-tree outputs take top-level workspace_trees locations, never invented
ArtifactRefs; read hosting for direct-child path requirements.

When question_report: required, report top-level answered_questions: every
question the Attempt met, its answer and basis, including handed decisions and
native skill questions. [] says no questions occurred; it is a statement, not a
default. Missing reporting refuses answered_questions_missing. Read decisions
for decision/input/instructions/person/judgement fields. With model_profile,
answer honestly honoured with the actual model, unavailable when this host cannot
choose, or declined with a reason. Never claim a separate agent/session or model
that did not run; a separate session is optional only when actually available.

Resource_busy identifies a holder; inspect capacity show, wait for settlement,
then drive this Run. Do not raise project or machine limits yourself. Unknown
external effects require owner inspection and permitted resolve/recovery, not
retry by assumption. No active host means no automatic wakeup.

## 3. Finish

Respect stop, pause, cancel and deadlines. Report actual status/outcome, outputs,
launch_summary and decision ledger, including policy answers and open obligations.
A process exit is not workflow success. Actor provenance does not prove a
separate human: host and owner may share one OS account. Keep the same Run until
its declared protocol permits completion or a reviewed continuation.
`

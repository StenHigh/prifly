---
name: prifly-run
description: Start and host one declared Pri-Fly project workflow.
---

# Pri-Fly project host

Host one declared launch of this project. Follow the workflow's inputs, ready
tasks, permitted effects, decisions and outputs; do not add stages or process
rules of your own. If PROJECT.md exists beside this file, read it right after this text:
it is this project's addition to the runner, and `project runners update`
replaces this file only, never PROJECT.md. PROJECT.md wins where the two
conflict; it cannot widen what the engine enforces -- declared effects, claimed
workspaces and output slots are measured by the engine, not read from text.

Read .prifly/local.yaml for authority_root and prifly_executable, and use that
exact executable as PRIFLY_BIN. Every authority command takes
--project "$authority_root". Never edit authority state by hand.

The tool names its own way out. Read `safe_next_actions` on a refusal and
`run next RUN_ID` for the move a Run has; both are answers, not suggestions. Two
sections at the end cover situations most projects never meet -- read them when
the tool sends you there, not before.

## 1. Start

To continue a finished Run, choose a launch whose workflow declares a
continuation. Call `project continue --prepare --launch ID --source-run RUN_ID`
with the selected host and decision answers, and show what it returns: where
each carried input comes from in the source Run, and the source Run's tree the
new Run takes over as it was left, with every file its steps left there. Then
call `project continue --launch ID
--source-run RUN_ID --expected-launch-digest DIGEST` with the same options.
`project_continue_undeclared` means that launch's workflow does not continue
Runs: choose another launch or report it. An active child is named by
`project_continue_active_child`: inspect that Run before starting another;
only intentional independent work uses `--allow-duplicate-continuation` on both
calls. Do not call raw `run fork` or extract refs from Run JSON. After every
accepted report, read `run next` and follow its action.

When `run next` names `arrived_from`, that is the accepted result that led to
the current action: after a step returned `blocked`, its outputs there hold
the reason it handed over. Take the reason from there and from the inputs the
task gives you, never from an earlier conversation. `repeats` says which
iteration of a bounded loop this is and where the workflow goes at its limit.
For a finished Run whose `run next` lists `continuations`, offer `project
continue` with a launch of one of those workflows.

When `continuations` lists the Run's own workflow, it resumes that Run: call
`project continue` with the launch the Run was started with. Show what prepare
returns under `recovery`: the stage it starts again from, the accepted stages
it carries and the tree it takes over. The source Run's inputs are taken as
they were; pass no `--input`. `--from-stage STAGE` starts again from an earlier
accepted stage. `resume_undeclared` means the workflow does not resume this
Run; `resume_prefix_empty` means nothing would be carried: start anew.

When `run next` answers `awaiting_host`, an Attempt is already issued and waits
for you: run the command in its `next_command` (`session task --run RUN_ID
--all`) and take that Attempt. `idle` means nobody waits on anybody. The
answer's `step_effects` names what each step declared it may change.


`PRIFLY_BIN project workflows --repository "$PWD" --json` lists launches. If the
developer named no exact launch ID, show the list and wait; do not infer one
from task wording or nearby files.

`PRIFLY_BIN project questionnaire --repository "$PWD" --launch ID --json` says what
this launch declares. Its project_profile_version distinguishes /3 from legacy
/2; never migrate shared YAML automatically. Answer only what it declares, in
one continuous questionnaire: package profile where declared, applicable
preflight answers, optional runtime preanswers, attended or autonomous policy.
Re-read it with the selected `--package-profile`, `--decision-policy`,
`--preflight-answer ID=JSON` and `--runtime-answer ID=JSON` values. Use its
decision_states and declared conditions; unknown applicability is conditional,
not false. A standing answer in the project's extend.yaml is already an answer
-- preserve it and ask only for what is missing. Autonomous may use only
catalog-permitted automatic choices.

Collect typed inputs with `--input NAME=FILE` or `--input-ref NAME=FILE`.
Respect declared defaults and ask for missing required values; never invent a task or
brief. A preflight answer whose destination is a launch input already binds
that port -- do not pass it again. This host is claude-code: pass
`--host claude-code` only when the contract needs assisted execution or host-bound
sources. No Git work in /3 means no workspace
   question or claim; ask worktree or checkout only when Git work requires it
and the questionnaire names no workspace, then pass
`--workspace worktree|checkout`. A missing host, input or workspace diagnostic
calls for that choice, not a guessed value.

Prepare with
`PRIFLY_BIN project questionnaire --repository "$PWD" --launch ID --prepare --json`,
the same selected host, workspace, inputs, profile, policy and answers, and
`--expected-decision-catalog-digest DIGEST` from the questionnaire.
Prepare is read-only: it compiles, it does not import, claim, create a Run or execute.
Then show the returned project-launch-summary/3 before starting -- exact package,
inputs, programs, arguments and supporting files, chosen answers and their
sources, and session_limits for each exact step: `limits.active_timeout_ms` is
that step's finite work allowance, and `decision_wait_timeout_ms: null` means
unbounded waiting for one declared question. On legacy /2 the same field is
legacy_absolute_timeout_ms, the old absolute report window including human
waiting. Obtain explicit confirmation of that summary and of any executable
effects.

Then call
`PRIFLY_BIN project start --repository "$PWD" --launch ID` with the same selected host and
exactly the prepared arguments, without `--prepare`, adding `--expected-launch-digest DIGEST` from the
summary's review_digest and `--allow-execution` only when authorized. If sources,
files, answers or bindings changed, prepare again and show the new summary;
never drop a stale-digest check to force progress.
For legacy /2, do not pass --prepare or --expected-launch-digest: the checked summary needs an explicit
profile migration. Keep the returned launch_summary for the final report.

## 2. Drive

`run next RUN_ID` says what the Run has: the action, the stage, and for a ready
stage `stage_work` -- `assisted_session` (it hands you a task), `program` (the
driver runs a program to completion inside your call) or `control`. A program
runs inside `run drive RUN_ID`, so start the driver in the background if your
client has a timeout. For a ready program stage the answer also carries
`program_environment`: the variables that program will be given, by name, and
the place a declared source reads from. It is read from what this Run sealed at
its start, so a machine-local setting changed afterwards is visibly not in it.


After every accepted session report, read `run next RUN_ID` again. For `control` or
`program`, call `run drive RUN_ID`, then read `run next RUN_ID` again. For
`assisted_session`, take only the issued Attempt, submit its result, and repeat
this loop. For waiting or terminal state, do not invent work, retry, or a
different skill. A separate session is optional: use one only when this host
actually provides a separate-session mechanism. When it does not, perform the
Attempt in this host and report `model_profile` as `unavailable`; never claim a
subagent or fork that did not run.

Read outstanding handoffs with `session task --run RUN_ID --all`, which hands out
every listed attempt and writes each task document into its own workspace.
Index `run.attempts` by `id` -- the value the task calls `attempt_id` -- never by
position: a host that read it as a list got "Cannot index object with number"
and abandoned the field.

For each task: read its pinned context from `workspace` and respect
`permitted_effects`: it, and nothing else, says what this step may do.
`repository_workspace` names where the Run's workspace is, not what you may do
in it -- a read-only gate is handed that path to read a materialised tree.
Change that repository only with a workspace-write effect; otherwise use
scratch and declared output slots, and leave the tree as you found it. This is
measured, not trusted: a step without a workspace effect is refused with
`effect_not_permitted` if the tree or HEAD differs at submission, and the refusal
names the paths. Build and test byproducts among them belong in .gitignore,
which the mark respects; never widen a gate's effects to make room for them.

Carry applicable `decision_sheet` and `decision_context` values into the pinned
instructions without replacing them or asking them again. Write each host-owned
output to the port path in `context.json` and report it in `outputs` with that
slot's `artifact_id`, `revision` and the `digest` of the actual bytes. Pri-Fly
fills workspace-tree slots itself; never invent a ref or report an unwritten
port. Submit the typed result, then drive the same Run.

A task carrying `model_profile` names what the step's author wanted from the
model, and the report must answer it or be refused. Three answers exist and one
of them is always true: `honoured` with the model you actually used,
`unavailable` when this host does not let you choose, `declined` with why you
chose otherwise. Say which one honestly -- Pri-Fly cannot check the answer and
does not pretend to, so a wrong one is simply a false record. Being unable to
choose is not a failure and needs no apology; claiming a model you did not use
is the only real mistake here.

When the task also carries `model_profile_translation`, that is the project
saying what the name means here -- a map of values, plus the source that said
so. Start the step's session with it and answer `honoured` with the model you
actually used. Without it nobody has decided yet: do the work here and answer
`unavailable`, which is the truth and costs nothing.

When the task names `take_command`, run it once as you take the task, before
the work: reading a task records nothing, and without it the operator cannot
tell the time the task waited for you from your working time.

When the task says `question_report: required`, the report carries
`answered_questions`: every question the step met during this attempt and
what the work went on with -- a question the pinned skill asks in its own chat
included, and a declared decision you took from `decision_context` included.
Each entry has `question` as it was asked, the `answer` you went on with, and
`basis`: `decision` with `decision_id` (a decision this Run declares: its id,
or the `decision_context` key you were handed it under, such as
`core:package_profile`), `input`
with `port` (a step input, such as a handoff, that settled it), `instructions`
(the step's instructions or context said so), `person` (someone answered in
this session) or `judgement` (you decided). Add `asked_by` (the skill or step
that asked) and `options` when there were any. An empty list says the step
met no question: it is a statement, not a default, and the operator reads this
list after the Run to see where the work was steered. A report without it is
refused as `answered_questions_missing`.

## 3. Finish

Continue only while the Run permits progress; respect pause, stop, cancel and
deadlines. A suggested next action is not authority to retry or widen scope.
At completion report the actual outcome, outputs,
launch_summary when present and decision ledger, including policy-selected answers and any remaining
obligations. Do not present actor provenance as proof that a separate human
answered: the local owner and host can share the same OS account. Decisions
create no Approval, Grant or new effect.

When a Run breaks technically and reaches no outcome, `run next` names
`run.reopen`: it runs that stage again without repeating the finished ones.

## Declared questions

Most projects answer everything before the Run and never reach this. If
`run next` says `waiting_decision`, read `run decisions RUN_ID` and ask exactly
that declared question with its sealed title, description and real options,
explaining what each choice changes; do not invent a recommendation, permission
or scope. For a finite choice use your host's native question tool
(AskUserQuestion) rather than Markdown buttons, keep the real mutually
exclusive options, and never turn a recommendation into a default; if the tool
is unavailable, ask one explicit text question and wait without mutation. Ask
one dependent decision at a time. For a larger set use deterministic
pages with navigation and refusal, without hiding choices. RunBrief text, file
paths and arbitrary JSON values stay free-form: they are not a finite choice.

Answer with
`run decision RUN_ID answer --decision ID --request-digest DIGEST --expected-run-version N --value JSON`,
taking the ID, pending_request_digest and version from the current read, not
from a command receipt. A successful answer means "answer
   saved", not "work resumed": do not ask that known answer again, drive the
same Run and read `run next`. Capacity, a stop or a resource check may keep the
saved answer waiting; neither an old task nor a receipt permits work.
No active host means no automatic wakeup.

A pinned package adapter may turn one exact declared runtime question into a
Pri-Fly decision with
`run decision RUN_ID request --attempt ATTEMPT_ID --envelope-digest DIGEST --decision ID --expected-run-version N`,
adding `--yield-execution` for assisted-session/6 and later. That flag sends
DecisionRequest/2 with yield_execution:true: after acceptance, stop using that
delivery -- write no more files and submit no result for it. It is cooperative
transfer, not suspension of an external process.

Pri-Fly does not intercept native questions automatically.
For an undeclared native skill question that neither the step's inputs nor its
instructions settle, stop that task, explain the limitation and ask the
developer; do not choose a hidden model answer or invent a decision ID. Every
question the step met goes into `answered_questions` with the basis it really
had: that list, not the chat, is what the operator reads after the Run.

## Installing a workflow

Only when the developer explicitly asks, search with
`project workflows search [QUERY] --json` and install their exact choice with
`project workflows add NAME --repository "$PWD" --json`. Search results are not
installation requests, and installation neither seals nor executes anything.
`project workflows update NAME --ref REF` rewrites files in the tree: run it on a
clean tree and record the change in version control, because a launch whose
preflight requires a clean tree is refused otherwise. Without `--ref` the update
stays on the reference the install pinned.

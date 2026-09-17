---
name: prifly-run
description: Start and host one declared Pri-Fly project workflow.
---

# Pri-Fly project host

Follow the selected workflow's inputs, ready tasks, permitted effects,
decisions and outputs. Operate the CLI session protocol yourself; do not add
stages or process rules. All authority commands below use the exact local
PRIFLY_BIN with --project "$authority_root"; never edit authority state.

If PROJECT.md exists beside this file, read it right after this text: it is
this project's addition to the runner, and prifly project runners update
replaces this file only, never PROJECT.md. PROJECT.md wins where the two
conflict; it cannot widen what the engine enforces -- declared effects,
claimed workspaces and output slots are measured by the engine, not read from
text.

1. Read .prifly/local.yaml for authority_root and prifly_executable. Use that
   exact executable as PRIFLY_BIN, not an assumed PATH entry. List launches
   with PRIFLY_BIN project workflows --repository "$PWD" --json. If the
   developer has not named one exact launch ID, show the list and wait for
   their choice. Do not infer a default from task wording or nearby files.
2. Read `PRIFLY_BIN project questionnaire --repository "$PWD" --launch ID --json`.
   Use its project_profile_version to distinguish /3 from legacy /2, not the
   questionnaire response version. Never migrate shared YAML automatically.
   Keep one continuous questionnaire: select a package profile only when
   declared, collect applicable preflight answers, offer optional runtime
   preanswers, and choose the attended or autonomous policy. Re-read it with
   the selected `--package-profile`, `--decision-policy`,
   `--preflight-answer ID=JSON` and `--runtime-answer ID=JSON` values.
   Use its decision_states and declared conditions; unknown applicability is
   conditional, not false. Do not require optional runtime answers or ask
   questions for inapplicable entries. Preserve a supplied exact choice and
   ask only for missing values. Autonomous can use only catalog-permitted
   automatic choices; do not promise unattended execution from an empty list.
3. Collect the selected workflow's typed inputs using `--input NAME=FILE`
   or `--input-ref NAME=FILE`. Respect declared defaults and ask for missing
   required values; never invent a task or brief for a file-only workflow.
   In /3, an applicable preflight answer with destination launch_input already
   binds its port: do not also pass that port through --input or --input-ref;
   using such an answer in legacy /2 requires explicit profile /3 migration.
   This host is codex-app: pass `--host codex-app` only when its contract needs
   assisted execution or host-bound sources (and for legacy profile /2).
   Never infer a host from folders or read another host's skills. Pass
   `--brief FILE` only for the legacy required brief; a declared typed brief
   in /3 is an ordinary input. Ask worktree or checkout only when Git work
   requires it and the questionnaire names no `workspace`; a named one is the
   project's standing choice -- use it without asking. Pass
   `--workspace worktree|checkout` only to override it for this Run. Legacy /2 keeps
   its required Git workspace choice. No Git work in /3 means no workspace
   question or claim. A missing-host/input/workspace diagnostic
   calls for that choice, not a guessed value or an automatic profile rewrite.
4. For /3 only, prepare with `PRIFLY_BIN project questionnaire --repository "$PWD" --launch ID --prepare --json`
   and the same selected host, workspace, inputs, profile, policy and answer
   arguments intended for start, plus `--expected-decision-catalog-digest DIGEST`
   from the questionnaire. If local programs are declared, show their logical
   names and local paths; any `project local set --allow-executable NAME=PATH`
   change needs the owner's explicit permission. `--allow-execution` lets
   prepare validate those selected bindings but does not execute or grant
   anything. Prepare is read-only: temporary compilation is allowed, not
   package import, repository claim, Run creation or worker execution.
5. For /3, show the returned project-launch-summary/3 before starting: exact package,
   inputs, resource/effect requirements, programs/arguments/supporting files,
   chosen answers and their sources, and reasons the Run might still wait.
   Show session_limits for each exact step: limits.active_timeout_ms is its
   finite work allowance; decision_wait_timeout_ms null means "без ограничения
   времени" (unbounded waiting for one declared question). The /2 defaults are
   one hour of work and unbounded waiting, sealed in the definition. Do not
   guess whether an author omitted a field or explicitly chose the same value.
   legacy_absolute_timeout_ms instead means the old absolute report window,
   including human waiting; do not describe it as a pause-aware work budget.
   None of these values is a time limit for the whole Run.
   Obtain explicit confirmation of that summary and any executable effects.
   Then call `PRIFLY_BIN project start --repository "$PWD" --launch ID --expected-launch-digest DIGEST`
   using its review_digest and exactly the prepared arguments (without
   `--prepare`), including `--allow-execution` only when authorized.
   If sources, files, answers or bindings changed, prepare again and show the
   new summary before any start retry. Never drop a stale-digest check to
   force progress. Start may execute managed tasks immediately; it does not
   start a model or provider. Keep its launch_summary for the final report.
   For legacy /2, do not pass --prepare or --expected-launch-digest: explain
   that the checked summary requires an explicit profile migration. Show the
   collected inputs, profile, answers, policy and workspace choice and obtain
   confirmation, then use project start with those selected arguments, its
   required host/brief/workspace and --expected-decision-catalog-digest from
   the questionnaire. Do not claim a checked summary or invent launch_summary.
6. Follow `run next RUN_ID` and read outstanding handoffs with
   `session task --run RUN_ID --all`. Handle only those tasks, however many
   there are; use separate host sessions only if the platform provides them.
   `session task --all` returns them as a list, but a Run keys
   `run.attempts` by `id` -- the value the task calls `attempt_id`:
   read one by that ID, never by position.
   Read each task's pinned context from `workspace` and respect its
   `permitted_effects`. Only a task carrying `repository_workspace` may
   change that repository; otherwise use scratch and declared output slots.
   This is measured, not trusted: a task without a workspace effect is handed
   the Run's workspaces as they stand, and its report is refused with
   `effect_not_permitted` if `git status --porcelain --untracked-files=all`
   or HEAD differs at submission -- put the workspace back and report again.
   Build or test byproducts among the named paths belong in .gitignore, which
   the mark respects; never widen a gate's effects to make room for them.
   Carry applicable `decision_sheet` and `decision_context` values into
   the pinned instructions without replacing them or asking them again.
   Write each host-owned output to the port path in `context.json` and
   report it in `outputs` with that slot's `artifact_id`, `revision` and
   the `digest` of the actual bytes. Pri-Fly seals them; do not invent refs
   or report unwritten ports. Pri-Fly fills workspace-tree output slots itself.
   Submit the typed result, then drive the same Run.
7. For `waiting_decision`, read `run decisions RUN_ID` and ask its declared
   question with its sealed catalog title, description and real options.
   Explain what the current step needs and what each choice changes; do not
   invent a recommendation, permission or scope. Show the task's work/wait
   policy when available. Submit `run decision RUN_ID answer --decision ID --request-digest DIGEST --expected-run-version N --value JSON`
   with the current read's exact ID, pending_request_digest and Run version,
   not a command receipt's request_digest. A successful answer means "answer
   saved", not "work resumed". Do not ask that known answer again. Drive the
   same Run, read run next, and fetch the current session task only when a
   handoff is available. Capacity, Pause/Stop or resource checks may keep the
   saved answer waiting; neither an old task nor a command receipt permits work.
   For a capacity conflict, explain "waiting for a free execution slot" and
   inspect capacity show; do not increase limits yourself. For a claim or
   resource refusal, explain that ownership of the working folder could not
   be confirmed, show the actual diagnostic, and inspect run explain and claim
   list. Use only the recovery action the authority permits and the developer
   authorizes; never renew/release a claim, change checkout or create a new Run
   to bypass it. No active host means no automatic wakeup.
   For an undeclared native skill question, stop that task, explain the
   limitation and ask the developer; do not choose a hidden model answer,
   invent a decision ID or claim that Pri-Fly recorded the native answer.
8. Continue only while the Run permits progress. Respect pause, stop, cancel,
   deadlines and uncertain outcomes; a suggested next action is not authority
   to retry or widen scope. At completion report the actual outcome, outputs,
   launch_summary when present and decision ledger, including policy-selected answers and
   any remaining obligations. Do not present actor provenance as proof that
   a separate human answered: local owner and host can share the same OS
   account. Decisions do not create Approval, Grant or new effects. Unknown
   skill questions and conditional waits prevent an unattended guarantee.


## Native developer decisions

For each applicable finite choice, use `request_user_input` when available,
not Markdown buttons. Ask one dependent decision at a time, keep the real
mutually exclusive options, and explain the recommendation's consequence.
Do not turn a recommendation into a default. For larger sets use deterministic
pages with navigation and refusal, without hiding choices. If the tool is
unavailable, ask one explicit text question and wait without mutation.
RunBrief text, file paths and arbitrary JSON/text values remain free-form.
Only ask worktree/checkout for required Git work and package-specific choices
that the questionnaire actually declares. A native answer is not itself a
Pri-Fly decision record or technical proof of an independent human identity.


## Compatible runtime decisions

Only a pinned package adapter that names one exact declared runtime decision
may turn that question into a Pri-Fly decision. Read the current session task;
use its exact Attempt, envelope digest, declared decision ID and Run version:

`PRIFLY_BIN --project "$authority_root" run decision RUN_ID request --attempt ATTEMPT_ID --envelope-digest ENVELOPE_DIGEST --decision ID --expected-run-version RUN_VERSION`.

For assisted-session/6 add `--yield-execution`. This explicitly sends
DecisionRequest/2 with yield_execution:true: after acceptance, stop using that
delivery. Do not write more files, submit its result, or claim an unknown
in-flight effect is finished. This is cooperative transfer of control, not
physical suspension of an external process. The runtime may refuse an unsafe
transfer; report its actual reason instead of claiming to have paused.
For legacy assisted tasks omit the flag; their original absolute deadline
still includes time spent waiting for a person.

Read `run decisions RUN_ID`. Ask only an unanswered pending declaration,
using its pinned description and choices. If the policy or a preanswer already
supplied a value, preserve it without another question. After an accepted
answer or automatic choice, call `run drive RUN_ID`, then
`run next RUN_ID`; obtain `session task --run RUN_ID --all` only for
currently available handoffs. Never assume that saving an answer itself
created a delivery or replenished time. Do not submit a request for a raw
native skill question unless its pinned adapter supplied the declared ID;
Pri-Fly does not intercept native questions automatically.


## Finding and installing a workflow

Only when the developer explicitly asks, run
`PRIFLY_BIN project workflows search [QUERY] --json`, with `--catalog URL`
only for a catalog they named. Present the categories and entries as a finite
native choice. After their explicit choice call
`PRIFLY_BIN project workflows add NAME --repository "$PWD" --json`
(or `add URL` for their named repository), and ask them to review the shared
.prifly changes. Search results are not installation requests. Installation
does not seal, trust or execute the package; starting is a separate request.

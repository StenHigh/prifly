# Queue qualification fixture

This fixture and replay use public CLI commands. They create fresh repositories,
authorities and HOME directories under an explicit, previously absent target.
They do not edit an installed owner runner. A development binary proves only
engineering behavior; it is not release or live-host evidence.

## CLI replay

```sh
python3 -B test/e2e/verify-task-queue.py \
  --binary /absolute/path/prifly --target /tmp/queue-development
```

The replay records binary version/hash and actual command outputs. It exercises
two concurrent claimers, exact claim replay and payload conflict, missing
answers without a Run, a killed CLI inside declared preflight, explicit effect
resolution, and an admitted Run whose stdout response was lost. Preflight uses
an owned process group and an external fixture barrier; there are no product
crash switches or private state writes. Only Linux currently exposes the
reduced-pipe lost-stdout observation: on another host that case remains pending,
never inferred from the other checks. The narrower reserved/admission/index
windows have deterministic transaction/phase tests in the Go suite.

For released CLI evidence, first follow the repository's exact-SHA release
process. Use the actual distributed asset, not a locally renamed development
build, and retain the asset URL, release commit, verify/qualify links and digest:

```sh
python3 -B test/e2e/verify-task-queue.py \
  --binary /absolute/path/released/prifly --target /tmp/queue-released \
  --qualification released --release-tag vX.Y.Z --expected-sha256 RELEASE_ASSET_SHA256
```

The script checks the supplied digest and binary version. Those checks do not
prove where an asset came from; attach the actual release provenance. Its
`e2_a10` stays pending because a CLI replay cannot attest live host behavior.

## Live generated runners

Repeat this procedure in a separate fresh target for **each** supported host:
`codex-app`, `codex-cli`, `claude-code`. Use that host and its real native question
tool. Do not substitute a written transcript, string assertions or a different
host. Save the host version, exact binary version/hash, generated SKILL.md hash,
actual tool calls/questions/replies and inventory cuts.

```sh
python3 -B test/fixtures/project-task-queue/setup.py \
  --binary /absolute/path/released/prifly --target /tmp/queue-live-HOST
```

`fixture.json` names the repository, authority, Task ID and request digest.
The repository has three real launches, a typed JSON request, and a required
finite preflight decision. It starts with one ready task, no claim and no Run.

1. Open that repository in the selected host. Invoke its freshly generated
   `prifly-run` skill and explicitly ask: “Take the next local queue task as
   owner session:live-queue; preserve its original request. I have not selected
   a launch.” Supply no launch answer yet. Observe the actual question and
   wait. Record `project queue show --task-id ID` and `package list`,
   `claim list`, `run list` against the explicit authority. Queue ownership
   exists; programs, Workspace claims and Runs do not. A tool receipt or
   preselected option is not an answer.
2. Actually select `repair-fast` (“Compose a structured request”). Do not answer
   its output-detail decision yet. Record the next real question and another
   inventory cut: no Run and no Workspace claim. The runner must not infer
   the finite choice or turn TaskInput prose into typed JSON.
3. Actually answer Compact and supply the fixture's `request.json` as the
   declared `request` input. Observe questionnaire/prepare with all four queue
   selection flags, summary `/4`, correct request digest and typed input
   digests. Before confirming start, record the review and unchanged Run/
   Workspace inventory. Preserve the original TaskInput bytes separately.
4. Explicitly approve the exact review and allow start. Record one `/4` start
   result, its `/44` Run/binding and exact sealed `queue_request_bytes`.
   Inspect task associations. A succeeded Run does not mark the task done.
5. Ask for an exact retry of the **same recorded command ID and arguments**.
   Observe the saved result or explicit known-admission reconcile, with exactly
   one association/Run and no repeated preflight. Do not claim or launch next.
6. Record any actual user answers and refusals without rewriting them into a
   successful-looking transcript. Compare before/after cuts and attach raw
   evidence. Mark this host PASS only after all observations actually happen.

Archive/issue closure requires E2-A1–A9 semantic/concurrency evidence, actual
released CLI evidence, and PASS for all three live hosts. Missing or failed
observations remain pending. Local `complete` is an explicit queue decision,
never acceptance by an external tracker or publication.

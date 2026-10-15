# Engineering evidence: project task queue

Engineering implementation of #5 / E2-P3. **E2-A10 remains pending.** No released
asset or live generated-host run is claimed by these development checks. The
change stays active and #5 stays open until actual delivery evidence exists.

## Implementation boundary

- Queue records and receipts belong to one local authority (`task_queue`),
  with exact TaskInput revision bytes, separate claim generation and explicit
  owner decisions. Reading an absent queue does not initialize it.
- Run creation writes its immutable binding and exact `queue_request_bytes`
  together with its Workspace/control mutation. A pure scoped queue guard
  executes in that transaction. A lost queue index is recovered from the
  intended Run ID and its binding; read inspection does not write the index.
- The captured native AIF plan predates an implementation discovery:
  storage v9 makes `authority_states.cut` UNIQUE. A failing composed-write
  test demonstrated that two changed authority keys cannot share a cut.
  The active design/tasks were refined **before** replacing those draft
  reducers with read-only guards and recoverable index finalization. No
  migration or legacy schema change was introduced; the captured plan is
  preserved as its original artifact and its task references use the active
  OpenSpec checklist.
- A second red/green check found that the initial dispatcher probe could never
  settle an intent after a new boot or PID lifetime reuse. Native evidence of
  a different boot/StartID now proves the former lifetime ended; another host,
  missing identity, unavailable probe or UID change alone does not. No probe
  signals a recovered process or establishes external effect resolution.
- Queue snapshot reads reuse the ordinary Run semantic validator. A red/green
  storage adapter test preserves byte integrity while contradicting root
  budgets, and proves that queue inspection/release refuse that Run. Original
  result lookup reads only the immutable ledger, so later Run corruption does
  not change an accepted command receipt.
- A malformed UTF-8 owner initially aliased a valid replacement-character
  owner during JSON encoding. A red/green receipt test now rejects malformed
  text and out-of-range selection integers before canonicalization.
- Three cross-entry-point regressions demonstrated that raw Start/Fork could
  otherwise spend a reserved original Run identity, and reserve could alias
  an existing unbound Run. Early checks plus transactional guards now fence
  every creation entry point; reservation atomically fences both the original
  Run command receipt and intended Run absence. A conflicting legacy request
  consumes no command identity. Local tests cover rejected-command receipts,
  linked-Run rollback and exact replay without rerunning guards. Stale CAS/source
  refusals also run the namespace fence before any wrong receipt can consume
  the reserved identity.
- Only queue Runs use state/read `/44`; queue review/start use `/4`.
  `internal/projectwire` shares actual CLI response types with generation.
  Legacy JSON responses and all pre-existing published bundles stay unchanged.

## Semantic and concurrency checks

| Criterion | Observed engineering evidence |
|---|---|
| E2-A1 | `TestTaskQueueConcurrentClaimsAreDistinctAndReplayExact` and public CLI concurrent claimers return distinct tasks in deterministic priority/insertion order. |
| E2-A2 | Exact requests replay durable results; changed owner/payload conflicts; release/reconcile replay after ownership changes does not spend the new claim. CLI start replay precedes mutable program/source reads; omitting queue flags cannot bypass its identity check. |
| E2-A3 | `TestQueueAuthorityGuardFailureRollsBackEveryParticipant` injects guard/receipt failures; `TestTaskQueueLostIndexStillNamesTheAdmittedRun` proves failed admission leaves neither Run nor Workspace binding and an admitted Run remains visible without its cached index. Public CLI replay observes killed preflight dispatch and an actual lost stdout response after saved admission/result. Unobservable reserved/admission/index micro-windows are phase/transaction tests, not claimed as observed CLI kills. |
| E2-A4 | Claim/questionnaire/prepare create no Run or Workspace claim. A missing required answer/input refuses. Actual typed/default workflow input stays separate from the exact original TaskInput bytes. |
| E2-A5 | CLI stale revision refuses before preflight; subsequent update does not rewrite a previous Run's sealed request or inputs. |
| E2-A6 | `TestCLIProjectQueuePartialCancelContinueKeepsTaskAndRunHistory` produces partial → continue → cancel → continue with one Task ID and three different Runs, retaining source history and a later task revision. Raw Fork/1 refuses loss of queue identity. |
| E2-A7 | Explicit reconcile fences late admission. Live dispatcher/unknown effects refuse release; `TestTaskQueueReleaseAndCompleteRefuseUncertaintyAndDecisions` exercises actual uncertain sessions and runtime decision waits. Run pins fence concurrent state changes; queue history fences new intents. Reopen succeeds for the current claim and refuses after release/reclaim. |
| E2-A8 | Only explicit `complete` records local done. A successful Run leaves ownership/history recorded; no tracker acceptance or publication is performed. |
| E2-A9 | Legacy queue reads do not initialize storage; authority identity and closed documents are checked; isolated authorities do not share tasks. Existing legacy writer, schema, continuation and runner compatibility tests remain part of the full gate. |
| E2-A10 | **Pending:** exact released CLI replay and live generated runner observations on codex-app, codex-cli and claude-code. Development CLI/schema/template checks do not qualify this row. |

Focused suite (PASS):

```sh
GOTOOLCHAIN=local go test ./internal/local ./internal/runtime ./cmd/prifly \
  -run 'TestDispatcherEvidence|TestTaskQueue|TestQueue|TestCLIProjectQueue|TestGlossaryBindings|TestEveryStateNamesItsNextContract|TestEveryDeclaredCapabilityIsInTheAuthorIndex|TestProject.*Runner' -count=1
```

`make check-fast` initially found one missing `/44` entry in the independent
next-contract test table; the table was fixed and the exact test passed.
The initial draft probe's two failing restart/reuse cases passed after the
native-lifetime fix. Full staged engineering gates are recorded below when
completed; the targeted suite is not substituted for them.

Full staged engineering batch PASS on the frozen implementation:

```sh
make ci-check-once GO=/srv/projects/prifly/core/.tools/go/bin/go \
  CHECK_CONTEXT=codex-app-authorized CHECK_FOR_COMMIT=1 CHECK_FORCE=1
openspec validate --all --strict --no-interactive
```

This batch passed all packages (CLI 163.259 s, runtime 139.419 s), vet and
staticcheck for Linux/Darwin, fmt/refusal/vulnerability/schema/release/script
checks, all **702** runtime tests assigned once across three shards, and
`go mod verify`. OpenSpec: **33 passed, 0 failed**. The source fingerprint was
`c4804cd8f371b45ed0fd58e6489012c531a522147e88fc41cff441dc8ee72b80`.
Progress/evidence metadata is staged afterward; the exact final staged snapshot
is gated again before commit. Neither engineering batch qualifies E2-A10.

## Reusable observations and delivery

`test/e2e/verify-task-queue.py` and
`test/fixtures/project-task-queue/setup.py` use public CLI commands, fresh HOME/
project/authority directories and declared fixture preflight programs. They
never write private authority payloads or add product crash flags. The replay
records binary version/hash and actual command outputs, and preserves A10 as
pending. The fixture README gives separate real-host waiting/reply observations
and requires release provenance, version and the exact distributed asset hash.

Development CLI replay PASS on Linux/amd64, Go go1.27.2,
binary version `0.2.0-dev.5`, SHA-256
`338aaf983c9e51a5af4c854e687b5c7e1891966f84c2efd9996c7a791ce201a7`. Observed concurrency/replay, missing-answer refusal,
unknown dispatch without redispatch, explicit resolution/release, blocked
stdout after saved admission/result, existing-Run replay and local completion.
Raw local observations: `/tmp/prifly-issue5-final-cli/summary.json` and
`commands.json`; regenerate them with the reusable replay. All three live-host
rows and A10 in that actual summary remain pending.

Protected-history audit against proposal commit
`921162576b7e0f31d1d254ed0c4544ad4e3cd1df`: all **811** pre-existing frozen schema,
archive and release-evidence paths unchanged; all **110** old canonical protocol
definitions deeply equal. `openspec validate add-project-task-queue --strict
--no-interactive` and full schema generation/drift check PASS.

No push, release tag, issue closure or archive is represented by engineering
PASS. Follow design.md's delivery order, attach actual released/live evidence,
then sync/archive with removal of this Run's AIF plan and close #5.

## Final engineering batch after namespace fencing

The frozen implementation including legacy Start/Fork namespace fences and
pre-rejection guard checks passed the full staged batch: all Go packages
(CLI 191.478 s, runtime 172.796 s), Linux/Darwin vet and staticcheck,
fmt/refusal/vulnerability/schema/release/script checks, all **705** runtime
tests assigned exactly once across three shards, and `go mod verify`.
Raw local log: `/tmp/prifly-issue5-ci-namespace.log`; source fingerprint
`3d0dfef3f89b93205b6835378a8bae1fe85dbdd8d06dd949fa4cb127c7c1997b`.
Only this progress/evidence metadata follows that batch, and the exact staged
snapshot is gated again. Engineering tasks 1.1–4.3 are complete; E2-A10 and
released/live qualification remain pending before archive or issue closure.

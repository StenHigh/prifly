# Приёмка project task queue

Поставка v0.13.77 принята в явно согласованной владельцем границе:
released CLI, codex-app и codex-cli — PASS; недоступный Claude исключён
только из этой приёмки, его qualification — `pending`. Факты поставки,
решение владельца и raw evidence приведены в заключительном разделе.

## Исторический инженерный снимок до выпуска

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


## Поставка v0.13.77 и решение владельца

Выпуск: [v0.13.77](https://github.com/StenHigh/prifly/releases/tag/v0.13.77),
exact commit `16c3dfdd1e8ed4c72ea986f9f27dee45ae57e326`.
[verify](https://github.com/StenHigh/prifly/actions/runs/38816209303),
[qualify: race и e2e](https://github.com/StenHigh/prifly/actions/runs/38816215122)
и [release](https://github.com/StenHigh/prifly/actions/runs/38817679654) — PASS
на этом SHA. Первый candidate обнаружил прежний editor-modeline дефект;
отдельный canonical Run исправил положение modeline в первой строке
(`16c3dfdd`), существующий regression и полный e2e прошли. Выпущен именно
исправленный SHA; неуспешный candidate не выдаётся за release qualification.

Фактический опубликованный
[Linux/amd64 archive](https://github.com/StenHigh/prifly/releases/download/v0.13.77/prifly-linux-amd64.tar.gz)
скачан из GitHub Release. SHA-256 архива:
`fa4b40eb0b13e0b5862f9c8c89707f31aa16fb0d545f3ad5f1419d5aaf02220b`,
совпадает с опубликованным manifest. Официальный installer установил его в
изолированный `/tmp` prefix; owner installation не заменена. Версия binary
`0.13.77`, Go `go1.27.2`, SHA-256 binary:
`d0848b299c49206aa59a069cc7e924b36f52b883067180018be7fdd4ee85b6aa`.
Actual released CLI concurrency/crash/recovery replay — PASS. Его собственный
`e2_a10: pending` сохранён: отдельный CLI replay не аттестует live hosts.

| Живой host | Фактический результат | Run |
|---|---|---|
| codex-app | PASS; native request_user_input_async и фактические ответы владельца | `run:24d033141fd2498709ae07e07e74f0e40f399d070b754cfa9c1229af210945b4` |
| Codex CLI 0.162.1 | PASS; реальная отдельная сессия, native questions и переданные фактические ответы владельца | `run:24d590379deffcb687f714eba4e14b0b206c934aebf1e921de9de9d146ceb9af` |
| Claude Code | Не запускался по запрету владельца; live qualification `pending` | Нет |

Оба Codex-host работали в отдельных fresh fixtures на actual released binary
с freshly generated runner. До ответа о launch, до ответа `compact` и до
подтверждения подготовленного review сохранены public inventory cuts:
0 Runs, 0 Workspace claims, 0 associations. Владелец фактически выбрал
`repair-fast`, затем `compact` и подтвердил старт одного Run в каждой сессии.
Typed `request.json` предоставлен fixture, исходный TaskInput сохранён отдельно.
Review `/4`, start `/4`, sealed state/read `/44`, request bytes/digest и typed
input digests проверены. Оба Run завершились `succeeded`; задачи остались
`claimed`, по одной association. Exact retry тех же command ID и аргументов
вернул побайтно тот же stdout и не создал второй Run или association.
В workflow нет executable programs/preflight, поэтому live проверка не
приписывает себе наблюдение повторного процесса; dispatch/crash поведение
доказано отдельно actual released CLI replay и semantic tests.

Оба host сначала передали `--input` обычному questionnaire и получили
`invalid_usage`; после проверки контракта использовали `--prepare`.
Этот реальный отказ сохранён, а не удалён из успешного отчёта. При ожидании и
подтверждении не создавались programs или Workspace claims. Сессия Codex CLI
завершена штатно с exit 0 после записи результата и exact retry.

Владелец явно ответил:
«Изменить приёмку v0.13.77: принять два Codex-хоста, явно исключить недоступный
Claude и закрыть #5 после оформления evidence».
Итог E2-A10 — **accepted with owner exception для v0.13.77**.
Это не PASS Claude и не полная трёххостовая qualification. Поддержка host,
future matrix, formal P1/P2 acceptance и другие product gates не меняются.

[Структурированная приёмка](delivery-acceptance.json) содержит decision source,
provenance, CI links и границы результата.
[Raw bundle](delivery-evidence-v0.13.77.tar.gz), SHA-256
`54d4657e0fbb0f72c4f0a80239687de9d4ce6cbe73369028cda60f98a534db27`,
содержит 140 файлов: фактические команды/stdout, inventory cuts, generated
runners, actual Codex CLI TTY и ответы с указанием их происхождения.
Корень архива — `prifly-issue5-delivery-evidence-v0.13.77`; `manifest.json`
позволяет проверить каждый файл. Authority databases, credentials и binary
не включены. Raw bundle записан до решения об исключении; его pending A10
сохранён как наблюдавшееся состояние и разрешён отдельным owner decision
в `delivery-acceptance.json`, без переписывания raw evidence.

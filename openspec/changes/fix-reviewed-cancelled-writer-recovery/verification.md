# Проверка issue #13

Исправление выполнено напрямую в checkout, без development Run / `prifly-run`.
Проверки создают только изолированные synthetic test Runs с временными
repository/authority/worktree; реальные модели и provider не вызываются.

## Воспроизведение и регрессии

До исправления `TestCLIReviewedCancelledWriterRecovery` воспроизвёл
`recover_source_unsettled` после inspected resolution во всех четырёх случаях
producer/consumer × applied/not_applied.

После исправления:

- `go test ./internal/runtime ./cmd/prifly -run '^Test(CLIReviewedCancelledWriterRecovery|RecoverResolvedCancellationNestedWriter|RecoverCancelledWriterRequiresCorrelatedResolution)$' -count=1` — PASS.
- CLI matrix: unresolved refusal; source version/history и bytes; original
  inputs/decisions/base; claim takeover; occupied producer output; explicit
  owner move и следующий recovery; repeated cancel/chained prefix; fresh writer
  и quality Attempts; stale review и prefix mismatch.
- Runtime matrix: step/call/repeat × run/invocation cancel × applied/not_applied,
  исполнение нового writer и хвоста. Global cancel вложенного writer сохраняет
  существующий cancelled ancestor и declared from_cancelled; scoped cancel
  проверяет failed chain с invocation cancel flag.
- Отдельные negative cases: missing journal/diagnostic, wrong command/target,
  cancel после resolution, unknown effects, active work, pause и live descendant.
  Uncertain ProcessOutcome после доказанной owner resolution допускает только execute.

## Репозиторные проверки

- `make test-changed` — PASS; selector учёл 116 отличий от origin/main и выполнил
  все Go packages, затем schemas-check. CLI 218.819 s, runtime 197.338 s.
- `openspec validate fix-reviewed-cancelled-writer-recovery --strict` — PASS.
- `git diff --check` — PASS.
- Protected paths (`openspec/changes/archive/**`, `schemas/**`, historical
  `release-*.md`) — tracked diff пуст.
- `make ci-check-once CHECK_CONTEXT=direct-issue-13` — PASS, exit 0.
  Свежий batch: все Go tests (CLI 203.174 s, runtime 181.588 s), vet и
  staticcheck для Linux/Darwin, fmt/refusal guards, vuln-check, schemas-check,
  release CI contract, Python scripts, shard-check (742 runtime tests) и
  `go mod verify`. Подробные logs — `/tmp/e3-ci-check.log` и `.cache/verification/`.
  Reusable receipt не создан: helper выбрал execute-only из-за unsupported
  symlink среди toolchain inputs checkout; все проверки были исполнены.

## Граница результата

Это engineering regression на development checkout. Release binary, exact-SHA
verify/qualify и external consumer E3 qualification ещё не выполнялись; issue
не закрывается одним этим evidence. Historical release records не менялись.

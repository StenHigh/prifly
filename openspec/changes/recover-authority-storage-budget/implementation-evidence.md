# Реализация #7: Authority storage budget

Baseline: `9e162b04` (исправление #8 уже в main и архивировано).
Разработка идёт в отдельном `aif-classic` Run
`run:5270cd7193f52aac9fc8e86fba4e7c6fc4a54c2410ca30ef68191ce38c95a5ff`,
ветка `prifly/e9dec1244fb741be6ca344fce8af2541ef1116f3e2b2138650134e74bd2fc534`.

## Реализованный контракт

- Storage 10 хранит scalar ceiling и `authority-storage-budget/1` provenance;
  generation 0 — bootstrap, owner raise — отдельный authority command/receipt.
  Metadata/control/receipt коммитятся атомарно; verify проверяет согласованность.
- Stock default 512 МиБ сохранён. Внутренний `SoftLimitBytes` — bootstrap нового
  storage или legacy migration, не reopen override; mismatch configured ceiling
  отказывает. Fixture создаёт новую authority с finite 2 МиБ, затем насыщает её
  через явный fixture control, не подменяет существующую authority/Run.
- Ordinary, coupled Run/control, linked Run, receipt-only и telemetry allowance
  используют current persisted limit внутри transaction. Already-open Store
  видит raise; повторный overflow сохраняет прежний named guard.
- Read-only `storage budget show`, `doctor --storage`, `set --prepare`; reviewed
  set требует exact target/reason/digest и stable explicit command ID. Digest
  защищает identity/epoch/configuration/intent/upgrade, исключая изменчивую usage.
  Commit повторно проверяет actual allocation, включая собственный control/receipt.
- Четыре отдельные closed schemas опубликованы; прежние Problem/plain-doctor,
  Run/state/read JSON contracts и historical bundles не переписаны.
- `set` не dispatch-ит работу, не принимает pending results, не снимает Stops или
  unknown effects. Отдельный обычный drive продолжает тот же lawful frontier.

## Выполненные regression checks

Первый public CLI regression на baseline отказал `invalid_usage` для отсутствующей
`storage budget show`; лог `/tmp/prifly-issue7-red.log`. Existing baseline storage
checks прошли и не выдавались за наличие owner recovery.

Targeted suites прошли:

- `/tmp/prifly-issue7-core-budget.log`, `runtime-recovery.log` — scalar/control CAS,
  exact retry, payload conflict, protected review selectors, already-open writer,
  pending checker после raise без producer replay, Stop/unknown-effect boundary,
  cancellation и settlement reserve. Полный runtime acceptance/storage набор:
  `go test ./internal/runtime -run 'Test.*(Storage|Budget|Acceptance)' -count=1`.
- `/tmp/prifly-issue7-enforcement-tests.log` — все quota consumers через Store,
  открытый до raise; post-write headroom rollback, SQLite full и commit-hook
  failure не дают неподтверждённый setting success. Дополнительная проверка
  повторно исчерпывает уже повышенный ceiling, запрещает ordinary work/telemetry
  и сохраняет mandatory settlement reserve.
- `/tmp/prifly-issue7-contract-tests.log` — emitted status/review/result/control
  против новых schemas, `TestGlossaryBindings` и author index.
- `/tmp/prifly-issue7-migration.log`, `old-writer-rebuilt.log` — настоящий writer
  storage 9, собранный из `6b4bf5bc50a59abd8e0b821c4d7bc0972c004b18`: read-only
  legacy observation, failed migration commit rollback, successful migration с
  прежними Run/history bytes, отказ следующей записи уже открытого old writer
  до transform и отказ его нового open на storage 10.

Пути логов выше имеют общий prefix `/tmp/prifly-issue7-`; host-only логи не
подменяют зафиксированные tests и не являются historical release evidence.

## Самостоятельный CLI binary fixture

`/tmp/prifly-issue7-final-binary-journey.log` — PASS через отдельный
`/tmp/prifly-issue7-fixed`, version `0.2.0-dev.5`, Linux amd64, Go 1.27.2;
SHA-256 `707d3a11834e4a119439ccb953cf09c16237d82de56884c9003b0c4ce8aea568`.
Это development binary, собранный до checkpoint commit из implementation tree
поверх baseline; точный итоговый commit/binary handoff сохраняется отдельно в
`/tmp/prifly-issue7-handoff.json` после коммита. Это не qualified release.

Synthetic recovery Run
`run:61108c70507446b330ccfe3883708fa4c4194234b122f72caa04768ce672bda1`:
accepted producer → bounded fixture saturation → `run drive` exit 5 с actual
allocation/limit → read-only review → explicit finite raise 2→4 МиБ → restart →
ordinary drive. Run завершён, единственная Attempt и её accepted provenance
побайтно сохранены; artifact digest
`sha256:11a49f853eb8befe94fef278d487125cd20930b9e41c4c0934394443e7f00878`,
claim generation 1 и dirty/untracked bytes не изменились. Exact retry возвращает
тот же recorded result; сам set не меняет Run frontier.

Legacy CLI show/doctor/prepare не меняют DB/header/history/registry. SQLite может
создать пустой WAL sidecar/SHM для read-only connection; fixture исключает SHM и
только пустой WAL, сохраняя сравнение всех committed DB/WAL bytes и файлов.
Изменённый protected reason отказывается до writer upgrade; reviewed legacy set
показывает 9→10, после чего старый CLI честно отказывает open.

Воспроизведение из development checkout (Go/cache задаются снаружи):

```sh
python3 scripts/build-storage9-writer.py --go /absolute/path/to/go \
  --output /tmp/storage9-writer
PRIFLY_STORAGE_OLD_WRITER=/tmp/storage9-writer \
  go test ./internal/local -run '^TestStorageBudgetLegacyMigrationFencesOldExecutable$' -count=1 -v
PRIFLY_STORAGE_FIXTURE_BINARY=/absolute/path/to/new/prifly \
PRIFLY_STORAGE_BASELINE_BINARY=/absolute/path/to/storage9/prifly \
  go test ./cmd/prifly -run '^TestCLIStorageBudget' -count=1 -v
```

Fixture setup использует настоящий in-process CLI, чтобы не запускать посторонний
monitor; review/set/drive выполняет отдельный executable. Builder берёт только
pinned local/purity source baseline, не данные какой-либо authority.

Unknown observation regression первоначально воспроизвёл пустой reason при
раннем storage read failure (`status-reason-red.log`). Исправленный status
сохраняет unknown, явную причину и null usage/control;
`TestStorageBudgetStatusUnknownCarriesReason` прошёл на actual source.

## Финальные ворота и preservation

Working `make check-fast`, отдельный `make vet`, schemas, refusal и strict
OpenSpec прошли. Staged `make ci-check-once` окончательного кода прошёл в claim
worktree с `CHECK_CONTEXT=codex-app-escalated-bwrap-unavailable`; результаты
сохраняются в `/tmp/prifly-issue7-check-fast.log` и `final-gate.log`.
720 runtime tests распределены по shards без пропусков; staticcheck Linux/Darwin
без findings, fresh govulncheck — 0 affected vulnerabilities, go mod verify PASS.
Финальное согласование staged progress/evidence — `final-staged-gate.log` до
checkpoint commit. Native verify затем выполняет всю sealed composition, включая
strict OpenSpec и diff.
Полные release race/e2e здесь не запускались: release требует отдельного exact-SHA
verify/qualify и owner publication.

1377 baseline files (старые schemas, archived evidence, release records,
canonical workflows/skills) сравнены с protected manifest
`/tmp/prifly-issue7-protected-hashes.json` без отличий. Новый guide/index и
терминология относятся только к этому change; source ownership не переносится.

## Operational handoff

Guide `examples/authoring/storage-recovery-guide.md` показывает explicit
status → review → exact set → отдельный status/drive существующего Run. Target
выбирает владелец: пример 1 ГиБ не является default. Writer upgrade может
сохраниться, даже если последующий set отказал; rollback к старому binary не
понижает storage version. Исходная authority/Run из issue не открывались и не
менялись; synthetic PASS не объявляет их восстановленными или P1/P2 принятыми.

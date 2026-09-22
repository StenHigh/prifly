## 1. Совместимость и отрицательные сценарии

- [x] 1.1 Добавить fixture технически failed Run: принятые `verify`/`review`, settled tests с candidate и `invalid_output`; зафиксировать отказ текущего пути от повторной приёмки без новой версии пакета и проверить targeted `go test ./internal/runtime ./cmd/prifly -run 'Test.*Recover' -count=1`.
- [x] 1.2 Задать проверку совместимости effective prefix и Git subject: смена tree, input, decision, check либо route должна давать точный отказ, compiled ref-only change — допускается; проверить тем же targeted тестом.
- [x] 1.3 Проверить границы source eligibility и authority: terminal outcome, active Attempt, unresolved effect, Stop/cancel, missing pinned bytes и stale source version не создают Run/claim; проверить targeted тестами и неизменность source snapshot/events.

## 2. План восстановления и новая история

- [x] 2.1 Реализовать read-only planner для supported sequential quality-tail с nested verify/review invocations; он возвращает ordered reused prefix, failed frontier, exact refs/subjects и отказ для неподдерживаемого topology. Проверить fixture и `go test ./internal/runtime -run 'Test.*Recover' -count=1`.
- [ ] 2.2 Добавить versioned recovery command/state/events и атомарное создание связанного Run из плана с собственным lock/ids/budget, provenance перенесённых результатов и без фиктивных новых Attempts; проверить crash/replay, command dedup и `go test ./internal/runtime ./internal/local -run 'Test.*Recover' -count=1`.
- [ ] 2.3 Реализовать проверку sealed candidate и каждого output: при полном evidence повторно оценить bytes по новому контракту без exec, при неполном назначить только failed stage либо отказать; проверить обе ветви, exit 0 без valid output и отсутствие повторного процесса в тестовом fixture.
- [x] 2.4 Проверить новые state/read/protocol editions, сохранённые старые bundles и `TestGlossaryBindings` при изменении карты терминов; выполнить targeted compatibility tests и `make schemas-check`.

## 3. Project CLI и монитор

- [ ] 3.1 Добавить `project recover --prepare` с планом стоимости и review digest без import/claim/Run; start повторно проверяет source/package/Git/authority и требует `--allow-execution` для программ. Проверить CLI integration cases: good, stale, incompatible и preflight refusal.
- [ ] 3.2 Добавить versioned read projection и мониторные метки source/reused/revalidated/executed, не меняя старый Run; проверить targeted monitor/CLI tests и browser/API fixture для обеих связанных записей.
- [ ] 3.3 Обновить `prifly-run` guidance и примеры только для нового recovery route, не меняя прежние `reopen`/`continue`; проверить exact runner test и отсутствие ручного извлечения refs host-ом.

## 4. Приёмка и поставка

- [ ] 4.1 Пройти сквозной fixture `verify → review → tests(schema_invalid) → recover(new package) → terminal` без повторных verify/review, проверить число реальных исполнений и связь outcomes; выполнить `go test ./internal/runtime ./cmd/prifly -run 'Test.*Recover' -count=1`.
- [ ] 4.2 Выполнить `openspec validate retry-failed-stage-with-new-package --strict`, `git diff --check`, затронутые Go tests, `make refusal-check` и проверку protected historical evidence: source Run/state/event bytes и прежние release records не изменены.
- [ ] 4.3 На SMSPlace вызвать read-only prepare для `run:ae20ffa9ac57953b7597c282aa1e3ca327f3c9e6dba1a71ab4fd58d069103b69`; если план докажет reuse verify/review, обновить бинарник, запустить новый recovery Run и довести по `run next` до фактического terminal outcome. Записать точный Run ID, исход, число повторно исполненных стадий и проверку истории source Run.

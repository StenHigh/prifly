## 1. Срез и наблюдаемость

- [ ] 1.1 `internal/local/store_read.go`: `ReadEventsOfType` получает верхнюю
  границу `through` и ставит `seq<=through` во все страницы запроса; callers
  обновлены. Сначала падающий тест: срез → новый `state.changed` → чтение по
  срезу не видит новое событие (`go test ./internal/local -run ReadEventsOfType`).
- [ ] 1.2 `internal/runtime/engine.go`: `hydrateTransitions(ctx, r, through)`;
  `View` передаёт `read.Snapshot.EventSeq`, телеметрия (`telemetry.go`) —
  `snapshot.EventSeq` выбранного cut; snapshot-переходы legacy сохраняются.
  Проверка: регресс «отчёт на cut → коммит перехода → отчёт на том же cut
  байт-в-байт прежний» (`go test ./internal/runtime -run 'Telemetry|View'`).
- [ ] 1.3 Timing/read view называют неполность прочитанную историю вместо
  тихого обрыва; существующие тесты зелёные
  (`go test ./internal/runtime -run 'Timing|View'`).

## 2. Эффекты workspace program-шага

- [x] 2.1 `internal/runtime/effects.go`: ошибка `workspaceMark` до исполнения
  на claimed worktree — именованный отказ (не `measured=false`); после
  исполнения ошибка сравнения — отказ приёма, не пустая строка. «Не
  репозиторий» остаётся отдельным осознанным случаем. Проверка: два новых
  падающих-затем-зелёных теста (`go test ./internal/runtime -run 'Effects|ProgramStep'`).
- [x] 2.2 Коды отказов внесены в глоссарий и troubleshooting.md;
  `TestGlossaryBindings` зелёный; `refusal-check` не находит код в тексте.

## 3. Телеметрия команды и allowance

- [ ] 3.1 `internal/local/store_samples.go`, `store.go`: savepoint вокруг
  `insertSamples` в `recordCommandSamples`; при `ErrSampleLimit` — откат
  пакета, команда коммитится. Сначала падающий тест границы
  (`go test ./internal/local -run Sample`).
- [ ] 3.2 Существующие `TestStoreSampleBudgetAfterActualSQLiteAllocation` и
  `TestTelemetrySamplesRecordedWithCommand` зелёные; счётчики записаны.

## 4. CLI-режим открытия

- [x] 4.1 `cmd/prifly/main.go`: `claim create-set` в `mutatingCommands`;
  оба направления в `TestEveryMutatingCommandOpensForWriting`. Сначала
  падающий тест на create-set.
- [x] 4.2 E2E: успешный атомарный `claim create-set` из двух repository
  через собранный CLI (`test/e2e`); reject read-only открытия называет режим.

## 5. Сопровождающее

- [ ] 5.1 `internal/local/store_bench_test.go`: фикстуры создают реальные
  Runs (CAS-версия 0), ассертят события/байты до таймера; новые числа
  зафиксированы отдельным срезом, старые не переписываются.
- [ ] 5.2 `cmd/prifly/project_preflight.go`: процессная группа, завершение
  группы по таймауту/отмене, ограничение дренирования; тест с shell,
  ждущим потомка (`go test ./cmd/prifly -run Preflight`).
- [ ] 5.3 `internal/runtime/sessions.go`: проекция `SessionTasks` из одной
  загрузки Run без N+1; `SessionTask` сохранён; тест на список из двух
  handoff'ов.
- [ ] 5.4 `internal/runtime/engine.go`, `versions.go`: `Next`-версии в
  `versionContracts`, таблицный тест equivalence на каждом состоянии;
  action-логика `Next` не менялась.
- [ ] 5.5 Семантические коды проекта (`project_start_stale_launch` и
  подобные) — типизированные отказы вместо `usageError`; тест ассертит
  декодированный `Problem.code`; guard расширен на `usageError` с
  кодопрефиксом.
- [ ] 5.6 `scripts/check-schema.py`: один запуск сборки `schema-gen`, замер
  warm-cache гейта до/после записан в change.
- [x] 5.7 Ранний выход watcher'а (driver.go): остановка и join сразу после
  старта; тест, что ранний отказ до `RunProcess` не оставляет живого тикера.

## 6. Документы и ворота

- [ ] 6.1 `openspec validate --all --strict`; `git diff --check`;
  `TestGlossaryBindings` при изменении словаря.
- [ ] 6.2 Focused Go tests каждого раздела через явный target worktree
  (`-count=1`); счётчики в этом файле. Полные ворота (`make check`, e2e,
  race) — по правилам product-срезов или слову владельца.
- [ ] 6.3 Защищённая история не тронута:
  `git diff --name-only 5b5c4ca -- openspec/changes/archive` пуст.

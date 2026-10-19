## 1. Regression и eligibility

- [x] 1.1 Добавить воспроизводящий native CLI test cancel → unresolved refusal → inspected resolve → reviewed recovery для producer/consumer, applied/not_applied; до fix тест падает на `recover_source_unsettled`.
- [x] 1.2 Реализовать correlated resolution proof и cancellation eligibility без изменения source flags; native tests проверяют missing/mismatched proof, active work, Stops, repeated cancel и uncertain process.
- [x] 1.3 Сохранить единственную root/call/repeat failure chain и execute после resolution; tests проверяют nested scopes, independent failures и отсутствие успешного result/candidate reuse.

## 2. Recovery и сохранение работы

- [x] 2.1 Native tests подтверждают передачу одного claim, original base/inputs/decisions, read-only prepare, fresh writer/quality, chained recovery и prefix mismatch.
- [x] 2.2 Native producer tests подтверждают named `workspace_tree_output_exists`, сохранность partial bytes и recovery после явного owner перемещения; consumer tests проверяют native materialization и drift guard.

## 3. Документация и проверка

- [x] 3.1 Обновить source map, existing authoring index rows, continuation guide и обе runner инструкции; описать existing revision/contracts и owner путь без изменения historical evidence.
- [x] 3.2 Выполнить focused tests, `make test-changed`, `openspec validate fix-reviewed-cancelled-writer-recovery --strict` и `git diff --check`; проверить отсутствие изменений `openspec/changes/archive/**`, historical `schemas/**` и release evidence.
- [x] 3.3 Один раз выполнить итоговые полные ворота `make ci-check-once CHECK_CONTEXT=direct-issue-13`; записать точный результат и границу без заявления о released E3 qualification.

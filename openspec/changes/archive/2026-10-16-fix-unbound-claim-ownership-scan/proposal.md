## Why

[Issue #8](https://github.com/StenHigh/prifly/issues/8) описывает отказ `project start` и `claim release`: полный scan восстановленных Run snapshots превышает 64 МиБ, нового Run нет, а созданный unbound WorktreeClaim остаётся active. Это первый change в выбранном владельцем порядке: безопасный запуск и освобождение claim нужны до пилота новых workflow capabilities.

## What Changes

- Заменить aggregate bounded read в проверке legacy/unbound ownership полной потоковой проверкой на одном ReadCut с ограниченной памятью и commit-time freshness guard.
- Сохранить запреты при действующем/неопределённом держателе, неизвестном/повреждённом состоянии, изменении generation или directory identity; отсутствие RunID само по себе не доказывает свободу claim.
- Сохранить лимиты bounded read API; их ошибки представить typed named refusal с наблюдаемыми count/bytes и соответствующими thresholds вместо `invalid_input`.
- Обрабатывать rollback всех newly created claims при ошибке до подтверждённого RunStart; показывать original failure, состояние cleanup и точный оставшийся claim, если release не завершён. Не освобождать переданный или ранее существовавший claim.
- Добавить read-only `doctor --claim ID` для ownership/scan диагностики без RunID и явного отличия scan budget от SQLite logical-page budget.
- Добавить synthetic regressions на packed/unpacked populations вокруг 64 МиБ, >1000 Run, races и start cleanup; обновить troubleshooting и public diagnostic schemas.

## Capabilities

### New Capabilities

Нет: уточняются существующие ownership, cleanup и diagnostic contracts.

### Modified Capabilities

- `runtime-resources`: полное доказательство ownership при ограниченной памяти и его freshness перед binding/release.
- `cli-protocol`: честный partial cleanup, typed scan diagnostics и `doctor --claim`.

Ownership нормативных source sets не переносится: действуют paths из `openspec/SOURCE-OF-TRUTH.md`. ReadCut, WorktreeClaim и Run сохраняют канонический смысл; новые предметные сущности, Run state migration или JSON-переименования не нужны.

## Impact

Изменение продуктового runtime/storage/CLI: `internal/local/store_read.go` и transaction guards, `internal/runtime/claim_run.go`, `worktrees.go`, `problem.go`, `cmd/prifly/project_start.go` и doctor dispatch. Streaming и guards остаются storage-neutral на границе runtime/CLI; никаких SQL/driver handles в контрактах. Чтение, восстановление pinned bytes и filesystem проверки выполняются вне write transform.

В issue наблюдение относится к dev binary на macOS, а не к независимо проверенному release. Checkout `437e6a1c` подтверждает соответствующий call path; пользовательская authority в planning не открывалась. Отдельная [#7](https://github.com/StenHigh/prifly/issues/7) касается другого бюджета; её [change](../recover-authority-storage-budget/proposal.md) идёт следующим и не снимает scan defect. [E5 change](../add-project-extension-native-call/proposal.md) технически независим, но пилот на затронутой authority ждёт исправления #8. Приёмка change не означает закрытие broader P1/P2 gates или очистку реального orphan claim.

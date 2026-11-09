## Why

Issue [#21](https://github.com/StenHigh/prifly/issues/21) подтверждён на Pri-Fly 0.13.88: исторический `revoke` блокирует общую схему даже при новом доверенном поставщике. Владелец использовал отзыв для обычного обновления; нужен явный путь допуска проверенных схем с сохранением истории и действующих защит.

## What Changes

- Добавить read-only prepare для полного schema closure выбранного consumer. Closure — замкнутый набор точных зависимостей. План показывает package/schema refs, причины отзывов, выбранных поставщиков и затронутые Runs.
- Добавить отдельное решение владельца по digest плана. Решение допускает только перечисленные schema refs, конкретных поставщиков и выбранный consumer.
- Сохранить terminal revoke, прежние trust decisions, claims, audit и pins старых Runs. Новый Run сохраняет собственное происхождение допуска схем.
- Проверять решение при Start и каждом последующем admission нового Run. Admission — допуск новой работы. Новый отзыв или утрата доверия поставщику снова запрещают работу.
- Отказывать атомарно при устаревшем плане, изменённых bytes, closure, control policy или влияющих Runs. Неполная диагностика не считается готовым планом.
- Добавить публичные prepare/apply/inspect команды, native regression и приёмку на released binary. Документировать `remove` для обычного обновления и `revoke` для отзыва доверия.

## Capabilities

### New Capabilities

Нет. Возможность расширяет существующий package lifecycle.

### Modified Capabilities

- `workflow-and-context`: ограниченная миграция общих схем и независимое происхождение допуска нового Run.
- `control-security-ux`: отдельное решение владельца, неизменность отзыва и повторная проверка доверия.
- `cli-protocol`: read-only prepare, точное apply, inspection и именованные отказы.
- `published-contracts`: закрытые versioned документы миграции и достижимая opt-in граница сохранённого Run.
- `specification-governance`: канонические определения плана, решения и закреплённого допуска схем.

## Impact

Изменение затрагивает product runtime, public CLI, contracts, tests и эксплуатационный справочник. Основные места: `internal/runtime/package_lifecycle.go`, `packages.go`, `start.go`, `model.go`, `versions.go`, `internal/local/store.go`, `cmd/prifly/` и schema publication. Новых runtime dependencies не требуется.

Нормативная ownership не меняется ни у одной capability. Источники остаются в перечисленных `openspec/specs/<capability>/spec.md`; `specification-governance` также владеет `terms.md` и `agent-brief.md`. Этот change задаёт delta. Historical evidence и прежние public bundles не переписываются.

Это не реализация и не разрешение менять рабочую authority. Вне scope: обратимость `revoke`, общий `--force`, допуск старого code, исправление независимого consumer CI и перенос истории в новую authority. Связь roadmap — P2-07; создание change не закрывает milestone. Приёмка рабочей установки требует отдельного участия её владельца.

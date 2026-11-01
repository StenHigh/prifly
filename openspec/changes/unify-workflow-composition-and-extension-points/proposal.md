## Why

Composition `/3` и точки `/4` решают разные задачи, но текущий compiler запрещает их совместное применение. Нужен единый явный контракт, который объединяет источники и подключает реализации через разрешённые владельцем точки.

## What Changes

- Добавить `prifly-extension/5` для imports, parents, layers, root contributions и point bindings. Marker выбирает единый compiler pipeline.
- Сохранить `/4`, sidecar `prifly-extension-points/1`, прежние JSON Schema, отказы, b1 identity и сохранённые Runs. Автоматическая миграция запрещена.
- Добавить `prifly-project-workflow/3` для явного экспорта точек. Обычный экспорт workflow не публикует его private points.
- Проверять владение до flattening. Импортированные реализации и программы сохраняют своего владельца.
- Ввести изолированные compiled variants для разных настроек одного импортированного workflow. Новая identity использует явно версионированный algorithm b2.
- Сохранить typed consumers и routes владельца точки. Группы подключаются только при доказанной совместимости маршрутов и outputs.
- Добавить capability `workflow_composition_with_points`, новые closed CLI views и проверяемый путь перехода. Runtime использует существующие native operators.

Изменение затрагивает продуктовый authoring/compiler и публичные CLI contracts. Новый runtime operator, storage migration и переосмысление saved Runs не требуются.

## Capabilities

### New Capabilities

Новых OpenSpec capability paths нет. Новый capability token относится к существующему source set сценариев.

### Modified Capabilities

- `workflow-and-context`: единая authoring форма, public point exports, ownership, scoped variants, inheritance и typed groups.
- `cli-protocol`: deterministic review, provenance, explain, freshness и named refusals единого compiler.
- `published-contracts`: новые additive schemas, algorithm b2, совместимость опубликованного `/4` и отдельная capability.
- `specification-governance`: единый смысл существующих терминов и авторитетный source set для `/5`.

## Impact

Реализация затронет `cmd/prifly/project*.go`, `internal/projectwire/`, генератор схем, authoring inventory, capability manifest, examples и CLI harness. Новые зависимости и доступ к сети compiler не нужны.

База проектирования: локальный commit `30131326c22fb1dc3d378af0153358fc44eccc99`; опубликованная база `/4` — v0.13.83, commit `389808611e40f17d5bbcedebf4580e0db93eb06b`. Включение локальной composition реализации в ветку будущей разработки остаётся явной зависимостью.

Нормативное ownership capabilities не переносится: main specs остаются source sets по `SOURCE-OF-TRUTH.md`. Этот change добавляет delta для `/5`. Active changes `/3` и `/4` сохраняют свои области; `/5` не переписывает их историю и release evidence.

Planning checkpoint `4c4a9cd6` зафиксировал проект до реализации. Владелец затем подтвердил Run aif-classic и поручил довести `/5` до рабочего Core release. Статус реализации и отдельной downstream qualification фиксирует `tasks.md`.

## Why

Несколько host-сессий не могут устойчиво распределить работу по Markdown backlog: после сбоя теряется владелец задачи и связь с уже созданным Run. Issue #5 требует локальную очередь до Run с атомарным захватом, проверяемым восстановлением и сохранением самостоятельной истории каждого исполнения.

## What Changes

- Ввести opt-in project queue: устойчивые QueueTask с исходным `TaskInput/1`, revision, priority/provenance, явными состояниями и отдельным QueueTaskClaim. Команды add/list/show/update/claim-next/release/complete/reconcile используют durable command identity и scoped CAS.
- До потенциального исполнения записывать QueueLaunchIntent; admission атомарно создаёт Run и его association. Неизвестный dispatch требует reconcile, а не повторного запуска или timeout requeue.
- Связать очередь с existing exact launch selection → questionnaire → prepare → reviewed start и продолжениями. Claim не выбирает launch и не разрешает programs; свободный request и typed launch inputs сохраняются раздельно.
- Добавить совместимую opt-in границу state/read и project launch summary/result для queue binding; старые Runs и ответы сохраняют свои версии и bytes.
- Обновить generated runner, CLI/read schemas, словарь и author index; добавить CLI concurrency/crash/recovery replay и отдельную квалификацию released binary/live hosts.

Это изменение продукта и runtime, не только процесса репозитория. Provider adapters, внешние статусы, автоматический draining, spawning, push/merge и автоматическая публикация не входят в capability.

## Capabilities

### New Capabilities

- `project-task-queue`: локальная identity задачи, владение, очередь, intent/admission/recovery, независимые Run associations и точное локальное завершение. Единственный новый нормативный source set после sync — `openspec/specs/project-task-queue/spec.md`; словарь и карта источников получают ссылки и соответствия.

### Modified Capabilities

- `cli-protocol`: opt-in queue CLI и generated runner сохраняют выбор launch, exact prepare/start и association продолжений.
- `published-contracts`: отдельные queue документы и новая opt-in state/read/project-result граница без изменения исторических bundles.

Ownership существующих capabilities не переносится; их current source sets остаются указанными в `openspec/SOURCE-OF-TRUTH.md`.

## Impact

Затронуты `internal/runtime` (очередь, admission, read/binding), `internal/local` (атомарная запись authority и Run), `cmd/prifly` (queue и launcher/runner), canonical/public schemas, tests/replay, `examples/`, README, словарь и карта источников. SQLite driver остаётся внутри local; transform-команды остаются чистыми. Queue не создаётся автоматически в legacy authority и не требует Git; Git/workspace claim остаётся отдельным механизмом.

Инженерная реализация проходит отдельный `aif-classic` Run, полные ворота и commit/merge. Закрытие #5 и archive требуют дополнительно E2-A10: fixtures на реально released binary и живой generated runner на поддерживаемых hosts. Development build и schema tests не заменяют эту квалификацию. Push и release tags остаются действиями владельца по правилам репозитория.

Для v0.13.77 владелец явно изменил приёмку: принять actual released CLI и
живые `codex-app`/`codex-cli`, исключив недоступный `claude-code`. Его live
qualification остаётся `pending`, а не PASS. Исключение относится только к
этому выпуску и закрытию #5; поддержка host и критерии следующих выпусков
не меняются. Владелец также явно разрешил агенту push и выпуск.

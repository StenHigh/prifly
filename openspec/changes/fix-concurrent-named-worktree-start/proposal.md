## Why

Два независимых `project start` с одинаковой основой имени worktree иногда дают `invalid_input` до создания Run.
Сбой воспроизведён на чистом baseline и блокирует полные ворота readability change.

## What Changes

- Найти точную причину отказа между получением Workspace claim и созданием Run при одновременных CLI-процессах.
- Сделать регрессию воспроизводимой через управляемое пересечение операций, без sleeps и повторов до случайного успеха.
- Исправить общий участок start/наблюдения состояния минимальной правкой после подтверждения причины.
- Сохранить разные worktree для разных command IDs и один результат для duplicate command ID.
- Проверить cleanup: отказ одного запуска не удаляет claim, package или дерево другого запуска.
- Повторить целевой стресс-тест и полные staged ворота. Затем отдельно продолжить readability change.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

Нет новых нормативных требований. `skip_specs: true`: исправление восстанавливает существующие требования:

- `runtime-resources`: «Читаемое имя выделяется вместе с durable claim reservation» и «Идемпотентность и lifecycle используют сохранённые имена».
- `workflow-and-context`: «Project объявляет общий шаблон имени disposable worktree».
- `cli-protocol`: reviewed naming policy допускает следующий свободный suffix без изменения review digest.

Текущие source sets перечислены в `openspec/SOURCE-OF-TRUTH.md`; ownership не меняется.
Если диагностика потребует нового контракта или изменения обещанного поведения, план нужно пересогласовать до implementation.

## Impact

Это исправление product CLI/runtime, а не изменение процесса проверки.
Предполагаемые места: `cmd/prifly/project_start.go`, naming/cleanup helpers и существующие tests.
`internal/runtime/start.go`, package loading и admission observation затрагиваются только при подтверждённой причине в этих слоях.

Новые API, flags, YAML-поля, schema/state versions и зависимости не предполагаются.
Сохранённые Runs, historical evidence и release contracts остаются неизменными.
Readability product patch и внешний AI Factory package не меняются этим исправлением.

Исходные результаты: [evidence](../improve-workflow-authoring-readability/evidence/worktree-gate-failure.json).
Причина package failure установлена и исправлена: конкурентная публикация одного каталога пакета.
Отдельный отказ подготовки worktree исследован: Git читает незавершённую регистрацию соседнего дерева.
Общий вызов Git защищает одну команду реестра; lifecycle независимых Runs остаётся параллельным.
[Результаты](acceptance.md) содержат обе RED/GREEN пары и последующий batch 50/50.

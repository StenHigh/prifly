## Why

[Issue #24](https://github.com/StenHigh/prifly/issues/24): оператор повторяет ID одобренной миграции при каждом prepare и start одной сборки. Authority уже хранит решение, но не хранит выбор владельца для launch.

## What Changes

- Добавить явный сохранённый выбор миграции для проекта и launch в одной authority.
- Предоставить команды просмотра, установки, замены и удаления выбора.
- Закрепить выбор за build key, exact consumer package и workflow Ref.
- Показывать ID решения и источник выбора в prepare. Включать выбранные данные в launch review digest.
- При изменении сборки или недействительном решении возвращать причину и маршрут повторной проверки.
- Сохранить разовый `--schema-migration` с приоритетом над настройкой. Разовый запуск не меняет сохранённый выбор.
- Сохранить terminal revoke, проверку suppliers, изоляцию старых Runs и запрет executable content.

Изменение затрагивает product runtime и public CLI. Ownership нормативных источников не меняется.

## Capabilities

### New Capabilities

Нет. Изменение расширяет существующий механизм schema migration и Project launch.

### Modified Capabilities

- `workflow-and-context`: выбор решения для нового Run может поступать из явно сохранённой настройки точной сборки.
- `control-security-ux`: owner управляет настройкой через атомарные authority commands; изменение настройки не создаёт trust decision.
- `cli-protocol`: команды управления выбором, приоритет явного flag, review provenance и безопасные отказы.
- `published-contracts`: отдельные closed contracts настройки и новые Project DTO boundaries без изменения старых bundles.
- `specification-governance`: словарь отличает сохранённый выбор от Plan, Decision и Run Binding.

## Impact

Затронуты `cmd/prifly/project.go`, `project_start.go`, `internal/projectwire/`, authority commands в `internal/runtime/` и новые versioned schemas.
Изменение потребует обновить public help, `examples/operations/`, authoring reference и embedded runner assets.
Новые внешние зависимости не требуются. YAML workflow и upstream package format не меняются.

Текущие источники: соответствующие `openspec/specs/<capability>/spec.md`, glossary и agent brief в `specification-governance`.
Активный change задаёт delta до archive+sync. Старые release evidence, сохранённые Runs и опубликованные bundles защищены.

## Non-goals

- Автоматический поиск последнего или подходящего разрешения.
- Создание миграции при установке, компиляции, prepare или start.
- Восстановление revoked package или изменение старого Run.
- Перенос выбора между authority, launch или сборками.
- Выпуск Core и проверка частной рабочей authority в рамках подготовки proposal.

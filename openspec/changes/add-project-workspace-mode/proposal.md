## Why

Разработчику нужен настоящий запуск объявленного Project workflow, а не набор
несвязанных compile, import, claim и Run команд. Worktree сохраняет безопасное
поведение по умолчанию для автоматизации, но в диалоге хоста место работы
должно быть явно выбрано до старта, чтобы пользователь не искал изменения не в
том checkout.

## What Changes

- Добавляется public Project launch path, который принимает только объявленный
  workflow launch, host и typed inputs, seal-ит/import-ит exact package и
  создаёт Run без скрытого выбора сценария или host.
- Project launch получает `workspace` mode: interactive host обязательно
  спрашивает `worktree` или `checkout` до старта; CLI сохраняет `worktree` as
  default только для non-interactive invocation.
- Оба режима получают exclusive authority claim одной physical repository;
  `checkout` никогда не создаёт, не переключает и не удаляет Git worktree.
- Launcher передаёт выбранный workspace в уже существующий assisted handoff;
  это не добавляет automatic model/provider launch, background driver или
  право на внешние действия.

## Capabilities

### New Capabilities

_Нет._

### Modified Capabilities

- `workflow-and-context`: Project launch становится исполнимым только через
  explicit declared launch и объявляет выбор рабочего режима.
- `domain-execution`: запуск связывает Project, Workspace, Authority и Run без
  смешения repository profile с локальными state.
- `runtime-resources`: claim поддерживает выделенный worktree и текущий
  checkout с одинаковой physical exclusivity и безопасным cleanup.
- `control-security-ux`: assisted workspace-write effect сохраняет exact
  workspace identity и не расширяет прежние grants или intents.
- `cli-protocol`: добавляется typed public command запуска project workflow и
  его diagnostics.
- `published-contracts`: новый state/read contract сохраняет прежние
  worktree-only Runs доступными прежним reader.

## Impact

Затрагиваются `cmd/prifly`, `internal/runtime` claims/sessions, публичные JSON
schemas, project authoring references, host skills, словарь, tests и OpenSpec. Меняется
поверхность binary, поэтому работа войдёт в следующий release, но не требует
новой зависимости или привязки Pri-Fly к AI Factory.

## Why

Ассистируемому шагу иногда нужно читать заявленное Git-дерево, не меняя его. Сейчас Pri-Fly создаёт claim только для записи или materialize-only tree, поэтому такой шаг не получает путь дерева; подмена этого права классом внешнего эффекта скрыто связала бы независимые возможности.

## What Changes

- StepDefinition v13 добавляет явное `repository_workspace: read_only` для ассистируемого шага; YAML `prifly-step/2` выбирает эту версию по содержимому.
- Project launch требует выбранный Git workspace, только когда шаг объявил это чтение или прежнюю Git-запись. Handoff передаёт точный claim, не добавляя разрешения на запись.
- Run без claim отказывается до dispatch; прежние definitions, Runs и сценарии без Git сохраняют свои контракты и поведение.
- **BREAKING:** нет; старые версии остаются закрытыми, новое поле допустимо только в v13.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `workflow-and-context`: декларация read-only Git workspace и её YAML lowering.
- `runtime-resources`: claim и handoff должны совпадать с декларацией шага, а запрет записи сохраняться.

## Impact

Затронуты versioned StepDefinition/schema, authoring frontend, Project launch validation и assisted handoff. Поле не называет внешнюю систему, провайдера, продукт или CLI и не меняет программные шаги.

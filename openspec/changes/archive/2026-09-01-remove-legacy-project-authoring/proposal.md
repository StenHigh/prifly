## Why

До первого public release Pri-Fly одновременно принимает три несовместимые
authoring формы проекта: profile v1, Python `task_recipe` и плоский
`prifly-package-source/1`. Они противоречат принятой модели, где YAML-папка
сценария — единственный читаемый исходник общего процесса команды, и создают
ложную совместимость, которой у продукта ещё нет.

## What Changes

- **BREAKING (pre-release):** принимать только `prifly-project-profile/2`;
  profile v1 и его отдельные roots больше не являются допустимым project
  source.
- **BREAKING (pre-release):** оставить у Project launch только `workflow`;
  Python `task_recipe`, direct JSON/plain workflow launch и инструкция launcher
  по их запуску удаляются.
- **BREAKING (pre-release):** package source принимает только directory
  `.prifly/workflows/NAME/` с `workflow.yaml`; плоский
  `prifly-package-source/1` не является входом `project compile`.
- Сохранить internal compiler как единственный путь от YAML folder к sealed
  package: он разрешает exact refs, валидирует closure и не создаёт граф.
- Заменить старые acceptance tests отрицательными проверками и сохранить
  сквозную компиляцию folder source.

## Capabilities

### New Capabilities

_Нет._

### Modified Capabilities

- `workflow-and-context`: Project execution profile и `project compile` имеют
  единственный YAML folder authoring route до sealing.

## Impact

Изменяются `cmd/prifly/project.go`, `cmd/prifly/project_compile.go`, их CLI
tests, generated Codex skill, актуальные authoring examples и OpenSpec glossary.
Runtime, sealed package bytes, сохранённые Runs, external dependencies и
authority state не меняются.

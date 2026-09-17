## Why

После того как шаг `implement` захватывает план (`workspace_trees` с входом и
выходом), шаги `verify` и `review` исполняются в claim-worktree без файла
плана: upstream `aif-verify` ищет план от корня repository, не находит и
уходит в fallback («No plan artifact found»: last commit / branch diff).
Единственная сегодняшняя форма, дающая шагу дерево, — binding с `output_port`
и `capture` на шаге `workspace_write`, а выдавать read-only gate право записи
ради чтения противоречит измеряемым `permitted_effects` (0.13.19).
Замер пилота и пакетчика: `workspace_trees: [{input_port: plan}]` на шаге с
`effects: none` отказывается `schema_invalid … the contract requires
output_port, capture`; другой формы в контракте нет (`internal/flow/compile.go`
требует `workspace_write` для любого binding).

Изменение затрагивает product runtime и опубликованные контракты
(StepDefinition, assisted SessionTask, workspace-tree guide), а не только
процесс репозитория. Ownership нормативных источников не меняется.

## What Changes

- **StepDefinition v8**: declared Workspace tree binding MAY быть
  materialize-only — `input_port` и `capture` без `output_port` — только на
  assisted шаге с `effects.class: none`. Binding с `output_port` остаётся
  прежним и по-прежнему требует `workspace_write`. v5–v7 не меняются.
- **Runtime**: перед handoff read-only шага runtime materialize-ит exact
  entries входного манифеста в claim-worktree того же Run по capture policy;
  отпечаток рабочей копии для проверки `effect_not_permitted` снимается
  **после** материализации; после settle попытки материализованные entries
  снимаются; захвата и нового манифеста нет.
- **Assisted SessionTask**: read-only шаг с таким binding'ом получает
  `repository_workspace` (сегодня поле есть только у workspace-write шагов);
  guide `workspace-trees.json` получает версию `workspace-tree-guide/2`, где
  запись без `output_port` означает «материализовано, в отчёте не объявлять».
- **Authoring** (`prifly-step/1`, `step-v2` authoring schema): та же форма в
  YAML; compiler отказывает materialize-only binding'у на шаге с
  `workspace_write` и binding'у с `output_port` на шаге с `effects: none`.
- Опубликованные bundle'ы: новый `schemas/core/step-definition-v8.schema.json`;
  v7 и ниже — байт в байт прежние.

## Capabilities

### New Capabilities

<!-- нет -->

### Modified Capabilities

- `domain-execution`: declared Workspace tree binding допускает
  materialize-only форму на read-only assisted шаге.
- `runtime-resources`: runtime materialize-ит вход read-only шага без capture,
  отпечаток эффектов — после материализации, снятие после settle.
- `workflow-and-context`: YAML authoring выражает materialize-only binding и
  compiler проверяет его согласованность с `effects`.
- `cli-protocol`: assisted handoff сообщает materialize-only binding и
  `repository_workspace` read-only шага; submission не объявляет такой порт.

## Impact

- `internal/flow`: `types.go` (StepDefinition v8, `output_port` optional),
  `schema.go` (embedded schema v8), `compile.go` (`checkWorkspaceTrees`),
  `authoring.go` (lowering `prifly-step/1`).
- `internal/runtime`: `workspace_trees.go` (materialize без capture),
  `effects.go`/`driver.go` (порядок отпечатка), `sessions.go`
  (`repository_workspace` read-only шага), `workspace_tree_guide.go` (guide/2),
  `compatibility.go` (v8 в допустимых версиях).
- `cmd/schema-gen`, `schemas/core/step-definition-v8.schema.json`,
  `scripts/check-schema.py` (новый bundle в списке), `schemas/authoring/README.md`,
  `examples/authoring/step-authoring-reference.yaml`, глоссарий
  (`terms.md`: Workspace tree binding).
- Пакеты: `aif-classic` получит проводку `plan` в `verify`/`review` в 1.38.0 у
  пакетчика после выпуска; этот change пакет не меняет.

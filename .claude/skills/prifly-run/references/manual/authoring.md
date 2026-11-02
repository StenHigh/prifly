[Индекс](index.md) · [Рабочее дерево](workspace.md) · [Данные](artifacts.md)

# Авторинг сценария

Authoring — исходные YAML-файлы, которые compiler превращает в sealed package.
Форма YAML не является runtime admission. Schema validation сама по себе не разрешает исполнение.
Начните с минимального близкого примера, затем добавляйте нужные declarations.

## Где находится каждое поле

| Документ/задача | Полный reference |
|---|---|
| Project profile, hosts, launches, worktree naming, queue selection | [project-profile](../examples/authoring/project-profile-authoring-reference.yaml) |
| Workflow stages, bindings, defaults, choice/call/repeat/parallel/map/wait | [workflow](../examples/authoring/workflow-authoring-reference.yaml) |
| Step executor, session limits, effects, workspace trees, model, hooks | [step](../examples/authoring/step-authoring-reference.yaml) |
| Instructions/context resources и substitutions | [context](../examples/authoring/context-authoring-reference.yaml) |
| Automatic result/content checks | [check](../examples/authoring/check-authoring-reference.yaml) |
| Programs, arguments, environment sources и live output | [execution-bindings](../examples/authoring/execution-bindings-authoring-reference.yaml) |
| Project settings, extension steps/calls, answers/resources/model mappings | [extension](../examples/authoring/extension-authoring-reference.yaml) |
| Typed preflight/runtime questions | [run-decision](../examples/authoring/run-decision-authoring-reference.yaml) |
| Workflow catalog entries и origin pins | [workflow-catalog](../examples/authoring/workflow-catalog-authoring-reference.yaml) |
| Imports, parents, layers, groups | [composition](../examples/authoring/workflow-composition-authoring-reference.yaml) |
| Package-declared extension points и carrier outputs | [extension points](../examples/authoring/extension-points-authoring-reference.yaml) |
| Unified composition с points | [unified composition](../examples/authoring/unified-composition-authoring-reference.yaml) |

References показывают поля и минимальные revisions. Не переносите новое поле в старый contract marker.
Если installed binary не объявляет нужную capability, source example сам по себе её не включает.
Точные editor schemas доступны через `schema`; используйте имя из его index, включая authoring contracts.

## Compile и inspect

```sh
"$PRIFLY_BIN" project compile --repository "$repository" --package "$PACKAGE" \
  --host codex-app --output /absolute/scratch/sealed-package --json
# Для declared build values добавьте повторяемый --value NAME=JSON.
```

Compile не запускает программы. Output directory должна находиться вне authority и не заменять source folder.
Host нужен только при assisted/host-bound resources выбранного package.
Inspect результата показывает exact compiled refs и build provenance; author version не означает «последний variant».
Для низкоуровневого `validate`/`preview` workflow path разрешается относительно authority, а не текущей shell directory.
Эти команды не читают произвольный внешний authoring folder; для него сначала используйте project compile.

```sh
"$PRIFLY_BIN" project extend --workflow FILE --workflow-id ID --extensions EXTEND.yaml \
  --output COMPILED.json --step-ref NAME=REF.json --json
# Локальный step source можно передать через --step-source NAME=FILE.
```

Project extend работает с declared root stages и routes. Вставка не получает inputs предыдущего шага автоматически.
Нужные input bindings объявляются явно; native call получает typed callee inputs/outputs.

## Граф и данные

`step` выполняет один контракт. `finish` задаёт outcome workflow.
`choice` имеет true/false/unknown; unknown не становится false.
`call` запускает native child workflow внутри того же Run, не отдельный project start.
`repeat` задаёт явный finite `max_iterations`; обычный backward edge вместо repeat отказывается.
`parallel` задаёт branches/join/quorum/remainder; `map` фиксирует коллекцию до создания children.
`wait` связывает declared correlation и schedule с устойчивым состоянием; late event не отменяет stop/cancel.
`partial` — declared workflow outcome, а `blocked` — verdict шага с собственными revision/schema требованиями.
`on_error` обрабатывает технический отказ по declared маршруту; accepted business fail не является тем же событием.
`technical_retries: N` задаёт 1..8 дополнительных технических попыток и требует WorkflowRevision 5.
Поле допустимо только у `kind: step`, когда `effects.retry_class` шага равен `pure` либо `idempotent`.
`pure` дополнительно требует `effects.class: none`; остальные retry classes вызывают `unsupported_retries`.
Предметный verdict не запускает технический retry. Пример поля находится у `stages.start` в workflow reference.

```text
source YAML -> refs/schema checks -> composition lowering -> native graph -> seal
```

Input ports имеют schema, format, required/default/configuration.
Bindings задают producer явно. JSON projection использует pointer и compatible projected_schema_ref.
Локальные aliases `refs` сокращают exact ссылки; они не выбирают новый package variant автоматически.
Версионные references включают identity/version/digest; другое имя с похожим смыслом не заменяет ref.

## Context, checks и publications

`instructions_ref` и `context_refs` фиксируют exact инструкции и supporting resources.
Context substitutions берут только объявленные значения; runtime context request не расширяет эффекты.
`result_check_refs` объявляет автоматические checks; waiver допустим только по policy и не означает pass.
Hooks могут сообщать state/event, публиковать artifact и выполнять content checks.
`cardinality:keyed_many` требует явного close. Subscription once/each/new_only имеет собственную семантику cursor и producer failure.
Blob publication отличается от JSON schema port. Схема публикации не заменяет typed subscription binding.

## Extensions и composition

Простая extension сохраняет forward graph и не создаёт произвольный цикл.
Native call extension выполняется compiler-ом внутри того же Run; effects caller не расширяют effects child.
Composition читает source exports, parents и ordered layers до flattening.
Общий ancestor включается один раз; overlapping additions требуют explicit arrange.
Package point объявляет anchor, inputs, routes и consumer ports. Project implementation задаёт explicit outputs.
Carrier появляется у reader только по binding. Факт публикации report не доказывает deploy/provision.

Contracts различаются: folder `/2` + extension `/3` описывают composition; extension `/4` + points `/1` — opt-in points.
Unified folder `/3` + extension `/5` объединяет их с b2 build identity и собственными boundaries.
Старые paths остаются на своих contracts. Не меняйте marker ради обхода compile refusal.

## Практические маршруты

Program без Git/ИИ: [CSV → validation → report](../examples/workflows/csv-report/README.md).
Blocked и продолжение: [blocked-condition](../examples/workflows/blocked-condition/README.md).
Readable worktree: [named-worktree](../examples/workflows/named-worktree/README.md).
Nested points: [пример](../examples/nested-extension-points/README.md).
Composition: [пример](../examples/workflow-composition/README.md).
Unified composition: [пример](../examples/unified-composition/README.md).

Низкоуровневая capability без публичного CLI handler описывается как contract boundary, а не выдуманная команда.
Например, наличие `schedule` в capabilities не доказывает наличие top-level `prifly schedule`.

См. также [индекс capabilities](capabilities.md), [команды](commands.md), [нормативный workflow contract](source-unavailable.md).

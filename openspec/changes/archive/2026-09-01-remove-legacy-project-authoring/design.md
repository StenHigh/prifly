## Context

См. `proposal.md`. Текущий v2 folder compiler уже строит internal
`projectPackageSource` из `workflow.yaml` и component folders, но CLI всё ещё
принимает v1, Python launch и flat package file как внешние source forms.

## Goals / Non-Goals

**Goals:**

- Оставить один tracked, читаемый YAML source для graph, steps, schemas,
  contexts и launches проекта.
- Сохранить sealing, exact refs и immutable package/Run boundary.
- Отказать старым source forms до output directory и authority mutation.

**Non-Goals:**

- Не менять StepDefinition, WorkflowRevision, sealed package bytes или
  сохранённые Runs.
- Не удалять provider-neutral `task prepare` как самостоятельную intake
  capability.
- Не создавать compatibility migration для дорелизных project sources.

## Decisions

### Folder — единственный внешний package source

`projectPackageSourceLocation` требует directory, а `projectCompile` всегда
читает folder. `projectPackageSource` и его parser остаются internal temporary
representation compiler: удаление его заставило бы дублировать validation и
изменило бы sealed contract без пользы.

Альтернатива — оставить flat YAML как expert mode — отклонена: именно два
авторских graph source делают непонятным, где правда.

### V2 workflow launches без Python recipe

Profile parser принимает только v2 и `kind: workflow`, который ссылается на
`workflow.yaml` folder root; Go data model и generated `prifly-run` skill не
содержат recipe или direct machine-workflow branch. `task prepare` не зависит
от launch и сохраняется как отдельная CLI capability.

Альтернатива — скрыть legacy за feature flag — отклонена: до public release
она только продлевает неподдерживаемый contract.

### Проверка через отрицательные границы и один живой folder scenario

Старые positive tests становятся explicit rejection tests. Existing folder
compile/import scenario доказывает, что единственный authoring route по-прежнему
создаёт sealed package; launcher listing доказывает workflow inputs. Это
покрывает boundary без новой test framework.

### Contributor gate не повторяет release race

Обычный change выполняет `make ci-check`, full e2e и targeted race для
затронутого CLI. Полный `make race` сохраняется отдельным mandatory manual
gate для RC/release evidence. Это соответствует delivery policy: expensive
global race не расходует CI minutes на каждом ограниченном change, но не
выдаётся за выполненную release qualification.

## Risks / Trade-offs

- [Локальный unreleased project использует старый source] → compiler даёт
  diagnostic до side effect; author переносит content в workflow folder.
- [Internal representation ошибочно принимают за authoring API] → tests
  вызывают только public project commands и profile files.
- [Documentation оставляет второй source правдой] → обновляются glossary,
  authoring spec и examples в одном change.

## Migration Plan

1. Удалить старые parser/compile/launcher branches и их fixtures.
2. Обновить current authoring documentation и examples.
3. Выполнить targeted CLI tests и race для затронутого CLI, folder compile
   scenario, `make ci-check` и `make e2e` локально; full `make race` оставить
   для ручного RC/release evidence.
4. Не отправлять промежуточные commits; включить срез в следующий готовый
   remote validation batch.

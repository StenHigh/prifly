## Why

Проверенные локально product gates не запускаются автоматически для merge
request и веток GitLab. После удаления document tooling pipeline должен
проверять только текущую сборку, Go/Schema gates и end-to-end путь.

## What Changes

- Добавить native GitLab CI pipeline с отдельными jobs для `make check` и
  `make e2e`.
- Использовать Go toolchain, соответствующий `go.mod`, и подготовить только
  developer-зависимости, которые уже нужны этим командам: C compiler и Python
  standard runtime.
- Кэшировать Go build/module cache между jobs без передачи state, binary или
  evidence между запусками.
- Не добавлять document checks, release publication, deployment или внешние
  credentials.

## Capabilities

### New Capabilities

<!-- Нет: это repository tooling, а не новая product capability. -->

### Modified Capabilities

<!-- Нет: очередность CI уже закреплена в delivery-roadmap; runtime behavior не меняется. -->

## Impact

Добавляется `.gitlab-ci.yml` и при необходимости небольшая проверка её
структуры. Product runtime, YAML authoring, JSON contracts и source ownership
не меняются.

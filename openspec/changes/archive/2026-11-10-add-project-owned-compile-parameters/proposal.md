## Why

Issue [#22](https://github.com/StenHigh/prifly/issues/22) требует выбирать предел task.description до первичной проверки входа. Существующий `compile --value` не участвует в prepare/start, а project settings меняют только defaults входов.

## What Changes

- Пакет объявляет типизированные параметры сборки, диапазоны, defaults и точные bindings в отдельном `compile-parameters.yaml` версии `/1`.
- Проект сохраняет значения в `project/compile-values.yaml` версии `/1`; update сохраняет этот файл вместе с каталогом `project/`.
- Compiler применяет параметры до валидации task через существующую scalar substitution и согласует refs полного closure.
- Compile, prepare и reviewed start читают одинаковые файлы. Значения участвуют в build identity и launch review.
- Отказы охватывают тип, диапазон, неизвестный параметр, чужую цель и несовместимый binding.
- Приёмка C1–C8 проверяет Unicode, несколько проектов в одной authority и неизменность старых Runs.

## Capabilities

### New Capabilities

Нет новых нормативных source sets.

### Modified Capabilities

- `workflow-and-context`: объявленные параметры сборки и project-owned значения.
- `cli-protocol`: единый resolver compile/prepare/start и диагностика отказов.
- `published-contracts`: новые самостоятельные authoring contracts без изменения старых published bundles.
- `specification-governance`: каноническое определение параметра сборки.

## Impact

Изменение затрагивает продукт: Project compiler и launch preparation. Authority runtime, saved Run contracts и YAML workflow operators сохраняют текущие версии. Основные файлы находятся в `cmd/prifly`, authoring schemas, `examples/authoring` и operations handbook. Владение нормативными источниками не переносится. Consumer workflows issue #3 остаётся отдельной приёмкой.

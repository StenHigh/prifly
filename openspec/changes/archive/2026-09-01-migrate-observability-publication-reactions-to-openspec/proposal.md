## Why

Нормы из `docs/roadmap/state-and-telemetry.md` ошибочно лежат рядом с
delivery roadmap, хотя они задают самостоятельный будущий договор продукта:
наблюдаемость, публикации шагов и durable реакции. Пока они не перенесены,
OpenSpec не является единственным местом изменения этих правил.

## What Changes

- Создать постоянную capability с 44 правилами и 88 проверяемыми сценариями
  наблюдаемости, публикаций и реакций.
- Сохранить legacy IDs, стадии, фактические declared statuses, traceability и исходные связи только
  в архивированной crosswalk, а постоянную спецификацию оставить читаемой без
  устаревших идентификаторов.
- Разделить ownership-карту: `state-and-telemetry.md` станет отдельным
  source set, а delivery roadmap не будет содержать повтор норм продукта.

## Capabilities

### New Capabilities

- `observability-publication-reactions`: договор телеметрии, объявленных hooks
  и реакций workflow на сохранённые наблюдения.

### Modified Capabilities

- Нет.

## Impact

Меняется только ownership документации. Go runtime, CLI, JSON Schema,
authoring YAML, тесты, roadmap-статусы, evidence и historical manifests не
меняются; профиль F2 остаётся `specified_not_implemented`.

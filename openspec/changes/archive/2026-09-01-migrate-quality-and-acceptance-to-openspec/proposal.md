## Why

Нормы качества и 148 product acceptance cases всё ещё распределены между
legacy-главами, CSV и traceability-таблицами. Поэтому OpenSpec пока не является
единственным местом, где меняют правило о доказательстве качества или приёмке.

## What Changes

- Создать capability `quality-and-acceptance`: правила доказательства качества,
  distinction между specified и executed, и полный читаемый каталог product
  acceptance scenarios.
- Перенести status, evidence boundary и traceability смысл, не объявляя
  historical `passed` evidence новой квалификацией.
- Оставить legacy case/requirement IDs, таблицы связей и точные ссылки на
  evidence только в индивидуальной archived crosswalk.

## Capabilities

### New Capabilities

- `quality-and-acceptance`: качество, квалификация и acceptance contract
  Pri-Fly.

### Modified Capabilities

- Нет.

## Impact

Меняется только ownership документации. Go runtime, тесты, release evidence,
статусы 148 cases, JSON Schema, manifests и roadmap implementation status не
изменяются.

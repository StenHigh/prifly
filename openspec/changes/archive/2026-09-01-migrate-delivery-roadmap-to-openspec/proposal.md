## Why

Действующие цели поставки, границы RC и будущая очередь всё ещё смешаны с
историей реализации в legacy roadmap и progress documents. Поэтому команда не
может отличить текущий план от старого evidence и OpenSpec пока не является
единственным местом обновления delivery-решений.

## What Changes

- Создать capability `delivery-roadmap` для двухфазного плана, milestones,
  статусов/evidence, границ release candidate, будущей очереди и состава первой
  поставки.
- Перенести текущий delivery snapshot и future queue без копирования тысяч
  строк исторических отчётов; exact history останется в Git и в archive
  migration crosswalk.
- Переключить ownership-карту только после инвентаризации roadmap, RC scope,
  current status, release/archive и dependencies.

## Capabilities

### New Capabilities

- `delivery-roadmap`: порядок развития Pri-Fly, meaning статусов и evidence,
  актуальные release boundaries и будущая очередь.

### Modified Capabilities

- Нет.

## Impact

Меняется только documentation/process ownership. Runtime, CLI, YAML, schemas,
исторические evidence и фактические release statuses не меняются.

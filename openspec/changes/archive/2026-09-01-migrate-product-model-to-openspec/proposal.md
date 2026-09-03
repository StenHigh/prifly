## Why

Модель продукта сейчас живёт в legacy-главе `docs/spec/01-product.md` и её
картах требований и приёмки. Чтобы OpenSpec стал единственным нормативным
источником до первого release candidate, этот смысл нужно перенести в
читабельную capability без сохранения старых внутренних номеров в постоянной
спецификации.

## What Changes

- Создать постоянную OpenSpec capability `product-model` с requirements и
  scenarios, эквивалентными правилам назначения, границ, универсальности,
  ролей, целей, пакетов и критериев целостности Pri-Fly.
- Сохранить связь со всеми существующими acceptance cases в архивной legacy
  coverage crosswalk этого change; старые `PROD-*` IDs не попадут в постоянный
  spec.
- После проверки replacement переключить строку `Модель продукта` в
  `openspec/SOURCE-OF-TRUTH.md` на OpenSpec и оставить legacy source
  неизменённым до final cleanup.

## Capabilities

### New Capabilities

- `product-model`: назначение Pri-Fly, его универсальность, границы,
  ответственность, целевой RunBrief, режимы, качество, packages и проверяемая
  независимость продукта.

### Modified Capabilities

- Нет.

## Impact

Изменяется только ownership нормативной документации. Go runtime, CLI,
пользовательский YAML, JSON wire contracts, acceptance evidence и historical
manifests не меняют поведения.

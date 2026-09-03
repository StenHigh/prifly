## Why

Архитектурные границы Pri-Fly и решения, которыми они обоснованы, остаются в
legacy главе и 22 ADR-файлах. Пока они не в OpenSpec, acceptance catalog не
может ссылаться на единый будущий источник понятными именами.

## What Changes

- Создать `architecture-decisions` с 17 архитектурными требованиями и
  читаемыми основаниями решений.
- Перенести содержание 22 decision records под capability без старых
  ordinal/ADR authoring IDs; сохранить точные старые имена только в archive.
- Сохранить переходные связи с acceptance и published contracts, не меняя
  runtime или существующую qualification.

## Capabilities

### New Capabilities

- `architecture-decisions`: архитектурные границы, compatibility decisions и
  основания их пересмотра.

### Modified Capabilities

- Нет.

## Impact

Меняется только ownership документации. Код, Go modules, JSON Schema, decision
history, evidence, manifests и сохранённые Runs не меняются.

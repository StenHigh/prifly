## Why

Базовый профиль Pri-Fly, его границы, пример и перечень будущей приёмки пока
живут отдельным комплектом `docs/foundation/`. Это не позволяет завершить
перенос качества без временных или выдуманных ссылок на источник требований.

## What Changes

- Перенести нормативные правила профиля `foundation-sequence/1` в постоянную
  capability OpenSpec с явными границами того, что профиль поддерживает и чего
  не заявляет.
- Сохранить пример последовательного workflow и его checker как исторические
  fixtures, а не превращать их в runtime-функцию OpenSpec.
- Перенести 24 сценария foundation-приёмки как требования будущей
  квалификации, не меняя их статус `specified_not_executed`.
- Создать архивную точную crosswalk старых путей и FND-идентификаторов, затем
  переключить только строку foundation profile в ownership-карте.

Изменение не затрагивает Go runtime, YAML authoring, публичные API, schemas
или существующие evidence.

## Capabilities

### New Capabilities

- `foundation-profile`: границы, инварианты и будущая квалификация первого
  профиля последовательного исполнения.

### Modified Capabilities

- Нет.

## Impact

Затронуты `docs/foundation/`, `openspec/SOURCE-OF-TRUTH.md` и новый постоянный
раздел `openspec/specs/foundation-profile/`. Исторические evidence и manifests
остаются неизменными.

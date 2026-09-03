## Why

YAML-папка уже является единственным исходником Project workflow, но редактор
не знает её версий и полей до запуска CLI. Автору приходится держать структуру
в голове или сверяться с большим example, хотя ошибки простых полей можно
увидеть прямо при редактировании.

## What Changes

- Добавить локальный, версионированный JSON Schema contract для YAML-источников
  Pri-Fly: project profile v2, workflow folder, extensions, workflow, step и
  context.
- Дать авторам понятный способ подключить contract из обычных YAML-редакторов
  через comment-modeline и documented local schema mappings.
- Добавить manifest contract и небольшую проверку, что опубликованные schema
  documents и authoring references согласованы.
- Явно оставить semantic validation, exact refs, lowering и sealing
  обязанностью compiler; editor schema не меняет runtime или sealed bytes.

## Capabilities

### New Capabilities

_Нет._

### Modified Capabilities

- `workflow-and-context`: YAML authoring получает локальный editor contract,
  не меняющий его compiler и sealed package contract.

## Impact

Добавляются static JSON Schema documents и их краткое руководство, modelines в
authoring references и проверка static contract. Go runtime, CLI protocol,
YAML authoring semantics, package bytes, Run history, external dependencies и
authority state не меняются.

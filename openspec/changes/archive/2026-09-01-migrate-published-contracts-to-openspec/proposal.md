## Why

Документация published contracts пока разделена между legacy Markdown,
baseline JSON fixtures, generated versioned schemas и Go definitions. Пока
ownership не перенесён, OpenSpec не даёт единой точки, где меняют правила
публикации, compatibility и границы доказательств contract surface.

## What Changes

- Создать capability `published-contracts`, описывающую publication,
  versioning, compatibility и verification boundary машинных contracts.
- Перенести current contract ownership в OpenSpec без копирования больших JSON
  schemas или Go types в Markdown и без изменения их bytes/runtime semantics.
- Заархивировать legacy contract-guide inventory, baseline fixture/status
  meaning и обнаруженное расхождение documented и actual component count;
  сначала установить факт, а не silently переписать число.
- Переключить одну строку карты источников только после crosswalk, strict
  validation и проверки, что runtime schemas/types/evidence не изменены.

## Capabilities

### New Capabilities

- `published-contracts`: правила публикации и совместимости JSON contracts,
  fixtures, generated schemas и их связи с Go runtime types.

### Modified Capabilities

- Нет.

## Impact

Меняется только ownership нормативной документации. Product runtime, CLI,
authoring YAML, JSON Schema bytes, Go definitions, accepted Runs, evidence и
historical manifests не меняются. Machine-readable schemas и generated
contracts остаются product artifacts рядом с кодом, а не второй Markdown
копией в OpenSpec.

## Purpose

Определяет правила публикации, совместимости и проверки machine-readable
contracts Pri-Fly, не подменяя сами JSON Schema и Go types Markdown-копией.

## ADDED Requirements

### Requirement: Семантика и machine shape имеют разные, явные источники
Published-contracts capability SHALL отделять semantic rule от machine shape.
OpenSpec MUST быть current source для смысла publication, compatibility,
validation boundary и ownership. Versioned JSON Schema, generated public
bundles и corresponding Go types MUST оставаться exact product artifacts;
OpenSpec MUST NOT дублировать их поля или bytes.

#### Scenario: Автор меняет публичный contract
- **WHEN** изменение затрагивает observable JSON DTO или definition
- **THEN** author изменяет semantic capability и exact product artifact вместе,
  а crosswalk/verification показывает их declared relationship

### Requirement: Опубликованный contract сохраняет compatibility boundary
A published contract MUST иметь явную version/identity boundary. Изменение
MUST NOT silently расширять closed historical shape, менять meaning accepted
Run или считать новый Go field частью старого public contract. Новый behavior
MUST использовать declared compatible version/bundle либо быть rejected before
use.

#### Scenario: Новое поле нужно новому reader
- **WHEN** новая capability требует дополнительное public поле
- **THEN** historical reader and saved Run сохраняют прежний shape и meaning,
  а новый contract объявляет собственную versioned boundary

### Requirement: Fixtures и schema checks не являются qualification evidence
Contract documentation SHALL различать structural schema validation, semantic
admission, runtime execution и release qualification. Positive fixture,
generated-schema match или successful document check MUST NOT обещать
installed adapter, permissions, external effect или qualified profile.

#### Scenario: Baseline fixture проходит shape validation
- **WHEN** fixture проходит declared schema check
- **THEN** result говорит только о форме и явно сохраняет отдельный execution
  status вместо заявления о runtime qualification

### Requirement: Contract publication проверяется против canonical artifacts
Published contract surface MUST иметь reproducible verification, которая
обнаруживает расхождение generated schema, embedded/runtime representation,
fixture registry и declared compatibility boundary. Documentation count or
inventory claim MUST быть проверен against canonical artifact before он
становится current fact.

#### Scenario: Inventory расходится с artifact
- **WHEN** документированное количество contracts отличается от actual
  machine-readable inventory
- **THEN** migration records the discrepancy, determines its source and fixes
  only the supported current claim with an explicit verification; it does not
  relabel historical evidence or silently normalize bytes

### Requirement: Legacy contract material остаётся историческим до final cleanup
Migration SHALL сохранять byte-identical legacy contract guide, fixtures,
schemas, Go definitions, evidence и manifests до общего final cleanup. Archive
crosswalk MUST record legacy inventory/status vocabulary and map it to the
permanent contract rules without exposing old record IDs as a new authoring API.

#### Scenario: Постоянная спецификация ссылается на legacy baseline
- **WHEN** reader needs provenance старого baseline contract
- **THEN** он находит exact historical record в archive or Git, а permanent
  spec остаётся readable current rule without a second editable baseline

Текущий source set — `openspec/specs/published-contracts/spec.md`. Ownership не меняется.
Machine shapes остаются рядом с Go types и canonical schemas.

## ADDED Requirements

### Requirement: Сохранённый выбор имеет отдельные closed contracts
Состояние выбора, inspection и результаты commands SHALL иметь собственные versioned closed schemas.
Новый reader SHALL трактовать отсутствие объекта выбора как отсутствие настройки без записи при чтении.
Неизвестная версия MUST давать именованный отказ до использования выбора.
Новые Project summary/start поля происхождения SHALL публиковаться в новых DTO boundaries.
Старые public bundles MUST оставаться неизменными.

#### Scenario: Reader открывает старую authority
- **WHEN** объект выбора отсутствует
- **THEN** reader сохраняет прежний admission и не создаёт объект

#### Scenario: Новый summary содержит источник выбора
- **WHEN** prepare использует saved либо explicit migration decision
- **THEN** фактический JSON проходит новую closed schema; старый bundle не расширяется неизвестными полями

#### Scenario: Старый binary открывает новую настройку
- **WHEN** binary без поддержки настройки запускает workflow без разового flag
- **THEN** настройка не создаёт неявного разрешения; прежний migration admission сохраняет запрет без явно выбранного решения

### Requirement: Сохранение выбора не требует новой Run boundary
Run SHALL закреплять существующий SchemaMigrationBinding независимо от источника выбора.
Хранение настройки MUST NOT переписывать `authority-packages/2` или прежние Runs ради добавления launch выбора.
Run без migration binding MUST сохранять прежнюю подходящую boundary.

#### Scenario: Новый Run использует saved
- **WHEN** start принимает сохранённое решение
- **THEN** Run использует существующие migration state/read contracts с прежней семантикой binding

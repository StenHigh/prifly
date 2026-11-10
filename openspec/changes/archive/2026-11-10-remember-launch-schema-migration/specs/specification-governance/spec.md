Текущий source set — `openspec/specs/specification-governance/spec.md`, `terms.md` и `agent-brief.md`.
Ownership не меняется. Конкретные Go/JSON bindings добавляются только вместе с реализацией.

## ADDED Requirements

### Requirement: Сохранённый выбор launch отличается от решения миграции
Словарь SHALL определять LaunchSchemaMigrationChoice как явный выбор владельца для проекта, launch и точной сборки в одной authority.
SchemaMigrationDecision SHALL сохранять смысл одобренного ограниченного допуска схем.
SchemaMigrationBinding SHALL сохранять смысл происхождения допуска, закреплённого отдельным Run.
Словарь и agent brief MUST объяснять, что сохранённый выбор не восстанавливает package и не создаёт новое разрешение.

#### Scenario: Автор разбирает повторный запуск
- **WHEN** reader читает словарь после реализации
- **THEN** он различает сохранённый выбор, одобренное решение и binding нового Run; term SchemaMigrationSelection продолжает обозначать схему и поставщика

## ADDED Requirements

### Requirement: Термины schema migration сохраняют различие доверия и отзыва
Словарь SHALL определять SchemaMigrationPlan как read-only снимок проверяемого
scope и preconditions. SchemaMigrationDecision SHALL обозначать записанное
решение владельца по точному плану. SchemaMigrationBinding SHALL обозначать
закреплённое новым Run происхождение ограниченного допуска schemas.
Эти понятия MUST NOT означать restore пакета, изменение старого trust decision,
новый schema Ref либо право на executable content.
Lifecycle generation SHALL обозначать версию решения для одного package;
новое принятое lifecycle решение меняет её даже при повторном status.
Словарь MUST обновить PackageStatus, точные Go/JSON bindings и agent brief
вместе с реализацией. Планируемые типы MUST NOT объявляться существующими до этого.

#### Scenario: Автор ищет смысл миграции
- **WHEN** reader открывает словарь после реализации
- **THEN** он отличает план, решение и binding от PackageStatus, package trust и сохранённого schema Ref

#### Scenario: Владелец снова отзывает тот же package
- **WHEN** новая команда принимает повторный revoke того же exact Ref
- **THEN** новая lifecycle generation отличает этот отзыв от покрытого прежним решением; повтор прежнего command id остаётся идемпотентным

Источник — `openspec/specs/specification-governance/spec.md`, `terms.md` и
`agent-brief.md`; ownership не меняется. Delta не переименовывает существующие
JSON fields и не меняет canonical meanings исторических Runs.

## ADDED Requirements

### Requirement: Читаемое имя Workspace не подменяет идентичность claim

Словарь SHALL уточнять существующие Project execution profile, Project questionnaire / Launch summary и WorktreeClaim / Workspace mode: профиль объявляет optional naming policy, prepare показывает незарезервированный кандидат, claim хранит фактические path/branch. Claim ID SHALL оставаться служебной идентичностью для ownership, generation, идемпотентности и привязки Run; basename MUST NOT использоваться как новый task ID или Run identity.

Уточнение SHALL сохранять значения и JSON bindings исторических claims/Runs. Ограничение независимых claims SHALL соответствовать текущей physical conflict relation worktree/checkout, без возврата к одному claim на repository. Новые Go/JSON bindings SHALL добавляться только после реализации; смысловое ревью и `TestGlossaryBindings` SHALL сопровождать изменение карты.

Текущий authoritative source set — `openspec/specs/specification-governance/spec.md`, `terms.md`, `agent-brief.md`; ownership не переносится. Эти уточнения выполняются вместе с реализацией, без переписывания historical evidence.

#### Scenario: Имя совпадает с названием задачи
- **WHEN** два независимых claim имеют основы для одной задачи и разные suffix
- **THEN** у каждого собственный claim ID и привязка Run, а общее читаемое имя не объединяет ownership или идемпотентность

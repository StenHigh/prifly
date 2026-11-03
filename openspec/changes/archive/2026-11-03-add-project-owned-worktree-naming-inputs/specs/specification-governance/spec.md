## ADDED Requirements

### Requirement: Project naming input is distinct from a workflow input

Словарь SHALL определять **Project naming input — Вход именования проекта** как типизированное JSON value потребителя для имени нового worktree. Он SHALL ссылаться на project-owned schema и сохраняться как metadata claim. Он MUST NOT обозначать workflow port, task identity, queue item, новый Artifact либо универсальный объект бизнес-данных.

Profile `/5`, отдельный CLI flag, JSON fields и Go mappings SHALL использовать это понятие последовательно. `input` binding SHALL сохранять значение workflow input; `naming_input` SHALL адресовать вход именования проекта. Словарь, затронутые specs, author reference и проверки MUST обновляться вместе. Новые bindings SHALL проходить `TestGlossaryBindings`.

Source set остаётся `openspec/specs/specification-governance/{spec,terms,agent-brief}.md`; ownership не переносится. Historical terms/evidence и прежние serialized fields MUST NOT переименовываться.

#### Scenario: An author adds naming metadata to a consumer
- **WHEN** проект объявляет naming input независимо от закрытой task schema upstream workflow
- **THEN** документация и outputs называют его входом именования проекта, не новым workflow port или task schema field

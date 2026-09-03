## MODIFIED Requirements

### Requirement: Каноническая терминология имеет один текущий словарь
Capability `specification-governance` MUST хранить действующий словарь в
`openspec/specs/specification-governance/terms.md`. Словарь определяет
канонические понятия Pri-Fly, их границы и перечисленные Go/JSON соответствия;
он не меняет wire-схему, сохранённые данные или фактический статус roadmap.
Словарь и `spec.md` составляют один явно названный source set этой capability,
а не независимо редактируемые конкурирующие документы. Описание Project
execution profile MUST называть host-specific skills roots neutral project
settings, а не выдавать один agent-host directory за обязательный путь Pri-Fly.

#### Scenario: Участник ищет значение понятия
- **WHEN** участник встречает термин Pri-Fly или хочет добавить сущность
- **THEN** он находит единственное каноническое определение в `terms.md` и
  уточняет предметную семантику в указанной capability specification

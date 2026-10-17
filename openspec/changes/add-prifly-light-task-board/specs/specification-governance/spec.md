## ADDED Requirements

### Requirement: Термины Light отделены от исполнения и data projection

Канонический словарь SHALL определять **Pri-Fly Light** как opt-in read-only интерфейс task/Run/review обзора и **Light projection mapping** как versioned observer declaration отображения exact pinned definitions/typed evidence. Mapping MUST NOT переименовывать существующий ProjectionManifest, создавать WorkflowRevision, новый queue, runtime state или gate policy.

UI phase/lane и review chip SHALL описываться как представление записанных фактов. Gate/report/lens/candidate/round в Light SHALL явно обозначать consumer typed evidence и его scope, не новые обязательные execution сущности. Go/JSON bindings SHALL добавляться только при реальной реализации типов; `TestGlossaryBindings` SHALL проверять изменённую карту. QueueTask/TaskInput/RunBrief/Run, status/verdict/outcome и ReadCut SHALL сохранять текущие значения.

Current source set — `openspec/specs/specification-governance/spec.md`, `terms.md`, `agent-brief.md`; ownership не переносится. Карта источников SHALL получить единственную новую capability `prifly-light`, без второго normative source для queue или local monitor. Historical terms/evidence и saved JSON MUST NOT переписываться ради UI vocabulary.

#### Scenario: Колонка «Готово» и review chip
- **WHEN** автор документирует Light phase, локальное task complete или consumer review gate
- **THEN** определения сохраняют scope записанного объекта, не объявляют новый Run status и не обещают merge либо успешный review по `StepResult pass`

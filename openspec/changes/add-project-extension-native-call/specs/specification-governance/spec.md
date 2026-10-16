## ADDED Requirements

### Requirement: Workflow extension охватывает step и native Call без смешения identities
Source set `specification-governance` SHALL сохранять единственное определение
Workflow extension в `terms.md` и ссылаться на `workflow-and-context` для точного
authoring contract. Определение SHALL описывать единичную compile-time вставку
объявленного Project step либо native Call в exact root route; legacy step
binding limit MUST отличаться от multiple bindings новой call ветви.

Call target SHALL называться WorkflowRevision, его runtime включение —
WorkflowInvocation внутри того же Run. Extension ID SHALL отличаться от
callee identity; прежний step stage ID MUST сохраняться. Compiler MUST NOT
вводить новую предметную сущность или Go тип только ради названия ветви.
Изменение определения SHALL сопровождаться затронутыми contract artifacts,
примерами и tests; опубликованные JSON fields и meaning pinned Runs MUST
сохраняться. Существующие Go/JSON bindings SHALL проверяться
`TestGlossaryBindings`; source ownership не переносится.

#### Scenario: Автор читает словарь перед новой вставкой
- **WHEN** автор выбирает новую call ветвь Workflow extension
- **THEN** словарь отличает authoring запись, exact target и runtime invocation и ведёт к единственному актуальному контракту без обещания независимого Run или рекурсивного patch

#### Scenario: Legacy step читается новым бинарником
- **WHEN** прежняя step extension и сохранённый Run читаются после расширения определения
- **THEN** имена и смысл существующих fields/identities остаются прежними, а новая call возможность не переинтерпретирует старую запись

## ADDED Requirements

### Requirement: Unified source composition retains canonical terms
Glossary SHALL уточнять Workflow composition, Workflow extension, Workflow extension point, Component export, Workflow layer и Workflow contribution для `/5`. Числа markers MUST описываться как версии конкретных документов, а не как линейная замена всех capabilities.

Compiled workflow variant SHALL означать immutable compiler result одного source workflow с определённой point policy и settings. Он MUST отличаться от source author identity, runtime WorkflowInvocation и отдельного Run. Реальные Go/JSON bindings SHALL добавляться только вместе с соответствующими types и проверками.

#### Scenario: One source produces two compiled variants
- **WHEN** author сравнивает source identity и exact compiled refs в review
- **THEN** glossary однозначно объясняет разные versions без нового runtime объекта или переименования saved JSON fields

### Requirement: Unified change owns only its new versioned delta
Main source sets SHALL сохранять ownership по `SOURCE-OF-TRUTH.md`. Новый active delta SHALL определять `/5`, folder `/3`, b2 и новые DTO. Прежние active changes SHALL сохранять baseline `/3` и `/4` до своего archive/sync.

Реализация SHALL обновлять source-of-truth map, glossary, agent brief и author entry points в одном change. Historical release evidence, archived changes и старые schemas MUST NOT переписываться как доказательство новой поддержки. Документная проверка MUST NOT закрывать runtime или продуктовый gate.

#### Scenario: Unified support is documented before implementation
- **WHEN** proposal и schemas design прошли OpenSpec validation
- **THEN** capability остаётся проектируемой; `/4` published qualification и будущий `/5` release evidence не смешиваются

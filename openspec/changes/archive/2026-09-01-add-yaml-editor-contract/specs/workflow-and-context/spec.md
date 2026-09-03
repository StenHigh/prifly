## ADDED Requirements

### Requirement: YAML authoring имеет локальный editor contract
Repository MUST публиковать versioned local JSON Schema documents и manifest
для Project profile v2, workflow folder root, extension list, workflow, step и
context YAML. Contract MUST называть document kind, version marker, known
top-level fields и portable local schema association. Он MUST работать без
сети, AI Factory, credentials или обязательного editor dependency.

#### Scenario: Автор подключает local schema
- **WHEN** автор открывает YAML source или reference в совместимом editor
- **THEN** он может связать document с published local schema и получает
  completion и diagnostics простых полей до compiler

### Requirement: Editor metadata не меняет YAML compiler contract
Portable editor association MUST использовать YAML comments или editor-local
mapping, а не новое data field в authoring document. Such metadata MUST не
входить в parsed YAML value, lowering, exact refs, sealing или Run. JSON Schema
MUST не объявляться semantic authority: compiler MUST продолжать отклонять
недопустимые refs, graph, permissions и limits.

#### Scenario: Reference содержит editor modeline
- **WHEN** authoring reference с local schema modeline проходит existing
  lowering path
- **THEN** compiler принимает тот же authoring document без нового field в
  canonical definition или sealed package

#### Scenario: Schema не может доказать graph
- **WHEN** YAML проходит static editor shape check, но содержит semantic
  нарушение graph или exact ref
- **THEN** `project compile` отказывает до sealing согласно действующему
  compiler contract

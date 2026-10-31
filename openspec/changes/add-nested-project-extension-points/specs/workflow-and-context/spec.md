## ADDED Requirements

### Requirement: Package declares closed nested extension points
Project workflow folder SHALL поддерживать opt-in `extension-points.yaml` с marker `prifly-extension-points/1`. Package author MUST фиксировать exact local workflow, entry либо один direct route, typed input sources, routes и именованные consumer ports. Root и child WorkflowRevision, включая native repeat body, SHALL поддерживаться. Unmarked/root extension сохраняет прежний смысл.

#### Scenario: Project supplies measurement in a repeat body
- **WHEN** marked project extension выбирает declared point и owned project step либо native Call
- **THEN** compiler вставляет stage до sealing child и явно связывает каждый объявленный consumer с выбранным output нового stage

#### Scenario: No opt-in measurement
- **WHEN** point не выбран либо его extension имеет enabled false
- **THEN** прежние entry, routes и bindings остаются passthrough

### Requirement: Nested extension data obeys native typed guarantees
Point extension SHALL использовать ordinary typed bindings и ArtifactRef. Следующий reader и repeat export MUST получать новый declared output, если package point их связывает. Unknown/ambiguous anchor, unavailable producer, schema mismatch, undeclared consumer, component substitution и control/call cycle MUST отказать до исполнения. Missing promised output MUST NOT стать success или прежним carrier.

#### Scenario: Repair updates the next iteration
- **WHEN** repair создаёт новый carrier и after-repair point завершается с guaranteed output
- **THEN** finish экспортирует именно этот output, а следующая iteration получает его через native next_bindings

#### Scenario: Partial output is not guaranteed
- **WHEN** configured route достигает consumer без guaranteed producer output
- **THEN** compile отказывает по native output availability contract

### Requirement: Point extensions preserve effects and reviewed recovery
Point extension MUST NOT расширять effects reader. Measurement SHALL проходить собственные execution/effect contracts. Reviewed continue SHALL повторять root call/repeat frontier и сохранять original Run/provenance; nested-stage resume SHALL оставаться unsupported.

#### Scenario: Continue after partial child
- **WHEN** settled source допускает reviewed continue текущего root frontier
- **THEN** новый Run повторяет measurement и reader, не используя stale measurement output провалившегося frontier

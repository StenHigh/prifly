## ADDED Requirements

### Requirement: Unified compiler exposes versioned source and point provenance
Root opt-in unified compilation SHALL выдавать `project-compile/4`, `project-launch-summary/7`, `project-start/7` и `workflow-composition/2`. CLI SHALL показывать source owners, point export, selected implementation, active/disabled choices, groups, consumer bindings и author-to-compiled variant mapping.

`project compile --explain` SHALL описывать тот же effective plan без исполнения. Paths SHALL быть repository-relative; absolute machine paths MUST NOT входить в build identity. Сборка без нового root opt-in MUST сохранять прежние output versions.

#### Scenario: Two aliases select different point policies
- **WHEN** author готовит запуск с двумя variants одного provider
- **THEN** review показывает оба scopes, их selection и exact compiled refs без смешивания provenance

### Requirement: Unified review remains bound to exact prepared inputs
Prepare SHALL оставаться read-only относительно authority, claims и исполнения. Sidecar bytes, point export, owner inventory, selected variants, layers, settings и supporting program bytes SHALL входить в deterministic review.

Start и continue MUST повторно проверять digest до claim либо dispatch. Mutation любого влияющего input MUST отвергать старый review. Build metadata MUST NOT считаться подписью, trust decision или разрешением effects.

#### Scenario: Point export changes after prepare
- **WHEN** provider меняет exported point, consumer output binding или supporting bytes
- **THEN** прежний expected launch digest не разрешает start нового package

#### Scenario: Same inputs are compiled twice
- **WHEN** binary, versioned contract и authoring inputs совпадают
- **THEN** build/variant identities и compiled definitions совпадают без зависимости от порядка обхода maps и source graph

Изменение authored parent order меняет source bytes и может изменить build identity. Оно MUST NOT молча выбирать одного из конфликтующих parents. Проверка порядка исполнения SHALL сравнивать effective graph с нормализованными compiled refs.

### Requirement: Unified refusals identify source and preserve old fault codes
Новые ошибки SHALL возвращаться как typed Fault с named code, Problem envelope и source context до исполнения. Неизвестный private point, неправильный owner, cycle, variant limit, несовместимые routes, consumer-bearing group и anchor conflict MUST иметь проверяемый refusal.

Failed compile MUST NOT оставлять готовый partial package. Metadata о declaring source SHALL сохраняться при обёртывании Fault. Прежний `/4` path MUST сохранять существующие refusal codes и exit behavior.

#### Scenario: Imported point lacks public export
- **WHEN** client выбирает private point через import selector
- **THEN** CLI возвращает `project_composition_point_not_exported`, не создаёт package и не меняет authority

#### Scenario: Point group has different failure destinations
- **WHEN** sidecar routes нельзя сохранить в binary group contract
- **THEN** CLI возвращает `project_composition_point_group_routes_incompatible`, не подменяя routes владельца

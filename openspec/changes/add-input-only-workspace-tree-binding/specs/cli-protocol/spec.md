## MODIFIED Requirements

### Requirement: Assisted handoff сообщает versioned declared Workspace tree bindings

Versioned assisted SessionTask MUST сообщать host finite declared Workspace tree
bindings: manifest input/output port names, typed capture policy, expected input
manifest ArtifactRef при его наличии и permitted typed location form for
output-only creation. Materialize-only binding MUST сообщаться без output port
и с declared location materialized entries; guide рядом с манифестом
(`workspace-tree-guide/2`) MUST говорить, что такой port host не объявляет.
Read-only step с materialize-only binding MUST получать `repository_workspace`
и `workspace_mode` той же WorktreeClaim, что и workspace-write step Run. Host MUST не получать authority handle, artifact-store
path или право выбирать другой Workspace path. Сопоставленный
SessionSubmission MUST использовать version того SessionTask, который был
handed Attempt; старые retained session versions остаются читаемыми и не
получают новую tree-binding семантику.

Runtime MUST capture declared output trees itself before accepting terminal
StepResult при каждой submission шага с declared bindings, независимо от
поддерживающей деревья session version и от наличия `workspace_trees` в
отправке. Host MUST называть capture location только там, где выбирает её
сам: для policy с единственным допустимым значением (`exact_file`) runtime
MUST брать declared path, а отсутствие location MUST NOT быть отказом. Host
MAY report only selected output-only capture location и MAY
повторить declared input location для binding, объявленного и входом, и
выходом, так что форма отправки одинакова для обоих видов binding; путь,
отличный от declared input location, MUST отклоняться named refusal. Host
MUST not подменять WorkspaceTreeManifest, contained ArtifactRef, digest или
capture policy prose-строкой либо arbitrary JSON. Unknown binding field,
version или несовпадение handoff/submission MUST отклоняться до изменения Run.

#### Scenario: Host получает зафиксированную форму Ultra bundle
- **WHEN** assisted workspace-write Attempt имеет output-only direct-child tree
  binding
- **THEN** SessionTask показывает declared parent and typed bundle form, а host
  не может заявить ArtifactRef, entry outside parent или другой output policy
  в result

#### Scenario: Host повторяет declared input location
- **WHEN** binding объявляет одно дерево и входом, и выходом, а submission
  называет для его output port путь, равный declared input location
- **THEN** runtime принимает отправку как при output-only binding и capture-ит
  дерево по declared location

#### Scenario: Host называет другой путь для input binding
- **WHEN** submission называет для такого port путь, отличный от declared
  input location
- **THEN** intake отказывает named refusal до изменения Run

#### Scenario: Exact-file binding без названной location
- **WHEN** submission не называет location для output-only binding с capture
  policy `exact_file`
- **THEN** runtime capture-ит declared path и принимает отправку, не требуя
  повторить единственное допустимое значение

#### Scenario: Submission без workspace_trees для input+output binding
- **WHEN** submission поддерживающей деревья версии не содержит
  `workspace_trees`, а step объявляет binding с входом и выходом
- **THEN** runtime capture-ит дерево по declared input location и заполняет
  output port сам, не требуя от host повторить путь

#### Scenario: Read-only step получает materialized план и путь рабочей копии
- **WHEN** assisted step с `effects.class: none` объявляет materialize-only
  binding
- **THEN** SessionTask называет `repository_workspace`, binding без output port
  и location materialized entries, а submission, объявляющая порт этого
  binding'а, отказывается named refusal до изменения Run

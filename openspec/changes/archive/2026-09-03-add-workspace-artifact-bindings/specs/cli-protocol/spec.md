## ADDED Requirements

### Requirement: Assisted handoff сообщает versioned declared Workspace tree bindings

Versioned assisted SessionTask MUST сообщать host finite declared Workspace
tree bindings: manifest input/output port names, typed capture policy, expected
input manifest ArtifactRef при его наличии и permitted typed location form for
output-only creation. Host MUST не получать authority handle, artifact-store
path или право выбирать другой Workspace path. Сопоставленный
SessionSubmission MUST использовать version того SessionTask, который был
handed Attempt; старые retained session versions остаются читаемыми и не
получают новую tree-binding семантику.

Runtime MUST capture declared output trees itself before accepting terminal
StepResult. Host MAY report only selected output-only capture location; it MUST
not подменять WorkspaceTreeManifest, contained ArtifactRef, digest или capture
policy prose-строкой либо arbitrary JSON. Unknown binding field, version или
несовпадение handoff/submission MUST отклоняться до изменения Run.

#### Scenario: Host получает зафиксированную форму Ultra bundle
- **WHEN** assisted workspace-write Attempt имеет output-only direct-child tree
  binding
- **THEN** SessionTask показывает declared parent and typed bundle form, а host
  не может заявить ArtifactRef, entry outside parent или другой output policy
  в result

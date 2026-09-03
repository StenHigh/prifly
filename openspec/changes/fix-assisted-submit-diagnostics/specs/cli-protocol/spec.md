Authoritative source set: `openspec/specs/cli-protocol/spec.md` (перенесено).
Compatibility path: shape `Problem`, `SessionSubmissionV4/V5` и
`WorkspaceTreeHandoff` не меняются; меняются только значения `code`,
содержимое `violations` и admission одной формы `workspace_trees`.

## MODIFIED Requirements

### Requirement: Result intake проверяет exact Attempt и sealed output

SubmitResult MUST verify active identities, envelope digest, ports, seals,
evidence, receipts, relevant claims and current controls. Progress/heartbeat is
not terminal result; late result is evidence with rejection reason, not input
for another Attempt. Отказ intake MUST быть named refusal, называющий предмет:
output port, который обязателен для reported verdict и не отчитан либо не
объявлен step, или identity/digest выхода, не совпавшие с admitted slot.
Для assisted submission эти проверки и result schema шага MUST выполняться
при intake до записи candidate: отклонённая отправка оставляет handoff
awaiting и MUST NOT создавать failed Attempt или terminal Run. Sealing выходов
остаётся частью acceptance.

#### Scenario: Output изменён после hash

- **WHEN** worker submits modified bytes under prior digest
- **THEN** acceptance rejects result

#### Scenario: Обязательный выход не отчитан

- **WHEN** submitted StepResult не содержит output port, обязательный для его
  verdict
- **THEN** intake отказывает stable code, pointer и message называют этот
  port, а Run не меняется

#### Scenario: Неверная отправка не сжигает Attempt

- **WHEN** assisted host присылает StepResult, который не прошёл бы acceptance
  по result schema, портам, identity или digest
- **THEN** submission отклоняется до записи candidate, handoff остаётся
  awaiting, и host может прислать исправленный отчёт под той же envelope

### Requirement: Problem и exit code сохраняют safe meaning

Problem MUST include stable code, message, correlation, violations and safe
next action without secrets or foreign-object detail. `retryable` describes
command/check retry only. CLI exit zero means read or command commit, not Run
success; typed result carries workflow state. Runtime refusal, поднятый со
stable code, MUST доходить до клиента под этим code независимо от того,
сопровождён ли он message; engine-authored detail такого отказа (port, path,
version) MUST сообщаться в `violations`. Только текст без stable code MUST
схлопываться в `invalid_input`, и такой текст MUST NOT попадать в ответ.

#### Scenario: External effect is unknown

- **WHEN** command reports unknown effect
- **THEN** safe next action is exact reconciliation, not blind retry

#### Scenario: Refusal поднят без сопроводительного message

- **WHEN** runtime отказывает stable code без detail
- **THEN** Problem несёт этот code, а не `invalid_input`

#### Scenario: Refusal несёт engine-authored detail

- **WHEN** runtime отказывает stable code с detail о предмете отказа
- **THEN** Problem несёт этот code и detail в `violations`, без raw parser
  input, argv, environment или foreign payload

### Requirement: Assisted handoff сообщает versioned declared Workspace tree bindings

Versioned assisted SessionTask MUST сообщать host finite declared Workspace tree
bindings: manifest input/output port names, typed capture policy, expected input
manifest ArtifactRef при его наличии и permitted typed location form for
output-only creation. Host MUST не получать authority handle, artifact-store
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

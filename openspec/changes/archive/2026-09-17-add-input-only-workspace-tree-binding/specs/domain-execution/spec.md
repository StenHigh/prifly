## MODIFIED Requirements

### Requirement: Declared Workspace tree binding сохраняет exact набор ArtifactRevision

StepDefinition MAY объявить конечный список declared Workspace tree bindings
для одного assisted step. Binding с capture называет один declared manifest
output port, optional compatible manifest input port и одну bounded capture
policy: exact file, direct child file или direct child tree; такой binding
допустим только на `workspace_write` step. Начиная со StepDefinition v8
binding MAY быть materialize-only: один compatible manifest input port и та
же bounded capture policy без output port — только на assisted step с
`effects.class: none`; materialize-only binding на `workspace_write` step и
binding с output port на step без `workspace_write` MUST быть отклонены при
компиляции. StepDefinition v5–v7 сохраняют прежнюю форму и meaning.
`WorkspaceTreeManifest` MUST быть sealed JSON ArtifactRevision с one
workspace-relative root, one entrypoint и конечным списком `{relative_path,
ArtifactRef}`. Он не содержит bytes файлов и не выбирает latest artifact.

При входном manifest runtime MUST materialize exact raw bytes всех его entries
перед handoff. После принятого host result runtime MUST capture and seal only
policy-conforming regular files as a new manifest and link it to the declared
output port. Input/output binding MUST preserve prior manifest and contained
ArtifactRevisions in provenance. Materialize-only binding MUST NOT создавать
новый manifest, output port или ArtifactRevision: после settle попытки
materialized entries снимаются, и Run не хранит их как результат шага. Output-only binding MAY accept a typed new
location only within its policy; it MUST NOT accept host-supplied ArtifactRef,
digest или arbitrary path. Directory, glob, symlink и unbounded multi-file
export не являются binding-ом.

#### Scenario: Улучшенный Ultra bundle становится следующим входом
- **WHEN** assisted step с input/output tree binding принимает изменённые
  `index.md` и phase files
- **THEN** его output port содержит sealed новый WorkspaceTreeManifest, а
  следующий binding materialize-ит именно все его exact entries вместо поиска
  плана по имени или JSON-пересказа

#### Scenario: Bundle не capture-ится частично
- **WHEN** host завершает step, но declared tree не содержит entrypoint либо
  одну из required regular entries нельзя seal-ить
- **THEN** result не принимается как успешный и Run сохраняет объяснимый
  refusal без нового WorkspaceTreeManifest

#### Scenario: Read-only gate читает захваченный план
- **WHEN** assisted step с `effects.class: none` объявляет materialize-only
  binding, а его input port связан с WorkspaceTreeManifest, который захватил
  предыдущий step
- **THEN** перед handoff runtime materialize-ит exact entries этого манифеста
  по capture policy, host читает их по declared location, а после settle Run не
  получает нового manifest и не считает дерево изменённым

#### Scenario: Materialize-only binding на шаге с правом записи
- **WHEN** StepDefinition v8 объявляет binding без output port на step с
  `effects.class: workspace_write`
- **THEN** компиляция отказывает named refusal: чтение без захвата на шаге,
  который может писать, оставило бы изменения дерева без manifest

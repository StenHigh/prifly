## ADDED Requirements

### Requirement: Declared Workspace tree binding сохраняет exact набор ArtifactRevision

StepDefinition MAY объявить конечный список declared Workspace tree bindings
для одного assisted `workspace_write` step. Каждый binding называет один
declared manifest output port, optional compatible manifest input port и одну
bounded capture policy: exact file, direct child file или direct child tree.
`WorkspaceTreeManifest` MUST быть sealed JSON ArtifactRevision с one
workspace-relative root, one entrypoint и конечным списком `{relative_path,
ArtifactRef}`. Он не содержит bytes файлов и не выбирает latest artifact.

При входном manifest runtime MUST materialize exact raw bytes всех его entries
перед handoff. После принятого host result runtime MUST capture and seal only
policy-conforming regular files as a new manifest and link it to the declared
output port. Input/output binding MUST preserve prior manifest and contained
ArtifactRevisions in provenance. Output-only binding MAY accept a typed new
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

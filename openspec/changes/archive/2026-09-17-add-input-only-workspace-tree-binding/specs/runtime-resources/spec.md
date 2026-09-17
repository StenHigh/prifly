## MODIFIED Requirements

### Requirement: Runtime confined materialize-ит и capture-ит declared Workspace tree

Runtime MUST materialize и capture-ить declared Workspace tree binding только
в RepositoryWorkspace current WorktreeClaim той Attempt. До записи он MUST
проверить normalized relative root/entry paths, отсутствие traversal,
regular-parent directories и отсутствие symlink escape. Existing bytes каждой
input entry MUST совпадать с pinned ArtifactRevision; output-only target MUST
соответствовать declared capture policy и recorded pre-handoff state. Любое
иное состояние MUST дать stable drift/refusal без автоматической перезаписи
repository file.

Для materialize-only binding runtime MUST materialize exact entries в
RepositoryWorkspace той же WorktreeClaim теми же проверками, MUST снять
отпечаток рабочей копии, по которому измеряется `effect_not_permitted`
read-only step, только после материализации, и MUST снять materialized
entries после settle попытки, не создавая output WorkspaceTreeManifest.
Entry, чьи bytes до handoff уже отличаются от pinned ArtifactRevision,
MUST дать тот же drift refusal, что и у input/output binding.

Capture MUST enumerate only regular files permitted by declared policy,
recheck confinement и entrypoint, seal every entry before creating the output
WorkspaceTreeManifest и сохранить full input provenance. Crash, missing blob,
non-regular entry, extra out-of-policy file или changed-during-capture tree
MUST не создавать accepted output. Binding не расширяет возможности host:
право изменять claimed Workspace остаётся результатом declared
`workspace_write` effect и existing WorktreeClaim.

#### Scenario: Рабочая копия содержит чужое изменение плана
- **WHEN** до handoff declared input plan entry существует с bytes, отличными
  от pinned ArtifactRevision
- **THEN** runtime отказывает до передачи host и не заменяет файл сохранённой
  копией молча

#### Scenario: Ultra bundle captured после handoff
- **WHEN** host завершил workspace-write Attempt и declared direct-child tree
  содержит valid `index.md` и phase files внутри claimed Workspace
- **THEN** runtime сохраняет все exact raw bytes как один sealed output
  WorkspaceTreeManifest с provenance prior input manifest, не читая другой
  похожий план

#### Scenario: Read-only step с materialized планом отчитывается без изменений
- **WHEN** read-only assisted step получил materialized entries и host не
  изменил рабочую копию
- **THEN** submission принимается: отпечаток, снятый после материализации,
  совпадает с состоянием при отчёте, а после settle рабочая копия возвращается
  к состоянию до материализации

#### Scenario: Read-only step изменил дерево рядом с materialized entries
- **WHEN** host read-only step записал файл вне materialized entries или
  изменил materialized entry
- **THEN** submission отказывает `effect_not_permitted` с именами путей, как у
  step без binding

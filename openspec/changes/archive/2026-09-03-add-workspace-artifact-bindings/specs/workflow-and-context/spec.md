## ADDED Requirements

### Requirement: YAML authoring явно объявляет Workspace artifact tree transform

`prifly-step/1` MUST позволять author-у выразить finite declared Workspace tree
binding только полной проверяемой формой: one manifest output port, optional
compatible manifest input port и bounded capture policy (`exact_file`,
`direct_child_file` или `direct_child_tree`). Compiler MUST reject duplicate
paths/ports, blob or arbitrary JSON ports, absent `workspace_write`, path
outside the claimed repository Workspace и any form that denotes recursive
sync, glob, symlink or implicit file discovery. YAML lowering MUST preserve
this declaration in sealed StepDefinition; compiler MUST not infer it from
skill prose, filename or output name.

#### Scenario: YAML связывает Ultra plan tree
- **WHEN** Project step declares compatible plan-manifest ports and direct
  child tree policy under a relative parent
- **THEN** `project compile` seals one explicit tree binding; a similar
  directory in instructions without that declaration creates no binding

### Requirement: AI Factory classic preserves native Fast, Full and Ultra plans

`aif-classic` SHALL use AI Factory's native plan files, not a Pri-Fly JSON
`summary/tasks` substitute. Its package-level profile MUST map documented
default layouts for Fast (`.ai-factory/PLAN.md`), Full (one direct Markdown
child of `.ai-factory/plans`) and Ultra (one direct bundle directory with
`index.md` and phase files) to declared Workspace tree bindings. A project
MUST select one declared package profile; a requested profile or layout outside
that set MUST receive an explicit compatibility refusal rather than a guessed
path. Compiler MUST not read or infer an AI Factory configuration.

The plan, improve and implement steps MUST declare compatible plan-manifest
ports; improve and implement MUST consume the exact prior manifest.
`aif-plan` creates the first native plan, every accepted `aif-improve` revision
becomes the next repeat input, and `aif-implement` receives the same final
revision while returning its final checked-plan manifest. For Ultra, `index.md`
and linked phase files are one atomic native plan.

#### Scenario: Classic Ultra plan crosses improve and implement byte-for-byte
- **WHEN** `aif-plan` produces a native Ultra bundle and an improve iteration
  accepts edits
- **THEN** the next improve iteration and `aif-implement` receive exact sealed
  `index.md` and phase bytes at the same native paths, not a synthesized JSON
  plan

## MODIFIED Requirements

### Requirement: AI Factory examples separate classic and fanout workflows
Repository SHALL публиковать две независимые optional AI Factory Project
workflow folders. `aif-classic` SHALL содержать documented sequential path:
warmup, plan, bounded plan improvement, implement, verify, security, review и
commit. Его improve repeat MUST получать initial native plan WorkspaceTreeManifest
только в first iteration, а затем corrected manifest из prior
`iteration_output`; в нём MUST NOT быть parallel improve или review stage.
Blocking quality result MUST завершаться explicit next action `/aif-fix` и
MUST NOT сам вызывать этот skill, изменять workspace или commit.

`aif-fanout` SHALL быть отдельным YAML package, который может параллельно
объединять independent declared profiles. Пока qualified assisted-host
model-profile protocol не существует, profile fields являются только data и
instructions; compiler, Core и documentation MUST NOT заявлять выбор provider,
model или reasoning level. Оба folder остаются optional Project packages;
`exclude` выбирает declared bypass и MUST NOT удалять stages или менять sealed
Run.

#### Scenario: Classic improve продвигает state
- **WHEN** `aif-classic` компилируется с включённым improve
- **THEN** его второй допустимый круг получает corrected native plan manifest
  output первого по declared Workspace tree binding, а не исходный workflow
  input или JSON-пересказ

#### Scenario: Classic quality gate находит blocker
- **WHEN** verify, security или review сообщает blocking result
- **THEN** `aif-classic` возвращает gate result и next action `/aif-fix` без
  automatic fix, нового review или commit

#### Scenario: Fanout выбирается явно
- **WHEN** Project profile называет `aif-fanout` вместо `aif-classic`
- **THEN** compiler закрепляет fanout graph с declared parallel join и не
  добавляет этот join в `aif-classic`

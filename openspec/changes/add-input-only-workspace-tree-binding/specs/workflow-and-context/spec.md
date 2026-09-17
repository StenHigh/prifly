## MODIFIED Requirements

### Requirement: YAML authoring явно объявляет Workspace artifact tree transform

`prifly-step/1` MUST позволять author-у выразить finite declared Workspace tree
binding только полной проверяемой формой: one manifest output port, optional
compatible manifest input port и bounded capture policy (`exact_file`,
`direct_child_file` или `direct_child_tree`), либо — для step с
`effects.class: none` — materialize-only форму: one compatible manifest input
port и та же bounded capture policy без output port, которая lower-ится в
StepDefinition v8. Compiler MUST reject duplicate paths/ports, blob or
arbitrary JSON ports, absent `workspace_write` у binding с output port,
materialize-only binding у step с `workspace_write`, path outside the claimed
repository Workspace и any form that denotes recursive sync, glob, symlink or
implicit file discovery. YAML lowering MUST preserve
this declaration in sealed StepDefinition; compiler MUST not infer it from
skill prose, filename or output name.

#### Scenario: YAML связывает Ultra plan tree
- **WHEN** Project step declares compatible plan-manifest ports and direct
  child tree policy under a relative parent
- **THEN** `project compile` seals one explicit tree binding; a similar
  directory in instructions without that declaration creates no binding

#### Scenario: YAML даёт read-only gate захваченный план
- **WHEN** Project step с `effects: {class: none}` declares only a compatible
  plan-manifest input port and a capture policy
- **THEN** `project compile` seals one materialize-only binding в StepDefinition
  v8, а тот же binding на step с `workspace_write` отказывается named refusal

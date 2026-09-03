## ADDED Requirements

### Requirement: Project authoring имеет один YAML route
До первого public release Project execution profile MUST принимать только
`prifly-project-profile/2`. Каждый declared package MUST ссылаться только на
directory `.prifly/workflows/NAME/` с root `workflow.yaml`, а каждый Project
launch MUST быть `workflow`, ссылающимся на такой root. Profile v1, его
отдельные source roots, `task_recipe`, direct machine workflow и file source
`prifly-package-source/1` MUST быть отклонены до sealing с понятной diagnostic.
Для этих дорелизных authoring forms не создаётся compatibility или migration
obligation.

#### Scenario: Старый authoring source подан compiler
- **WHEN** profile содержит v1, `task_recipe`, direct machine workflow или
  file package source
- **THEN** Pri-Fly отказывает до создания output package, authority mutation
  или Run

#### Scenario: YAML folder подан compiler
- **WHEN** profile v2 называет допустимую workflow folder и workflow launch
- **THEN** `project workflows` объявляет её inputs, а `project compile`
  выпускает тот же sealed package contract

## MODIFIED Requirements

### Requirement: Project source компилируется из declared files
Tracked `.prifly/` project source MUST использовать только Project execution
profile v2 и declared workflow folders. Root `workflow.yaml` folder MUST
объявлять package identity, external refs, graph и known component directories;
compiler рекурсивно читает только YAML documents из этих declared locations.
Placeholder MUST заменять только whole YAML scalar exact ref или explicit
value; environment, shell, tags, anchors и prose interpolation MUST быть
запрещены. `project compile` MUST создать sealed package без import, authority
mutation или Run.

#### Scenario: Placeholder не найден
- **WHEN** YAML в declared workflow folder ссылается на undeclared component
- **THEN** compile отказывает без угадывания ref или изменения authority

### Requirement: Compact workflow folder остаётся одним графом
Workflow folder MUST быть единственным внешним Project package authoring source
и содержать root `workflow.yaml`, optional `extend.yaml` и one-document
components only in known schemas/contexts/steps/workflows paths. Directory
name MUST не создавать ref or control flow. Simple extension MAY replace
exactly one direct route with input-free step; complex repeat, parallel, map
or bindings MUST быть явно записаны в graph.

#### Scenario: Extension пытается изменить parallel join
- **WHEN** author описывает сложную вставку через `extend.yaml`
- **THEN** compiler отказывает и требует явный workflow graph

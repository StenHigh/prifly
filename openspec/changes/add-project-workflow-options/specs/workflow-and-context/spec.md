## MODIFIED Requirements

### Requirement: Compact workflow folder остаётся одним графом
Workflow folder MUST быть единственным внешним Project package authoring source
и содержать root `workflow.yaml`, optional `extend.yaml` и one-document
components only in known schemas/contexts/steps/workflows paths. Directory
name MUST не создавать ref or control flow. `extend.yaml` MAY задать
`settings` только для exact project-scoped configuration inputs, объявленных
в YAML package, и `exclude` только для named optional features, объявленных
автором соответствующего workflow. Workflow YAML MUST явно содержать
проверяемый безопасный маршрут для каждого такого feature; compiler не
создаёт stages, routes, bindings или feature по имени. Simple extension MAY
replace exactly one direct route with input-free step; complex repeat,
parallel, map or bindings MUST быть явно записаны в graph.

Compiler MUST применить selected settings и exclusions до обычной semantic
validation и sealing. Unknown workflow/input/feature, value outside declared
schema, attempt исключить undeclared part или contradictory selection MUST
давать понятный отказ без sealed package, authority mutation или Run.
`exclude` не является удалением произвольного stage и не меняет уже начатый
Run.

#### Scenario: Команда отключает необязательные части AIF-cycle
- **WHEN** `extend.yaml` исключает author-declared `improve`, `verify` и
  `review`, а YAML graph объявляет их alternative routes
- **THEN** `project compile` выпускает новую WorkflowRevision, в которой
  runtime выберет объявленные обходные маршруты, а plan всё ещё может перейти
  к implement и затем к commit

#### Scenario: Настройка не объявлена автором
- **WHEN** `extend.yaml` называет неизвестный workflow, input или feature либо
  одновременно исключает feature и присваивает его setting несовместимое value
- **THEN** compiler отказывает до sealing и не игнорирует запись молча

#### Scenario: Исключение не переписывает историю
- **WHEN** project compile создаёт package с другим `exclude`
- **THEN** новый выбор входит только в новую sealed WorkflowRevision, а
  существующий Run сохраняет прежний graph и effective configuration

#### Scenario: Extension пытается изменить parallel join
- **WHEN** author описывает сложную вставку через `extend.yaml`
- **THEN** compiler отказывает и требует явный workflow graph

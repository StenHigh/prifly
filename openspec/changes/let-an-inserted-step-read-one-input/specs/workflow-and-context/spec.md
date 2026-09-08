Authoritative source set: `openspec/specs/workflow-and-context/spec.md`.

## MODIFIED Requirements

### Requirement: Compact workflow folder остаётся одним графом
Workflow folder MUST быть единственным внешним Project package authoring source
и содержать root `workflow.yaml`, optional `extend.yaml` и one-document
components only in known schemas/contexts/steps/workflows paths. Directory
name MUST не создавать ref or control flow.

Simple extension MAY replace exactly one direct route with a step declaring at
most one input, bound to an output of a stage on the same route. Такая привязка
MUST проверяться теми же правилами, что привязка авторской стадии: названный
выход MUST существовать у названной стадии, и производитель MUST исполняться до
потребителя на этом пути. Вставка MUST NOT вводить собственную проверку взамен.

Complex repeat, parallel, map, или более одной привязки MUST быть явно записаны
в graph.

#### Scenario: Extension пытается изменить parallel join
- **WHEN** author описывает сложную вставку через `extend.yaml`
- **THEN** compiler отказывает и требует явный workflow graph

#### Scenario: Вставленный шаг читает результат предыдущего
- **WHEN** вставка объявляет один вход, связанный с выходом стадии, которая
  исполняется раньше на том же маршруте
- **THEN** `project compile` запечатывает привязку в граф, и вставленный шаг
  получает артефакт штатно, а не через пересказ в промпте

#### Scenario: Привязка к тому, чего нет или что ещё не исполнялось
- **WHEN** вставка связывает вход с выходом, которого у названной стадии нет,
  либо со стадией, не исполняющейся до потребителя
- **THEN** компиляция отказывает теми же отказами, что и для авторской стадии

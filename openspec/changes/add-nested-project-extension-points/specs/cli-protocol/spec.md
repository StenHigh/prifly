## ADDED Requirements

### Requirement: Point compilation has deterministic identity and refusals
CLI SHALL включать прочитанный point manifest в sources и build identity. Compiled child refs, selected outputs, disabled choices и supporting program bytes MUST участвовать в reviewed package. Prepare/start/continue SHALL использовать штатные digest checks. Ошибки declaration SHALL возвращать именованные typed refusals до исполнения.

#### Scenario: Manifest changes after prepare
- **WHEN** package point, binding либо supporting program меняется после prepare
- **THEN** прежний review digest не разрешает start другого effective package

#### Scenario: Project tries unrestricted rebinding
- **WHEN** point entry задаёт consumer, workflow, route либо binding вне declaration
- **THEN** CLI отказывает, не публикуя частичный package

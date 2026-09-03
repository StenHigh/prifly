## ADDED Requirements

### Requirement: Ответ решения связывается с declared ожидающим входом
Decision request MUST называть Run, expected decision ID, definition digest и
ровно declared input/continuation, который может получить ответ. Accepted
answer MUST пройти catalog schema и current Run generation checks до передачи
ожидающему step. Ответ MUST NOT изменить RunBrief, graph, permissions или
другой step by prose convention.

#### Scenario: Ответ приходит после изменения Run
- **WHEN** host отправляет answer для superseded decision request
- **THEN** Core сохраняет его как late evidence или отклоняет typed diagnostic,
  но не подаёт его текущему step

#### Scenario: Ответ совместим с ожидающим шагом
- **WHEN** current pending request получает valid value для declared input
- **THEN** продолжение получает этот exact typed value и никакой другой step не
  становится ready из-за одного answer


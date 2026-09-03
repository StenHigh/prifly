## ADDED Requirements

### Requirement: Интерфейс показывает решения до и во время Run
CLI и supported host UI MUST использовать один typed decision model. До first
dispatch они MUST показать requested package profile, known mandatory
decisions, defaults/recommendations, autonomous policy и последствия выбора.
При dynamic request интерфейс MUST показать question, allowed response and
why Run waits; после completion — source каждого выбора. UI MUST NOT label
automatic recommendation как подтверждение человека.

#### Scenario: Владелец выбирает autonomous Run
- **WHEN** preflight показывает entries, которые могут быть answered
  automatically
- **THEN** интерфейс отдельно показывает entries, которые всё равно остановят
  Run при новом или restricted вопросе

#### Scenario: Динамический вопрос требует внешнего действия
- **WHEN** requested value может изменить scope или потребовать Approval
- **THEN** интерфейс не предлагает generic agent recommendation как обход
  человеческого решения


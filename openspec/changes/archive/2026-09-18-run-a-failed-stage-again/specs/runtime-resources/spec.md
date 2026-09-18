## MODIFIED Requirements

### Requirement: Resume и fork не переписывают исходный Run

Resume MUST продолжать тот же Run с immutable inputs и definitions после
revalidation current permissions/resources; он не снимает stop и не проходит
completed steps заново. Transport и whole-step retry следуют отдельным
protocols. Fork создаёт новый Run с source provenance и exact source version;
reuse возможен лишь через declared immutable inputs с rechecked port/trust
status. Старые approvals, mutable worker state и safety external effects не
переходят в fork.

Стадия, завершившаяся technical failure в Run, который не достиг ни одного
outcome, MUST быть повторяемой без повторения уже завершённых стадий: их
StepInstance и запечатанные выходы MUST оставаться как есть, а повтор MUST
идти новой Attempt того же StepInstance, сохраняя неудачную в записи. Повтор
MUST перепроверять права и ресурсы заново и MUST NOT наследовать approvals,
grants или незакрытые external effects. Run, достигший outcome, MUST NOT быть
повторяем: accepted вердикт — это ответ на вопрос, а не поломка. Повтор MUST
отказываться, пока есть неразрешённый эффект, активная попытка, действующий
stop или запрошенная отмена.

#### Scenario: Пользователь пытается resume после stop

- **WHEN** stop действует или появился между release и resume
- **THEN** resume отклоняется и не снимает stop

#### Scenario: Программная стадия упала по причине окружения

- **WHEN** стадия завершилась technical failure, Run не достиг outcome, а
  предыдущие стадии завершены и их выходы запечатаны
- **THEN** владелец повторяет эту стадию новой попыткой того же StepInstance,
  завершённые стадии остаются завершёнными и заново не исполняются

#### Scenario: Run ответил на свой вопрос

- **WHEN** Run завершился объявленным outcome, в том числе `rejected` после
  предметного вердикта
- **THEN** повтор отказан: это ответ, а не поломка

#### Scenario: Повтор не возвращает прав

- **WHEN** повтор стадии требует права, которое исходный Run получал
  одобрением или грантом
- **THEN** право требуется заново, а не наследуется вместе с работой

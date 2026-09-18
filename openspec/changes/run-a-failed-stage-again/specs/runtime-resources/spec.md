## MODIFIED Requirements

### Requirement: Resume и fork не переписывают исходный Run

Resume MUST продолжать тот же Run с immutable inputs и definitions после
revalidation current permissions/resources; он не снимает stop и не проходит
completed steps заново. Transport и whole-step retry следуют отдельным
protocols. Fork создаёт новый Run с source provenance и exact source version;
reuse возможен лишь через declared immutable inputs с rechecked port/trust
status. Старые approvals, mutable worker state и safety external effects не
переходят в fork.

Стадия, завершившаяся technical failure, MUST быть повторяемой без повторения
уже завершённых стадий: повтор MUST переиспользовать их запечатанные выходы по
ссылкам и MUST NOT переписывать историю исходного Run. Повтор MUST
перепроверять права и ресурсы заново и MUST NOT наследовать approvals, grants
или незакрытые external effects. Accepted предметный вердикт стадии MUST NOT
быть отменён повтором: повторяется техническая неудача, а не решение.

#### Scenario: Пользователь пытается resume после stop

- **WHEN** stop действует или появился между release и resume
- **THEN** resume отклоняется и не снимает stop

#### Scenario: Программная стадия упала по причине окружения

- **WHEN** стадия завершилась technical failure, а предыдущие стадии Run
  завершены и их выходы запечатаны
- **THEN** владелец повторяет эту стадию, переиспользуя запечатанные выходы, и
  не проходит завершённые стадии заново

#### Scenario: Повтор не возвращает прав

- **WHEN** повтор стадии требует права, которое исходный Run получал
  одобрением или грантом
- **THEN** право требуется заново, а не наследуется вместе с работой

## ADDED Requirements

### Requirement: Continuation tail имеет явные typed inputs и route
Project workflow authoring MUST объявлять continuation как отдельный typed
route с exact inputs для task, handoff, plan и Implementation. Этот route MUST
явно связывать inputs с quality tail и MUST не выбирать latest artifact по
схеме, имени или тексту host-а. Изменение continuation route MUST поднять
авторские версии затронутых package components и сохранить normal route
совместимым.

#### Scenario: Обычный запуск package

- **WHEN** launch не объявлен continuation
- **THEN** он сохраняет прежний entry и не получает дополнительные
  обязательные inputs или скрытый пропуск стадий

#### Scenario: Продолжение package

- **WHEN** launcher предоставляет complete typed continuation inputs
- **THEN** compiler seal-ит exact route и inputs, а первый quality gate получает
  именно объявленную Implementation, plan и handoff

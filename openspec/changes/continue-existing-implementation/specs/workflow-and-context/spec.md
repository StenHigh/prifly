## ADDED Requirements

### Requirement: Continuation route имеет явные typed inputs
Project workflow authoring MUST объявлять continuation как отдельный typed
workflow, чьи входы получают источники из source Run только через объявление
продолжения. Этот workflow MUST не выбирать latest artifact по схеме, имени или
тексту host-а. Изменение continuation route MUST поднять авторские версии
затронутых package components и сохранить обычный route совместимым.

#### Scenario: Обычный запуск package
- **WHEN** launch не является continuation
- **THEN** он сохраняет прежний entry и не получает дополнительные
  обязательные inputs или скрытый пропуск стадий

#### Scenario: Продолжение package
- **WHEN** launcher предоставляет входы, объявленные продолжением
- **THEN** compiler seal-ит exact route и inputs, а entry stage получает
  именно объявленные артефакты

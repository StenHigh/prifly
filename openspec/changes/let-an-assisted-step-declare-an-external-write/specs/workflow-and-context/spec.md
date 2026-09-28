## ADDED Requirements

### Requirement: YAML автор внешней записи сохраняет границу шага

`prifly-step/2` MUST принимать `external_write` и опускать его вместе с
`effects.class: external_write` в StepDefinition v11. Явный pin более ранней
версии MUST быть отказан; `prifly-step/1` MUST не принимать поле. Отсутствие
поля MUST оставлять прежние lowered bytes неизменными.

#### Scenario: Автор объявил внешнюю запись
- **WHEN** assisted YAML step объявляет систему, изменяющие операции и цель
- **THEN** compiled StepDefinition v11 несёт ту же границу без её интерпретации

#### Scenario: Старый authoring marker
- **WHEN** `prifly-step/1` содержит `external_write`
- **THEN** compile отказывает до Run, не удаляя поле молча

## ADDED Requirements

### Requirement: CLI объявляет результат explicit binary update
Public CLI MUST предоставлять `prifly update` как отдельную команду без
`--project` prerequisite. Её structured result MUST различать current version,
installed version и отказ; update не смешивается с command protocol Run и не
создаёт authority mutation. Invalid arguments MUST завершаться existing
`invalid_usage` diagnostic.

#### Scenario: Managed installation получает новую версию
- **WHEN** пользователь запускает `prifly update` и signed compatible Release
  новее installed version
- **THEN** CLI сообщает прежнюю и установленную version только после успешной
  atomic replacement

#### Scenario: Вызов содержит неожиданный аргумент
- **WHEN** пользователь передаёт не поддержанный аргумент команде `update`
- **THEN** CLI возвращает `invalid_usage` и не начинает network operation

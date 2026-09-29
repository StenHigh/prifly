## ADDED Requirements

### Requirement: Сборка объявляет внешнюю запись программы и объясняет повтор

`prifly capabilities` MUST объявлять возможность `program_external_write` и
контракт шага 14. Для Run в статусе `uncertain` `run status` MUST называть в
`failure` диагностику, которая остановила Run, как уже делает для `failed` и
`cancelled`. Для незавершённой внешней записи программы это
`external_write_unreconciled`, и её сообщение MUST называть стадию, объявленный
retry-класс, `run resolve` и исходы `applied` и `not_applied`. `run next` такого
Run MUST предлагать `run.resolve`. Опубликованные контракты чтения MUST NOT
меняться: новый код не добавляется в перечисление `reason_code`.

Возобновление или восстановление Run в статусе `uncertain` MUST отказывать с
`recover_source_unsettled` и называть `run resolve` как выход, а не
`recover_source_ineligible`: такой Run становится пригодным сразу после
свидетельства.

Справка CLI MUST говорить, что граница внешней записи — заявление автора, а не
ограничение программы, и что retry-класс решает, будет ли прерванная программа
исполнена снова без свидетельства. `idempotent` — заявление автора, которое
authority не проверяет, а `pure` для внешней записи отказывается.

#### Scenario: Агент встречает остановленный Run
- **WHEN** агент читает `run status` и `run next` Run, в котором прервалась
  программа с `reconcile_required`
- **THEN** `failure.code` равен `external_write_unreconciled`, сообщение
  диагностики называет стадию, `reconcile_required` и `run resolve` с `applied`
  и `not_applied`, а `run next` предлагает `run.resolve`

#### Scenario: Возобновление до свидетельства
- **WHEN** оператор вызывает `project continue` для такого Run до `run resolve`
- **THEN** отказ `recover_source_unsettled` называет `run resolve`

#### Scenario: Агент проверяет, умеет ли сборка
- **WHEN** агент читает `prifly capabilities`
- **THEN** список содержит `program_external_write`, а контракты шага содержат 14

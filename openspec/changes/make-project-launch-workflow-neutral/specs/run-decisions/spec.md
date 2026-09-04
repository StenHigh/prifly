## ADDED Requirements

### Requirement: Общая анкета не подменяет runtime request
Предзапусковая форма SHALL показывать объявленные и применимые к profile
runtime decisions вместе с preflight, сохраняя различие фаз. Владелец MAY
запечатать typed runtime answer заранее, но отсутствие такого ответа MUST NOT
само по себе запрещать attended/autonomous launch: runtime request может не
возникнуть. Для условия, зависящего от ещё неизвестного runtime value, форма
MUST показывать условность, не угадывать значение и не обещать отсутствие
ожидания. Необъявленные native skill questions MUST NOT считаться перехваченными
или покрытыми анкетой.

#### Scenario: Runtime вопрос не возник
- **WHEN** владелец оставил optional runtime answer пустым, а step не поднял request
- **THEN** workflow не останавливается ради этого вопроса и не фабрикует ответ

#### Scenario: Skill задаёт неописанный вопрос
- **WHEN** host встречает вопрос, отсутствующий в sealed catalog/bridge
- **THEN** он сообщает ограничение и запрашивает решение, не применяет скрытый
  ответ модели и не выдаёт этот путь за квалифицированный unattended

### Requirement: Источник ответа не обещает разделения local owner и агента
UI, runner и документация SHALL отличать actor/policy provenance от
технического доказательства присутствия человека. При общем OS principal
local-owner profile MUST NOT обещать изоляцию агентского ответа от ответа
владельца. Scope-changing и approval-like decisions MUST NOT становиться
ordinary ради автономного запуска; прежние admission/approval boundaries
сохраняются. Более сильная аутентификация требует отдельного qualified contract.

#### Scenario: Агент использует тот же local owner account
- **WHEN** decision ledger называет actor
- **THEN** интерфейс не утверждает, что это технически доказанный ответ человека

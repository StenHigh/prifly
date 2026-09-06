## MODIFIED Requirements

### Requirement: Политика отсутствия человека ограничена объявленным выбором
Attended Run MUST получить ответ владельца для каждого обязательного решения,
не имеющего явного сохранённого значения. Autonomous Run MUST применять только
тот default или recommendation, для которого catalog явно разрешает automatic
selection и владелец выбрал соответствующую policy при запуске. Необъявленное,
scope-changing, approval-like либо запрещённое catalog решение MUST перевести
Run в ожидание, если владелец не запечатал ответ на него при запуске; модель
MUST NOT выбрать его вместо человека. Термин
`unattended` MUST использоваться только для profile, в котором такой ожидания
не требуется по sealed catalog и policy.

Launch MUST принимать typed ответ владельца на объявленное решение фазы
`runtime` и запечатывать его в decision sheet. Значение MUST проверяться против
объявленных choices или value schema при запуске, а не при запросе. Когда мост
получает request на решение с запечатанным ответом, Run MUST применить ровно
это значение, продолжить ту же доставку и записать источником `actor`, а не
policy. Источник `actor` MUST означать происхождение ответа, а не доказанное
присутствие отдельного человека: при общем OS principal local owner и агент
неотличимы.

Launch под autonomous policy MUST объявлять до первого dispatch, какие
объявленные runtime-решения, применимые к выбранному profile, эта политика
взять не сможет и на которые владелец не запечатал ответ, и MUST называть для
каждого причину из sealed catalog: не разрешён automatic selection,
чувствительность выше ordinary, либо отсутствует recommendation. Решение с
запечатанным ответом MUST NOT попадать в перечень: оно Run не остановит. Перечень MUST быть отчётом, а не отказом: решение MAY так и не
быть запрошено, и launch MUST NOT отказывать из-за его наличия. Пустой перечень
MUST означать, что ни одно применимое runtime-решение не остановит Run
из-за отсутствия человека.

#### Scenario: Autonomous Run встречает новый вопрос
- **WHEN** step возвращает decision request с ID, которого нет в sealed catalog
- **THEN** Run сохраняет wait reason и не продолжает step с ответом модели

#### Scenario: Автоматический default разрешён владельцем
- **WHEN** autonomous policy разрешает catalog entry с non-scope-changing
  recommended value
- **THEN** Run использует ровно объявленное значение и записывает, что его
  источником была policy, а не человеческим ответом

#### Scenario: Владелец запускает autonomous Run со scope-changing решением
- **WHEN** он запускает Run под autonomous policy, а sealed catalog объявляет
  применимое runtime-решение со `sensitivity` выше `ordinary`
- **THEN** launch создаёт Run и называет это решение с его причиной, вместо
  того чтобы отказать или молча дойти до ожидания посреди работы

#### Scenario: Владелец отвечает на runtime-решение перед уходом
- **WHEN** он запечатывает при запуске ответ на объявленное runtime-решение со
  `sensitivity` выше `ordinary`, а шаг позже поднимает по нему request
- **THEN** Run применяет ровно это значение, не переходит в ожидание и
  записывает источником `actor`

#### Scenario: Запечатанный ответ не проходит объявленную проверку
- **WHEN** владелец запечатывает значение, которого нет среди объявленных
  choices решения
- **THEN** launch отказывает до создания Run, а не посреди работы

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

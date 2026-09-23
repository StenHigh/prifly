## Purpose

Каталог решений даёт package автору способ заранее объявить человеческий выбор
для одного Run, сохранить его происхождение и не позволить отсутствию человека
стать неявным разрешением или изменением задачи.

## ADDED Requirements

### Requirement: Каталог решений является явным sealed контрактом Run
Project workflow package MUST объявлять каждое поддерживаемое решение через
stable ID, вопрос, конечное множество или schema ответов, область действия,
допустимый default/recommendation и правило автоматического выбора. Compiler
MUST validate-ить каталог вместе с выбранным package profile и включать exact
definition в sealed Run inputs. Он MUST NOT извлекать решение из prose skill,
создавать новое решение по имени step или разрешать неизвестный ID.

#### Scenario: Package объявляет выбор уровня планирования
- **WHEN** workflow package объявляет решения `plan_profile` со значениями
  `fast`, `full` и `ultra`
- **THEN** предзапусковая форма показывает только эти значения и sealed Run
  сохраняет exact catalog definition

#### Scenario: Milestone зависит от выбора linkage
- **WHEN** package объявляет `roadmap_milestone` с predicate
  `roadmap_linkage = "link"`
- **THEN** questionnaire показывает milestone только после exact выбранного
  linkage value, а predicate не создаёт stage, route или permission

#### Scenario: В authoring source есть неописанный вопрос
- **WHEN** compiler встречает ссылку на absent decision ID
- **THEN** он отказывает до sealing и не создаёт Run

### Requirement: Решение package profile выбирается для каждого Run до compilation
Выбор profile MUST быть explicit per-Run launch value и происходить до
compilation, validation и sealing package. Project configuration MAY задать
reviewed default, но MUST NOT быть изменена для разового выбора разработчика.
Explicit launch value MUST быть одним из profiles выбранного package; Run MUST
record selected value and its source. Один profile MUST NOT silently заменить
другой после sealing.

#### Scenario: Разработчик запускает Full после прежнего Fast Run
- **WHEN** пользователь выбирает `full` для нового запуска того же tracked
  package, в котором project default равен `fast`
- **THEN** compiler seal-ит Full definition для нового Run, а tracked project
  configuration и предыдущий Fast Run не изменяются

#### Scenario: Передан неизвестный profile
- **WHEN** launch получает profile, которого нет в package catalog
- **THEN** launch возвращает typed validation diagnostic до package registration
  и Run creation

### Requirement: Политика отсутствия человека ограничена объявленным выбором
Attended Run MUST получить ответ владельца для каждого обязательного решения,
не имеющего явного сохранённого значения. Autonomous Run MUST применять только
тот default или recommendation, для которого catalog явно разрешает automatic
selection и владелец выбрал соответствующую policy при запуске. Необъявленное,
scope-changing, approval-like либо запрещённое catalog решение MUST перевести
Run в ожидание; модель MUST NOT выбрать его вместо человека. Термин
`unattended` MUST использоваться только для profile, в котором такой ожидания
не требуется по sealed catalog и policy.

#### Scenario: Autonomous Run встречает новый вопрос
- **WHEN** step возвращает decision request с ID, которого нет в sealed catalog
- **THEN** Run сохраняет wait reason и не продолжает step с ответом модели

#### Scenario: Автоматический default разрешён владельцем
- **WHEN** autonomous policy разрешает catalog entry с non-scope-changing
  recommended value
- **THEN** Run использует ровно объявленное значение и записывает, что его
  источником была policy, а не человеческий ответ

### Requirement: Журнал решений остаётся читаемым и отличает источник ответа
Run MUST хранить immutable record каждого presented, answered, defaulted,
rejected или pending decision: catalog/definition digest, stable ID, allowed
values/schema, selected value when present, actor or policy source, time and
applicable scope. До первого dispatch пользователь MUST увидеть итог known
решений; terminal report MUST включать полный decision ledger. Decision record
MUST NOT стать Grant, Approval или заменой ActionIntent.

#### Scenario: Пользователь возвращается к завершённому Run
- **WHEN** он читает terminal report
- **THEN** он видит выбранный package profile и все автоматические/ручные
  ответы с их источником, не предполагая, что они были его личным выбором

### Requirement: Universal Decision Bridge не зависит от package или skill
Core MUST предоставлять один versioned Decision Bridge для compatible executor
и host: executor объявляет поддержку и посылает typed `DecisionRequest`, host
читает pending request и возвращает typed `DecisionAnswer`. Bridge MUST NOT
содержать имя package, skill, provider или model и MUST NOT пытаться
перехватывать native chat question, который executor не отправил через
protocol. Несовместимый executor MAY использовать preflight catalog, но MUST
получить explicit unsupported result до admission runtime decision.

#### Scenario: Два разных package используют один bridge
- **WHEN** compatible executor двух package отправляет declared runtime
  decision request
- **THEN** host и Core обрабатывают оба через одинаковые versioned DTO без
  per-package Core adapter

#### Scenario: Чужой skill открывает свой native dialog
- **WHEN** skill вызывает native question tool, не отправляя DecisionRequest
- **THEN** Pri-Fly не записывает ложный decision record и не заявляет, что
  universal bridge получил этот ответ

#### Scenario: Package adapter заменяет известный native вопрос
- **WHEN** primary pinned context package описывает exact runtime ID и вызывает
  Universal Decision Bridge до native question tool
- **THEN** Pri-Fly сохраняет typed request, ждёт answer и redelivers тот же
  Attempt с declared session-context value; supporting upstream skill остаётся
  pinned, а его прочие native dialogs не получают ложный decision record

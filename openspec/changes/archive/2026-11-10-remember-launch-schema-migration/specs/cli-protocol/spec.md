Текущий source set — `openspec/specs/cli-protocol/spec.md`. Ownership не меняется.
Разовый flag и legacy команды сохраняются. Новые ответы используют отдельные closed contracts.

## MODIFIED Requirements

### Requirement: Start явно выбирает решение миграции
`run start` и `project start` SHALL принимать opt-in `--schema-migration ID`.
Project questionnaire prepare SHALL принимать тот же выбор и включать его в
review digest и читаемый launch summary. Start MUST проверять соответствие
решения exact consumer package, применимость к sealed workflow closure и текущий
trust. Project launch SHALL также использовать действующий сохранённый выбор владельца при отсутствии разового flag.
Запуск без обоих источников выбора MUST сохранять прежнее поведение.
CLI MUST NOT выбирать последнее решение, выполнять apply или создавать
новое trust decision во время prepare/Start.

#### Scenario: Выбор решения меняется после project prepare
- **WHEN** project start получает другое решение, чем закреплено ожидаемым launch digest
- **THEN** Start возвращает именованный mismatch отказ до создания Run

#### Scenario: Consumer имеет другую sealed revision
- **WHEN** Start выбирает решение для consumer с отличающейся version или digest
- **THEN** CLI возвращает именованный scope отказ и не создаёт Run или постоянный claim

## ADDED Requirements

### Requirement: Public CLI управляет сохранённым выбором launch
CLI SHALL предоставлять `project schema-migration inspect`, `set` и `clear` для repository и launch.
Inspect SHALL показывать отсутствие записи либо сохранённый scope, decision ID, generation и причины недействительности.
Set SHALL принимать ID существующего решения, ожидаемые build key и generation, причину и command identity.
Clear SHALL принимать ожидаемую generation, причину и command identity.
Set SHALL заменять прежнюю запись только после проверки нового scope.
Сохранённый выбор SHALL поддерживать reviewed Project profiles `/3` и новее; legacy profile MUST получать именованный refusal.
Clear MUST работать для недействительной записи без требования заново собрать launch.

#### Scenario: Владелец сохраняет решение
- **WHEN** owner использует set с проверенным build key, decision ID и текущей generation
- **THEN** команда записывает выбор; inspect показывает его scope и новую generation

#### Scenario: Исходники больше не компилируются
- **WHEN** owner удаляет существующий выбор через clear с текущей generation
- **THEN** команда удаляет выбор без компиляции; отсутствие выбора сохраняет следующую generation для защиты от старых команд

### Requirement: Явный flag имеет приоритет над сохранённым выбором
Prepare и start SHALL выбирать явный непустой `--schema-migration` перед сохранённой записью.
Разовый flag MUST NOT менять сохранённый выбор.
Явный недействительный flag MUST давать отказ без fallback.
Пустое значение flag SHALL означать отсутствие разового выбора; действующий saved выбор при этом сохраняет приоритет над отсутствием миграции.
Standalone `run start` MUST NOT читать project launch выбор.

#### Scenario: Сохранённый выбор устарел, но передан проверенный ID
- **WHEN** prepare получает действующий явный ID для текущей сборки
- **THEN** он проверяет этот ID независимо от устаревшей записи; сохранённая запись не меняется

#### Scenario: Явное решение недействительно
- **WHEN** owner передаёт недействительный ID при наличии действующего сохранённого выбора
- **THEN** команда отказывает по явному ID и не использует сохранённое решение

### Requirement: Prepare раскрывает происхождение выбора и Start проверяет review
Prepare SHALL показывать effective decision ID, источник `explicit` либо `saved` и exact scope.
Для источника `saved` summary SHALL показывать generation выбора.
Эти данные MUST входить в launch review digest.
Start с источником `saved` SHALL требовать `--expected-launch-digest` из prepare.
Start MUST повторно проверять выбор, build identity и существующие migration checks перед созданием Run.
Недействительный выбор MUST сообщать причину и supported команды inspection, повторного prepare, замены либо удаления.
Движок MUST NOT советовать отменять terminal revoke или редактировать SQLite.

#### Scenario: Сохранённый ID заменили после prepare
- **WHEN** start получает digest для предыдущего выбора
- **THEN** он возвращает именованный stale refusal до Run и постоянного claim

#### Scenario: Источник поменялся при том же ID
- **WHEN** prepare использовал saved, а start получил тот же ID через явный flag
- **THEN** прежний digest не соответствует новому источнику и start требует повторный prepare

#### Scenario: Сохранённый выбор используют без review
- **WHEN** start использует saved и не получает expected launch digest
- **THEN** команда требует prepare и проверенный digest до Run

#### Scenario: Supplier потерял доверие
- **WHEN** текущее решение больше не допускает выбранного поставщика
- **THEN** команда показывает точную причину и supported следующий шаг без подбора другого решения

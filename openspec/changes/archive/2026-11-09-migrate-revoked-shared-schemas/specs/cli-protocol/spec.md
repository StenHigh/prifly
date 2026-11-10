## ADDED Requirements

### Requirement: CLI предоставляет reviewed schema migration
Public CLI SHALL предоставлять `package schema-migration prepare --request FILE`,
`package schema-migration apply --plan FILE --expected-review-digest DIGEST --reason TEXT`
и `package schema-migration inspect --id ID`. Apply SHALL поддерживать существующий
`--command-id` для повторяемого запроса. Prepare MUST работать без чтения SQLite вручную.
Request SHALL выбирать установленный consumer по exact package Ref и задавать
явное соответствие schema refs выбранным supplier package refs.
Подготовка без полного выбора SHALL возвращать диагностический неприменимый план.
Plan SHALL содержать полный scope, исходные lifecycle facts, последствия для Runs,
причины невозможности применения и digest. Inspect SHALL показывать сохранённое
решение и его текущую пригодность с причинами устаревания.

#### Scenario: Оператор исследует отказ Start
- **WHEN** оператор передаёт prepare exact consumer Ref без полного выбора suppliers
- **THEN** CLI показывает все заблокированные схемы, legacy packages, status reasons и точных кандидатов без изменения authority

#### Scenario: Оператор применяет проверенный план
- **WHEN** owner вызывает apply с применимым планом и его ожидаемым digest
- **THEN** CLI возвращает decision identity, receipt и команды inspection и явного Start

### Requirement: Start явно выбирает решение миграции
`run start` и `project start` SHALL принимать opt-in `--schema-migration ID`.
Project questionnaire prepare SHALL принимать тот же выбор и включать его в
review digest и читаемый launch summary. Start MUST проверять соответствие
решения exact consumer package, применимость к sealed workflow closure и текущий
trust. Запуск без выбора MUST сохранять прежнее поведение.
CLI MUST NOT молча выбирать последнее решение, выполнять apply или создавать
новое trust decision во время prepare/Start.

#### Scenario: Выбор решения меняется после project prepare
- **WHEN** project start получает другое решение, чем закреплено ожидаемым launch digest
- **THEN** Start возвращает именованный mismatch отказ до создания Run

#### Scenario: Consumer имеет другую sealed revision
- **WHEN** Start выбирает решение для consumer с отличающейся version или digest
- **THEN** CLI возвращает именованный scope отказ и не создаёт Run или постоянный claim

### Requirement: Отказы миграции имеют проверяемые коды и безопасный маршрут
CLI SHALL различать invalid request, incomplete plan, scope mismatch, stale plan,
integrity failure, invalid decision и object access denial. Каждый отказ MUST
нести типизированный код в Problem либо control receipt. Stale/incomplete отказ
MUST направлять к повторному prepare; integrity отказ MUST направлять к
inspection/verification. Ни один отказ MUST не советовать редактировать SQLite,
снимать revoke или удалять history.
Обновлённый справочник SHALL объяснять remove для ordinary retirement и terminal
revoke для security withdrawal. Public help, commands reference и embedded runner
assets SHALL доставлять один и тот же supported путь.

#### Scenario: План устарел
- **WHEN** apply получает план для предыдущего package/control/Run snapshot
- **THEN** CLI показывает именованный stale отказ и команду повторного prepare без частичного разрешения схем

#### Scenario: Оператор обновляет обычную версию
- **WHEN** оператор читает upgrade guidance
- **THEN** документация объясняет remove и его ограничения по holders/dependents, не предлагая security revoke для освобождения версии

Источник — `openspec/specs/cli-protocol/spec.md`; ownership не меняется.
Новые opt-in команды используют отдельные versioned DTO. Прежние commands,
defaults и JSON без выбора миграции сохраняют свои контракты.

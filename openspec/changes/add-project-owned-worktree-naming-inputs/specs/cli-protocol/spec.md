## ADDED Requirements

### Requirement: Project CLI reviews separate naming inputs before allocation

Questionnaire SHALL показывать объявления naming inputs отдельно от inputs workflow и обозначать применимость к созданию worktree. `project questionnaire --prepare` и `project start` SHALL принимать повторяемый `--naming-input NAME=FILE`. Значения MUST быть JSON, проходить объявленную schema и поступать из явно названного файла. Повтор одного NAME и неизвестный NAME MUST отказываться. Неявные значения из task, environment или queue MUST NOT подставляться.

Prepare SHALL показывать naming declaration, schema paths/digests, полные JSON values с canonical digests, sources, bindings и разрешённую основу. `review_digest` MUST закреплять полные canonical values, schemas и declaration, включая поля вне выбранных pointers. Start MUST перечитывать файлы и проверять тот же snapshot до preflight program, claim, import, Run и dispatch. Валидное изменение snapshot после prepare SHALL давать `project_start_stale_launch`; malformed/invalid inputs SHALL давать предметный typed отказ до effects.

Prepare MUST оставаться read-only. Candidate availability SHALL сохраняться вне review digest. Конкурентный allocation SHALL использовать существующий bounded suffix policy. Start result SHALL показывать фактические path/branch и принятый naming snapshot.

Missing/invalid schema или value SHALL называть naming input и schema field/pointer без маскировки под workflow input. `--input` и `--input-ref` SHALL сохранять прежнее назначение. Первая редакция MUST NOT вводить `--naming-input-ref` или автоматическую загрузку metadata.

Source set остаётся `openspec/specs/cli-protocol/spec.md`; ownership не переносится. Новые поля выдаются только в новых closed output versions.

#### Scenario: Reviewed values create the expected worktree
- **WHEN** prepare показал `demo-tracker-issue-19`, а start получает тот же snapshot и review digest
- **THEN** start создаёт соответствующий worktree либо следующий допустимый suffix и возвращает фактическое имя

#### Scenario: An unused JSON field changes after prepare
- **WHEN** schema допускает поле, которое не участвует в template, и его валидное значение изменилось после prepare
- **THEN** start возвращает `project_start_stale_launch` до preflight и claim effects

#### Scenario: The naming schema changes after prepare
- **WHEN** project-owned schema изменилась при прежних допустимых values и старом review digest
- **THEN** start возвращает `project_start_stale_launch` до создания worktree или Run

#### Scenario: Prepare leaves authority untouched
- **WHEN** caller готовит запуск с naming inputs
- **THEN** authority, registry, claims и Git topology остаются неизменными; preview явно сообщает отсутствие reservation

### Requirement: Naming documentation includes an executable consumer journey

Author reference SHALL показывать `/5`, schema file, naming value file, все binding forms и минимальные Project/claim contracts. Operations reference SHALL показывать отдельный флаг. Capability SHALL присутствовать в `prifly capabilities` и таблице `examples/README.md`.

Executable example SHALL охватывать install/update/prepare/start неизменённого официального package и сравнение upstream bytes. Отдельный native CLI probe SHALL проверять именование, stale refusals и продолжение без LLM, tracker и приложения. Synthetic session reports SHALL называться проверкой engine protocol, а не реальным AI execution. Приёмка SHALL отдельно фиксировать результат probe на released Core.

#### Scenario: A consumer follows the example
- **WHEN** пользователь выполняет пример в чистом Git repository с released Core
- **THEN** prepare показывает имя, start создаёт ожидаемое дерево, package update сохраняет consumer files, а отчёт называет точную версию Core

## ADDED Requirements

### Requirement: Prepare показывает пространство имён без reservation

Project prepare для нового named worktree SHALL показывать разрешённую основу, фиксированные root/prefix, текущие candidate path/branch, bounded collision policy и явный признак отсутствия reservation. Candidate SHALL быть наблюдением, не обещанием окончательного suffix. `review_digest` MUST закреплять profile naming declaration, bindings, разрешённые значения, repository/root и правило выделения. Availability observation и advisory candidate SHALL быть отделены от digest: конкурентный новый claim не меняет уже проверенную политику.

Start с `--expected-launch-digest` SHALL отвергать изменение naming declaration/values до создания claim/Run и dispatch. Он SHALL допускать следующий свободный suffix по проверенному правилу и возвращать фактические имена. Для передачи существующего claim prepare SHALL показывать recorded claim/path/branch и handover, а не кандидата нового дерева. Prepare MUST оставаться read-only для authority, registry, claims, filesystem и Git topology и не запускать preflight programs.

Текущий source set — `openspec/specs/cli-protocol/spec.md`; ownership не переносится. Новые поля публикуются только новыми wire versions.

#### Scenario: Между prepare и start заняли первое имя
- **WHEN** prepare показал `base`, а конкурентный start зарезервировал его до выполнения проверенной команды
- **THEN** start выделяет `base-02` по согласованной policy, показывает фактическое имя и не выдаёт preview за reservation

#### Scenario: После review изменили template или task ID
- **WHEN** старый digest передан с изменённым template, binding либо разрешённым значением input
- **THEN** start возвращает `project_start_stale_launch` до claim/Run/worktree effects

#### Scenario: Подготовка передачи claim
- **WHEN** продолжение может принять существующий named claim
- **THEN** prepare показывает его точные path/branch и generation; naming inputs нового дерева не требуются

### Requirement: Диагностика именования отделяет данные от ownership

CLI SHALL возвращать стабильные typed refusal codes для invalid template/binding, missing/invalid value, исчерпанного namespace и незавершённого allocation. Диагностика SHALL называть поле/binding или claim ID/generation и безопасный следующий шаг, не маскируясь общим `invalid_input`. Failed start SHALL сохранять исходный refusal и достоверный cleanup outcome; duplicate либо handed-over claim MUST NOT очищаться как собственный новый claim.

Объявленная capability SHALL иметь строку в `examples/README.md`, а авторский справочник SHALL показывать общий профиль, передачу значений через существующий `--input PORT=FILE`/`--input-ref`, минимальную версию профиля и новые review/result contracts. Host handoff SHALL использовать фактический recorded repository workspace; host MUST NOT переименовывать его самостоятельно.

#### Scenario: Передан traversal
- **WHEN** task ID содержит `../outside`
- **THEN** typed Problem называет invalid naming value и источник, ни Git command с этим значением, ни дерево не создаются

#### Scenario: Allocation не доказал завершение
- **WHEN** процесс прервался и повтор не может доказать состояние созданного ref/дерева
- **THEN** CLI называет существующий preparing claim и recovery/inspection path, не предлагает слепой новый start и не расходует новый suffix

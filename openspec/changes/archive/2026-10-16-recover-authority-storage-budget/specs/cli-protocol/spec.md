## ADDED Requirements

### Requirement: CLI показывает фактический storage allowance и его границу
Read-only `storage budget show` и `doctor --storage` SHALL выдавать
`storage-budget-status/1` с authority identity/epoch, allocated bytes, page
size/count/free pages, effective finite limit, source/generation setting и
применимыми действиями. Неизвестное измерение MUST оставаться unknown с
причиной. Статус SHALL явно отделять logical pages от physical disk/WAL,
aggregate scan bytes и per-snapshot limit; чтение MUST NOT выполнять checkpoint,
VACUUM, GC, изменение configuration либо storage migration.

`storage_budget_exhausted` SHALL сохранять stable code/exit class и сообщать
доступные observed usage/limit и concrete operator path к show/review/set,
не объявляя безусловный retry существующим правом. Plain doctor и прежний
Problem SHALL сохранять published shape; дополнительные structured данные
SHALL иметь отдельную version boundary.

#### Scenario: Оператор выясняет причину остановки
- **WHEN** drive вернул `storage_budget_exhausted`, а оператор вызывает storage status
- **THEN** он видит observed allocation/effective ceiling и явный recovery путь без изменения authority или запуска Run

### Requirement: Storage limit меняется только reviewed owner командой
`storage budget set --bytes N --reason TEXT --prepare` SHALL быть read-only и
возвращать `storage-budget-review/1`: exact authority/epoch, current
generation/limit, requested target/reason, необходимые storage compatibility
изменения, observation usage и review digest. Digest SHALL защищать решение
владельца, а не timestamp изменяемой telemetry.

Mutation SHALL требовать те же target/reason, explicit
`--expected-review-digest` и command identity. До commit MUST проверяться
current authority/configuration и применимость target к актуальному
allocation. Stale generation/authority/review MUST давать named refusal;
состояние quota или setting не должно меняться по старому review. Если запись
из-за physical storage failure не подтверждена, CLI MUST сохранять честный
unknown/failure и путь reconciliation по command, а не советовать слепой set.
Команда SHALL возвращать `storage-budget-result/1` с recorded
before/after/generation и command evidence. Setting MUST NOT подразумевать
execution consent или автоматически запускать drive.

#### Scenario: Старое согласование неприменимо
- **WHEN** authority/epoch/configuration generation либо protected request изменены после prepare
- **THEN** set отказывается до budget mutation и требует новый review

#### Scenario: Usage вырос, а решение осталось прежним
- **WHEN** после prepare обычные writes изменили allocation без смены config
- **THEN** set заново проверяет актуальную allocation; он либо коммитит тот же явно выбранный достаточный target, либо отказывает без auto-grow

#### Scenario: Ответ принятой команды потерян
- **WHEN** оператор повторяет тот же exact command/payload после потери ответа
- **THEN** возвращается recorded result без повторного повышения generation или ещё одной mutation

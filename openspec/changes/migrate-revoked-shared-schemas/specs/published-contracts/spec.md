## ADDED Requirements

### Requirement: Schema migration имеет самостоятельные closed contracts
Request, plan, result и inspection миграции SHALL иметь собственные closed
versioned schemas. Нормативный machine shape MUST находиться рядом с Go types
и canonical schemas; Markdown MUST NOT заменять эти artifacts.
Authority package state с lifecycle generations и решениями SHALL использовать
новую `authority-packages/2`. Reader MUST поддерживать прежнюю `/1` без
приписывания ей новых полей. Переход MUST сохранять все существующие lifecycle
и trust facts. Старый binary MUST честно отказывать на неподдерживаемом state.

#### Scenario: Новое решение записывается в старую authority
- **WHEN** owner впервые применяет миграцию в authority с package state `/1`
- **THEN** движок атомарно сохраняет решение под `/2`, сохраняя исходные packages, reasons и историю

#### Scenario: Старый reader встречает миграцию
- **WHEN** reader без поддержки `/2` либо новой Run boundary открывает соответствующее состояние
- **THEN** он отказывает до исполнения и не игнорирует решение как неизвестное необязательное поле

### Requirement: Закреплённая миграция достигает собственной Run boundary
Run с SchemaMigrationBinding SHALL запечатываться новой opt-in state boundary
и читаться соответствующей новой read boundary. Проверка достижимости MUST
валидировать реально записанный Run против его опубликованного bundle.
Прежние Runs и bundles MUST оставаться неизменными. Run без binding MUST NOT
получать новое поле или новую версию только из-за наличия решения в authority.
Project summary/start с выбранной миграцией SHALL использовать новые closed
DTO boundaries; ответы без выбора сохраняют прежнюю совместимость.

#### Scenario: Новый Run использует миграцию
- **WHEN** Start создаёт Run с закреплённым решением
- **THEN** его schema version объявляет binding, public read его показывает, а старый bundle не принимает это как legacy state

#### Scenario: В authority есть решение, но Start его не выбирает
- **WHEN** обычный consumer запускается без migration binding
- **THEN** его state/read и Project DTO сохраняют прежнюю подходящую boundary

### Requirement: Qualification миграции отделяет generic и private acceptance
Release acceptance SHALL проверять native public CLI на fixed released binary:
исходный отказ, несколько shared schemas, apply, новый Run и последующие
admissions в той же временной authority. Regression SHALL покрывать старые
Runs, повторный revoke, несколько revoked suppliers, stale inputs, concurrency,
restart и прежние clean/remove/quarantine/dependency правила.
Отчёт MUST называть binary version/digest, fixtures, результаты и cleanup.
Успешное воспроизведение отказа MUST NOT объявляться квалификацией исправления.
Generic acceptance MUST NOT объявлять рабочую authority восстановленной.

#### Scenario: Generic fixture прошёл
- **WHEN** released binary завершает полный синтетический migration scenario
- **THEN** evidence подтверждает только generic scope; private authority acceptance остаётся отдельным решением её владельца

Источник — `openspec/specs/published-contracts/spec.md`; ownership не меняется.
Public identifiers прежних bundles и archived evidence сохраняются byte-for-byte.

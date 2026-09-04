## Purpose

Определяет архитектурные границы Pri-Fly, основания ключевых решений и
условия, при которых совместимость или profile может быть пересмотрен.

## Requirements

### Requirement: Core ограничен protocol исполнения

Pri-Fly MUST оставаться самостоятельным executor declared workflows без
обязательных Git, tracker, LLM, web service или cloud account. Неизвестная
capability MUST давать explicit unsupported path, а не arbitrary agent command.

#### Scenario: Пустая установка

- **WHEN** installation не содержит packages и внешних credentials
- **THEN** diagnostics и list работают, а unknown workflow не создаёт effect

### Requirement: Модули имеют единые границы ответственности

Protocol, definitions, router, authority, journal, artifacts, adapters и
presentation MUST разделять authority, execution и read concerns. Worker MUST
NOT писать journal или выбирать route напрямую.

#### Scenario: Worker импортирует внутренний storage

- **WHEN** package пытается обойти command/authority boundary
- **THEN** оно не получает прямого mutation path

### Requirement: Все способы управления используют один contract

CLI, library, local daemon и future transport MUST сохранять command identity,
authorization, dedup, limits, control и recovery semantics. Transport MUST NOT
создавать быстрый public mutation path.

#### Scenario: Stale UI пытается resume

- **WHEN** current stop появился между release и resume
- **THEN** continuation блокируется current control state

### Requirement: Authority и artifact store имеют разные atomicity boundaries

SQLite journal MUST сохранять authority facts короткими transactions, а blob
MUST быть sealed до принятия reference. Shared network filesystem MUST NOT
объявляться multi-host SQLite authority.

#### Scenario: Blob не завершён

- **WHEN** bytes не прошли sealing
- **THEN** они не становятся ready output по journal reference

### Requirement: Step execution не является blanket permission

ExecutionAdmission MUST ограничивать Step Attempt, а каждый effectful tool
operation MUST иметь own ActionIntent и Admission. Retry и whole-step rework
MUST NOT скрывать changed target, arguments или provider.

#### Scenario: Один tool call неизвестен

- **WHEN** response второго вызова потерян
- **THEN** система не повторяет уже applied первый вызов как blind restart

### Requirement: Сильные свойства включаются qualification profile

Local, managed and remote profiles MUST заявлять только подтверждённые
properties и их boundaries. Изменение OS, runtime, workload или isolation
mechanism MUST trigger affected qualification rather than inherited claim.

#### Scenario: Managed adapter не подтверждает confinement

- **WHEN** required profile property отсутствует
- **THEN** соответствующее execution не начинает работу

### Requirement: Architectural decisions имеют versioned основания пересмотра

Каждое architecture decision MUST хранить context, selected boundary,
consequences и trigger for reconsideration. Новая library или platform MUST
NOT сама по себе отменять adopted contract.

#### Scenario: Предложен новый database

- **WHEN** alternative не доказывает measured need или compatible recovery
- **THEN** current architecture не меняется автоматически

### Requirement: Extension не требует бесконечного SDK

Versioned packages и adapters MUST использовать declared public contracts.
Subject logic MUST NOT получать authority через in-process hook, monkey patch
или implementation-specific integration.

#### Scenario: Новый adapter требует private API

- **WHEN** integration cannot express its capability through public contract
- **THEN** она требует versioned extension instead of hidden dependency

### Requirement: Capability readiness подтверждается evidence

Capability MUST быть distinguished as specified, implemented, qualified or
unsupported by actual build/profile evidence. Schema or document validation
MUST NOT be promoted to runtime qualification.

#### Scenario: DTO schema валиден

- **WHEN** fixture passes shape validation
- **THEN** report does not claim external adapter qualification

### Requirement: Definition of done относится к конкретной поставке

Release readiness MUST name build, profile, applicable checks, observations,
limitations and rollback criteria. Общий label production-ready MUST NOT replace
this evidence.

#### Scenario: Новый build

- **WHEN** package or interpreter version changes
- **THEN** prior readiness evidence is reassessed for the delivered build

### Requirement: Owner decisions имеют safe defaults

Project-level choice, permissions and defaults MUST preserve empty core and
least authority. Missing owner intent MUST NOT invent task provider, workflow
or privileged operation.

#### Scenario: Launch имеет неявный default

- **WHEN** profile offers several declared launches
- **THEN** host requests exact selection instead of guessing from task text

### Requirement: Security update является delivery responsibility

Security change MUST identify affected definitions, runs and evidence while
restricting future execution. It MUST preserve investigation history rather
than erase prior facts.

#### Scenario: Compromised package выявлен

- **WHEN** quarantine becomes current
- **THEN** new admission stops and historical observations remain available

### Requirement: Storage migration, restore и export не возобновляют effects

Upgrade, backup, restore, export and clone MUST preserve identity and recovery
boundaries without dispatching old operations. Unknown external effect MUST
remain subject to reconciliation.

#### Scenario: Imported journal содержит pending dispatch

- **WHEN** state is opened after migration
- **THEN** it does not automatically resend the operation

### Requirement: Cost и capacity управляются измерениями

Capacity decision MUST use stated workload, resources, measurements and limits.
New broker, cluster or provider MUST NOT be introduced for speculative scale.

#### Scenario: Latency target не достигнут

- **WHEN** measurement shows a capacity shortfall
- **THEN** profile or implementation is changed with recorded evidence

### Requirement: License provenance и runtime trust проверяются отдельно

Package licensing, source provenance, installation trust and runtime authority
MUST remain separate decisions. A trusted source MUST NOT by itself grant an
effectful capability.

#### Scenario: Package имеет допустимую licence

- **WHEN** runtime grant or host permission is absent
- **THEN** installation does not enable the protected operation

### Requirement: Pilot сохраняет principles, а не fixed lifecycle

Reference pilot MAY prove a bounded integration path, but it MUST NOT make its
stage names, external skills or host provider mandatory for Pri-Fly core.

#### Scenario: Другой project workflow

- **WHEN** project does not use the pilot's skills
- **THEN** it can express a declared workflow without inheriting their lifecycle

### Requirement: Handover связывает результат с поставкой и ограничениями

Delivery handover MUST include build/profile, evidence, unresolved uncertainty,
known limitations and operator next actions. It MUST NOT state broader
qualification than collected evidence supports.

#### Scenario: External effect неизвестен

- **WHEN** handover includes pending reconciliation
- **THEN** it does not label the Run or release unconditionally successful

### Requirement: Версии state contract образуют один упорядоченный ряд

Реализация MUST вычислять совместимость Run со state, read, next, preview и
step-read contracts из одного порядкового ряда версий и одной таблицы
соответствий. Проверка «версия не ниже» MUST быть единой функцией, а не
цепочкой сравнений строк; выбор версии при создании Run MUST быть максимумом
требуемых рангов. Один и тот же state MUST давать одинаковый read/next/preview
contract во всех точках чтения. Опубликованные идентификаторы версий не
меняются.

#### Scenario: Добавлена новая версия state
- **WHEN** разработчик добавляет следующую версию contract
- **THEN** изменяется одна запись таблицы, а все точки чтения сообщают новую
  версию согласованно

### Requirement: Persistence layer не протекает в runtime

Runtime MUST различать сбой персистентности через exported predicate слоя
хранения, а не через импорт конкретного database driver. Runtime и CLI MUST
собираться без cgo для проверки этой границы, даже если выбранный driver в
release требует cgo. Замена driver MUST быть отдельным ADR с измерениями.

#### Scenario: Сборка без cgo
- **WHEN** `CGO_ENABLED=0 go vet ./...` выполняется в CI
- **THEN** runtime и CLI проходят vet, а отказ возникает только в самом
  storage driver

## Основания решений

Контекст, выбранные границы, последствия и условия пересмотра собраны в
[реестре архитектурных решений](decisions/index.md). Эти записи объясняют
требования выше, но не являются отдельным набором квалификационных evidence.

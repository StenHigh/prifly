## Purpose

Определяет единственный актуальный план поставки Pri-Fly, границы release
profiles и значение статусов. Он не смешивает целевой план с журналом уже
сделанных инженерных срезов.

## ADDED Requirements

### Requirement: План поставки разделяет две фазы продукта
Pri-Fly SHALL публиковать последовательный план надёжной локальной фазы и
целевой фазы расширений. План MUST отличать scope capability от факта её
реализации, не объявлять future operators текущей возможностью и сохранять
явную зависимость между фазой, milestone и gate.

| Фаза | Результат | Условие закрытия |
|---|---|---|
| 1. Надёжная последовательность | Локальный Go CLI с доверенными шагами, immutable inputs/outputs, history, outcomes, diagnostics, базовыми measurements, human control и recovery | Закрыты все P1 milestones на конкретном build/profile; unsupported capabilities отвергаются до исполнения |
| 2. Целевой продукт | Все заявленные operators, packages/context/rights, managed actions, remote lane, recovery, full measurements и эксплуатационные profiles | Закрыты все P2 milestones, выполнена полная квалификация заявленных profiles |

Фаза 1 не обещает автоматическую изоляцию, remote execution, SaaS, HA-кластер,
account system, billing, secret store, visual editor или обязательный public SDK.
Фаза 2 имеет конечный объём текущих product capabilities; будущие отраслевые
packages, модели ИИ и дополнительные OS не входят в неё молча.

#### Scenario: Будущая capability видна в плане
- **WHEN** reader открывает milestone второй фазы
- **THEN** он видит её intended scope, prerequisite и gate без заявления, что
  она уже доступна в текущем profile

### Requirement: Фаза 1 имеет полный последовательный inventory
Phase-one delivery plan SHALL содержать следующие milestones в указанном
порядке. Каждый следующий milestone требует принятия предыдущего; внутри
milestone допустима параллельная работа, но не объявление готовности без его
prerequisite и regression.

| Milestone | Intended result | Stage gate |
|---|---|---|
| P1-01 | Go foundation, pinned toolchain, local profile и инженерные границы | Воспроизводимая сборка без model SDK или исходного проекта |
| P1-02 | Empty install, local definitions и exact local inventory | Empty install и unknown workflow не вызывают network/tool effects |
| P1-03 | Durable identities, SQLite history, atomic commands и bounded single-writer control | Restart и competing CAS не оставляют частичную историю |
| P1-04 | Validated sequential workflow and typed artifacts | Invalid paths, bindings, outputs и unsupported operators отвергаются до run |
| P1-05 | Local process runner и durable dispatch boundary | Lost response не создаёт второй process; ранний result не освобождает slot |
| P1-06 | Result acceptance, terminal invariants и honest settlement | Foreign, malformed или incomplete result не продвигает run |
| P1-07 | Pause, cancel и restart recovery | Crash/stop races не создают повторный effect или false terminal state |
| P1-08 | YAML authoring, public CLI и first user journey | User проходит empty install → own workflow → inspect/control по документации |
| P1-09 | Qualification и release первой фазы | Выполнены applicable gates на named build/OS, опубликованы limitations и runbook |

Обычное обязательство каждого milestone: проверить затронутые contracts,
реализовать минимальный путь без hidden bypass, проверить normal/error/control
interleavings, обновить supported/unsupported surface, прогнать regression и
сохранить evidence до его закрытия.

#### Scenario: Первый этап не подменяется следующим
- **WHEN** код более позднего milestone уже существует, а prerequisite ещё не
  принят
- **THEN** plan сохраняет prerequisite незакрытым и не считает наличие кода
  квалификацией следующего milestone

### Requirement: Фаза 2 имеет полный последовательный inventory
Phase-two delivery plan SHALL содержать следующие milestones в указанном
порядке после фазы 1. Каждый milestone сохраняет compatibility прежних Runs,
общие control/resource boundaries и честно отказывает неподдержанному пути.

| Milestone | Intended result | Stage gate |
|---|---|---|
| P2-01 | Version compatibility, pinned capability profile и общие prerequisites | Старые Runs читаются без переинтерпретации; unsupported extension rejected pre-dispatch |
| P2-02 | Deterministic conditions and branching | Decision, selected route и recovery сохраняются без arbitrary evaluation |
| P2-03 | Nested workflows | Child invocation имеет own identity, common budget и scoped control |
| P2-04 | Bounded repeat | Каждая iteration имеет distinct body identity и explicit state transfer |
| P2-05 | Full context, artifacts и content/result evidence | Context bytes, checks и overflow boundaries проверяемы до use |
| P2-06 | Rights, approvals, grants и quality limits | Current authority, revocation и scoped decisions не обходятся stale command |
| P2-07 | Packages, dependencies и trust | Exact closure, install/update/remove и trust boundary не исполняют hidden content |
| P2-08 | Resources, generations и concurrent scheduling | Conflicting owners and over-capacity admissions cannot coexist |
| P2-09 | Managed actions и qualified executors | External effect имеет admission, delivery evidence, isolation/remote boundary where claimed |
| P2-10 | Parallel branches и declared joins | Join не маскирует living branch, unknown effect или resource overlap |
| P2-11 | Map over sealed collection | Item identities, cardinality и fan-out bounded before first child admission |
| P2-12 | Waits, observations, reactions и schedules | Event/timer/guard races не создают duplicate transition или hidden callback |
| P2-13 | Retry, reconcile, fork и reuse | Каждая форма повторения имеет own identity, authority и evidence |
| P2-14 | Compensation and partial-work settlement | Residual obligation остаётся видимым; compensation не становится universal rollback |
| P2-15 | Full public protocol and operator interface | Public commands используют controlled handlers и доступное, explainable state |
| P2-16 | Operations, backup, retention и performance | Restore, bounded storage/query/load и measured operational limits квалифицированы |
| P2-17 | End-to-end scenarios и profile qualification | Reference scenarios отличают core correctness от deployment/AI quality |
| P2-18 | Full product acceptance and release | Complete claimed scope, gates, release material и independent-install path pass |

#### Scenario: Поздний milestone выбран для работы
- **WHEN** team выбирает поздний P2 milestone
- **THEN** plan показывает preceding sequence и не называет milestone ready
  только из-за definition, schema или local demo

### Requirement: Статус и evidence не подменяют друг друга
Delivery record SHALL различать specified scope, engineering implementation,
executed check и qualified release. Document check, local demo, source test или
historical report MUST NOT закрывать broader product gate. Current status MUST
называть дату, scope и evidence boundary.

На 2026-09-01 формальная последовательная приёмка P2 не объявлена закрытой:
исторический engineering progress не меняет её статусы сам по себе. Архивный
F1 candidate остаётся своим immutable result; позднее evidence не меняет его
bytes, version или outcome задним числом. Current plan не создаёт отдельную
таблицу статусов: record формальной приёмки и concrete evidence должны быть
readable together at the time of a future close decision.

#### Scenario: Узкая проверка прошла
- **WHEN** component, compiler or example test succeeds
- **THEN** delivery snapshot не объявляет broader phase qualified без полного
  applicable evidence для named build/profile

### Requirement: Release candidate имеет явные scope и exclusions
Каждый RC SHALL называть target profile, обязательные gates, known gaps,
deferred work и rollback boundary. Qualified narrow RC MUST NOT становиться
заявлением о whole-product release, managed execution или external-provider
qualification.

Текущий RC profile — `core-local` AI Factory с человеком в контуре. Его
проверенный путь: из чистого проекта установить trusted package, подготовить
readable YAML workflow, запустить planning cycle и сохранить result. Improve
читает исправленный plan в следующей iteration; review следует только за
выполненной работой; человек выбирает improvements и warnings.

На 2026-09-01 закрыты physical suspend/resume gate и финальный reproducible
gate этого exact candidate. Это квалифицирует только local owner-host путь.
В RC намеренно отсутствуют daemon wakeup, automatic external action/retry/
reconcile, throughput promise, retention/GC, backup/restore RPO/RTO, managed
isolation, remote trust boundary и provider cost/usage qualification.

#### Scenario: Core-local RC представлен пользователю
- **WHEN** reader открывает current RC boundary
- **THEN** он видит supported local scope, qualification boundary и отдельно
  перечисленные unsupported integrations и future gates

### Requirement: Текущая очередь отделена от future catalogue
Delivery plan SHALL хранить active priority, contributor-readiness work и
post-RC queue отдельно от catalogue возможных workflow и дальних идей.
Изменение future idea MUST NOT неявно менять committed release scope или
runtime contract.

Текущая engineering priority после qualified core-local RC: закончить
OpenSpec migration и contributor-ready track — reproducible native GitLab CI,
local YAML editor contract, один YAML-first authoring route с legacy
compatibility и общий corpus для independent validators. Эти работы защищают
поставленную поверхность и не меняют порядок или formal status P1/P2.

Future catalogue может предлагать packages и workflows, но их install/listing
внешнего происхождения требует future action authority. Empty install и
default execution не ходят в сеть; install не предлагается молча в init.
Будущими, а не promised current capabilities, остаются полное provider usage
and cost view, additional workflow operators, workspace-visible delivery
records, trusted reuse, full dry run, helper continue command и MCP surface.

#### Scenario: В каталог добавлен новый workflow
- **WHEN** team добавляет возможный workflow или integration
- **THEN** он остаётся proposal с prerequisite и explicit authority boundary,
  а не появляется как supported scenario текущего release

### Requirement: Первая поставка называет состав и внешние границы
Delivery documentation SHALL фиксировать first-build dependency inventory,
лицензионные/операционные ограничения и reproducible native verification
boundary. Dependency presence MUST NOT сама по себе доказывать trust,
qualification или поддержку каждого platform/profile.

Первый build использует Go toolchain, SQLite driver, JSON Schema validator,
YAML parser, JSON canonicalization и required text dependency. Их exact
versions, checksums и notices закреплены release build material. Runtime не
зависит от LLM SDK, Temporal, Redis, broker, PostgreSQL, Docker, GitLab API
или Git; Python допустим только для developer tools, examples или explicitly
chosen user steps. Availability dependency or successful build is not proof of
runtime trust, cross-platform support or qualification.

#### Scenario: Зависимость присутствует в сборке
- **WHEN** reader видит third-party dependency
- **THEN** inventory называет её purpose и boundary, не выдавая её наличие за
  доказательство runtime capability

### Requirement: Исторические отчёты не становятся второй правдой
Current delivery spec SHALL хранить поддерживаемый snapshot и правила его
обновления. Historical progress, release reports и archived evidence MAY
оставаться в Git and migration archive, but MUST NOT override current status,
source ownership or future plan.

#### Scenario: Исторический отчёт противоречит current plan
- **WHEN** старый release snapshot упоминает прежний status или authoring path
- **THEN** reader использует current OpenSpec snapshot; dated history остаётся
  evidence и не возвращает старую очередь или старый contract

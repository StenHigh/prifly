# Delivery Roadmap Specification

## Purpose

Определяет актуальный план поставки Pri-Fly, границы release profiles и
значение статусов, не смешивая целевой план с журналом инженерных срезов.

## Requirements

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
readable YAML workflow, запустить planning cycle и сохранить result. Packages
`aif-classic` и `aif-fanout` этого пути поставляются внешним repository
`StenHigh/prifly-aif-workflows` через Workflow catalog, а не `examples/`
Pri-Fly: в `aif-classic` improve читает исправленный plan в следующей
iteration, review следует только за выполненной работой, а blocking
verify/security/review предлагает `/aif-fix` без automatic fix; отдельный
`aif-fanout` показывает declared parallel profiles, но не квалифицирует выбор
provider, model или reasoning level. Compile-проверки этих packages
выполняются в их repository против released Pri-Fly.

На 2026-09-01 закрыты physical suspend/resume gate и финальный reproducible
gate этого exact candidate. Это квалифицирует только local owner-host путь.
В RC намеренно отсутствуют daemon wakeup, automatic external action/retry/
reconcile, throughput promise, retention/GC, backup/restore RPO/RTO, managed
isolation, remote trust boundary, provider cost/usage qualification и
assisted-host model-profile selection.

#### Scenario: Core-local RC представлен пользователю
- **WHEN** reader открывает current RC boundary
- **THEN** он видит supported local scope, qualification boundary и отдельно
  перечисленные unsupported integrations и future gates

### Requirement: Текущая очередь отделена от future catalogue
Delivery plan SHALL хранить active priority, contributor-readiness work и
post-RC queue отдельно от catalogue возможных workflow и дальних идей.
Изменение future idea MUST NOT неявно менять committed release scope или
runtime contract.

Текущий backlog находится в этом документе и содержит три уровня.

| Уровень | Запись | Статус | Prerequisite | Следующий шаг |
|---|---|---|---|---|
| Highest | [`make-project-launch-workflow-neutral`](../../changes/make-project-launch-workflow-neutral/tasks.md): универсальный Project launch | Спланировано, реализация не начата | Существующие local-process, assisted-session и package contracts | Последовательно: сосуществование сборок → запуск без Git/ИИ → общая анкета и mixed flow → внешний AIF gate |
| Active | `add-run-decision-catalog`: per-Run Fast/Full/Ultra и universal decision bridge | В работе | Versioned Project launch, sealed package profile и durable Run-state | Завершить typed catalog, preflight, wait/recovery и host/CLI evidence, не выдавая upstream AIF compatibility или live-pilot qualification за результат Core |
| Active | `add-native-host-question-ux`: один конечный вопрос в Codex и Claude Code | Осталось ручное наблюдение UI | Доступ к обоим host runtimes | Закрыть task 2.3 active change без заявления product qualification |
| High | `assisted-model-profile-protocol` | Не начато | Versioned assisted-host contract | Создать OpenSpec change до заявления о provider/model/reasoning selection |

Приоритет на 2026-09-05 — общий путь запуска, а не расширение AIF или управление
моделями. Детальные задачи и условия приёмки находятся только в linked change.
Его первые два среза снимают блокеры самостоятельного использования Pri-Fly;
затем общий decision UX и внешний AIF проверяются поверх тех же mechanisms.
Незакрытые ручные наблюдения двух Active changes сохраняются и получают ссылки
на evidence, а не отметки готовности по факту нового плана. Этот порядок не
меняет формальную последовательность P1/P2 и не удаляет future catalogue.

Ревью надёжности и производительности authority выполнено и закрыто archived
change `2026-09-04-harden-authority-reliability-and-performance`: измерения,
не достигнутые цели и отклонённые решения остаются в его evidence. Оно не
закрывает roadmap milestone и не объявляет product gate.

Работа над самими AI Factory packages — живой pilot `aif-classic` на задаче
и совместимость с опубликованным AI Factory package — ведётся в backlog
repository `StenHigh/prifly-aif-workflows`; в Pri-Fly остаются только
engine-стороны этих задач.

Обязательная product sequence остаётся полной и линейной. P1-01…P1-07 были
закрыты для historical F1 candidate и не являются незавершённой работой.
P1-08 и P1-09 ожидают formal acceptance; для P2 такая приёмка не объявлена.
Наличие кода или прежнего engineering slice не меняет эти статусы без
назначенного evidence.

| Milestone | Intended result | Статус | Prerequisite | Следующий шаг |
|---|---|---|---|---|
| P1-08 | YAML authoring, CLI и user journey | Ожидает formal acceptance | P1-07 closed historical candidate | Открыть acceptance change для named build/profile |
| P1-09 | Qualification и release phase one | Ожидает formal acceptance | P1-08 | После принятия P1-08 открыть release qualification change |
| P2-01 | Compatibility и profile pinning | Приёмка не объявлена | P1-09 | После принятия P1-09 открыть acceptance change |
| P2-02 | Deterministic branching | Приёмка не объявлена | P2-01 | После принятия P2-01 открыть acceptance change |
| P2-03 | Nested workflows | Приёмка не объявлена | P2-02 | После принятия P2-02 открыть acceptance change |
| P2-04 | Bounded repeat | Приёмка не объявлена | P2-03 | После принятия P2-03 открыть acceptance change |
| P2-05 | Full context и evidence | Приёмка не объявлена | P2-04 | После принятия P2-04 открыть acceptance change |
| P2-06 | Rights, approvals и quality limits | Приёмка не объявлена | P2-05 | После принятия P2-05 открыть acceptance change |
| P2-07 | Packages, dependencies и trust | Приёмка не объявлена | P2-06 | После принятия P2-06 открыть acceptance change |
| P2-08 | Resources и concurrent scheduling | Приёмка не объявлена | P2-07 | После принятия P2-07 открыть acceptance change |
| P2-09 | Managed actions и qualified executors | Приёмка не объявлена | P2-08 | После принятия P2-08 открыть acceptance change |
| P2-10 | Parallel branches и joins | Приёмка не объявлена | P2-09 | После принятия P2-09 открыть acceptance change |
| P2-11 | Map over sealed collection | Приёмка не объявлена | P2-10 | После принятия P2-10 открыть acceptance change |
| P2-12 | Waits, observations, reactions и schedules | Приёмка не объявлена | P2-11 | После принятия P2-11 открыть acceptance change |
| P2-13 | Retry, reconcile, fork и reuse | Приёмка не объявлена | P2-12 | После принятия P2-12 открыть acceptance change |
| P2-14 | Compensation и partial-work settlement | Приёмка не объявлена | P2-13 | После принятия P2-13 открыть acceptance change |
| P2-15 | Full public protocol и operator interface | Приёмка не объявлена | P2-14 | После принятия P2-14 открыть acceptance change |
| P2-16 | Operations, retention и performance | Приёмка не объявлена | P2-15 | После принятия P2-15 открыть acceptance change |
| P2-17 | End-to-end profile qualification | Приёмка не объявлена | P2-16 | После принятия P2-16 открыть qualification change |
| P2-18 | Full product acceptance и release | Приёмка не объявлена | P2-17 | После принятия P2-17 открыть release change |

Post-P2 catalogue — не committed scope. Каждая идея получает отдельный
proposal только после указанного prerequisite.

| Идея | Prerequisite | Следующий шаг |
|---|---|---|
| Внешние source adapters задач (GitLab, GitHub, Jira) | P2-09 action authority и один завершённый живой pilot | Создать отдельный proposal; репозитории и каталог YAML workflow folders уже реализованы archived change `2026-09-03-add-project-workflow-catalog` (`project workflows search/add/update/remove`), `init` и default execution остаются offline |
| Полное provider usage и cost view | P2-09 и P2-16 | Включить в соответствующие acceptance changes, не вычислять цену самим |
| Workspace-visible delivery record | Согласованный review/retention contract | Создать отдельный proposal |
| Замена SQLite driver на pure-Go (`modernc.org/sqlite`) | ADR с измерениями поверх evidence archived change `2026-09-04-harden-authority-reliability-and-performance` | Создать отдельный proposal с benchmark evidence и расширенной release matrix |
| Принятие сохранённого result candidate при recovery | `run resolve` и recovery по доказательствам завершения | Создать отдельный proposal о повторной валидации без driver ownership |
| Trusted reuse готовых шагов | P2-13 | Проработать в P2-13 change |
| Full dry run | P2-15 | Проработать в P2-15 change |
| Helper `continue` command | Стабильный операторский interface | Создать отдельный proposal после P2-15 |
| MCP surface | Controlled public protocol | Создать отдельный proposal после P2-15 |

Закрытая запись получает ссылку на свой OpenSpec change или release evidence;
новая запись создаётся только вместе с OpenSpec change. Historical reports и
архивные документы остаются в Git и archived changes, но не являются вторым
backlog.

#### Scenario: Команда выбирает contributor-ready работу
- **WHEN** team начинает следующий post-RC change
- **THEN** она берёт первую незавершённую работу из active backlog и не создаёт
  compatibility scope для unreleased source form

#### Scenario: В каталог добавлен новый workflow
- **WHEN** team добавляет возможный workflow или integration
- **THEN** он остаётся proposal с prerequisite и explicit authority boundary,
  а не появляется как supported scenario текущего release

### Requirement: Текущий backlog поставки полный и единый
`delivery-roadmap` SHALL быть единственным редактируемым местом, где читатель
видит всю незавершённую работу Pri-Fly. Backlog MUST различать: текущие
changes, линейные P1/P2 milestones и необязательный post-P2 catalogue. Каждая
запись MUST называть свой статус, prerequisite и следующий способ начать
работу; future idea не становится committed product scope без отдельного
OpenSpec change.

Исторические отчёты, прежние roadmap и release evidence остаются только в Git
и archived OpenSpec changes. Они MUST NOT дублироваться в current backlog или
использоваться для закрытия его записей.

#### Scenario: Владелец ищет следующую работу
- **WHEN** он открывает `delivery-roadmap`
- **THEN** он видит текущий порядок незавершённых changes, полную P1/P2
  последовательность и отдельный каталог будущих идей без обращения к legacy
  документам

#### Scenario: Завершённая работа меняет backlog
- **WHEN** OpenSpec change или release закрывает запись backlog
- **THEN** current roadmap получает ссылку на этот результат, а подробное
  evidence остаётся в его immutable historical location

#### Scenario: Появляется новая будущая идея
- **WHEN** команда добавляет её в delivery backlog
- **THEN** она имеет явный scope и prerequisite, но не меняет runtime contract
  или committed release scope до отдельного OpenSpec change

### Requirement: Выбор model profile остаётся отдельной high-priority задачей
После разделения `aif-classic` и `aif-fanout` delivery roadmap SHALL хранить
отдельную high-priority задачу `assisted-model-profile-protocol`. Она должна
изменить versioned assisted-host contract до того, как Pri-Fly заявит реальный
выбор provider, model или reasoning level. `aif-fanout` из repository
`StenHigh/prifly-aif-workflows` является её future acceptance fixture, но
successful compilation не является evidence такого выбора.

#### Scenario: Веер компилируется до нового host protocol
- **WHEN** `aif-fanout` успешно проходит authoring checks
- **THEN** roadmap и report называют это проверкой graph/profile data, а не
  доказательством фактического model selection

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

### Requirement: Каталог решений Run остаётся отдельной active работой
Единый current backlog MUST хранить `add-run-decision-catalog` как active
high-priority change до закрытия его OpenSpec tasks. Запись MUST называть
prerequisite: versioned Project launch, sealed package profile and durable
Run-state; следующий шаг: реализовать typed catalog, preflight selection,
wait/recovery и host/CLI evidence. Она MUST отделять этот scope от
`assisted-model-profile-protocol`, upstream AI Factory compatibility и live
pilot qualification.

#### Scenario: Команда выбирает следующую работу
- **WHEN** владелец читает current delivery backlog
- **THEN** он видит, что per-Run Fast/Full/Ultra и безопасный autonomous
  decision policy требуют этой отдельной change, а не правки `extend.yaml`

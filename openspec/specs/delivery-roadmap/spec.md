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

Решение владельца от 2026-10-30 о развитии собственных инструментов:
[14 выбранных направлений](#factory-development),
[расширенный обзор Warp](reports/2026-10-30-warp-lessons.html) и
[разбор читаемости YAML](reports/2026-10-30-readable-workflows.html).
Основание изменения —
[`prioritize-readable-and-measurable-workflows`](../../changes/prioritize-readable-and-measurable-workflows/tasks.md).
HTML — датированные аналитические снимки; актуальные статусы задаёт этот roadmap.

Текущий backlog на 2026-09-23 сверён с неархивированными OpenSpec changes.
Закрытый срез цепочки возобновлений добавлен 2026-10-02 как release/pilot evidence, а не открытая работа.
Счётчик означает только отметки в `tasks.md`, а не квалификацию release или
формальную приёмку milestone. Порядок внутри группы не устанавливает новую
зависимость между независимыми changes.

| Очередь | Change | Статус | Prerequisite | Следующий шаг |
|---|---|---|---|---|
| Исправление admission | [`route-worktree-claim-by-run`](../../changes/route-worktree-claim-by-run/tasks.md) | Локально проверено: 5/6 | Два отдельных worktree claims одного authority | После обновления бинарника проверить два новых Run в разных worktree |
| Запуск и решения | [`make-project-launch-workflow-neutral`](../../changes/make-project-launch-workflow-neutral/tasks.md) | 23/24; открыта 3.5 | Доступ к Codex и Claude Code | Завершить generic host guidance, связать UI evidence и сохранить отдельный commit среза |
| Продолжение и recovery | [`continue-existing-implementation`](../../changes/continue-existing-implementation/tasks.md) | 10/12; открыты 1.4 и 3.3 | Существующие Project CLI, claims и AI Factory package | Добавить CLI integration cases и провести новый Run до terminal quality outcome |
| Продолжение и recovery | [`retry-failed-stage-with-new-package`](../../changes/retry-failed-stage-with-new-package/tasks.md) | 5/13 | Sealed source Run, новый package edition и доказательства пригодности результатов | Реализовать recovery command/projection, проверить reuse и сквозной Run |
| Цепочка восстановления/возобновления | [`fix-chained-workflow-resume`](../../changes/archive/2026-10-02-fix-chained-workflow-resume/tasks.md) | Закрыто: 14/14; release 0.13.75, пилот E succeeded | Immutable source ancestry, доказательства принятого пути и текущий source claim | Приёмка среза завершена; SMSPlace merge/product QA не является gate Pri-Fly |
| Монитор | [`make-monitor-list-human-readable`](../../changes/make-monitor-list-human-readable/tasks.md) | 3/4; открыта 2.1 | Существующий read-only монитор | Выполнить приёмочные проверки |
| Монитор | [`clarify-run-list-execution-counts`](../../changes/clarify-run-list-execution-counts/tasks.md) | 0/4 | Существующий RunSummary и read-only список | Переименовать и пояснить счётчики StepInstance/Attempt |
| Монитор | [`explain-terminal-partial-runs`](../../changes/explain-terminal-partial-runs/tasks.md) | 0/4 | Terminal Run outcome и доказанный маршрут | Разделить terminal/live empty-state и объяснить `partial` |
| Монитор | [`show-live-execution-ownership`](../../changes/show-live-execution-ownership/tasks.md) | 2/5 | Закреплённые Run/Attempt/Session/Process facts | Завершить read-only activity projection и проверить монитор |
| Монитор | [`observe-running-program-progress`](../../changes/observe-running-program-progress/tasks.md) | 0/11 | Local program runner и versioned read projections | Начать с versioned progress contract и bounded fd 4 |

Завершённые changes `separate-work-time-from-human-wait`,
`add-local-run-monitor`, `workspace-tree-preparation-rollback`,
`assisted-model-profile-protocol` и шесть changes, архивированных 2026-09-23,
находятся в `openspec/changes/archive/`; они не являются открытой работой.
Архив этого среза: [`add-native-host-question-ux`](../../changes/archive/2026-09-23-add-native-host-question-ux/tasks.md),
[`add-run-decision-catalog`](../../changes/archive/2026-09-23-add-run-decision-catalog/tasks.md),
[`clarify-monitor-run-status-graph`](../../changes/archive/2026-09-23-clarify-monitor-run-status-graph/tasks.md),
[`group-related-runs-in-monitor`](../../changes/archive/2026-09-23-group-related-runs-in-monitor/tasks.md),
[`harden-aif-classic-gate-routing`](../../changes/archive/2026-09-23-harden-aif-classic-gate-routing/tasks.md)
и [`refresh-delivery-backlog-2026-09-23`](../../changes/archive/2026-09-23-refresh-delivery-backlog-2026-09-23/tasks.md).
Выпуск 0.13.68 (2026-09-29) закрыл
[`auto-retire-unused-package-editions`](../../changes/archive/2026-09-29-auto-retire-unused-package-editions/tasks.md)
вместе с [`make-run-reading-plain-for-any-model`](../../changes/archive/2026-09-29-make-run-reading-plain-for-any-model/tasks.md).
Host UI observations в архивированных `add-native-host-question-ux` и
`add-run-decision-catalog` отмечены по подтверждению владельца от 2026-09-23,
а не по независимому release evidence. Пункт 3.5
`make-project-launch-workflow-neutral` остаётся открытым до обновления generic
host инструкций и отдельного commit среза. Завершение change не меняет
формальную приёмку P1/P2.

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

#### Scenario: Владелец сверяет очередь с changes
- **WHEN** reader открывает текущий backlog
- **THEN** он видит каждый неархивированный change с его фактическим статусом,
  а завершённые архивированные работы не принимаются за открытые задачи

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

На 2026-09-20 versioned assisted-host contract изменён: шаг объявляет профиль
модели в запечатанном плане (StepDefinition v9), задача его несёт, а отчёт
хоста обязан назвать, что он с объявлением сделал, иначе отказывается. Это
снимает prerequisite задачи и **не** является model selection: authority не
выбирает модель, не проверяет заявление хоста и нигде не называет его
подтверждённым выбором. Заявление хранится как наблюдение хоста о себе.

#### Scenario: Веер компилируется до нового host protocol
- **WHEN** `aif-fanout` успешно проходит authoring checks
- **THEN** roadmap и report называют это проверкой graph/profile data, а не
  доказательством фактического model selection

#### Scenario: Шаг объявил профиль и хост ответил
- **WHEN** шаг объявил профиль модели и отчёт хоста назвал исход
- **THEN** это записано как заявление хоста о себе, а не как квалификация
  provider/model/reasoning selection, и RC по-прежнему её не содержит

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

### Requirement: Исправление цепочки возобновлений имеет отдельную границу приёмки

Delivery backlog SHALL учитывать `fix-chained-workflow-resume` как приоритетное исправление блокирующего дефекта существующего пути. Prerequisite — доступные sealed source Runs и доказательства префикса, существующие recovery/resumable contracts и текущий claim. Следующий шаг — воспроизведение цепочки без ИИ, исправление общего planner и выпуск проверенного бинарника. Прогресс SHALL храниться в tasks этого change; закрытие MUST различать инженерную регрессию, поставку и фактическое продолжение пилота SMSPlace #163.

Проверка документа или зелёный fixture MUST NOT объявлять SMSPlace завершённым, закрывать независимый `retry-failed-stage-with-new-package` либо формальную приёмку P1/P2. Живой pilot SHALL использовать read-only prepare от текущего владельца дерева и отдельное проверенное разрешение start; evidence MUST называть installed build, Run IDs, transferred claim и фактический outcome.

Нормативный source set — `openspec/specs/delivery-roadmap/spec.md`; historical release records не изменяются.

#### Scenario: Инженерный фикс готов, пилот ещё не продолжен
- **WHEN** регрессионная цепочка прошла, но установленный бинарник или живой Run ещё не проверены
- **THEN** delivery record сохраняет незавершённую поставку/приёмку и не объявляет бизнес-задачу выполненной

<a id="factory-development"></a>

### Requirement: Развитие собственных инструментов использует проверяемые подходы фабрик

Delivery roadmap SHALL сохранять решение владельца от 2026-10-30 развивать
Pri-Fly и собственные workflows. Опыт Warp используется как источник подходов,
а не как обязательная платформа, зависимость или план миграции.

Roadmap SHALL показывать следующие 14 направлений в трёх группах развития.
Внутри группы указан порядок выбора новой работы; переход к следующей группе
требует её prerequisites, а не молчаливого расширения текущего RC. Порядок MUST
NOT отменять активные исправления или последовательную приёмку P1/P2.
Статус каждого направления — «Запланировано; отдельная реализация и приёмка
не объявлены». Наличие существующей части не означает готовность всего пути.

**Ближайшая полезность и измерения.**

| Направление | Prerequisite | Следующий шаг | Проверяемый результат |
|---|---|---|---|
| Читаемое авторство сценариев | Текущий YAML contract и существующие authoring references | Использовать `refresh-author-guides-and-examples` для навигации; отдельным authoring change сравнить структуру и формы записи одинаковых сценариев | Автор находит маршрут, инструкции, данные и остановки; contracts и совместимость сохранены |
| Понятный путь до результата и очередь внимания владельца | Существующие launch, decisions и read-only monitor; их активные changes | Проверить полный путь; отдельным change спроектировать общую очередь вопросов, блокировок и готового review | Владелец находит требующие его участия Runs, evidence и безопасное следующее действие; скрытое подтверждение невозможно |
| Сравнение конфигураций на одинаковых задачах | Закреплённые задачи, исходные commits, критерии и реально доступные host configurations | Оформить отдельное исследование повторяемого сравнения; задачи и skills оставить в репозитории workflows | Результаты сравнимы по качеству, времени и известным расходам; unavailable и разброс видимы |
| Учёт результата и участия владельца | Существующие timing/telemetry и определённый полезный результат сценария | Определить минимальный состав отчёта и пробелы текущих измерений отдельным change | Активное участие отличается от ожидания; принятый результат — от успешного процесса; неизвестные расходы — от нуля |
| Регулярная оценка качества завершённых Runs | Закреплённые criteria, версия оценщика, сопоставимое evidence и проверка оценщика человеком | Отдельным change определить выборку запусков, классификации и ручную сверку одного вида ошибки | Причина оценки и версия criteria доступны; модельная оценка не подменяет обязательный gate и не меняет прошлый outcome |
| Улучшение процесса по повторным отказам | Регулярная оценка, сравнение на закреплённых задачах и review человеком | Начать с одного класса отказов и предложения правки skill/package; задачи вести у владельца этих artifacts | Предложение связано с failing Runs и regression check; изменение принимает владелец, прежние Runs сохраняются |

**Управляемое исполнение.**

| Направление | Prerequisite | Следующий шаг | Проверяемый результат |
|---|---|---|---|
| Управляемый запуск host и model profile | Существующая задача model-profile selection; versioned host contract, lifecycle и источник фактических execution metadata | Продолжить эту задачу отдельным executor change: выбирать host/model при создании сессии, затем фиксировать наблюдаемое исполнение | Requested, selected и наблюдаемые сведения различаются; unavailable не становится honoured; отдельная сессия не выдаётся за другую модель |
| Пул исполнителей и квалифицированная изоляция | P2-08/P2-09, общие budgets/control, владение и fencing ресурсов | Начать с одного локального managed worker; remote backend квалифицировать отдельно | Очередь, capacity, cancel, потеря worker и recovery не создают двойного owner/effect; контейнер сам по себе не доказывает sandbox |
| Повторяемые среды сборки и browser verification | Закреплённые toolchain/image/setup, права запуска и cleanup; P2-05/P2-09 для заявленного managed profile | Отдельным change описать переиспользуемую среду и один UI/UX-путь с actual browser evidence | Evidence связано с candidate, средой и session; setup/cleanup имеют owner и явный исход; недоступный browser не даёт pass |

**Внешний контур и дальнейшие исследования.**

| Направление | Prerequisite | Следующий шаг | Проверяемый результат |
|---|---|---|---|
| Фоновая обработка расписаний и событий | P2-12/P2-16, durable correlation, текущие control/revocation и пределы эксплуатации | Отдельным change определить живущий вне чата service loop, затем один schedule/webhook source | При restart сохраняются due state, dedup и bounded catch-up; отменённая работа не стартует; polling текущего CLI не называется daemon |
| Внешний приём задач и обратная связь | Существующая запись source adapters, P2-09/P2-12 и один живой pilot | Уточнить эту запись одним adapter GitHub или GitLab: intake, correlation, фильтры и возврат статуса | Внешняя задача связана с Run; повторная доставка и callbacks проверяются; фильтр события не заменяет authorization |
| Публичные API/MCP для других инструментов | Существующая запись MCP surface и controlled protocol P2-15 | Спроектировать одну boundary над существующими handlers; начать с read-only и отдельно ограниченного dispatch/control | Клиент использует versioned operations, scoped credentials и audit; обход authority через прямую запись в state невозможен |
| Ограниченные credentials и роли | P2-06/P2-09, trust boundary и способ независимо проверить действующее право | Отдельным security change разделить доступ к модели, инструментам, repository и административным операциям | Секреты передаются только нужному executor, значения не попадают в evidence; revocation и human-only действия проверяются в точке применения |
| Проверяемая память проекта между сессиями | P2-05/P2-13, provenance, изоляция проектов и правила устаревания/отзыва | Исследовать opt-in повторное использование знаний с явным source; сначала сравнить с обычным versioned context | Источник, применимость и версия знания видны; память не расширяет права, не переносит данные между проектами и не меняет прошлые Runs |

Работа над общим compiler, CLI, monitor и authoring contract SHALL оставаться
в Pri-Fly. Изменения конкретных AI Factory packages, prompts и наборов задач
SHALL планироваться в `StenHigh/prifly-aif-workflows`, без второго backlog здесь.
Новая возможность движка получает отдельный capability change и author-facing
документацию; строка roadmap сама по себе не создаёт её contract.

Существующие записи о model-profile selection, source adapters, MCP surface
и provider usage/cost SHALL сохранять свои prerequisites. Их строки выше
уточняют цели, а не создают вторую задачу на ту же возможность. Наличие этих
направлений MUST NOT объявлять model routing, managed execution, daemon wakeup,
team service или независимое подтверждение модельного отчёта доступными сегодня.

Читаемость — одно направление, не вся продуктовая разница. Roadmap MUST
отличать проверяемые сильные стороны локального ядра Pri-Fly от готовности
операторского интерфейса, инфраструктуры и цикла оценки Warp. Документированные
возможности конкурента не являются evidence нашей реализации или общим
доказательством, что один продукт надёжнее другого.

Нормативный source set — `openspec/specs/delivery-roadmap/spec.md`; до sync
этот change уточняет его. Runtime, YAML markers, опубликованные JSON-поля
и сохранённые Runs не меняются; historical evidence не переписывается.

#### Scenario: Владелец выбирает следующую работу после сравнения с Warp
- **WHEN** владелец открывает выбранные направления roadmap
- **THEN** он видит порядок, prerequisites, следующий шаг и проверяемый результат без предложения перейти на Warp

#### Scenario: Направление требует изменения продукта
- **WHEN** выбранная работа требует нового runtime, authoring или protocol contract
- **THEN** отдельный capability change определяет её scope и приёмку, а строка roadmap остаётся планом

#### Scenario: Улучшается конкретный AI Factory package
- **WHEN** предложение меняет только prompt, skill или набор задач конкретного package
- **THEN** его backlog и приёмка остаются в репозитории workflows, а Pri-Fly хранит только необходимую engine-сторону

### Requirement: Упрощение авторства оценивается без потери контракта

Roadmap SHALL поставить читаемость авторских исходников первой среди новых
направлений и разделить улучшения существующей формы и исследование новой.
Сначала оцениваются блочная YAML-запись, Markdown через существующий
`context.source`, короткий обзор маршрута и ясные роли файлов.

Сокращение служебных ссылок, объединённый Markdown-файл шага или другая
организация компонентов SHALL оставаться кандидатами для отдельного
исследования, а не объявленным новым синтаксисом. Исследование MUST сравнивать
одинаковое поведение, включая error routes и bounded repair, а не короткий
agent prompt с полным machine contract.

Приёмка будущего authoring change MUST проверять entry, routes, ports,
bindings, limits, effects, retry, policy, verdict handling и bytes контекста.
Новые compiled identities и digests из-за изменения authoring closure MUST
называться явно; семантическая эквивалентность не обещает прежний build digest.
Прежние source forms и закреплённые Runs SHALL сохранять своё значение.

Читаемость SHALL оцениваться заданиями человека: найти первый шаг, изменить
его инструкции, проследить передачу результата и определить остановку при
отказе или исчерпании лимита. Количество строк и файлов MUST NOT заменять
эту оценку. Измерение проводится на простом program workflow и составном
assisted workflow; результаты не объявляют общий product gate закрытым.

#### Scenario: Короткая запись скрывает поведение
- **WHEN** кандидат убирает явный failure route, permission или repeat limit
- **THEN** он не проходит сравнение читаемости как эквивалент прежнему сценарию

#### Scenario: Markdown облегчает чтение инструкций
- **WHEN** существующий context получает те же bytes через отдельный Markdown source
- **THEN** приёмка отличает сохранение context contract от изменения authoring closure и compiled identity

#### Scenario: Новый формат ещё не выбран
- **WHEN** reader видит предложение объединить настройки и Markdown шага
- **THEN** roadmap называет его исследованием и не предлагает такой файл как поддержанный исходник

### Requirement: Аналитические HTML-отчёты доступны из roadmap

Roadmap SHALL ссылаться на сохранённые в repository датированные HTML-отчёты
`reports/2026-10-30-readable-workflows.html` и
`reports/2026-10-30-warp-lessons.html`. Первый сохраняет согласованный разбор
читаемости, второй расширяет сравнение до всех направлений развития.

Отчёты SHALL называться ненормативными аналитическими снимками. Актуальные
scope, prerequisites и статусы остаются в `delivery-roadmap`, не в HTML.
Ссылки на repository artifacts MUST быть относительными и разрешаться после
клонирования; absolute machine paths и ссылки на временные файлы недопустимы.
Отчёты MUST не требовать установки custom document builder, сетевых scripts
или обязательного внешнего ресурса для чтения. Их наличие не меняет release
gates, опубликованные contracts или historical evidence.

#### Scenario: Владелец возвращается к основанию направления
- **WHEN** он открывает ссылку на отчёт из roadmap в другом checkout
- **THEN** HTML находится в repository, локальные ссылки разрешаются, а отчёт указывает нормативный roadmap и дату анализа

#### Scenario: Состояние разработки изменилось после отчёта
- **WHEN** текущий roadmap и датированный аналитический снимок называют разные статусы
- **THEN** reader использует roadmap; HTML остаётся объяснением прежнего анализа, не второй очередью работ

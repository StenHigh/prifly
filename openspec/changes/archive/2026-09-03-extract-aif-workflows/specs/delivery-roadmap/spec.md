## MODIFIED Requirements

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
| Active | `add-run-decision-catalog`: per-Run Fast/Full/Ultra и universal decision bridge | В работе | Versioned Project launch, sealed package profile и durable Run-state | Завершить typed catalog, preflight, wait/recovery и host/CLI evidence, не выдавая upstream AIF compatibility или live-pilot qualification за результат Core |
| Active | `add-native-host-question-ux`: один конечный вопрос в Codex и Claude Code | Осталось ручное наблюдение UI | Доступ к обоим host runtimes | Закрыть task 2.3 active change без заявления product qualification |
| High | `assisted-model-profile-protocol` | Не начато | Versioned assisted-host contract | Создать OpenSpec change до заявления о provider/model/reasoning selection |

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

## REMOVED Requirements

### Requirement: Canonical AI Factory plan bridge остаётся active delivery work
**Reason**: Change `add-workspace-artifact-bindings` завершён и архивирован;
его tree binding contract живёт в `domain-execution`, `runtime-resources` и
`workflow-and-context`, а package `aif-classic` — во внешнем repository.
**Migration**: archived change `2026-09-03-add-workspace-artifact-bindings`;
живой pilot и upstream compatibility — backlog `StenHigh/prifly-aif-workflows`.

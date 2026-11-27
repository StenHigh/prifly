## MODIFIED Requirements

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

Текущий backlog сверён 2026-11-10 с `main` (`60931dc`), всеми 21
неархивированными OpenSpec changes и опубликованным stable
[0.13.91](https://github.com/StenHigh/prifly/releases/tag/v0.13.91).
Счётчик означает только отметки в `tasks.md`, а не квалификацию release или
формальную приёмку milestone. Завершённый checklist в неархивированной папке
не становится открытой реализацией; archive/sync остаётся отдельным действием.
Порядок внутри группы не устанавливает новую зависимость между changes.
Prerequisites и полные критерии каждого change сохранены в его proposal/tasks;
таблица называет оставшуюся работу, не заменяя эти критерии.

| Очередь | Change | Checklist | Следующий шаг / граница |
|---|---|---|---|
| Исправление admission | [`route-worktree-claim-by-run`](../../changes/route-worktree-claim-by-run/tasks.md) | 5/6 | Живой допуск двух новых Runs с отдельными worktree после обновления бинарника |
| Запуск и решения | [`make-project-launch-workflow-neutral`](../../changes/make-project-launch-workflow-neutral/tasks.md) | 23/24 | Generic host guidance и отдельные Codex/Claude UI observations: 3.5 |
| Продолжение и recovery | [`continue-existing-implementation`](../../changes/continue-existing-implementation/tasks.md) | 12/13 | Реальный Run до terminal quality outcome: 3.3 |
| Продолжение и recovery | [`retry-failed-stage-with-new-package`](../../changes/retry-failed-stage-with-new-package/tasks.md) | 12/13 | Адресный pilot recovery с доказанным reuse и terminal outcome: 4.3 |
| Монитор | [`make-monitor-list-human-readable`](../../changes/make-monitor-list-human-readable/tasks.md) | 4/4 | Checklist завершён; archive/sync отдельно |
| Монитор | [`clarify-run-list-execution-counts`](../../changes/clarify-run-list-execution-counts/tasks.md) | 4/4 | Checklist завершён; archive/sync отдельно |
| Монитор | [`explain-terminal-partial-runs`](../../changes/explain-terminal-partial-runs/tasks.md) | 4/4 | Checklist завершён; archive/sync отдельно |
| Монитор | [`show-live-execution-ownership`](../../changes/show-live-execution-ownership/tasks.md) | 5/5 | Checklist завершён; archive/sync отдельно |
| Монитор | [`observe-running-program-progress`](../../changes/observe-running-program-progress/tasks.md) | 10/11 | Отдельная адаптация SMSPlace tests-worker: 3.3 |
| Recovery | [`fix-nested-preparation-recovery`](../../changes/fix-nested-preparation-recovery/tasks.md) | 9/9 | Checklist завершён; archive/sync отдельно |
| Recovery | [`fix-reviewed-cancelled-writer-recovery`](../../changes/fix-reviewed-cancelled-writer-recovery/tasks.md) | 8/8 | Checklist завершён; archive/sync отдельно |
| Admission | [`fix-concurrent-named-worktree-start`](../../changes/fix-concurrent-named-worktree-start/tasks.md) | 9/9 | Checklist завершён; archive/sync отдельно |
| Effects | [`let-a-program-step-declare-an-external-write`](../../changes/let-a-program-step-declare-an-external-write/tasks.md) | 19/20 | Сообщить выпуск и проверить настоящий Run пилота: 6.3 |
| Проверки | [`reuse-unchanged-verification-results`](../../changes/reuse-unchanged-verification-results/tasks.md) | 13/13 | Checklist завершён; archive/sync отдельно |
| Light | [`add-prifly-light-task-board`](../../changes/add-prifly-light-task-board/tasks.md) | 22/26 | Сверить release evidence 7.1/7.2; production/domain acceptance 7.3/7.4 не объявлена |
| Композиция | [`add-nested-project-extension-points`](../../changes/add-nested-project-extension-points/tasks.md) | 7/8 | Capability выпущена в 0.13.83; сопоставить released replay/handoff с gate 4.2, не закрывать по наличию release |
| Композиция | [`add-workflow-component-imports-and-layers`](../../changes/add-workflow-component-imports-and-layers/tasks.md) | 57/59 | Capability выпущена в 0.13.85; сопоставить exact-SHA gates и generic released replay с 8.1/8.2 |
| Авторство | [`improve-workflow-authoring-readability`](../../changes/improve-workflow-authoring-readability/tasks.md) | 11/11 | Checklist завершён; пакетная поставка отдельно в workflows v1.54.0/v1.55.0 |
| Планирование | [`prioritize-readable-and-measurable-workflows`](../../changes/prioritize-readable-and-measurable-workflows/tasks.md) | 6/6 | Checklist планирования завершён; реализации направлений не следуют из этого статуса |
| Документация | [`refresh-author-guides-and-examples`](../../changes/refresh-author-guides-and-examples/tasks.md) | 2/10 | Выполнить оставшуюся сверку руководств и примеров, затем их проверки по tasks |
| Host UI | [`verify-claude-launch-choice-ui`](../../changes/verify-claude-launch-choice-ui/tasks.md) | 0/7 | DEFERRED: только после отдельного запроса владельца; реальные UI observations не заменять fixture |

Закрытая цепочка `fix-chained-workflow-resume`: 14/14, release 0.13.75,
пилот E succeeded; [release/pilot evidence](../../changes/archive/2026-10-02-fix-chained-workflow-resume/evidence.md).
Это не открытая реализация и не приёмка SMSPlace merge/product QA.
Light поставлен в 0.13.81, nested extension points — в 0.13.83,
imports/layers и unified composition — в 0.13.85; открытые пункты их tasks
обозначают недосверенную приёмку, а не отсутствие выпущенной capability.
Свежие выпуски вплоть до 0.13.91 не закрывают эти checklist автоматически.

Отдельные новые capabilities уже приняты и архивированы, не являются открытым backlog:
- 0.13.89: scoped reviewed shared-schema migration; [задачи и границы](../../changes/archive/2026-11-09-migrate-revoked-shared-schemas/tasks.md), [released evidence](../../changes/archive/2026-11-09-migrate-revoked-shared-schemas/evidence/released-v0.13.89/README.md).
- 0.13.90: project-owned compile parameters для объявленных schema limits; [задачи](../../changes/archive/2026-11-10-add-project-owned-compile-parameters/tasks.md), [released evidence](../../changes/archive/2026-11-10-add-project-owned-compile-parameters/evidence/released-v0.13.90/README.md).
- 0.13.91: сохранённый выбор владельца для миграции общих схем конкретного launch и точной сборки; [задачи и границы](../../changes/archive/2026-11-10-remember-launch-schema-migration/tasks.md), [выпуск и проверка опубликованного binary](https://github.com/StenHigh/prifly/releases/tag/v0.13.91). Приёмка частной authority остаётся отдельной операцией владельца.
Эти срезы не закрывают формальные P1/P2 gates и не являются поставкой новых consumer packages.


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

### Requirement: Исправление цепочки возобновлений имеет отдельную границу приёмки

Delivery backlog SHALL учитывать `fix-chained-workflow-resume` как закрытое исправление: archived tasks 14/14, release 0.13.75 и пилот E succeeded. Повторная реализация не является следующим шагом. Evidence SHALL оставаться в archived change и различать инженерную регрессию, поставку и фактическое продолжение пилота SMSPlace #163; закрытие этого среза не означает merge или product QA потребителя.

Проверка документа или зелёный fixture MUST NOT объявлять SMSPlace завершённым, закрывать независимый `retry-failed-stage-with-new-package` либо формальную приёмку P1/P2. Живой pilot SHALL использовать read-only prepare от текущего владельца дерева и отдельное проверенное разрешение start; evidence MUST называть installed build, Run IDs, transferred claim и фактический outcome.

Нормативный source set — `openspec/specs/delivery-roadmap/spec.md`; historical release records не изменяются.

#### Scenario: Инженерный фикс готов, пилот ещё не продолжен
- **WHEN** регрессионная цепочка прошла, но установленный бинарник или живой Run ещё не проверены
- **THEN** delivery record сохраняет незавершённую поставку/приёмку и не объявляет бизнес-задачу выполненной

### Requirement: Развитие собственных инструментов использует проверяемые подходы фабрик

Delivery roadmap SHALL сохранять решение владельца от 2026-10-30 развивать
Pri-Fly и собственные workflows. Опыт Warp используется как источник подходов,
а не как обязательная платформа, зависимость или план миграции.

Roadmap SHALL показывать следующие 14 направлений в трёх группах развития.
Внутри группы указан порядок выбора новой работы; переход к следующей группе
требует её prerequisites, а не молчаливого расширения текущего RC. Порядок MUST
NOT отменять активные исправления или последовательную приёмку P1/P2.
Направления остаются планом развития, но отдельные части уже выполнены:
`improve-workflow-authoring-readability` имеет завершённый checklist 11/11,
а пакетные изменения читаемости выпущены workflows v1.54.0/v1.55.0.
Оставшаяся навигация ведётся в `refresh-author-guides-and-examples`.
Наличие принятой части не означает готовность всего направления или измеренного ускорения.

**Ближайшая полезность и измерения.**

| Направление | Prerequisite | Следующий шаг | Проверяемый результат |
|---|---|---|---|
| Читаемое авторство сценариев | Текущий YAML contract и существующие authoring references | Завершить навигацию в `refresh-author-guides-and-examples`; использовать принятое сравнение `improve-workflow-authoring-readability`, не повторяя завершённую пакетную переработку | Автор находит маршрут, инструкции, данные и остановки; contracts и совместимость сохранены |
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

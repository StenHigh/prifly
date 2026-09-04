Authoritative source set: `openspec/specs/delivery-roadmap/spec.md`
(перенесено). Compatibility path: только backlog; runtime scope, formal
acceptance status и historical evidence не меняются.

## MODIFIED Requirements

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
| Active | `harden-authority-reliability-and-performance` фазы 1–2: явная резолюция uncertain obligation, recovery по сохранённым доказательствам, сбой authority ≠ сбой worker, идемпотентный retry, чистые transform'ы | Не начато | Ревью 2026-09-03, зафиксированное в change | Выполнить tasks 1.x–2.x, записать evidence, не объявляя закрытие product gate |
| Active | `harden-authority-reliability-and-performance` фазы 3–4: in-process validator, incremental verify при open, дедупликация pinned bytes, переходы в journal, fsync-политика | Не начато | Фазы 1–2 с evidence | Выполнить tasks 3.x–4.x под storage v5 и новой state version |
| Active | `harden-authority-reliability-and-performance` фазы 5–8: кэши и индексы горячих путей, единый ранг версий и типизированные ошибки, поставка и benchmark gate | Не начато | Фазы 3–4 с evidence | Выполнить tasks 5.x–8.x и опубликовать benchmark evidence |
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
| Замена SQLite driver на pure-Go (`modernc.org/sqlite`) | ADR с измерениями после `harden-authority-reliability-and-performance` фазы 6 | Создать отдельный proposal с benchmark evidence и расширенной release matrix |
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


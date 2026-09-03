# Архивная crosswalk: delivery roadmap

Это историческая карта переноса source set delivery. Постоянная capability
держит только текущий двухфазный план, snapshot на 2026-09-01, RC boundary и
будущую очередь. Старые identifiers, подробные отчёты и dated engineering
slices остаются здесь и в Git; они не являются editable status system.

## Formal milestones

| Historical milestone | Source | Permanent section | Prerequisite | Gate boundary |
|---|---|---|---|---|
| P1-01 | `roadmap.md`, Фаза 1 | Фаза 1 inventory | — | reproducible foundation build |
| P1-02 | `roadmap.md`, Фаза 1 | Фаза 1 inventory | P1-01 | empty install/local definitions |
| P1-03 | `roadmap.md`, Фаза 1 | Фаза 1 inventory | P1-02 | durable state and atomic commands |
| P1-04 | `roadmap.md`, Фаза 1 | Фаза 1 inventory | P1-03 | validated sequence/artifacts |
| P1-05 | `roadmap.md`, Фаза 1 | Фаза 1 inventory | P1-04 | observed local dispatch |
| P1-06 | `roadmap.md`, Фаза 1 | Фаза 1 inventory | P1-05 | honest result settlement |
| P1-07 | `roadmap.md`, Фаза 1 | Фаза 1 inventory | P1-06 | pause/cancel/recovery interleavings |
| P1-08 | `roadmap.md`, Фаза 1 | Фаза 1 inventory | P1-07 | documented user path |
| P1-09 | `roadmap.md`, Фаза 1 | Фаза 1 inventory | P1-08 | qualified first-phase release |
| P2-01 | `roadmap.md`, Фаза 2 | Фаза 2 inventory | P1-09 | compatibility and profile pinning |
| P2-02 | `roadmap.md`, Фаза 2 | Фаза 2 inventory | P2-01 | deterministic choice |
| P2-03 | `roadmap.md`, Фаза 2 | Фаза 2 inventory | P2-02 | nested invocation |
| P2-04 | `roadmap.md`, Фаза 2 | Фаза 2 inventory | P2-03 | bounded repeat |
| P2-05 | `roadmap.md`, Фаза 2 | Фаза 2 inventory | P2-04 | full context and checks |
| P2-06 | `roadmap.md`, Фаза 2 | Фаза 2 inventory | P2-05 | rights/approvals/grants |
| P2-07 | `roadmap.md`, Фаза 2 | Фаза 2 inventory | P2-06 | package trust and closure |
| P2-08 | `roadmap.md`, Фаза 2 | Фаза 2 inventory | P2-07 | resources and scheduler |
| P2-09 | `roadmap.md`, Фаза 2 | Фаза 2 inventory | P2-08 | managed/qualified actions |
| P2-10 | `roadmap.md`, Фаза 2 | Фаза 2 inventory | P2-09 | parallel join settlement |
| P2-11 | `roadmap.md`, Фаза 2 | Фаза 2 inventory | P2-10 | sealed map/fan-out |
| P2-12 | `roadmap.md`, Фаза 2 | Фаза 2 inventory | P2-11 | durable waits/reactions |
| P2-13 | `roadmap.md`, Фаза 2 | Фаза 2 inventory | P2-12 | retries/reuse evidence |
| P2-14 | `roadmap.md`, Фаза 2 | Фаза 2 inventory | P2-13 | compensation settlement |
| P2-15 | `roadmap.md`, Фаза 2 | Фаза 2 inventory | P2-14 | complete public interface |
| P2-16 | `roadmap.md`, Фаза 2 | Фаза 2 inventory | P2-15 | operations/retention qualification |
| P2-17 | `roadmap.md`, Фаза 2 | Фаза 2 inventory | P2-16 | end-to-end profile qualification |
| P2-18 | `roadmap.md`, Фаза 2 | Фаза 2 inventory | P2-17 | full-product release gate |

Count: 27 (nine first-phase and eighteen second-phase milestones). The order
is linear; implementation work may be parallel inside a milestone only.

## Current records and classification

| Source heading or record | Source file | Classification | Permanent destination or reason |
|---|---|---|---|
| Two phases, sequence, gate meaning and milestone text | `docs/roadmap/roadmap.md` | current delivery | Two phases and both milestone inventories |
| Obligations of every milestone; status and evidence rules | `docs/roadmap/roadmap.md` | current delivery | Inventory obligations; Status and evidence |
| Current formal boundary: no automatic acceptance from implementation | `docs/roadmap/roadmap.md`, `docs/f2-progress.md` | current delivery | Status and evidence |
| First `core-local` AI Factory RC scope and exclusions | `docs/rc-scope.md` | current delivery | Release candidate scope and exclusions |
| RC-0/RC-4 result on exact candidate | `docs/rc-scope.md`, `docs/f2-progress.md` | current delivery | Release candidate scope and exclusions |
| DCL-01 through DCL-04 completed authoring track | `docs/roadmap/roadmap.md`, `docs/f2-progress.md` | current delivery | Current queue; YAML-first route remains specified by workflow capability |
| OSS-00 through OSS-04 contributor-ready priorities | `docs/roadmap/roadmap.md`, `docs/f2-progress.md` | current delivery | Current queue |
| Future package/workflow catalogue and action boundary | `docs/beyond-phase-two.md`, "Что предлагается" and "Порядок" | current delivery | Current queue and future catalogue |
| Cost, extra operators, workspace record, reuse, dry run, continue command and MCP ideas | `docs/beyond-phase-two.md`, "Чего не хватает" | current delivery | Future catalogue; not current promise |
| First-build dependency and licence inventory | `docs/dependencies.md` | current delivery | First-build inventory |
| Historical F1 archive and immutable candidate facts | `docs/release.md` | historical_git_only | Exact dated/release evidence remains Git history; permanent spec only states its boundary |
| Dated P2 engineering slices and interim test reports | `docs/f2-progress.md` | historical_git_only | Exact prose/revisions remain Git history; headings inventoried below |

## Historical heading inventory

All headings below are preserved in their exact source file. The progress
document covers 2026-08-28 through current snapshot 2026-09-01; a heading
without a date is an undated historical section. `historical_git_only` means
the full dated claim and evidence remain available by Git history, not an
editable second plan.

| Source | Line | Heading | Classification |
|---|---:|---|---|
| `docs/release.md` | 11 | Evidence | historical_git_only |
| `docs/release.md` | 24 | Что считается первой фазой | historical_git_only |
| `docs/release.md` | 30 | Архив | historical_git_only |
| `docs/f2-progress.md` | 5 | Текущее состояние на 2026-09-01 | current delivery |
| `docs/f2-progress.md` | 33 | Разработка и приёмка | historical_git_only |
| `docs/f2-progress.md` | 39 | Объём P2-01 | historical_git_only |
| `docs/f2-progress.md` | 55 | Evidence и ограничения P2-01 | historical_git_only |
| `docs/f2-progress.md` | 73 | Объём P2-02 | historical_git_only |
| `docs/f2-progress.md` | 88 | Evidence и ограничения P2-02 | historical_git_only |
| `docs/f2-progress.md` | 101 | Реализация P2-03 | historical_git_only |
| `docs/f2-progress.md` | 116 | Реализация P2-04 | historical_git_only |
| `docs/f2-progress.md` | 128 | P2-06: начало authority control plane | historical_git_only |
| `docs/f2-progress.md` | 138 | AIF-01: session principal, object access и current control checks | historical_git_only |
| `docs/f2-progress.md` | 150 | AIF-02: явный импорт локального пакета | historical_git_only |
| `docs/f2-progress.md` | 164 | AIF-03: исключительный claim Git worktree | historical_git_only |
| `docs/f2-progress.md` | 172 | AIF-04: ассистируемое исполнение через сессию хоста | historical_git_only |
| `docs/f2-progress.md` | 186 | AIF-05: пилот на внешних skills AI Factory | historical_git_only |
| `docs/f2-progress.md` | 201 | P2-06, второй срез: approvals, quorum и атомарное потребление | historical_git_only |
| `docs/f2-progress.md` | 215 | P2-06, третий срез: grants и фильтрация чтения по правам | historical_git_only |
| `docs/f2-progress.md` | 229 | P2-06, четвёртый срез: quality waivers и видимость снижения | historical_git_only |
| `docs/f2-progress.md` | 241 | Квалификация P2-06 | historical_git_only |
| `docs/f2-progress.md` | 249 | P2-07, первый срез: жизненный цикл и текущее доверие пакета | historical_git_only |
| `docs/f2-progress.md` | 259 | P2-07, второй срез: замыкание зависимостей между пакетами | historical_git_only |
| `docs/f2-progress.md` | 269 | P2-07, третий срез: запечатанный архив, корни доверия и подписи | historical_git_only |
| `docs/f2-progress.md` | 279 | Квалификация P2-07 | historical_git_only |
| `docs/f2-progress.md` | 287 | Наблюдавшийся невоспроизведённый сбой | historical_git_only |
| `docs/f2-progress.md` | 295 | P2-08, первый срез: сроки владения, опознание владельца и псевдонимы | historical_git_only |
| `docs/f2-progress.md` | 303 | P2-08, второй срез: атомарный multi-claim и конкурентная приёмка | historical_git_only |
| `docs/f2-progress.md` | 318 | P2-10, фундамент: ограниченная ёмкость допуска вместо одного слота | historical_git_only |
| `docs/f2-progress.md` | 328 | P2-10, компилятор: ветви, join и заявленные границы | historical_git_only |
| `docs/f2-progress.md` | 340 | P2-10, исполнение: ветви как invocations и durable join | historical_git_only |
| `docs/f2-progress.md` | 364 | P2-10, квалификация: композиция, восстановление и остановка | historical_git_only |
| `docs/f2-progress.md` | 392 | P2-08, ёмкость допуска: записанная граница вместо константы | historical_git_only |
| `docs/f2-progress.md` | 404 | P2-08, очередь допуска: порядок вместо удачи | historical_git_only |
| `docs/f2-progress.md` | 420 | P2-08, общий бюджет: подтверждено, а не заявлено | historical_git_only |
| `docs/f2-progress.md` | 480 | P2-10, сводка ветвей: поставляемая стандартная форма | historical_git_only |
| `docs/f2-progress.md` | 494 | Ассистируемый шаг без записи | historical_git_only |
| `docs/f2-progress.md` | 502 | Сквозной прогон: план → две рецензии → выбор → применение | historical_git_only |
| `docs/f2-progress.md` | 515 | P2-10, одновременные ветви | historical_git_only |
| `docs/f2-progress.md` | 527 | P2-10, приёмка AC-043: достигнутый кворум не публикует поверх живой ветви | historical_git_only |
| `docs/f2-progress.md` | 539 | P2-12: ожидание события или срока | historical_git_only |
| `docs/f2-progress.md` | 571 | P2-12: расписание как запись проекта, а не часть запуска | historical_git_only |
| `docs/f2-progress.md` | 595 | P2-12: живые условия над фактами, которые запуск уже держит | historical_git_only |
| `docs/f2-progress.md` | 623 | P2-11: map по закреплённой коллекции | historical_git_only |
| `docs/f2-progress.md` | 643 | Ядро цикла AI Factory как рабочий сценарий | historical_git_only |
| `docs/f2-progress.md` | 667 | Мониторинг: окно, а не второй пульт | historical_git_only |
| `docs/f2-progress.md` | 689 | P2-05: промежуточный checkpoint контекста | historical_git_only |
| `docs/f2-progress.md` | 697 | P2-05: checkpoint приёмки результатов | historical_git_only |
| `docs/f2-progress.md` | 703 | AI Factory: improve/review cycles и project limits | historical_git_only |
| `docs/f2-progress.md` | 713 | AI Factory: передача исправленного плана между improve-проходами | historical_git_only |
| `docs/f2-progress.md` | 723 | AI Factory: веер рецензентов внутри improve-прохода | historical_git_only |
| `docs/f2-progress.md` | 733 | Core examples: проверка уже поставленных операторов | historical_git_only |
| `docs/f2-progress.md` | 743 | Runtime: точная стоимость от названного внешнего источника | historical_git_only |
| `docs/f2-progress.md` | 753 | P2-12: первый срез ранней публикации артефакта | historical_git_only |
| `docs/f2-progress.md` | 765 | P2-12: одноразовая доставка ранней публикации | historical_git_only |
| `docs/f2-progress.md` | 810 | P2-09: параметры действия проверяются до предложения | historical_git_only |
| `docs/f2-progress.md` | 827 | P2-09: входные данные действия должны существовать | historical_git_only |
| `docs/f2-progress.md` | 843 | P2-12: точное закрытие набора ранних артефактов | historical_git_only |
| `docs/f2-progress.md` | 890 | P2-12: ограниченная потоковая подписка на ранние артефакты | historical_git_only |
| `docs/f2-progress.md` | 937 | Workflow authoring: читаемый YAML без второй семантики | historical_git_only |
| `docs/f2-progress.md` | 988 | P2-12 / PUB-003: проверка содержимого до публикации артефакта | historical_git_only |
| `docs/f2-progress.md` | 1022 | P2-12 / PUB-004: `new_only` для источника ранних публикаций | historical_git_only |
| `docs/f2-progress.md` | 1055 | P2-12 / PUB-004: terminal producer failure прерывает объявленный subscriber | historical_git_only |
| `docs/f2-progress.md` | 1087 | P2-12 / PUB-003–004: sealed blob delivery для named subscribers | historical_git_only |
| `docs/f2-progress.md` | 1113 | P2-09 foundation: sealed ToolDescriptor в registry | historical_git_only |
| `docs/f2-progress.md` | 1140 | RC-AIF: scripted owner-host проходит весь core cycle | historical_git_only |
| `docs/f2-progress.md` | 1171 | P2-09 foundation: closed ActionIntent до admission | historical_git_only |
| `docs/f2-progress.md` | 1193 | RC-AIF: clean recipe не требует GPG для своего seed repository | historical_git_only |
| `docs/f2-progress.md` | 1216 | P2-10: branch bindings видят output предшествующей стадии | historical_git_only |
| `docs/f2-progress.md` | 1237 | P2-15: help не отрицает уже работающие операторы | historical_git_only |
| `docs/f2-progress.md` | 1256 | P2-09: durable ActionIntent proposal до Admission | historical_git_only |
| `docs/f2-progress.md` | 1286 | P2-09: ActionAdmission с атомарным consume Approval | historical_git_only |
| `docs/f2-progress.md` | 1325 | P2-09: ActionAdmission с точным расходом Grant | historical_git_only |
| `docs/f2-progress.md` | 1348 | P2-09: подготовленная доставка действия | historical_git_only |
| `docs/f2-progress.md` | 1371 | P2-13: новый запуск при изменении работы | historical_git_only |
| `docs/f2-progress.md` | 1391 | P2-13: доверие при переносе результата | historical_git_only |
| `docs/f2-progress.md` | 1404 | P2-07: отзыв доверия действует без перезапуска | historical_git_only |
| `docs/f2-progress.md` | 1425 | P2-07: просмотр точного установленного пакета | historical_git_only |
| `docs/f2-progress.md` | 1446 | P2-07: отзыв доверия защищён в момент допуска | historical_git_only |
| `docs/f2-progress.md` | 1470 | P2-09: отзыв инструмента останавливает подтверждение действия | historical_git_only |
| `docs/f2-progress.md` | 1492 | P2-09: отозванный инструмент не создаёт новое предложение | historical_git_only |
| `docs/f2-progress.md` | 1512 | P2-10: живая задача AI Factory на неизменяемом кандидате | historical_git_only |
| `docs/f2-progress.md` | 1535 | RC-0: native suspend/resume подтверждён | historical_git_only |
| `docs/f2-progress.md` | 1554 | RC-4: перед финальным прогоном исправлены устаревшие проверки | historical_git_only |
| `docs/f2-progress.md` | 1567 | RC-4: deadline-проверка получила запас на остановку процесса | historical_git_only |
| `docs/f2-progress.md` | 1578 | RC-4: первый core-local AIF release candidate квалифицирован | historical_git_only |
| `docs/f2-progress.md` | 1594 | Project onboarding: общий profile отделён от authority | historical_git_only |
| `docs/f2-progress.md` | 1627 | Project launcher: Codex ведёт AI Factory cycle из repository | historical_git_only |
| `docs/f2-progress.md` | 1656 | Task intake: одна проверяемая форма для чата и трекера | historical_git_only |
| `docs/f2-progress.md` | 1672 | Project launcher: local binary больше не предполагается в PATH | historical_git_only |
| `docs/f2-progress.md` | 1689 | Project launcher: выбор сценария принадлежит profile | historical_git_only |
| `docs/f2-progress.md` | 1711 | DCL-01: YAML — единственный источник графа и шагов | historical_git_only |
| `docs/f2-progress.md` | 1740 | DCL-01: YAML завершает авторство AI Factory recipe | historical_git_only |
| `docs/f2-progress.md` | 1775 | DCL-02: нейтральный YAML compiler project package | historical_git_only |
| `docs/f2-progress.md` | 1808 | DCL-03: компактная YAML-папка сценария | historical_git_only |
| `docs/f2-progress.md` | 1843 | DCL-04: компактный YAML шага | historical_git_only |
| `docs/f2-progress.md` | 1867 | DOC-01: актуальность документации | historical_git_only |
| `docs/f2-progress.md` | 1892 | EXM-01: чистые примеры и справочник YAML | historical_git_only |
| `docs/f2-progress.md` | 1913 | REP-01: структура для пользователей и contributors | historical_git_only |
| `docs/f2-progress.md` | 1936 | REP-02: legacy fixtures вне пользовательских примеров | historical_git_only |
| `docs/f2-progress.md` | 1952 | OSS: ближайший блок готовности к открытой разработке | historical_git_only |

The inventory has 101 headings: three release headings and ninety-eight
progress headings. It contains one permanent current snapshot and 100
historical Git-only records.

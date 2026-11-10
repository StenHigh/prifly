# План реализации: сохранённый выбор миграции project launch

Ветка: `prifly/prifly-aif-classic`.
Дата: 2026-11-10.
Базовый commit: `bc5604f0e99b9956cf1742f56af5c3561e50377e`.
Change: `openspec/changes/remember-launch-schema-migration`.

## Original Request

Implement openspec/changes/remember-launch-schema-migration: all open task ids; requirements in specs/ and design.md. Исходный запрос: issue #24 (https://github.com/StenHigh/prifly/issues/24). Владелец согласовал change: «Согласен с remember-launch-schema-migration. Зафиксируй change и запусти реализацию через prifly-run → aif-classic. Соблюдай AGENTS.md; без push». Выполнить все 24 задачи согласованного change. Сохранить terminal revoke, exact build scope, совместимость старых Runs и схем. Не менять рабочую private authority для проверки этой функции; использовать временные fixtures. Push не выполнять.

## Настройки

- Профиль: `full`, источник `project_default`.
- Тесты: `plan_tests=true`. Сначала получить падающий regression для нового поведения.
- Логирование: `minimal`. Каждый отказ использует runtime.Fault и Problem; stdout содержит один итоговый документ.
- Документационный checkpoint: `plan_docs=false`. Обязательные public references из change всё равно обновляются; `/aif-docs` не используется.
- Политика warnings: `ignore`. Ошибки и blockers сохраняют обязательность исправления.
- Ограничения взяты из decision_context: весь approved change, без push; сохранить terminal revoke, exact scope, старые Runs и public bundles.
- Git branch creation выключен. Использовать текущий claim; не создавать дополнительный worktree.
- Идентификатор плана получен из описания, поскольку `git.create_branches=false`. Старый план `prifly-prifly-aif-classic.md` относится к issue #15 и не изменяется.
- Следующие этапы получают этот файл через native workspace-tree port `plan`. Не выбирать старый план по совпадению branch stem.
- Источник progress — только change `tasks.md`. Таблица ниже задаёт привязки к коду без второго checklist.
- Research не влияет на план: `.ai-factory/RESEARCH.md` отсутствует.

## Roadmap Linkage

Milestone: `none`.
Change не называет milestone. Formal P1/P2 acceptance и private authority acceptance остаются отдельными.
Roadmap `openspec/specs/delivery-roadmap/spec.md` здесь read-only.

## Requirements Reconciliation

Приоритет: AGENTS.md → approved change specs/design → main source sets → project maps.
Main specs требуют явного разового ID. Approved delta уточняет источник выбора: owner может сохранить его заранее.
Это согласованное расширение, а не восстановление package trust.

| Правило или сочетание | Нормативный источник change | Проверка в реализации |
|---|---|---|
| Нет explicit и saved | specs/workflow-and-context — сохранённый выбор; specs/cli-protocol — Start | Прежний admission и прежняя DTO boundary |
| Saved действителен, exact scope совпал | specs/workflow-and-context — точный launch | Повторные CLI Runs с разными task inputs и одним Binding decision |
| Saved устарел или сборка изменилась | specs/workflow-and-context — точный launch | Именованный отказ до Run; сохранённая запись остаётся доступной |
| Explicit действителен, saved устарел | specs/cli-protocol — приоритет flag | Используется explicit; saved не меняется |
| Explicit недействителен, saved действителен | specs/cli-protocol — приоритет flag | Отказ без fallback |
| Пустое значение flag | specs/cli-protocol — приоритет flag | Отсутствие explicit; saved остаётся применимым |
| Другие authority/project/root/launch/build | specs/workflow-and-context — точный launch | Ни один scope не наследует разрешение |
| Set/clear без owner права | specs/control-security-ux — owner commands | object_access_denied; состояние не меняется |
| Replace либо clear/set после prepare | specs/control-security-ux — конкурирующие команды | Generation меняется; прежний review отказывает |
| Выбор меняется после чтения start | specs/control-security-ux — atomic Start | Runtime guard отказывает в транзакции Run/claim |
| Меняется другой launch | design — отдельный scoped объект | Его generation не инвалидирует текущий review |
| Повтор принятой start command после clear | design — review/replay | Ранний CLI receipt возвращает прежний Run |
| Supplier утратил trust или источник снова revoked | specs/control-security-ux; исходная migration policy | Существующие lifecycle/integrity checks блокируют допуск |
| Clear при ошибке compile или исчезнувшем launch | specs/cli-protocol — public управление | Canonical root и saved scope доступны без чтения полного profile |
| Уже принятый Run после clear | specs/published-contracts — существующий Binding | Binding и /45 сохраняются; обычный admission проверяет trust |

Представительный путь использует существующую CLI fixture из `cmd/prifly/schema_migration_test.go`.
Она собирает настоящий project package и выполняет reviewed migration через public CLI.
Добавить реальный compile-value drift по образцу `project_compile_parameters_test.go`.

## Решения для реализации

1. Хранить отдельный объект выбора с собственной schema `/1` и generation, включая tombstone после clear.
2. Сформировать ключ из canonical scope через полный SHA-256 и допустимое кодирование authority key. Сырой путь и полный hex segment не подходят.
3. Повторно использовать ранний LookupAuthorityReceipt для set/clear. Принятый retry не перепроверяет изменившийся мир.
4. Для Start добавить typed precondition и local.AuthorityGuard после окончательного выбора queueGuards в `internal/runtime/start.go`. Ветка QueueTask заменяет исходный список; guard выбора добавляется после неё.
5. Сохранить packageAdmissionGateWithMigration: он уже защищает migration lifecycle facts.
6. Для guard refusal использовать local.Reject; runtime.Fault внутри guard не даёт того же durable receipt.
7. В CLI выполнять resolver после compile и до review digest. Ранний accepted Project receipt lookup оставить перед resolver.
8. Inspect/clear должны находить canonical root без успешного разбора повреждённого profile. Использовать canonicalProjectPath и отдельный discovery существующего project scope.
9. Получать build key общим compiler путём. Не вычислять копию алгоритма в новом handler.
10. Добавить новый Project summary/start bundle `/10`, если номер остаётся свободен при реализации. Защитить `/9` от новых reflect-полей.

```text
owner inspect -> set exact choice -> prepare shows saved -> reviewed start
                      |                    |                    |
               versioned object       launch digest       atomic guard
                                                               |
                                                       Run + existing Binding
```

## Задачи

Каждая строка ссылается на одноимённый id в `openspec/changes/remember-launch-schema-migration/tasks.md`.
Порядок и критерий приёмки задаёт source task. Дополнения ниже определяют файлы и существующие образцы.
Для каждой задачи действует logging=minimal: типизированный отказ, штатный Problem или receipt; никаких новых фоновых логов.

| Source id | Файлы и конкретный ориентир | Проверка source task |
|---|---|---|
| 1.1 | cmd/prifly/schema_migration_test.go; новая CLI regression рядом с TestCLIProjectSchemaMigrationIsExplicitReviewedAndSerializedAtNine | До реализации новый set/повторный launch не проходит; Run/claim не остаются |
| 1.2 | openspec/specs/specification-governance/{terms,agent-brief}.md | TestGlossaryBindings; Choice не подменяет Selection, Decision или Binding |
| 1.3 | internal/runtime/launch_schema_migration.go и новый schema файл; cmd/schema-gen/ | Closed objects, неизвестная версия/поле, отсутствие объекта без записи |
| 2.1 | internal/runtime/launch_schema_migration.go; образцы readPackages и queueCommand | Scope isolation, generation и reopen; persisted absence после clear |
| 2.2 | Runtime inspection и cmd/prifly/project_schema_migration.go | Записанная часть доступна при compile/profile error; read-only state не меняется |
| 2.3 | Runtime set; ApplySchemaMigration как образец owner guard и early replay | Exact refs/build, чужой Decision, stale package/control facts |
| 2.4 | Runtime clear и CLI root discovery без полного profile | Удаление invalid/исчезнувшего launch, CAS и корректный no-op |
| 2.5 | internal/runtime/launch_schema_migration_test.go | Retry, conflict, concurrent CAS, потеря права на commit |
| 2.6 | internal/runtime/start.go, engine.go только при необходимости | Guard после ветки QueueTask, в одной transaction с Run/claim; internal/purity |
| 3.1 | cmd/prifly/project.go, project_schema_migration.go, main.go | Public help, flags, JSON и typed Problems |
| 3.2 | cmd/prifly/project_start.go; compileDeclaredProjectPackage и projectCompileLaunch | Explicit/saved/none, пустой flag; continue/recover сохраняют source restrictions |
| 3.3 | internal/projectwire/queue.go, cmd/schema-gen/{main,project_launch_schema_migration}.go, internal/runtime/contracts.go, scripts/check-schema.py | Actual summary/start JSON проходит новый bundle; `/9` не изменился |
| 3.4 | project_start.go review; runtime saved choice guard | Все review drift cases отказывают до Run/постоянного claim |
| 3.5 | project_start.go ранний NamedProjectLaunchResult/QueueLaunchCommand | Accepted retry возвращает прежний Run после смены выбора |
| 4.1 | CLI integration fixture: native project commands и два последовательных Runs | Без flag и копирования ID; reopen сохраняет Choice |
| 4.2 | Runtime/CLI regression tables, существующие migration и compile-value fixtures | Полная negative matrix source task; terminal revoke и старый Run сохраняют запрет |
| 4.3 | cmd/prifly/schema_migration_test.go; existing legacy/continue/recover suites | Нет Choice, explicit и standalone пути совместимы; queue + saved Choice сохраняет guard при generation drift |
| 4.4 | examples/operations/{index,commands,launch,schema-migration}.md; examples/authoring/schema-migration-guide.md | Public маршрут inspect/set/prepare/start/replace/clear; версия поддержки названа честно |
| 4.5 | internal/runtime/compatibility.go при новом capability; examples/README.md, operations/capabilities.md, SOURCE-OF-TRUTH.md | Capability index совпадает; embedded handbook fresh; ownership прежний |
| 5.1 | Затронутые Go packages и guards | Во время работы использовать выбранный source task fast gate; безопасные tests parallel |
| 5.2 | Change artifacts и общий diff | Строгая OpenSpec validation и whitespace check |
| 5.3 | Итоговый implementation snapshot и staged bytes | Sealed gate_checks; actual CHECK_CONTEXT=codex-app-authorized; единый Go 1.27.3 в PATH |
| 5.4 | Архив, старые schema bundles и release evidence | Diff от fixed base bc5604f0 не меняет защищённые файлы; новые bundles проверяются отдельно |
| 5.5 | Native implementation output и handoff следующему gate | Точные executed checks; source/release/private acceptance различаются |

Зависимости: раздел 1 → раздел 2 → раздел 3 → раздел 4 → итоговые проверки раздела 5.
Source task 2.2 делит CLI и runtime работу, но не разрешает пропустить CLI часть в разделе 3.
Новые runtime типы и glossary bindings должны появляться согласованно.

## Commit Plan

Логические checkpoints задают группы source tasks. Коммиты разрешает только native commit stage после итоговых ворот.
Не создавать промежуточный красный commit для первой regression.

| Checkpoint | Source tasks | Suggested commit message |
|---|---|---|
| 1 | 1.1–1.3 | docs(schema): define exact launch migration choice |
| 2 | 2.1–2.5 | feat(runtime): persist owner launch schema choices |
| 3 | 2.6–3.4 | feat(project): fence reviewed saved schema choices |
| 4 | 3.5–4.3 | test(project): preserve migration retry and scope boundaries |
| 5 | 4.4–5.3 | docs(project): expose saved migration launch operations |
| 6 | 5.4–5.5 | chore(project): record verified migration choice delivery |

Если native commit stage выбирает единый coherent commit, использовать `feat(project): remember exact launch schema migration choices`.
Push и merge в main выполняет владелец отдельно; этот Run их не выполняет.

## Проверенное baseline evidence

Команды выполнялись в actual claimed workspace до записи плана, а не в соседнем checkout:

- `openspec validate remember-launch-schema-migration --strict`: exit 0.
- `git diff --check`: exit 0 на исходном снимке.
- Go 1.27.3: `go test ./internal/runtime ./cmd/prifly -run 'SchemaMigration|GlossaryBindings' -count=1`: exit 0.
- Existing migration tests прошли: runtime 10.833s, CLI 4.156s. Это baseline, не проверка новой возможности.

На момент планирования новые tests и final gates из source tasks ещё не исполнялись.
Подготовительный full gate на main прошёл до commit bc5604f0; он не заменяет проверку implementation snapshot этого Run.
Полный diff получать заново через `git diff bc5604f0 --`; его текущий размер не закреплён в плане.

## Уточнение improve, одобренное владельцем

Decision `improve_apply=all` применена к задачам 2.6 и 4.3.
Добавлен порядок установки guard после выбора queueGuards и regression совместного queue + saved запуска.
Состав, IDs и порядок всех 24 source tasks сохранены.

## Handoff реализации

Все 24 source tasks отмечены в change `tasks.md`.
`make check-fast` и полный `ci-check-once CHECK_CONTEXT=codex-app-authorized CHECK_FOR_COMMIT=1` прошли.
Go toolchain: 1.27.3; полный batch выполнен, это не reuse.
OpenSpec strict validation: 41 passed; `git diff --check`: exit 0.
Отдельный собранный source CLI выполнил set, два prepare/start без ID и inspect.
Каждая CLI команда была отдельным процессом; временная authority удалена.
Старые bundles, archive и release evidence не изменены; добавлен новый launch-schema-migration bundle.
Нативные verify/security/review/commit следуют после implement в том же Run.
Source evidence не означает выпуск Core, private authority acceptance или закрытие roadmap gate.
Push и merge не выполнялись.

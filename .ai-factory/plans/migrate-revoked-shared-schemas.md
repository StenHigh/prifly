<!-- aif:plan-mode:full -->
# План реализации: migrate-revoked-shared-schemas

Ветка: `prifly/prifly-aif-classic`
Дата: 2026-11-09

## Original Request

Implement openspec/changes/migrate-revoked-shared-schemas: all open task ids (1.1–6.3); requirements in specs/ and design.md. Исходный запрос владельца: «переходи к реализации». Сохранить terminal revoke, старые Runs и audit; реализовать отдельные prepare/apply/inspect и явный выбор решения новым Run. Source tests не закрывают release acceptance. Private authority acceptance выполняется отдельно с владельцем.

## Настройки

- Проверки: да (`plan_tests=true`).
- Logging: minimal. Использовать существующие границы CLI Problem, receipt и audit. Не печатать содержимое schemas или подробный поток обхода. Это правило действует для каждой задачи.
- Документация: без отдельного шага aif-docs (`plan_docs=false`). Задачи 5.1–5.4 сохраняют обязательные public help, examples, словарь и embedded assets.
- Дополнительные ограничения: передано пустое значение. Правила проекта и change продолжают действовать.
- Предупреждения gate: ignore. Ошибка проверки остаётся ошибкой.
- Профиль: full; политика attended. Веткой управляет существующий claim.

## Связь с roadmap

Milestone: P2-07, `openspec/specs/delivery-roadmap/`.
Изменение исправляет package lifecycle. Оно не закрывает последующие этапы приёмки.

## Requirements Reconciliation

Приоритет источников: `specs/**` и `design.md` названного change, затем main specs по `openspec/SOURCE-OF-TRUTH.md`, затем контекст проекта.
Единственный журнал выполнения — `tasks.md` change. Этот план ссылается на его IDs без второго checklist.
Неразрешённых смысловых противоречий нет. Terminal revoke сохраняется. Явное исключение ограничено точными schemas.

| Решение или сочетание | Источник change | Требуемое доказательство |
|---|---|---|
| Read-only prepare, явный supplier, полный closure и неполная диагностика | design; workflow-and-context | 2.1–2.2: deterministic planner, ограниченный scan, отсутствие mutation |
| Атомарное решение owner, stale guards, receipt replay | control-security-ux | 2.4, 4.4–4.5: сохранённый typed отказ, управляемые гонки, duplicate receipt |
| Legacy authority `/1`; переход `/2` при первом успешном apply | published-contracts | 1.2, 2.3, 4.6: реальные serialized fixtures и lifecycle matrix |
| Run только с migration, только с queue, с обоими | published-contracts; существующий queue contract | 1.2, 3.1: migration-only 45 без queue; queue-only 44 без изменений; joint 45 с полной парой queue полей |
| Новый выбранный Run и old Run; schema и code | workflow-and-context | 3.1–3.4, 4.3: exact binding, последующие admissions, restart, отказы старому Run и executable refs |
| Явный выбор через run и Project | cli-protocol | 4.1–4.2: native CLI, Project summary/start 9, digest выбора |
| Source checks, released binary acceptance, private acceptance | задачи 6.1–6.3 | три отдельных статуса; source tests не закрывают 6.2 |

## Карта реализации

Перед правкой модели/API прочитать словарь и agent brief. Сохранить прежние bundles и historical release evidence побайтово.

1. Модель и serialization: `internal/runtime/model.go`, `config.go`, `invocation.go`, `versions.go`, `contracts.go`; `cmd/schema-gen/` и новые bundles. Текущая граница Run/read — 44; новая — 45. Текущая Project boundary — 8; новая — 9. Перед правкой повторно проверить свободные номера.
2. Lifecycle/planner: `internal/runtime/package_lifecycle.go` и отдельный модуль migration. Обновить все read/import/status readers `/1`. Учесть `retireEditions` вместе с прямыми lifecycle commands.
3. Authority: использовать `LookupAuthorityReceipt`, `AuthorityGuard`, `ReadAuthorityRuns`, snapshot scans и `ApplyAuthority` из `internal/local/`. Не создавать второй API без необходимости. Runtime/CLI не зависят от driver types.
4. Admission: `start.go`, общий package gate, `driver.go`, `check_execution.go`, `session_timing.go`. Вложенный Run требует нового явного выбора и проверки. Tool/code gate в `actions.go` не получает исключения.
5. CLI/Project: dispatch package commands, `cmd/prifly/project_start.go`, `internal/projectwire/queue.go`, generator Project owned naming. Включить выбор в review digest и достижимую boundary.

```
request -> bounded prepare -> owner apply -> decision
                                               |
explicit Start -> Run binding -> каждый admission
```

## Условия целостности и атомарности

- Planner читает согласованный набор packages/control/Runs. Проверять cut до и после нескольких запросов. Callback scan сам ограничивает число records и bytes. Неполный scan не выдаёт применимый plan.
- Вычислять declared consumer closure. Supplier непосредственно владеет schema и входит в closure. Проверять bytes до write-транзакции. Сортировать наборы по полному Ref и канонизировать digest.
- Хранить trust reason отдельно от status reason. Decision покрывает все revoked suppliers и их generations. Новая lifecycle command увеличивает generation, включая repeat revoke и retirement. Exact trusted reimport сохраняет generation и trust facts, даже если меняется общий cut.
- Сначала искать receipt команды. Только новая команда читает текущие bytes и строит решение. Изменённый payload прежнего command id даёт conflict. Retry возвращает исходный receipt после последующих изменений authority.
- В одном commit проверять packages, control, cut и версии влияющих Runs. Проверка только количества Runs недостаточна.
- `ApplyAuthority` сейчас сохраняет `version_conflict`, `runs_changed` и `claim_ownership_conflict`. Добавить узкую настройку внутреннего stale-отказа migration до сохранения receipt. Сохранить default semantics остальных callers и old receipts. Не переименовывать access denial или integrity failure.
- Рассчитать размер подготовленного state до transform. Превышение budget даёт typed migration refusal. Transform не читает файлы/blob и не запускает процессы.
- Хранить decisions и generations вместе в `authority-packages/2`. Последующие import/status commands сохраняют decisions. Не менять revoked status, old Runs или автоматически выбирать последнее решение.
- `queueBindingInvariant` сейчас требует queue для всех versions >=44. Сохранить queue-only 44. Версия 45 требует migration и допускает отсутствие queue либо полную валидную пару queue полей. Generator не должен переносить обязательность queue из 44 в migration-only 45.
- Presence/null decoder, MarshalJSON, выбор версии, reachable-state test и serialized read fixtures должны совпадать. Старые версии отвергают новое поле. Migration-only Start достигает 45 без queue.
- Для задачи 3.1 проверить принадлежность запускаемого workflow точному consumer package. Вычислить настоящий workflow closure. Binding содержит только нужные workflow одобренные schemas. Лишние package-level schemas не расширяют lock. Добавить отдельные проверки другого workflow и подмножества schemas.

## Задачи

Каждый пункт сохраняет ID, порядок и описание из change. Источник проверки — одноимённый пункт `openspec/changes/migrate-revoked-shared-schemas/tasks.md`. Logging каждой задачи следует настройкам выше.

### 1. Regression и compatibility contracts

**1.1** Добавить synthetic fixture с несколькими shared schemas, двумя последовательными шагами и старым Run. Проверка до fix: clean/remove успешны, revoke даёт `package_not_resolvable` и 0 новых Runs; положительный migration test воспроизводит отсутствие пути.

Источник проверки: change `tasks.md`, пункт 1.1.

**1.2** Добавить closed migration DTO, `authority-packages/2` и следующую свободную opt-in Run/Project boundary. Проверить реальные serialized fixtures новыми schemas; legacy `/1` и Run без binding сохраняют прежние shape/version.

Источник проверки: change `tasks.md`, пункт 1.2.

### 2. Planner, lifecycle generations и atomic apply

**2.1** Реализовать deterministic read-only planner полного consumer closure с явными schema suppliers. Проверить все refs, всех revoked источников, независимые trust/status reasons, supplier bytes и одинаковый digest для эквивалентного порядка ввода.

Источник проверки: change `tasks.md`, пункт 2.1.

**2.2** Добавить bounded scan влияющих текущих и сохранённых Runs, диагностику кандидатов и `applicable=false` при неполном выборе. Проверить отсутствие mutation, Run, claim и исполняемых hooks; превышение scan limit не выдаёт применимый план.

Источник проверки: change `tasks.md`, пункт 2.2.

**2.3** Реализовать переход `/1` → `/2` при первом apply и generations во всех package handlers. Проверить неизменность прежних status/trust facts, увеличение при новом повторном revoke, стабильность exact trusted reimport и сохранение decisions после последующих imports/status changes.

Источник проверки: change `tasks.md`, пункт 2.3.

**2.4** Реализовать owner apply и сохранённый decision одним `ApplyAuthority` с package/control/cut/Run guards. Проверить owner denial, digest mismatch, pure transform и отсутствие частичного разрешения; `internal/purity` и refusal guard должны проходить.

Источник проверки: change `tasks.md`, пункт 2.4.

### 3. Start и повторные admissions

**3.1** Добавить явный выбор decision в runtime Start и `SchemaMigrationBinding` нового Run. Проверить exact consumer/supplier/schemas, scope mismatch, запись достижимой state boundary и отказ без активного нового claim или Run.

Источник проверки: change `tasks.md`, пункт 3.1.

**3.2** Подключить binding к общему admission для следующего шага и вложенной работы. Проверить минимум два admissions после Start, engine restart и неизменность закреплённого supplier.

Источник проверки: change `tasks.md`, пункт 3.2.

**3.3** Сохранить запреты для старых Runs, trusted reuse, executable components и refs вне scope. Проверить old Run с тем же schema Ref, Start без выбора и попытку переноса binding в связанный Run без нового выбора/проверки.

Источник проверки: change `tasks.md`, пункт 3.3.

**3.4** Проверять актуальные generations и полный набор revoked совпадений при каждом admission. Проверить повторный revoke с новым command id, двух revoked поставщиков и появление нового отозванного источника после apply.

Источник проверки: change `tasks.md`, пункт 3.4.

### 4. Public CLI и поведенческая матрица

**4.1** Добавить public prepare/inspect и versioned JSON/human вывод. Native CLI test показывает несколько schema refs, все legacy package refs/reasons, suppliers и последствия для Runs без ручного чтения SQLite.

Источник проверки: change `tasks.md`, пункт 4.1.

**4.2** Добавить public apply, `run start --schema-migration` и Project questionnaire/start integration. Проверить owner reason/receipt, явный ID в review digest, Start mismatch и сохранённый decision после restart.

Источник проверки: change `tasks.md`, пункт 4.2.

**4.3** Выполнить отрицательную scope матрицу: другой consumer/digest, неполный plan, executable ref, новый supplier, непокрытый revoke и старый Run. Каждый случай должен сохранять trust/history и возвращать ожидаемый код отказа.

Источник проверки: change `tasks.md`, пункт 4.3.

**4.4** Проверить stale plan после package state, control policy, closure, supplier bytes и изменения версии влияющего Run без изменения количества Runs. Каждый отказ должен оставлять весь scope неприменённым.

Источник проверки: change `tasks.md`, пункт 4.4.

**4.5** Добавить конкурентные native tests apply/revoke, apply/Run change и Start/revoke. Проверить единый результат commit, отсутствие частичных разрешений, повтор принятого command id после потери ответа и conflict для изменённого payload.

Источник проверки: change `tasks.md`, пункт 4.5.

**4.6** Выполнить compatibility matrix clean/remove/quarantine/restore/dependency trust, old Run history, claims и audit. Проверить тот же public CLI без migration flags на authority `/1` и `/2`.

Источник проверки: change `tasks.md`, пункт 4.6.

### 5. Словарь и поставляемая документация

**5.1** Обновить `terms.md`, PackageStatus, Go/JSON bindings и agent brief вместе с implemented types. Выполнить `TestGlossaryBindings`; убедиться, что прежние термины и saved JSON не переименованы.

Источник проверки: change `tasks.md`, пункт 5.1.

**5.2** Обновить public help, `examples/operations/index.md`, `commands.md` и upgrade guidance. Проверить полный маршрут compile/import → prepare → apply → explicit Start и объяснение remove/revoke с holders/dependents.

Источник проверки: change `tasks.md`, пункт 5.2.

**5.3** Добавить capability с минимальной Core/contract boundary, строку `examples/README.md` и реальное поле binding в подходящий authoring/read reference. Выполнить `TestEveryDeclaredCapabilityIsInTheAuthorIndex`; не добавлять несуществующее YAML-поле миграции.

Источник проверки: change `tasks.md`, пункт 5.3.

**5.4** Обновить embedded operator/runner assets и `openspec/SOURCE-OF-TRUTH.md` ссылкой на active delta. Проверить delivered handbook и одинаковые команды; сохранить пользовательские `PROJECT.md` и ownership main source sets.

Источник проверки: change `tasks.md`, пункт 5.4.

### 6. Итоговая проверка и release acceptance

**6.1** Выполнить focused native tests, `make test-changed`, `make schemas-check`, `make refusal-check`, `make vet`, `openspec validate migrate-revoked-shared-schemas --strict --no-interactive` и `git diff --check`. Для итогового implementation snapshot выполнить один `make ci-check-once CHECK_CONTEXT=issue-21-schema-migration`. Зафиксировать результаты и проверить отсутствие правок `openspec/changes/archive/**`, прежних schema bundles и historical release evidence.

Источник проверки: change `tasks.md`, пункт 6.1.

**6.2** После release gates на фиксированном SHA выполнить generic public-CLI acceptance released binary. Сохранить version/digest, исходный отказ, несколько schemas, apply, новый Run с последующей работой, old Run refusal, restart и cleanup всех временных authority/monitor registrations. Source tests не закрывают эту задачу.

Источник проверки: change `tasks.md`, пункт 6.2.

**6.3** Подготовить owner handoff для private authority acceptance со списком точных команд и проверок. Отдельно указать её фактический статус; без отдельного участия владельца не выполнять её и не заявлять ремонт живой установки. Проверка завершения — документированный handoff и граница generic/private результатов.

Источник проверки: change `tasks.md`, пункт 6.3.

## План commits

Контракты, authority и admission связывают задачи 1.1–5.4. Промежуточная работа остаётся локальной до успешных проверок. Не коммитить отдельно заведомо падающие regression tests.

- Рубеж A: задачи 1.1–2.4. Проверить diff модели/planner/transaction и focused tests. Commit допустим, если этот срез собирается и сохраняет прежние контракты.
- Рубеж B: задачи 3.1–5.4. Проверить public behavior вместе с документацией. Предпочесть единый feature commit, если рубеж A не образует самостоятельный срез.
- Рубеж C: задача 6.1 и handoff 6.3. Commit проверенной реализации содержит честный статус оставшейся приёмки. Задача 6.2 остаётся открытой до проверки выпущенного бинарника.

## Выполненные baseline проверки

Точный claimed worktree, 2026-11-09:

- `openspec validate migrate-revoked-shared-schemas --strict --no-interactive`: PASS.
- `git diff --check`: PASS.
- Пять runtime tests terminal revoke, revoked admission, одного resolvable supplier, quarantine и dependency withdrawal: PASS, 0.322s. Log: `/tmp/prifly-issue21-plan-baseline-tests.log`.
- Public reproduction выпущенного 0.13.88 закреплена в change `evidence/baseline.md`: clean/remove успешны; revoke запрещает Start без создания Run. Это baseline, а не приёмка migration.

Будущие проверки заданы задачей 6.1 и handed gate_checks. Полный gate выполняется на итоговом снимке с Go 1.27.3 в PATH. Фактический permissions context — `codex-app-authorized`; профиль change — `issue-21-schema-migration`. Не использовать evidence другого снимка или профиля. Source gates не заменяют release race/e2e.

При ревью заново получать состав diff из текущего дерева. Не записывать в план изменяемые количества строк.

## Применённые уточнения

Владелец выбрал `improve_apply=all`: перевести текст плана на русский и уточнить проверки задачи 3.1. IDs и scope всех 23 задач сохранены. `Original Request` сохранён побайтово.

## Граница завершения

Workflow выполняет source implementation и owner handoff. Задача 6.2 требует последующего released-binary evidence. Push, release, owner merge и ремонт private authority не входят в разрешённые действия этого Run.

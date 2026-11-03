# План реализации: входы именования проекта

Branch: `prifly/prifly-aif-classic`
Created: 2026-11-03

## Original Request

Implement openspec/changes/add-project-owned-worktree-naming-inputs: all; requirements in specs/ and design.md.

Запрос владельца: «сделай aif-warmup и возьми в работу issue 19». Владелец подтвердил proposal для реализации через aif-classic. Дополнительных ограничений нет; соблюдать согласованный OpenSpec change.

Issue: https://github.com/StenHigh/prifly/issues/19

На Pri-Fly 0.13.87 требуется документированный способ передать отдельные typed inputs для worktree_naming через конфигурацию потребителя. Если такой способ уже есть, достаточно executable example.

Публичный project profile /4 связывает naming с inputs workflow. Официальный aif-classic v1.57.0 принимает task и security_enabled. Его task schema закрыта и не объявляет source/id. Extension /5 описывает points, но не способ добавить отдельный вход корневого workflow.

Источники:
https://github.com/StenHigh/prifly/blob/v0.13.87/examples/authoring/project-profile-authoring-reference.yaml
https://github.com/StenHigh/prifly/blob/v0.13.87/examples/authoring/unified-composition-authoring-reference.yaml
https://github.com/StenHigh/prifly-aif-workflows/blob/v1.57.0/aif-classic/workflow.yaml

Ожидание: использовать существующий механизм либо минимальный project-owned контракт. Не нужно добавлять metadata в каждую продуктовую task schema, копировать development graph или править bytes upstream. Core не должен знать конкретный трекер, приложение или формат имени.

Приёмка:
1. Неизменённый официальный workflow получает отдельные typed naming inputs. Prepare показывает имя, start создаёт ожидаемый worktree.
2. Смена inputs после prepare даёт stale refusal. Missing/invalid metadata отказывает до claim. ID не выводится из prose.
3. Package update сохраняет конфигурацию потребителя; upstream files остаются точными. Пример охватывает install/update/prepare/start.
4. Continue/resume сохраняет исходные naming inputs/claim без нового default. Старые Runs сохраняют прежнее поведение.
5. Generic native probe работает на released Core без LLM и приложения.

Не входит: новый runner, registry, универсальный объект бизнес-данных или автоматическое извлечение ID из текста.

## Settings

- Testing: yes; выполняются проверки из исходных задач.
- Logging: minimal; существующий Problem JSON, без дополнительной печати в CLI.
- Docs: no для отдельного aif-docs checkpoint; author/operations references обязательны по change.
- Profile: full; decision policy: attended.
- Ограничения: дополнительных ограничений нет; сохранить согласованный OpenSpec contract.
- Gate checks: точный `decision_context.gate_checks` данного Run, включая исправленный PATH и CHECK_CONTEXT.
- Runtime improve_apply: preanswer отсутствует; изменения scope требуют фактического решения владельца.

## Roadmap Linkage

Milestone: "none"
Rationale: change не называет product milestone; roadmap остаётся read-only, согласно project overlay aif-plan.

## Requirements Reconciliation

Authority: явный ответ владельца от 2026-11-03 о 128 KiB > `openspec/changes/add-project-owned-worktree-naming-inputs/specs/**` и `design.md` > main source sets > AI-context/roadmap.
Источник задач и единственный progress ledger: [openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md](../../openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md). Число и порядок задач сохранены; этот план не дублирует checkboxes.

| Комбинация / правило | Источник | Проверка |
|---|---|---|
| `/2`–`/4`, прежние bindings | `openspec/changes/add-project-owned-worktree-naming-inputs/specs/published-contracts/spec.md` | Старые bytes/markers; новые поля под `/4` отказываются. |
| `/5`, новое worktree, naming inputs | `openspec/changes/add-project-owned-worktree-naming-inputs/specs/workflow-and-context/spec.md` | Typed JSON, exact namespace, полный snapshot ≤131072 bytes; expected basename. |
| `/5`, checkout/no Workspace | Та же delta | Values не требуются; явный неприменимый flag отказывается. |
| Новое worktree и preflight/queue | `openspec/changes/add-project-owned-worktree-naming-inputs/specs/cli-protocol/spec.md` | Missing/invalid/stale/oversized snapshot не создаёт preflight/queue/claim effects. |
| Canonical JSON и pointers | Та же delta | Whitespace сохраняет canonical value; изменение любого поля или schema даёт stale. |
| Collision после prepare | Main naming contract + CLI delta | Candidate не зарезервирован и исключён из review digest; suffix остаётся first-free/1. |
| Handover / resume / recover | `openspec/changes/add-project-owned-worktree-naming-inputs/specs/runtime-resources/spec.md` | Тот же snapshot/path/branch; штатное повышение generation. |
| Новый continuation claim от commit | Та же delta | Исходная основа и snapshot, новый допустимый suffix; current policy не подставляется. |
| Snapshot внутри claim, 128 KiB | Ответ владельца + уточнённый design §4 | Граница 131072/131073, repeated carriers и current full record проверяются bounded. |
| Queue + unified composition | `openspec/changes/add-project-owned-worktree-naming-inputs/specs/published-contracts/spec.md` | `/8` выбирается последним; реальные DTO проходят новую closed schema. |
| Официальный workflow и consumer update | Workflow/CLI deltas | Неизменённый v1.57.0, closed task schema, exact upstream bytes и сохранённые consumer files. |
| Source-built / released | Design §6, задача 5.4 | Released приёмка остаётся отдельной до публикации exact Core. |

### Разрешённое уточнение владельца

Вопрос: «Как уточнить контракт issue №19? ... snapshot внутри claim с общим пределом 128 KiB для declaration, schemas и values».
Ответ: «Snapshot внутри claim, максимум 128 KiB».
По этому ответу согласованно обновлены existing proposal/design/specs/tasks в этом worktree. Намерение change сохранено.

### Точки интеграции и риски

- Profile gates: `projectNeutralProfile` и `readProjectProfile` должны явно принять `/5`.
- Freshness: `projectPrepareAndStart` сейчас запускает non-queue preflight раньше compilation; queue reservation тоже предшествует late reread.
- Безопасные пути: использовать образец `projectExecutionSource` для всех компонентов пути schema; `readFile` защищает только конечный файл.
- Schema validation: временный `flow.Registry`, существующий `flow.ValidateSchema`, fragment-only references; helper выполняется вне transform.
- Snapshot: отдельное поле полного claim; semantic equality и deep copy, поскольку allocation сравнивается оператором !=.
- Decode: новый `/5` record использует bounded strict state decoder; command/result ceilings остаются 1 MiB, state ceiling 16 MiB.
- Замороженные generators: `worktreeNamingField` и content-addressed imported definitions не должны пропустить новые fields в старые bundles.
- Полные nested claims: summary.worktree_claim, start.workspace/launch_summary; reduced ContinuationClaim можно сохранить, если snapshot туда не добавляется.
- Recovery с отсутствующим рабочим claim сохраняет прежний отказ; naming metadata не даёт нового права восстановления.

```text
profile + schemas + values
          |
          v
validate + snapshot budget + review digest
          |
          v
freshness before preflight/queue
          |
          v
claim reservation -> worktree -> Run
          |
          v
source snapshot -> handover / new continuation claim
```

## Commit Plan

- Группа 1, задачи 1.1–1.4: `feat(project): declare typed naming inputs outside workflows`.
- Группа 2, задачи 2.1–2.4: `feat(project): review naming snapshots before launch effects`.
- Группа 3, задачи 3.1–3.5: `feat(runtime): preserve bounded naming snapshots across claims`.
- Группа 4, задачи 4.1–4.4: `docs(project): demonstrate consumer-owned worktree naming`.
- Группа 5, задачи 5.1–5.3: `test(project): verify naming snapshot compatibility and handoff`.
- Task 5.4 остаётся внешним released checkpoint после публикации владельцем; её нельзя отметить source tests.
- Если файлы групп пересекаются, staging выполняется по фактическим hunks; checkpoints не разрешают незелёный final candidate.

## Tasks

Все IDs ниже ссылаются на неизменённые IDs исходного `tasks.md`. Название и критерий каждой задачи определяются исходной строкой.

| ID | Файлы / область | Практический шаг | Проверка |
|---|---|---|---|
| [1.1](../../openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md) | `internal/projectwire/queue.go`, `cmd/schema-gen/*`, `schemas/` | Зафиксировать SHA `3d8fd301bccddd18a5c2be752570918439eb1c5d`, carriers и digests прежних bundles; сохранить evidence в change при реализации. | Исходная проверка задачи 1.1; commands/scenarios из tasks.md. |
| [1.2](../../openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md) | `cmd/prifly/project_worktree_naming_inputs_test.go` | Воспроизвести отдельный naming input с closed task schema; сначала получить ожидаемый red до реализации flag/profile. | Исходная проверка задачи 1.2; commands/scenarios из tasks.md. |
| [1.3](../../openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md) | `cmd/prifly/project.go`, `project_build.go`, `project_worktree_naming.go`, новый `project_naming_inputs.go`, новый authoring schema `/5` | Включить `/5` только явно; declarations и bindings разбирать закрыто; использовать безопасный обход directories, а не только O_NOFOLLOW последнего файла. | Исходная проверка задачи 1.3; commands/scenarios из tasks.md. |
| [1.4](../../openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md) | `openspec/specs/specification-governance/terms.md`, `agent-brief.md`, `openspec/SOURCE-OF-TRUTH.md` | Добавить понятие Project naming input и фактические Go/JSON mappings; source ownership сохранить. | Исходная проверка задачи 1.4; commands/scenarios из tasks.md. |
| [2.1](../../openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md) | `cmd/prifly/project.go`, `project_start.go`, `project_naming_inputs.go` | Выделить отдельный повторяемый flag с typed duplicate/unknown refusals; анкету и Workflow inputs не смешивать. | Исходная проверка задачи 2.1; commands/scenarios из tasks.md. |
| [2.2](../../openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md) | `cmd/prifly/project_start.go`, `project_questionnaire.go`, `project_extension_call.go` | Проверить snapshot до non-queue preflight и до QueueReserve/Dispatch; повторно проверить перед claim. В digest входят все values/schema fields. | Исходная проверка задачи 2.2; commands/scenarios из tasks.md. |
| [2.3](../../openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md) | `cmd/prifly/project_worktree_naming_inputs_test.go`, `project_launch_summary_test.go` | Сравнивать authority/registry/claims/topology до и после prepare; availability остаётся вне digest; проверить изменение файла через существующий writer hook. | Исходная проверка задачи 2.3; commands/scenarios из tasks.md. |
| [2.4](../../openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md) | `cmd/prifly/project_start.go`, compile/launch tests | Проверить одинаковые compiled refs при изменении naming values; checkout/no Workspace не требуют metadata, явный неприменимый flag отказывает. | Исходная проверка задачи 2.4; commands/scenarios из tasks.md. |
| [3.1](../../openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md) | `internal/runtime/worktrees.go`, `worktree_names.go`, новый snapshot helper и тесты | Сохранить отдельный immutable snapshot на claim. Проверить 128 KiB, serialized before/after/results, state ceiling и idempotency; не помещать maps в сравниваемый allocation. | Исходная проверка задачи 3.1; commands/scenarios из tasks.md. |
| [3.2](../../openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md) | `internal/runtime/claim_run.go`, `cmd/prifly/project_start.go`, resume/recovery tests | Передавать полный source snapshot вместе с claim, path/branch и generation; не читать удалённые текущие schemas/values. | Исходная проверка задачи 3.2; commands/scenarios из tasks.md. |
| [3.3](../../openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md) | `internal/runtime/continuation.go`, `recovery.go`, `cmd/prifly/project_continuation.go`, `project_start.go` | Для нового claim от commit найти source metadata по Run ancestry даже при released source claim; current policy не подставлять; прежний recover_workspace_missing сохранить. | Исходная проверка задачи 3.3; commands/scenarios из tasks.md. |
| [3.4](../../openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md) | `internal/projectwire/queue.go`, `cmd/schema-gen/main.go`, новый generator, `internal/runtime/contracts.go`, `scripts/check-schema.py`, новые core schemas | Добавить snapshot-aware `/8`, questionnaire `/5`, claims `/5` и read `/4`; старые reflection generators заморозить. Новую версию выбирать после composition и до QueueFinalize. | Исходная проверка задачи 3.4; commands/scenarios из tasks.md. |
| [3.5](../../openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md) | Runtime/CLI границы и затронутые tests | Сохранять pure transform payload, typed faults и CGO-disabled driver boundary; выполнить исходные guard commands. | Исходная проверка задачи 3.5; commands/scenarios из tasks.md. |
| [4.1](../../openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md) | `examples/README.md`, `examples/authoring/`, `examples/operations/`, embedded handbook assets, capabilities | Показать `/5`, schema/value files, flag, 128 KiB и minimum wire contracts; объявить capability и строку author index вместе. | Исходная проверка задачи 4.1; commands/scenarios из tasks.md. |
| [4.2](../../openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md) | Новый consumer example и integration fixture | Pinned официальный `aif-classic v1.57.0`; install/update/prepare/start, exact upstream inventory и неизменные task schema/graph; consumer files переживают update. | Исходная проверка задачи 4.2; commands/scenarios из tasks.md. |
| [4.3](../../openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md) | Новый generic probe в `scripts/` либо `test/` и fixture в `examples/` | Exact executable argument, минимальный assisted repository-writing contract и synthetic session reports; без LLM/tracker/application. Не менять managed scratch semantics. | Исходная проверка задачи 4.3; commands/scenarios из tasks.md. |
| [4.4](../../openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md) | Evidence текущего change | Связать пять критериев issue с фактическими tests; различить source-built probe, official package journey и released binary. | Исходная проверка задачи 4.4; commands/scenarios из tasks.md. |
| [5.1](../../openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md) | Staged candidate actual claim worktree | Выполнить source commands из исходной задачи и запечатанный decision_context.gate_checks; CHECK_CONTEXT фактический, Go 1.27.3 доступен также через PATH. | Исходная проверка задачи 5.1; commands/scenarios из tasks.md. |
| [5.2](../../openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md) | Protected schemas и archived evidence | Сверить старые bytes с baseline; новые schema files допустимы, modified/deleted прежних bundles нет. | Исходная проверка задачи 5.2; commands/scenarios из tasks.md. |
| [5.3](../../openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md) | Release handoff и commit-stage report | Зафиксировать actual candidate SHA и probe invocation после source quality gates; exact-SHA verify/qualify и публикация остаются действиями владельца. | Исходная проверка задачи 5.3; commands/scenarios из tasks.md. |
| [5.4](../../openspec/changes/add-project-owned-worktree-naming-inputs/tasks.md) | Released Core и внешний приёмочный gate | После публикации владельцем запустить probe/journey на exact released executable. До этого задача остаётся открытой; source success не подменяет её. | Исходная проверка задачи 5.4; commands/scenarios из tasks.md. |

Логирование для всех задач: minimal, typed Fault/Problem; не добавлять stdout или отдельный logger.

## Проверенные команды планирования

Эти команды выполнены на текущем worktree. Они проверяют planning snapshot и не доказывают реализацию feature.

```sh
openspec validate add-project-owned-worktree-naming-inputs --strict
git diff --check
git diff --stat 3d8fd301bccddd18a5c2be752570918439eb1c5d -- cmd internal schemas examples scripts openspec
```

Статистику изменений получать заново этой командой; числовая сводка diff здесь не закрепляется.

Future acceptance commands принадлежат исходным tasks.md и запечатанному gate_checks. При реализации требуется фактический результат на candidate bytes.

## Граница готовности

Engineering Run реализует source scope и готовит release handoff. Formal P1/P2 acceptance и task 5.4 не закрываются этим Run автоматически.
Claim branch сохраняется до owner merge/push. План удаляется вместе с archive change, а не через aif-archive.

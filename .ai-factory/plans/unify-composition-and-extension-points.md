# План реализации: единый composition и точки расширения

Branch: `prifly/prifly-aif-classic`
Created: 2026-11-01

## Original Request

давай спроектируем единый контракт для composition и точек расширения, сохранив совместимость с опубликованным /4

Твоя цель довести эту задачу до рабочего релиза

## Settings

- Testing: yes (`plan_tests=true`).
- Logging: minimal; typed Fault/Problem и deterministic review, без новых stdout logs.
- Docs: no checkpoint; обязательные references и author index входят в OpenSpec change.
- Constraints: исполнять `plan_constraints` Run; опубликованный /4, historical evidence и preserved worktree/stash сохраняются.

## Roadmap Linkage

Milestone: "none"
Rationale: change не закрывает P1/P2/F2 или downstream consumer qualification.

## Requirements Reconciliation

Authority: `openspec/changes/unify-workflow-composition-and-extension-points/specs/**` и `design.md`.
Published baseline: v0.13.83 / `389808611e40f17d5bbcedebf4580e0db93eb06b`.
Composition base: `30131326c22fb1dc3d378af0153358fc44eccc99`.
Planning checkpoint: `4c4a9cd62b75883592c957ecd7d7b639afac0def`.

| Сочетание | Требуемое поведение | Evidence |
|---|---|---|
| Прежний /4 без opt-in | прежние b1 refs, bytes, DTO и refusals | exact published binary baseline и unchanged native harness |
| Root /5 или folder /3 | общий compiler с positive owner map | mixed native harness и adversarial sources |
| Два aliases / общий child | scope-specific policy, dedup одинакового варианта | exact refs и независимые outputs |
| Point group без consumers | declared routes и native sequence/parallel/choice | outcomes, failures, disabled/default |
| Point group с consumers | typed refusal; один explicit typed Call допустим | negative case и carrier Call |
| Release | exact-SHA verify/qualify и published replay | CI и archive/binary hashes |

Новых runtime operators и AggregateManifest dereference нет. Source imports и variant dependencies имеют разные DAG.
Новые версии: extension /5, folder /3, compile /4, summary/start /7, provenance /3 b2, composition /2.
Legacy functions сохраняют старый dispatch. Новый compiler планирует owner/variant до flattening и не определяет владельца по ID.

## Tasks

Единственный progress ledger — OpenSpec `tasks.md`. Таблица ссылается на его ids; она не создаёт второй набор checkboxes.
Порядок: 1 → 2 → 3 → 4 → 5 → 6 → 7 → 8. Пункты одной группы выполняются по их зависимостям.
Каждый пункт использует проверку своей строки. Новые проверки сначала воспроизводят отсутствие /5; полные ворота исполняются на итоговом снимке.

| ID | Источник progress | Действие и проверка |
|---|---|---|
| 1.1 | [Задача 1.1](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Подготовить implementation branch на базе с commit `30131326`; явно зафиксировать отношение к released `38980861`. Проверка: reproducible base refs и отсутствие merge/release утверждений в planning evidence. |
| 1.2 | [Задача 1.2](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Снять deterministic `/4` baseline на exact published v0.13.83. Проверка: package/build refs, native definition bytes, DTO versions, disabled behavior, effect/consent guards и typed refusals в новых immutable fixtures. |
| 1.3 | [Задача 1.3](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Зафиксировать protected manifest существующих schemas, archived changes и release evidence относительно согласованной базы. Проверка: повторное сравнение каждого исходного path/hash/mode; новые additive файлы не заменяют старые. |
| 1.4 | [Задача 1.4](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Добавить closed authoring schemas extension `/5` и folder `/3` с exact selectors и `exports.points`. Проверка: schema positive/negative cases; `/4` и folder `/2` отвергают новые fields; sidecar `/1` byte-identical. |
| 1.5 | [Задача 1.5](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Добавить новые Project DTO и schemas: compile `/4`, summary/start `/7`, provenance `/3`, composition `/2`. Проверка: `make schemas-check`; generated/public/embedded pairs совпадают, emitted fixtures проверяются своими версиями. |
| 2.1 | [Задача 2.1](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Добавить общий typed compiler representation с source owner, origin, author identity, attachment и implementation origin. Проверка: colliding ID/kind из разных owners не даёт полномочий после flattening. |
| 2.2 | [Задача 2.2](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Сохранить старый dispatch без нового root opt-in; добавить явный dispatch `/5` либо root folder `/3`. Проверка: differential `/4` baseline и named refusal нового provider под старым root. |
| 2.3 | [Задача 2.3](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Нормализовать parent `/3` и `/4` по собственным closed readers. Проверка: `/4` не получает imported target или object selector, даже при unified consumer. |
| 2.4 | [Задача 2.4](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Разрешать local/inherited, direct-parent и import point selectors без глобального поиска. Проверка: ambiguous names, unknown parent/import, wrong kind и guessed private target дают refusal. |
| 2.5 | [Задача 2.5](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Проверять исходную sidecar inventory до flattening и сохранять root/child mapping при inheritance. Проверка: derived root points, child/repeat body и foreign/project-owned target substitution. |
| 3.1 | [Задача 3.1](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Разрешить `exports.points` только через local point и public workflow export folder `/3`. Проверка: hook вне reachable closure, private/unowned target и root-launch export отвергаются. |
| 3.2 | [Задача 3.2](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Добавить scoped selection по import alias и workflow export. Проверка: два aliases одного provider создают configured и passthrough variants без source mutation. |
| 3.3 | [Задача 3.3](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Реализовать зависимые variants для exports с общим nested child. Проверка: hook одного export не меняет другой; same effective policy внутри сборки даёт один exact variant. |
| 3.4 | [Задача 3.4](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Разделить source cycles и definition/variant dependency cycles. Проверка: допустим consumer-owned implementation внутри provider point; actual Call cycle, recursive point dependency и repeated-alias budget reset отвергаются. |
| 3.5 | [Задача 3.5](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Связать programs только с owned selected variants; сохранить source directory и каждый прочитанный supporting path. Проверка: private missing file не читается, foreign binding отказывает, shared exact program сохраняет все paths. |
| 3.6 | [Задача 3.6](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Ввести общий предел 256 workflow variants и сохранить остальные compile/native budgets. Проверка: 256/257, cached depth 8/9, folders/layers limits и generated-wrapper accounting. |
| 4.1 | [Задача 4.1](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Определить canonical variant-key record без compiled refs и final package digest. Проверка: fixed vectors для owner, effective settings, point policy и dependency keys; map traversal не меняет key; перестановка parent declarations не разрешает conflicts и сохраняет нормализованный effective graph. |
| 4.2 | [Задача 4.2](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Добавить b2 build key и compiled-version tuple из design, сохранив b1 code path. Проверка: exact formula vectors, distinct variants получают разные refs, unchanged `/4` b1 vector совпадает с baseline. |
| 4.3 | [Задача 4.3](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Добавить provenance `/3` writer и reader с mapping source/author/variant/compiled. Проверка: package round-trip, несколько variants одной author identity, forged mapping и swapped owner/ref отвергаются. |
| 4.4 | [Задача 4.4](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Обновить package sealing снизу вверх без изменения source IDs. Проверка: nested Call/Repeat refs указывают на правильные variants, внешние exact refs и literal data не переписываются. |
| 5.1 | [Задача 5.1](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Разрешить `/5` point leaf с local project implementation либо explicit imported export. Проверка: unknown implementation, wrong kind, provider program override и client routes/inputs/consumers отвергаются. |
| 5.2 | [Задача 5.2](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Проверять exact schema, availability и каждый declared consumer до sealing. Проверка: reader и repeat next_bindings получают новый carrier; missing/partial output не заменяется прежним. |
| 5.3 | [Задача 5.3](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Сохранить disabled passthrough и closure-scoped execution consent. Проверка: disabled imported program не читается и не требует allow-execution; active program использует allowlist/review. |
| 5.4 | [Задача 5.4](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Объединить parents/layers по source declarations и ancestry. Проверка: common ancestor once, explicit duplicate layer refusal, opaque replacement, independent conflicts и consumer resolution. |
| 5.5 | [Задача 5.5](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Добавить point arrange только для empty-consumer point и compatible step/call routes. Проверка: sequence/parallel/choice, unknown/default, distinct failure routes, missing/mismatched on_error и impossible_verdicts. |
| 5.6 | [Задача 5.6](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Сохранить origins и inherited groups при следующих уровнях composition. Проверка: каждый active member один раз; duplicate/unknown/disabled/missing selector и nested group depth дают named refusal. |
| 5.7 | [Задача 5.7](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Сохранять technical failures без outcome до outer point on_error. Проверка: Step и Call failure в sequence/parallel/choice; branches settle; publication после ошибки не происходит. |
| 5.8 | [Задача 5.8](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Отказать на automatic group с consumer ports и на overlap root/point anchors. Проверка: exact unsupported/conflict codes, absence partial output, отдельный typed Call с carrier продолжает работать. |
| 6.1 | [Задача 6.1](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Выдавать новые DTO только при новом root opt-in и сохранить naming/queue/recovery fields. Проверка: real compile/prepare/start/continue fixtures проходят новые closed schemas; старые version paths не меняются. |
| 6.2 | [Задача 6.2](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Показать owner/variant/point/implementation/consumer mapping и disabled choices в review/explain. Проверка: deterministic relative paths, scoped aliases и отсутствие programs/claims/authority mutation при prepare. |
| 6.3 | [Задача 6.3](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Включить новые inputs в freshness digest и сохранить source-aware typed Fault. Проверка: изменение export, sidecar, layer, settings или supporting bytes отменяет старый review до claim/dispatch. |
| 6.4 | [Задача 6.4](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Добавить capability `workflow_composition_with_points`, author index и reference с actual `/5`, folder `/3`, selectors и exports. Проверка: `TestEveryDeclaredCapabilityIsInTheAuthorIndex`, обе стороны таблицы и ручное ревью minimum native revisions. |
| 6.5 | [Задача 6.5](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Обновить glossary, agent brief и source-of-truth map в scope нового delta. Проверка: `TestGlossaryBindings`; existing terms и saved JSON names сохранены; main `/4` rules и historical evidence не переписаны. |
| 6.6 | [Задача 6.6](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Подготовить runnable generic example с parent `/4`, imports и public nested hook. Проверка: examples показывают zero-consumer UI+audit group и отдельный typed carrier Call; явно отмечены synthetic и реальные browser/release gates. |
| 7.1 | [Задача 7.1](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Добавить regression-first проверки mixed source contracts и adversarial ownership. Проверка: red на прежнем candidate, green после реализации; новые Go tests используют `t.Parallel()`, кроме process/global mutations. |
| 7.2 | [Задача 7.2](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Запустить полный unchanged `/4` harness и новый mixed harness на одном exact binary. Проверка: identity baseline, outputs, disabled/effects/consent, applicability required/not-required/unknown, isolated variants, groups и freshness. |
| 7.3 | [Задача 7.3](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Проверить partial/cancel/continue на native root frontier и fresh nested ArtifactRef. Проверка: source/target Runs и exact package refs записаны; stale carrier reuse и nested-stage resume не объявлены допустимыми. |
| 7.4 | [Задача 7.4](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Выполнить `make test-changed` либо `make check-fast` по текущему diff и `openspec validate --all --strict --no-interactive`. Проверка: реальные логи без заявления feature qualification по одной document validation. |
| 7.5 | [Задача 7.5](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Выполнить финальные `make ci-check-once CHECK_CONTEXT=<actual-profile> CHECK_FOR_COMMIT=1` на Go проекта и `git diff --check`. Проверка: staged bytes, receipt context, fresh vuln/mod verify и повторный protected-file manifest check. |
| 7.6 | [Задача 7.6](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Провести security и independent review source/variant ownership и b2 compatibility. Проверка: findings разрешены, docs/evidence называют конкретный candidate; local commit создаётся только по поручению владельца. |
| 8.1 | [Задача 8.1](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Перед выпуском запустить `python3 scripts/ensure-release-checks.py --sha SHA --ref main --wait` на фиксированном release commit. Проверка: зелёные exact-SHA verify/qualify; публикация требует отдельного поручения. |
| 8.2 | [Задача 8.2](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Выпустить через `python3 scripts/tag-release.py vX.Y.Z` и повторить оба CLI harness на exact published binary. Проверка: version/SHA256/signatures, unchanged `/4` и mixed `/5` evidence отдельно от source candidate. |
| 8.3 | [Задача 8.3](../../openspec/changes/unify-workflow-composition-and-extension-points/tasks.md) | Передать unified authoring downstream workflows и отдельно провести живую consumer приёмку. Проверка: реальные browser lifecycle, permissions, fresh candidate correlation и owner acceptance; Core fixture не закрывает F2/P1/P2/smsplace. |

## Точки реализации

- `cmd/prifly/project_composition*.go`: versioned source readers, ownership, imports/parents/layers, scoped variants и groups.
- `cmd/prifly/project_extension_points.go`: сохранить /4; unified typed point plan применяет те же native port/route guards.
- `cmd/prifly/project_compile.go`, `project_build.go`: unified dispatch до legacy validation, temporary compile identities, b2 bottom-up sealing.
- `cmd/prifly/project_execution*.go`: programs связывать через owner/variant, не через неоднозначный AuthorRef map.
- `internal/projectwire`, `cmd/schema-gen/workflow_composition*.go`: additive DTO types и schemas; старый /1 bundle frozen.
- `internal/runtime/authoring`, `schemas/authoring`: новые closed forms, существующие schemas byte-identical.
- `test/e2e/verify-extension-points.py`, `verify-workflow-composition.py`, новый mixed harness: один exact candidate и published replay.

## Проверенные команды для исходной базы

- `go test ./cmd/prifly -run 'TestProject(Point|ExtensionPoint|Composition)' -count=1`: PASS, 8.833s; `/tmp/prifly-unified-baseline-tests.log`.
- `openspec validate --all --strict --no-interactive`: PASS 41/41; `/tmp/prifly-unified-plan-openspec.log`.
- `git diff --check`: PASS до записи плана.

Эти результаты относятся к исходной базе. Они не доказывают поддержку /5. Gate commands берутся из sealed `gate_checks`; staged gate и release checks задаются tasks 7.5 и 8.1.

## Commit Plan

- Checkpoint A, tasks 1–4: coherent source preparation и b2; commit только после согласованного snapshot и required checks.
- Checkpoint B, tasks 5–7: groups, CLI, docs и accepted source gates; `feat: unify composition and declared extension points`.
- Checkpoint C, tasks 8.1–8.2: интеграция, exact-SHA qualification, tag и published replay в разрешённом release scope.
- Task 8.3 — отдельная downstream qualification; Core release не объявляет её выполненной.

Commit checkpoints не требуют partial commit, который не проходит gates. Один coherent implementation commit допустим при пересечении файлов.

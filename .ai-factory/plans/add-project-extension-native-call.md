# План реализации: native Call в Project extensions

Branch: prifly/ba5628982f0031d716a09b1dd7d75d5108e53b2d2a8246b8f14af5173b86e9c2
Created: 2026-10-16
Baseline: 00cbccac08329520271565d0d104fed672c43dcd (#7 в main, change архивирован).

## Original Request

Влей исправление #7 в main и архивируй change recover-authority-storage-budget. Затем переходи к реализации change для #9.

## Настройки

Testing: yes; Logging: minimal; Docs: no (отдельного checkpoint нет; изменение справочников обязательно по change).
Профиль full, policy attended, host codex-app. Все decision_context values сохранены.
Прогресс ведётся только в `openspec/changes/add-project-extension-native-call/tasks.md`; данный план не дублирует галочки.

## Roadmap Linkage

Milestone: "none"
Rationale: change не назначает milestone, исправляет #9 и не закрывает release/profile gates.

## Requirements Reconciliation

Authority: active change specs/** и design.md > main specs > AI Factory maps/roadmap; AGENTS старше устаревших советов base rules/brief о частоте ворот и push.
Source ownership по SOURCE-OF-TRUTH не переносится; новый runtime operator/state migration не нужен.

| Поддержанная комбинация / решение | Источник | Доказательство |
|---|---|---|
| unmarked step / marked step / marked call / profile /2 | design §1, workflow-and-context delta | неизменные legacy fixtures и b1 vectors; marker/one-of/profile negatives |
| local callee / wrong kind / ambiguous / missing / cycle | design §2 | inventory до compilation, explicit extension dependency edge, filename-order и mixed cycle tests |
| call inputs 0/1/3, required/config/default, exact schema | design §1–3, typed ports delta | обычная native flow validation; dedicated negatives и два public consumers |
| succeeded/partial/rejected/other allowed, error, cancel | design §3/6 | полное on по callee, output guarantees, on_error без exports, native waiting/settlement |
| root stage vs child/sibling, authored-order chained extensions | design §1–3 | detached root-only splice, extension id vs callee id, source hashes |
| inserted call closure program / outside closure / upstream override | design §3 | deferred reachable-owned-step scope; existing execution sealing и consent |
| callee/dependency/binding/route/resource/file changed after prepare | design §5 | stale digest before dispatch для каждого selector; read-only sources preservation |
| unchanged/changed accepted prefix, inherited chain, root replay | design §5 | lawful reuse vs named refusal; whole subtree replay и source/claim preservation |
| new authoring schemas vs old sealed contracts | design §7 | new selecting extension + extension-v2, v1 bytes retained, unique editor patterns |
| source-built/synthetic vs released/live | design Migration Plan §4, tasks 7.1–7.3 | implementation commit допускается сейчас; qualification остаётся pending до реального evidence |

Реальные existing artifacts для regression: legacy extensionInput fixture, fixed b1 build vectors, project-authoring corpus, continuationCLI/resume-chain и runtime call fixtures. Приватный child не становится root producer.

## План коммита

Проверочные checkpoints: 1.1–1.3 + 2.1 (baseline/parser), 2.2–2.5 (resolver/splice/identity), 2.6 + 3.1–3.3 (program/native lifecycle), 3.4 + 4.1–4.2 + 5.1 (limits/freshness/replay/schema), 5.2–5.4 + 6.1–6.3 (delivery/final checks).
Один итоговый compatibility commit: `feat(project): seal native call extensions with exact local closure`.
Причина объединения: parser, dependency ordering, closure binding scope и schemas образуют одну authoring boundary; промежуточный compile path без guards не публикуется.
Tasks 7.1–7.3 сохраняются в плане и ledger; release/tag/push/live external-host qualification не авторизованы и не подменяются local PASS.

## Маршрут задач

Полная приёмка каждого пункта — в одноимённом id исходного tasks.md. Формулировки ниже сохранены, task не сужается. Зависимости: baseline → parser/inventory → native splice/sealing → runtime/freshness/replay → schema/docs/runner → final gates → последующая release/live приёмка.
Logging всех задач: существующие JSON Problem и read/task views, без отдельного stdout/debug logging или чувствительных данных.

### 1. Baseline и воспроизводимые contracts

- **1.1** Сверить актуальные main, source map и пересекающиеся active recovery/docs changes с design; сохранить legacy step fixture outputs, b1 vectors и hashes защищённых historical artifacts. Проверка: baseline paths/commit записаны в implementation evidence; `git status --short` отделяет предыдущие правки; #7/#8 не входят в scope и не объявлены устранёнными.
- **1.2** До реализации добавить failing parser/compile fixtures для marker, one-of step/call, unknown/wrong-kind/ambiguous target и неполного outcome routing. Проверка: `go test ./cmd/prifly -run 'TestProject.*(Extension|Call)' -count=1` воспроизводит отсутствие capability на baseline; старые step fixtures не редактируются. Новые tests имеют `t.Parallel()`, кроме случаев process env/package state по AGENTS.md.
- **1.3** Подготовить независимые readable fixtures review-подобного child (request → report/gate) и UI/UX-подобного child (configuration/ui/ux → gate/ui_report/ux_report/environment/release), supporting schemas/steps в project-owned bundle и downstream extension consumer. Проверка: root/sibling source hashes зафиксированы; names не требуют AI Factory, AI/browser или generation contracts по именам.

### 2. Authoring и native compilation

- **2.1** Реализовать `authoring: prifly-extension/2`, one-of ветви и call-specific optional bindings/on_error; сохранить legacy step IDs, cardinality и impossible_verdicts. Проверка: parser tests покрывают absent/unknown marker, both/neither branch, пустые/несколько bindings call, step >1 binding, call impossible_verdicts и profile `/2` refusal; неизменные unmarked step fixtures дают прежние bytes.
- **2.2** Добавить scoped kind-aware callee source inventory и dependency edge из extension до compilation parent; разрешать workflows/project-workflows одной folder в exact compiled ref. Проверка: filenames before/after parent, child с descendant call, unknown/wrong-kind/ambiguous/conflicting refs, missing dependency, self/mixed call-repeat cycle и installed-but-undeclared workflow покрыты targeted compile tests; latest lookup отсутствует.
- **2.3** Расширить detached splice веткой native call с stage ID extension `id`, bindings, on и on_error; оставить root-only scope и authored порядок записей. Проверка: collision, неизвестный root/internal target, отсутствующий/двусмысленный route и targets отвергаются; две записи с разными IDs вызывают один child независимо; root/sibling source hashes неизменны.
- **2.4** Пропустить вставку через native port/outcome validation; обеспечить pre-dispatch completeness всех allowed child outcomes без изменения старых sealed graphs. Проверка: `go test ./internal/flow ./cmd/prifly -run 'Test.*(Call|Extension)' -count=1` подтверждает multiple inputs, existing configuration/default rules, unknown/late/unreachable producer, unknown ports, missing required input, exact schema mismatch, incomplete/extra outcome и success-only output на partial/error route.
- **2.5** Сохранить legacy b1 canonical representation через omission новых полей; включить новый call payload и closure в compiled identities и source inventory. Проверка: `go test ./cmd/prifly -run 'TestProject.*Build|TestProject.*Extension' -count=1` сохраняет fixed legacy vectors; изменение callee/binding/route меняет новый build; provenance связывает authored и compiled refs, external exact refs не переписываются.
- **2.6** В marked `/2` разрешить explicit `execution_bindings.steps` для reachable owned program steps inserted call closure через existing sealing; сохранить legacy scope и запрет package binding override. Проверка: child program с supporting file компилируется/исполняется без upstream edits; outside-closure/ambiguous/wrong-kind/override bindings отвергаются; prepare показывает exact executable/args/files/limits и требует обычный consent. Standalone `project extend` сохраняет step utility и даёт `project_extension_call_requires_package` для call.

### 3. Native execution, управление и budgets

- **3.1** Исполнить оба synthetic consumers с downstream reader через public CLI, включая child с внутренними step/call stages. Проверка: новый reusable harness `python3 -B test/e2e/verify-call-extensions.py --binary /absolute/path/to/source-built-prifly` доказывает один Run, реальные invocation/caller связи, accepted artifacts с exact schema/provenance и отсутствие root/sibling source edits; каждый заявленный export читается downstream без hidden graph patch.
- **3.2** Покрыть authored succeeded/partial/rejected и остальные allowed outcomes callee, technical child error с/без on_error, timeout и scoped cancel. Проверка: runtime/public CLI fixtures показывают точные routes, waiting/blocked_child и settlement, отсутствующие outputs остаются отсутствующими; partial/blocked не становится implicit success; native cancel не считается остановкой внешнего actor.
- **3.3** Проверить общий claim/candidate контекст и shared resource ownership descendant Attempts, competing Run и освобождение после settlement. Проверка: disposable fixture сравнивает записанные claim generation/tree/candidate provenance и holder facts; competing admission получает existing resource diagnostic; второй независимый Run для child не создаётся.
- **3.4** Покрыть depth/step/transition/storage/resource/effect/trust ceilings внутри inserted child и descendant call. Проверка: boundary positive/negative tests не позволяют лишнего dispatch, сохраняют accounting и штатную причину; storage refusal тестируется синтетически без изменения production limit/authority и не объявляется исправлением #7/#8.

### 4. Review freshness и продолжение

- **4.1** Проверить read-only prepare и start со старым digest отдельно после изменения callee, descendant dependency, каждого binding, outcome/error route, resource/limit и declared supporting file/context. Проверка: `go test ./cmd/prifly -run 'TestProject.*(Launch|Prepare|Extension|Call)' -count=1` даёт `project_start_stale_launch` до dispatch; sources перечисляют реально прочитанные files; prepare не меняет authority/registry/claim/tree.
- **4.2** Проверить resume/recover после accepted call с прежним и изменённым effective contract, включая inherited prefix через последовательность Runs. Проверка: targeted runtime/CLI resume tests подтверждают lawful unchanged reuse; изменённый child не переносит stale outputs, explicit допустимый root `--from-stage` повторяет whole subtree; empty prefix, unsettled/uncertain source и недопустимая replay point дают named refusal, source history/claim не сбрасываются.

### 5. Schema, diagnostics и доставка автору

- **5.1** Добавить embedded/public `extension-v2` и общую selecting `extension` schema, обновить оба authoring manifests и mappings по design. Проверка: `make schemas-check` и editor contract tests принимают legacy-v1 / marked-v2 без конфликтующих automatic patterns; explicit modelines работают; v1 schema и все прежние closed runtime bundles byte-identical.
- **5.2** Ввести `project_call_extensions` в capability inventory и обновить `examples/README.md`, extension reference, continuation guide/troubleshooting и необходимые entry-point ссылки; обновить `terms.md` Workflow extension в том же изменении. Проверка: `TestEveryDeclaredCapabilityIsInTheAuthorIndex` и `TestGlossaryBindings` проходят в своих пакетах; смысловое review видит marker/profile minima, step/call distinction, три bindings, exports/outcomes/root scope и отсутствие неподтверждённой release version.
- **5.3** Провести новые ошибки через typed Fault/Problem и сохранить native flow violations с field/stage context. Проверка: `make refusal-check` и public negative corpus проверяют все E5-A4 причины, on_error/export negatives, nonzero exit и отсутствие Run/Attempt/Session/process/valid partial package при compile refusal; тесты не извлекают codes из prose.
- **5.4** Обновить generated runner templates/readable guidance в затронутой части и добавить native call leaf handoff fixture для трёх hosts. Проверка: tests подтверждают ordinary session task с invocation ancestry, тем же Run/claim и отсутствие nested `project start`; обновление runner сохраняет owner edits; существующие closed DTO/state contracts проходят compatibility без фиктивной новой версии.

### 6. Инженерная проверка итогового снимка

- **6.1** В рабочем цикле выполнить `make test-changed` или `make check-fast`, public authoring verifier `python3 -B test/e2e/verify-authoring.py --binary /absolute/path/to/source-built-prifly` и новый E5 harness. Проверка: PASS зафиксирован отдельно для compile/semantic/runtime/control/freshness/replay; unit helpers не выданы за released или live host evidence.
- **6.2** Выполнить `openspec validate add-project-extension-native-call --strict`, `git diff --check` и один полный gate итогового снимка перед implementation commit через `make ci-check-once CHECK_CONTEXT=<actual-profile>`; при staged check добавить `CHECK_FOR_COMMIT=1`. Проверка: context отражает реальные inputs/toolchain/permissions, evidence reuse obeys helper, обязательные fresh vuln/mod checks выполнены; полный race/e2e не запускается вне release gate.
- **6.3** Проверить `git diff --name-status <implementation-baseline>` и hashes protected archive/release evidence/manifests, старых public schemas/bundles и canonical workflow source. Проверка: historical evidence неизменно; source map обновлён только для актуальной навигации при необходимости, ownership не перенесён; старые Runs и projects без новой extension проходят прежние checks.

### 7. Released CLI и host qualification

- **7.1** Для fixed release SHA выполнить existing `verify` / `qualify` процедуру: `python3 scripts/ensure-release-checks.py --sha SHA --ref main --wait`; publication/tag остаются у владельца по правилам репозитория. После stable release запустить тот же E5 harness exact released CLI. Проверка: записаны version/commit/SHA256 бинарника и outcomes всех fixtures; v0.13.77 отмечен только как baseline unsupported syntax. Эта задача остаётся открытой до фактического выпуска.
- **7.2** Выполнить native generated-runner harness на `codex-app`, `codex-cli`, `claude-code` с synthetic task payload и отдельно live host handoff/control check в заявленном release profile. Проверка: по каждому host сохранены version и invocation/task/submit/settlement evidence, scope live/synthetic обозначен; недоступный host — pending до проверки или явного release-scope решения владельца, не PASS по template tests.
- **7.3** Свести E5-A1…A10 evidence и blockers, передать released Core version пакетному W6 для независимых consumer tests. Проверка: release acceptance относится только к Core; #7/#8 и незавершённые host/release проверки остаются видимыми; закрытие issue/archive или P2 milestone не объявляется по одному document/compile check.

## Точки изменения

- 1–2: cmd/prifly/project.go, project_compile.go, project_build.go; новый project_extension_call.go и dedicated tests рядом; execution sealing остаётся в project_execution.go.
- 3–4: test/fixtures/project-call-extensions/, test/e2e/verify-call-extensions.py; cmd/prifly extension/public launch/replay tests; existing internal/flow/runtime native fixtures.
- 5: internal/runtime/authoring/ и schemas/authoring/ только новые schema files и текущие manifests, scripts/check-schema.py; examples/README.md и authoring references/continuation/troubleshooting; terms.md; generated runner template в cmd/prifly/project.go.
- 6–7: source-built exact-binary evidence в active change; existing release helpers не меняются; canonical workflows и historical bundles не меняются.

## Выполненные исходные проверки

На этом claim tree до записи плана выполнены успешно:
`go test ./internal/flow ./cmd/prifly -run 'Test.*(Call|Extension|Project.*Build)' -count=1` (baseline log /tmp/prifly-issue9-baseline-tests.log), `openspec validate add-project-extension-native-call --strict`, `git diff --check`.
Baseline green доказывает прежний контракт; regression нового marker/target сначала должна покраснеть.
Нормативные дальнейшие проверки и harness commands заданы tasks.md и sealed gate_checks; новый harness появится в реализации, release commands сейчас не исполняются.
Protected manifest /tmp/prifly-issue9-protected-hashes.json: 1484 baseline files; versioned v1/old bundles и archive неизменны, manifests могут изменяться только как требует task 5.1.

## Фактический diff

Состав, число и строки файлов вычисляются заново из Git от записанного Baseline до текущего tree/HEAD; здесь moving counts не закрепляются.

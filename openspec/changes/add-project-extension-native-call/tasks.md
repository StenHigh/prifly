## 1. Baseline и воспроизводимые contracts

- [x] 1.1 Сверить актуальные main, source map и пересекающиеся active recovery/docs changes с design; сохранить legacy step fixture outputs, b1 vectors и hashes защищённых historical artifacts. Проверка: baseline paths/commit записаны в implementation evidence; `git status --short` отделяет предыдущие правки; #7/#8 не входят в scope и не объявлены устранёнными.
- [x] 1.2 До реализации добавить failing parser/compile fixtures для marker, one-of step/call, unknown/wrong-kind/ambiguous target и неполного outcome routing. Проверка: `go test ./cmd/prifly -run 'TestProject.*(Extension|Call)' -count=1` воспроизводит отсутствие capability на baseline; старые step fixtures не редактируются. Новые tests имеют `t.Parallel()`, кроме случаев process env/package state по AGENTS.md.
- [x] 1.3 Подготовить независимые readable fixtures review-подобного child (request → report/gate) и UI/UX-подобного child (configuration/ui/ux → gate/ui_report/ux_report/environment/release), supporting schemas/steps в project-owned bundle и downstream extension consumer. Проверка: root/sibling source hashes зафиксированы; names не требуют AI Factory, AI/browser или generation contracts по именам.

## 2. Authoring и native compilation

- [x] 2.1 Реализовать `authoring: prifly-extension/2`, one-of ветви и call-specific optional bindings/on_error; сохранить legacy step IDs, cardinality и impossible_verdicts. Проверка: parser tests покрывают absent/unknown marker, both/neither branch, пустые/несколько bindings call, step >1 binding, call impossible_verdicts и profile `/2` refusal; неизменные unmarked step fixtures дают прежние bytes.
- [x] 2.2 Добавить scoped kind-aware callee source inventory и dependency edge из extension до compilation parent; разрешать workflows/project-workflows одной folder в exact compiled ref. Проверка: filenames before/after parent, child с descendant call, unknown/wrong-kind/ambiguous/conflicting refs, missing dependency, self/mixed call-repeat cycle и installed-but-undeclared workflow покрыты targeted compile tests; latest lookup отсутствует.
- [x] 2.3 Расширить detached splice веткой native call с stage ID extension `id`, bindings, on и on_error; оставить root-only scope и authored порядок записей. Проверка: collision, неизвестный root/internal target, отсутствующий/двусмысленный route и targets отвергаются; две записи с разными IDs вызывают один child независимо; root/sibling source hashes неизменны.
- [x] 2.4 Пропустить вставку через native port/outcome validation; обеспечить pre-dispatch completeness всех allowed child outcomes без изменения старых sealed graphs. Проверка: `go test ./internal/flow ./cmd/prifly -run 'Test.*(Call|Extension)' -count=1` подтверждает multiple inputs, existing configuration/default rules, unknown/late/unreachable producer, unknown ports, missing required input, exact schema mismatch, incomplete/extra outcome и success-only output на partial/error route.
- [x] 2.5 Сохранить legacy b1 canonical representation через omission новых полей; включить новый call payload и closure в compiled identities и source inventory. Проверка: `go test ./cmd/prifly -run 'TestProject.*Build|TestProject.*Extension' -count=1` сохраняет fixed legacy vectors; изменение callee/binding/route меняет новый build; provenance связывает authored и compiled refs, external exact refs не переписываются.
- [x] 2.6 В marked `/2` разрешить explicit `execution_bindings.steps` для reachable owned program steps inserted call closure через existing sealing; сохранить legacy scope и запрет package binding override. Проверка: child program с supporting file компилируется/исполняется без upstream edits; outside-closure/ambiguous/wrong-kind/override bindings отвергаются; prepare показывает exact executable/args/files/limits и требует обычный consent. Standalone `project extend` сохраняет step utility и даёт `project_extension_call_requires_package` для call.

## 3. Native execution, управление и budgets

- [x] 3.1 Исполнить оба synthetic consumers с downstream reader через public CLI, включая child с внутренними step/call stages. Проверка: новый reusable harness `python3 -B test/e2e/verify-call-extensions.py --binary /absolute/path/to/source-built-prifly` доказывает один Run, реальные invocation/caller связи, accepted artifacts с exact schema/provenance и отсутствие root/sibling source edits; каждый заявленный export читается downstream без hidden graph patch.
- [x] 3.2 Покрыть authored succeeded/partial/rejected и остальные allowed outcomes callee, technical child error с/без on_error, timeout и scoped cancel. Проверка: runtime/public CLI fixtures показывают точные routes, waiting/blocked_child и settlement, отсутствующие outputs остаются отсутствующими; partial/blocked не становится implicit success; native cancel не считается остановкой внешнего actor.
- [x] 3.3 Проверить общий claim/candidate контекст и shared resource ownership descendant Attempts, competing Run и освобождение после settlement. Проверка: disposable fixture сравнивает записанные claim generation/tree/candidate provenance и holder facts; competing admission получает existing resource diagnostic; второй независимый Run для child не создаётся.
- [x] 3.4 Покрыть depth/step/transition/storage/resource/effect/trust ceilings внутри inserted child и descendant call. Проверка: boundary positive/negative tests не позволяют лишнего dispatch, сохраняют accounting и штатную причину; storage refusal тестируется синтетически без изменения production limit/authority и не объявляется исправлением #7/#8.

## 4. Review freshness и продолжение

- [x] 4.1 Проверить read-only prepare и start со старым digest отдельно после изменения callee, descendant dependency, каждого binding, outcome/error route, resource/limit и declared supporting file/context. Проверка: `go test ./cmd/prifly -run 'TestProject.*(Launch|Prepare|Extension|Call)' -count=1` даёт `project_start_stale_launch` до dispatch; sources перечисляют реально прочитанные files; prepare не меняет authority/registry/claim/tree.
- [x] 4.2 Проверить resume/recover после accepted call с прежним и изменённым effective contract, включая inherited prefix через последовательность Runs. Проверка: targeted runtime/CLI resume tests подтверждают lawful unchanged reuse; изменённый child не переносит stale outputs, explicit допустимый root `--from-stage` повторяет whole subtree; empty prefix, unsettled/uncertain source и недопустимая replay point дают named refusal, source history/claim не сбрасываются.

## 5. Schema, diagnostics и доставка автору

- [x] 5.1 Добавить embedded/public `extension-v2` и общую selecting `extension` schema, обновить оба authoring manifests и mappings по design. Проверка: `make schemas-check` и editor contract tests принимают legacy-v1 / marked-v2 без конфликтующих automatic patterns; explicit modelines работают; v1 schema и все прежние closed runtime bundles byte-identical.
- [x] 5.2 Ввести `project_call_extensions` в capability inventory и обновить `examples/README.md`, extension reference, continuation guide/troubleshooting и необходимые entry-point ссылки; обновить `terms.md` Workflow extension в том же изменении. Проверка: `TestEveryDeclaredCapabilityIsInTheAuthorIndex` и `TestGlossaryBindings` проходят в своих пакетах; смысловое review видит marker/profile minima, step/call distinction, три bindings, exports/outcomes/root scope и отсутствие неподтверждённой release version.
- [x] 5.3 Провести новые ошибки через typed Fault/Problem и сохранить native flow violations с field/stage context. Проверка: `make refusal-check` и public negative corpus проверяют все E5-A4 причины, on_error/export negatives, nonzero exit и отсутствие Run/Attempt/Session/process/valid partial package при compile refusal; тесты не извлекают codes из prose.
- [x] 5.4 Обновить generated runner templates/readable guidance в затронутой части и добавить native call leaf handoff fixture для трёх hosts. Проверка: tests подтверждают ordinary session task с invocation ancestry, тем же Run/claim и отсутствие nested `project start`; обновление runner сохраняет owner edits; существующие closed DTO/state contracts проходят compatibility без фиктивной новой версии.

## 6. Инженерная проверка итогового снимка

- [x] 6.1 В рабочем цикле выполнить `make test-changed` или `make check-fast`, public authoring verifier `python3 -B test/e2e/verify-authoring.py --binary /absolute/path/to/source-built-prifly` и новый E5 harness. Проверка: PASS зафиксирован отдельно для compile/semantic/runtime/control/freshness/replay; unit helpers не выданы за released или live host evidence.
- [x] 6.2 Выполнить `openspec validate add-project-extension-native-call --strict`, `git diff --check` и один полный gate итогового снимка перед implementation commit через `make ci-check-once CHECK_CONTEXT=<actual-profile>`; при staged check добавить `CHECK_FOR_COMMIT=1`. Проверка: context отражает реальные inputs/toolchain/permissions, evidence reuse obeys helper, обязательные fresh vuln/mod checks выполнены; полный race/e2e не запускается вне release gate.
- [x] 6.3 Проверить `git diff --name-status <implementation-baseline>` и hashes protected archive/release evidence/manifests, старых public schemas/bundles и canonical workflow source. Проверка: historical evidence неизменно; source map обновлён только для актуальной навигации при необходимости, ownership не перенесён; старые Runs и projects без новой extension проходят прежние checks.

## 7. Released CLI и host qualification

- [ ] 7.1 Для fixed release SHA выполнить existing `verify` / `qualify` процедуру: `python3 scripts/ensure-release-checks.py --sha SHA --ref main --wait`; publication/tag остаются у владельца по правилам репозитория. После stable release запустить тот же E5 harness exact released CLI. Проверка: записаны version/commit/SHA256 бинарника и outcomes всех fixtures; v0.13.77 отмечен только как baseline unsupported syntax. Эта задача остаётся открытой до фактического выпуска.
- [ ] 7.2 Выполнить native generated-runner harness на `codex-app`, `codex-cli`, `claude-code` с synthetic task payload и отдельно live host handoff/control check в заявленном release profile. Проверка: по каждому host сохранены version и invocation/task/submit/settlement evidence, scope live/synthetic обозначен; недоступный host — pending до проверки или явного release-scope решения владельца, не PASS по template tests.
- [ ] 7.3 Свести E5-A1…A10 evidence и blockers, передать released Core version пакетному W6 для независимых consumer tests. Проверка: release acceptance относится только к Core; #7/#8 и незавершённые host/release проверки остаются видимыми; закрытие issue/archive или P2 milestone не объявляется по одному document/compile check.

## Acceptance traceability

| Issue #9 | Tasks | Наблюдаемое доказательство |
|---|---|---|
| E5-A1 | 2.1, 5.1, 5.2 | Marker/discriminator/ref/outcome contract, migration и прежние step fixtures |
| E5-A2 | 2.2, 2.3, 2.6, 3.1, 7.1 | Released CLI: вставка и nested child в одном Run, upstream hashes прежние |
| E5-A3 | 1.3, 2.4, 3.1 | Один/три inputs и реальные downstream exact exports |
| E5-A4 | 1.2, 2.1–2.4, 5.3 | Named negative corpus, отсутствие частичного execution |
| E5-A5 | 2.4, 3.2 | Все declared outcomes и output guarantees без implicit success |
| E5-A6 | 3.2, 3.3, 4.2 | Cancel/timeout/error, settlement/resources, whole-child replay |
| E5-A7 | 3.3, 3.4 | Descendant budgets/effect/trust/resource ceilings |
| E5-A8 | 2.5, 4.1, 4.2 | Старый review отвергнут; stale accepted outputs не reused |
| E5-A9 | 2.3, 5.2, 6.3 | Root-only scope, неизменные source/sibling semantics и legacy behavior |
| E5-A10 | 5.1–5.4, 6.1, 7.1–7.3 | Schemas/docs/Problem/read/handoff, released и host evidence раздельно |

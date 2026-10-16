## Context

Мотивация и разбор открытых #7/#8/#9 — [proposal.md](proposal.md). Baseline исследования: checkout `437e6a1c`, issue #9 называет released v0.13.77. Реальная пользовательская authority не открывалась и не изменялась.

`parseProjectWorkflowOptions` принимает только `step` и один binding; `applyProjectExtension` всегда создаёт `kind: step` с ID из `extension.Step`. `projectApplyExtensions` видит только уже собранные step components. `compileProjectPackage` ожидает unresolved placeholders, но extension target сам по себе placeholder в parent YAML не создаёт: native call insertion требует отдельного dependency edge. Root document единственный несёт `Extensions`; `settings` / `exclude` имеют более широкий workflow scope, который не надо переносить на вставки.

Folder reader уже читает `project/{workflows,steps,schemas,contexts,checks}` рядом с upstream directories. Эти project-owned files можно вендорить явно; folder update сохраняет их отдельно от upstream drift. Project `/3` b1 связывает owned definitions с compiled versions, включает extension payload и canonical documents в build key и оставляет exact external refs неизменными. `internal/flow` уже проверяет native calls, scoped configuration, outputs по outcome, cycles и общие budgets. Closed `CallStage` уже имеет `workflow_ref`, `input_bindings`, `on`, `on_error`.

Нормативные deltas — [workflow-and-context](specs/workflow-and-context/spec.md), [cli-protocol](specs/cli-protocol/spec.md) и [specification-governance](specs/specification-governance/spec.md). Общие domain-execution / runtime-resources / published-contracts правила применяются без нового operator или переноса ownership. Active `fix-nested-preparation-recovery` и `retry-failed-stage-with-new-package` учитываются при интеграции recovery regressions; незакрытые live checks этих changes не становятся PASS от E5.

## Goals / Non-Goals

**Goals:** свести authoring запись к обычной exact native call stage; явно учесть её dependency до sealing; доказать неизменность legacy bytes и целостность reviewed execution / accepted prefix. Синтетические consumers показывают reusable contracts без привязки Core к именам пакетов AI Factory.

**Non-Goals:** исправление #7/#8 в этом change, произвольный graph patch, automatic cross-package discovery/import, target internal graph, inline workflow, новый Run/bridge, новые outcomes, model selection, W5/monitor/queue, product repair policy и изменение канонических aif-classic/profiled. Project source и пользовательская authority не мигрируются при старте.

## Decisions

### 1. Новая форма opt-in; старые bytes остаются старыми

Выбран marker `authoring: prifly-extension/2`, только для Project profile `/3`. Без marker читается ровно нынешняя step-only форма. `/2` profile при call получает existing profile-required refusal; явное изменение shared profile остаётся решением автора. В marker `/2` допустимы step и call entries в одном списке, каждая запись содержит ровно одну ветвь.

Форма планируемого authoring contract; **не поддерживается v0.13.77**:

```yaml
authoring: prifly-extension/2
extensions:
  - id: product-acceptance        # root stage ID; не callee ID
    workflow: consumer            # tail authored ID ROOT workflow этой folder
    call: project-product-quality # tail authored ID одного local child
    between: {from: prepare-product, to: commit}
    input_bindings:
      configuration: $stages.prepare-product.configuration
      ui: $stages.prepare-product.ui
      ux: $stages.prepare-product.ux
    on: {succeeded: commit, partial: stopped, rejected: stopped}
    on_error: stopped             # optional; native technical failure
```

Здесь root уже объявляет `prepare-product`, `commit`, `stopped` и их контракты, а child — ровно эти три allowed outcomes. Child `configuration` — обычный input; если автор объявил `configuration.scope: project`, binding закономерно запрещён native правилом. Названия ничего не генерируют.

Stage ID call равен extension `id`, поэтому следующий root stage читает `$stages.product-acceptance.gate`, если output гарантирован на его пути. Две call записи с разными IDs могут вызывать один callee независимо. Legacy `step: lint` по-прежнему вставляет stage `lint`, даже при `id: lint-after-build`. Call stage ID проверяется на collision; её `on` / `on_error` адресуют существующие root stages. Последующие записи могут вставляться в route ранее вставленной stage, как сейчас для steps; список применяется в authored порядке. Одна запись не меняет несколько routes.

`input_bindings` call — optional map, отсутствие/`{}` означает отсутствие bindings, максимум 256 как у native CallStage. Значения только `$stages.STAGE.PORT`; первый срез не вводит literals, workflow-input shorthand, projections или deep paths в extension. Multiple inputs разрешены только call ветви; `impossible_verdicts` остаётся только у step. Ни `kind` discriminator, ни inline graph не нужны: one-of `step` / `call` однозначен и сохраняет привычный syntax.

Отдельная utility команда `project extend` знает только переданные `--step-ref` / `--step-source`, не inventory folder. Она сохраняет step путь, а call отвергает как `project_extension_call_requires_package` с направлением к `project compile`. Новый standalone resolver/CLI flags не вводятся.

Альтернатива: добавить call в немаркированную форму. Отклонена ради явной compatibility boundary и сохранения published v1 schema. Альтернатива: использовать callee short name как stage ID. Отклонена: мешает двум вызовам и смешивает extension instance с reusable definition.

### 2. Exact resolver работает с source inventory, а не случайно готовым component

До compilation root индексировать объявленные component identities/kinds и зависимости. Call target ищется по tail `id` среди child workflows этой folder, включая project-owned subtree; root target не является child и self-call отвергается. Одинаковый tail нескольких workflow definitions неоднозначен даже при разных namespaces/versions. Совпадение tail у step без workflow — wrong-kind, отсутствие любого кандидата — unknown target. Установленный workflow вне source inventory не является кандидатом.

Добавить dependency edge root → target из extension к существующим placeholder/ref edges. Callee и его own dependencies должны быть собраны до splice/sealing root независимо от filename order; цикл даёт typed refusal, а не бесконечное ожидание. Порядок extensions не меняет target selection. После обычного `/3` rebuild вставленный `workflow_ref` указывает compiled exact ref, а build provenance показывает authored → compiled mapping. Правила duplicates/conflicts, confinement, symlink/traversal и dependency budgets сохраняются.

Не требуется новый generic import system: явно скопированный reusable bundle живёт в `project/workflows`, его supporting schemas/steps/contexts/checks — в соответствующих project directories; authored IDs и ссылки должны быть согласованы. Core не угадывает их по пакетным именам. Уже объявленные exact external dependencies внутри bundle используют existing closure/trust механизм; сам `call:` не делает registry lookup.

Альтернатива: искать target в map уже собранных components, как step. Отклонена: root без placeholder может скомпилироваться раньше child. Альтернатива: выбирать installed latest либо допускать неоднозначность через map overwrite. Отклонена из-за недетерминизма и смешения compiled editions.

### 3. Splice создаёт native CallStage; validation общая

Общая часть вставки проверяет root selector, unique route, stage collision и существующие targets до изменения detached graph. Step ветвь сохраняет прежнее представление и проверки; call ветвь пишет `kind: call`, exact `workflow_ref`, bindings и outcome/error routes. Никакого `StepDefinition` для call не создаётся.

Обычный compiler проверяет child ports, exact schema refs, requiredness, project-scoped configuration, producer availability и exports по каждому incoming path. Call `on` проверяется на покрытие всех allowed outcomes callee. Если existing native validation где-либо проверяет только наблюдаемый route, добавить pre-dispatch completeness для новой extension ветви, сохранив поведение прежних sealed WorkflowRevisions. `impossible_verdicts` не применим к outcomes child. Optional `on_error` не создаёт exports.

Недостаточные policy/graph limits дают обычный compile/admission refusal: compiler не добавляет глубину, steps или transition allowance. Call entry/return и descendant work учитываются обычным whole-closure расчётом. Minimum native call contract — WorkflowRevision 1 и call-capable core state/read 2; дополнительные поля child/root требуют своих более поздних contracts. Не понижать authored revision и не заводить новую state version только из-за способа authoring.

После построения call closure marked `/2` проверяет `execution_bindings.steps` также для reachable owned program steps этого closure. Форма binding и short-name ключ сохраняются. `projectReadExtensionBindings` сейчас допускает только имена `extension.Step`; новую scope проверку нужно отложить до разрешения callee, затем seal-ить через existing `projectReadExecution` / `projectSealExecution`. Binding unrelated sibling, ambiguous short name и override уже связанной package программы запрещены. Legacy parser/scope не расширяется. Supporting files читаются/confine-ятся обычным способом, exact executable/argv/files/limits видны в prepare; execution по-прежнему требует owner allowlist/consent. Сам call не получает worker binding. Новая форма не добавляет overrides для checks и не изобретает недостающие bindings: такая потребность child остаётся обычным preflight refusal.

Это закрывает выявленный authoring разрыв для вендоренного child с program steps: без него либо нужна правка upstream root execution map, либо synthetic harness проверяет только assisted child. В fixtures проверяются обе формы, включая deterministic controlled session submit без AI и managed program с явным project binding.

### 4. Diagnostics отделяют причины до исполнения

Новые branch/target ошибки передаются `runtime.Fault`, существующие flow validation violations сохраняют свои codes и paths при упаковке в Problem. План именования:

| Причина | Code / существующий механизм |
|---|---|
| step+call, ни одной ветви, запрещённое поле | `project_extension_invalid`, path конкретной записи/поля |
| call без marker / неизвестный marker | `project_extension_unsupported_authoring` |
| call в Project `/2` | `project_compile_profile_required` |
| call в standalone `project extend` | `project_extension_call_requires_package` |
| отсутствующий local target | `project_extension_unknown_call` |
| target найден только другого kind | `project_extension_call_wrong_kind` |
| несколько workflow candidates | `project_extension_call_ambiguous`; существующий duplicate/ref conflict допустим, если точно называет неоднозначность до resolution |
| неверный scope / collision / route | existing `project_extension_unknown_workflow`, `project_extension_stage_conflict`, `project_extension_route_missing` / `project_extension_route_ambiguous` |
| unknown port, late producer, required binding, schemas, цикл | existing native flow violations (`unknown_port`, `unavailable_output`, `missing_binding`, schema compatibility/ref и `call_cycle`) |
| непокрытый child outcome | `project_extension_call_outcome_unmapped` с stage/outcome; existing native violation используется, если уже выражает ту же compile причину |
| устаревший launch review | existing `project_start_stale_launch` |
| incompatible accepted prefix | existing resume/recovery typed refusal с причиной и стадией |

Stable negative fixtures проверяют Problem code/violations, не произвольный prose. Legacy usages с кодами в строках не расширяются; новые ветви проходят refusal guard. Ошибка compile не создаёт Run/Attempt/Session или worker process; failed output directory не должен выглядеть valid sealed package.

### 5. Build identity и review сохраняют весь effective child

Новые optional поля внутреннего extension payload сериализуются с omission при legacy path, чтобы не изменить фиксированные b1 vectors. Не переупорядочивать старый step payload и не заменять один binding на map в его canonical representation. Call target/bindings/error route попадают в новый payload и итоговую native definition; child closure присутствует в canonical documents. Новый marker не попадает в machine WorkflowRevision.

`ReadPaths` включает все фактически прочитанные declared sources; supporting execution files и contexts входят в existing sealing/freshness mechanisms. Review должен менять digest при изменении callee/его dependencies, каждого binding, маршрута и resource declaration/limit. Проверять каждый случай отдельно, а не только один root YAML diff. Start со старым digest отказывает **до dispatch**. Read-only prepare не регистрирует package и не создаёт claim.

Accepted prefix сравнивается по effective contract/provenance существующим resume planner, не по callee short name или authored version. Изменённый child внутри переносимого prefix требует refusal; разрешённый `--from-stage <extension-id>` повторяет whole child, если есть законный непустой prefix и settled source. Если earlier point не проходит условия — named refusal; автоматического перескакивания или reuse stale outputs нет. Unchanged child reuse и inherited prefix через цепочку Runs покрываются отдельными positive regressions.

### 6. Native execution и host delivery используют существующий путь

Новые runtime команды не нужны. Native invocation tree доказывает один Run, parent/caller связи, общий claim и ordinary candidate/tree/artifact provenance. Callee может содержать свои step/call stages; их Attempts сохраняют effects/resources/trust и counters ancestors.

`cancelled` child не является workflow outcome и не вызывает `on_error`: caller остаётся waiting/`blocked_child`. Timeout и technical failure следуют действующим step/child settlement правилам. Partial/rejected маршрутизируются по authored `on`; Core не интерпретирует UI/UX gate content. Освобождение ресурса проверяется по реально settled ownership. Native cancellation не доказывает остановку внешнего browser actor.

Runner templates объясняют root call stage и leaf task без самостоятельного `project start`. Обновление generated runner сохраняет owner edits по existing update rules; репозиторные `.agents` / `.claude` экземпляры не правятся как side effect planning. Существующие native views используются без новых JSON fields, пока их достаточно. Если реализация докажет необходимость новой closed формы, сначала уточнить specs/design и отдельную version boundary.

### 7. Editor contract публикуется рядом, qualification отдельно

Сохранить exact v1 authoring schema в embedded и public copies. Добавить `extension-v2.schema.json` с marker и one-of ветвями и общую `extension.schema.json` (`urn:prifly:yaml-authoring:extension`) для legacy-v1 / marked-v2 selection. Единственный automatic `extend.yaml` pattern принадлежит общей schema; v1/v2 entries остаются доступны explicit modeline. Обновить оба manifests, generator/check inventory и references; project-owned paths также должны получать подходящие editor mappings без дублирования patterns.

Новая capability `project_call_extensions` публикуется отдельно от runtime `call`, без повышения state version для старых запусков. `examples/README.md` и `extension-authoring-reference.yaml` показывают marker, stage/callee различие, три bindings, outputs/outcomes и минимальные contracts; continuation guide/troubleshooting связывают freshness и whole-child replay. В этом же срезе обновить словарь Workflow extension, не вводя сущность «call extension run».

Синтетический public CLI harness покрывает один и три входа, вложенный call, все outcomes, negative compile, controls, limits, freshness и replay. Его fixtures не требуют AI/browser. Выполнять и source-built, и released candidate CLI с записью exact version/commit/binary SHA256; v0.13.77 служит baseline отказа нового syntax, не успешной приёмкой capability. Live host check отдельно проверяет generated runners на `codex-app`, `codex-cli`, `claude-code`; отсутствие доступного host означает `pending`, не synthetic PASS. Явное исключение host из release acceptance возможно только отдельным решением владельца, прежнее исключение Claude для другого выпуска сюда не наследуется.

Чтобы доказать downstream exports без изменения upstream graph, fixture добавляет вторую extension в success route вставленного call: один step consumer для одного output либо call consumer для нескольких. Его bindings читают `$stages.<первый-extension-id>.<port>`. Negative fixture на partial/error route требует success-only export и должна отказать до dispatch. Нельзя подменять эту проверку чтением одного лишь declaration или скрытой правкой upstream consumer. При сравнении sibling graphs отделять semantic/source immutability от штатного `/3` repinning: общий b1 build key меняет compiled versions и refs owned components даже без изменения их authored graphs.

### 8. Связанные issues не сливаются

#8 касается полного, согласованного ownership чтения и cleanup без Run; #7 — storage recovery существующего Run. Их общая тема «лимиты authority» не означает одинаковый алгоритм или приёмку. В E5 не добавлять scan pagination/index, не менять logical-page budget и не скрывать проблему новой пользовательской authority. Disposable authority допустима только для изолированных fixtures. Порядок и ограничения пилота записаны в proposal.

`refresh-author-guides-and-examples` имеет пересечение по индексам и продолжению: при реализации сверить его актуальные artifacts и сохранить интегрированные пояснения; это координация docs, не prerequisite capability.

## Risks / Trade-offs

- [Новые поля меняют legacy b1 bytes] → omission старой ветви, fixed vectors и byte comparison unchanged fixtures.
- [Root готов раньше child] → explicit dependency edge от extension и filename-order regression.
- [Short names неоднозначны] → scoped kind-aware inventory и fail-closed resolution без выбора версии.
- [Неполные outcome routes проходят compile] → completeness по callee contract до dispatch, без переписывания старых Run semantics.
- [Success exports читаются после partial/error] → native guaranteed-output validation и отрицательные public fixtures.
- [Смена authored версии скрывает stale child] → effective subtree comparison, fresh review и whole-child replay.
- [Ресурс привязан к callee имени вместо stage] → resource key указывает extension `id`, проверить descendant holder и competing Run.
- [Marker и schema увеличивают authoring surface] → explicit opt-in, общая editor mapping и неизменный legacy route.
- [Live pilot блокируется #7/#8 или отсутствием host] → отдельный статус/issue, отсутствие заявления E5 release qualification до applicable evidence.

## Migration Plan

1. Сохранить legacy fixtures/vectors и добавить failing refusal/positive E5 fixtures до реализации.
2. Реализовать marked parser, dependency resolution, native lowering и versioned schemas; обновить terminology и author-facing docs вместе.
3. Подтвердить source-built CLI, native runtime/control/replay и host harness. В рабочем цикле — targeted checks; один полный final-snapshot gate перед implementation commit по AGENTS.md.
4. На fixed release SHA пройти existing verify/qualify gates; после публикации stable выполнить тот же harness exact released binary и записать отдельное live host evidence. Не блокировать implementation commit ожиданием ещё не существующего release и не помечать release tasks выполненными заранее.
5. Только после этого закрывать E5 release acceptance; W6 получает released Core version и свои consumer tests, без изменения канонических workflows в этом change.

Существующие немаркированные extensions и pinned Runs не мигрируются. Rollback authoring — убрать call запись/marker и вернуться к прежнему source; старый CLI должен отвергать новый marker до use. Rollback binary допустим только при совместимости reader с уже созданными Runs: обычные existing state/version guards сохраняются, принятый child и authority history не удаляются.

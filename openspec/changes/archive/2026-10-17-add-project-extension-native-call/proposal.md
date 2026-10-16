## Why

Проект не может подключить переиспользуемый quality workflow через `extend.yaml`: существующая вставка создаёт только `step`, хотя native `call` уже исполняет дочерний WorkflowRevision внутри одного Run. [Issue #9 (E5)](https://github.com/StenHigh/prifly/issues/9) требует добавить этот authoring/compiler слой, сохранив upstream graph и существующие step extensions.

## What Changes

- Ввести явную новую редакцию extension authoring с взаимоисключающими `step` / `call`. Для call отдельно задаются target WorkflowRevision, несколько typed `input_bindings`, маршруты workflow outcomes и optional `on_error`.
- Разрешать call только в объявленный local workflow component той же folder, включая явно вендоренный `project/workflows/`; seal-ить exact compiled ref и весь dependency closure. Установка пакета сама по себе не делает его workflows целями вставки.
- В новой редакции разрешить явные project execution bindings для owned program steps внутри вставленного child closure, сохраняя запрет переопределять upstream program bindings.
- Применять вставку только к root graph выбранной folder и одному exact прямому route. Stage ID нового call равен extension `id`; прежний step сохраняет свой stage ID и ограничение одного binding.
- Использовать native call validation, выполнение, ресурсы, limits, effect/trust policy и whole-child replay. Не создавать отдельный Run или host bridge.
- Включать callee closure, bindings, routing, execution/resource/source dependencies в reviewed launch; проверять effective accepted prefix при продолжении.
- Доставить schema, diagnostics, runner guidance, capability declaration и примеры вместе с реализацией. Новые authoring bytes не объявляются поддержанными в v0.13.77; исторические schemas и Runs сохраняются.

## Capabilities

### New Capabilities

Нет: это расширение существующих Project authoring и native Call, без новой предметной сущности.

### Modified Capabilities

- `workflow-and-context`: две ветви extension, root scope, exact local target resolution и стандартные port/outcome проверки; заменить прежний общий запрет нескольких bindings ограничением legacy step ветви.
- `cli-protocol`: read-only prepare, freshness до dispatch, typed named refusals и читаемый native child handoff.
- `specification-governance`: расширить определение Workflow extension в словаре, сохранив различие Call, WorkflowInvocation и Run.

Normative source ownership не переносится ни у одной capability: current source sets остаются указанными в `openspec/SOURCE-OF-TRUTH.md`. `domain-execution`, `runtime-resources` и `published-contracts` сохраняют действующие общие правила; change проверяет их применение к вставке.

## Impact

Изменение влияет на продуктовый authoring/compiler и получаемое native runtime поведение, а не только на процесс репозитория. Основные области: `cmd/prifly/project.go`, `project_compile.go`, `project_build.go`, Project prepare/start/continue, `internal/flow` validation, authoring schemas и manifest в `internal/runtime/authoring/` / `schemas/authoring/`, capability inventory, runner templates, `examples/` и public CLI fixtures. Новый runtime operator, state migration или StepResult contract не требуются.

### Dependencies and related issues

Проверены **все три** открытых issue `StenHigh/prifly` на 2026-10-16, включая их comments (отсутствуют). Порядок ниже — рекомендация по приоритету, не формальное закрытие milestone.

| Issue | Отношение к E5 | Рекомендация |
|---|---|---|
| [#8](https://github.com/StenHigh/prifly/issues/8): unbound claim / scan >64 МиБ | Эксплуатационный blocker старта и cleanup на затронутой накопленной authority; не prerequisite compiler/native call | Отдельный change первым для этой authority: полный согласованный ownership scan и честный rollback/diagnostic. E5 fixtures исполнять в disposable authority; не выдавать это за устранение #8 |
| [#7](https://github.com/StenHigh/prifly/issues/7): SQLite logical-page budget | Blocker продолжения конкретного Run при исчерпании storage; другой лимит и другой recovery contract | Отдельный change перед продолжением такого Run. Не увеличивать лимит и не подменять authority в E5 |
| [#9](https://github.com/StenHigh/prifly/issues/9): native call extension | Scope этого change | Можно реализовывать независимо от #7/#8; пилот на затронутой authority ждёт соответствующего исправления |

Основание разделения подтверждено checkout `437e6a1c`: `checkLegacyClaimHolders` вызывает bounded `Store.ReadAll`, rollback в Project start игнорирует ошибку `ReleaseWorktree`; SQLite soft limit отдельно задан в `internal/local/store.go`. Дефекты не воспроизводились на пользовательской authority в этой работе.

Связанные active changes по recovery уже задают whole-child и inherited-prefix правила; E5 использует их и добавляет regressions для inserted call. Пакетный W6 в `prifly-aif-workflows` остаётся отдельным follow-up после stable release E5 и собственных consumer tests. Monitor/queue/W5 и выбор модели не являются prerequisites. Канонические AI Factory workflows, adapters и продуктовые repair policies не входят в change.

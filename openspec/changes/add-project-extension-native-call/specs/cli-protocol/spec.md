## ADDED Requirements

### Requirement: Project prepare закрепляет полную семантику вставленного call
Read-only prepare выбранного Project launch SHALL учитывать exact compiled
callee closure, typed bindings, outcome/error routing, execution bindings,
resources, effect/trust requirements и declared source/context dependencies
в existing review/freshness contract. Показанные источники SHALL включать все
фактически прочитанные declared files, включая project-owned child bundle.
Prepare MUST NOT создавать Run, claim, registry mutation или dispatch.

Start с `--expected-launch-digest` SHALL повторно проверять этот effective
launch. После изменения callee, его dependency, binding, route, resource или
declared source dependency прежний digest MUST отказываться до dispatch;
новые bytes MUST NOT исполняться под прежним review. Existing saved Runs
SHALL продолжать чтение pinned definitions без lookup по текущему extend.

Standalone `project extend`, располагающий только explicit step refs/sources,
SHALL сохранять legacy utility contract. При call записи он MUST возвращать
named refusal с указанием package-scoped `project compile`, а не угадывать
callee без полного source inventory.

При continue/recover CLI SHALL сверять effective accepted prefix, включая
subtree вставленного call. Несовместимый принятый child MUST давать named
refusal с причиной; допустимый explicit root replay point SHALL повторять
whole child по существующим правилам. Ранее принятые outputs MUST NOT
переиспользоваться при изменившемся effective contract. Unchanged compatible
prefix SHALL сохраняться вместе с provenance и source history.

#### Scenario: Review устарел до dispatch
- **WHEN** после prepare изменён child workflow, его supporting file, binding, outcome route или resource, а start получает прежний digest
- **THEN** CLI отказывает с named freshness diagnostic до выдачи Attempt/Session или запуска process

#### Scenario: Preparation остаётся read-only
- **WHEN** prepare читает root и project-owned child bundle для call extension
- **THEN** sources называют эти declared files, а authority, registry, claims и рабочее дерево остаются неизменными

#### Scenario: Изменившийся принятый child требует replay
- **WHEN** continue после accepted child обнаруживает изменение его effective contract
- **THEN** prepare отказывает в reuse с причиной; допустимый более ранний root `--from-stage` повторяет child целиком, если остальные условия resume выполнены

### Requirement: CLI объясняет call extension без второго execution protocol
Public compile и запуск SHALL выдавать typed named refusals с extension/stage
и offending field/target там, где такой контекст доступен. Invalid shape,
missing/wrong-kind/ambiguous callee, missing input, exact schema mismatch,
incomplete outcome routing и unavailable output SHALL иметь проверяемую
named причину без частичного исполнения. Коды отказов MUST передаваться через
Problem/Fault contract, а не извлекаться из текста ошибки.

Run read, next/task handoff и runner guidance SHALL представлять inserted
call как обычную native stage: exact child ref и записанные invocation/caller
связи, declared outputs/outcome и фактическую причину ожидания/ошибки в
подходящих существующих views. Call MUST NOT получать фиктивный StepResult,
worker или отдельный RunStart. Существующие версии public DTO SHALL
сохраняться, если их native call representation достаточна; новая closed
форма MUST публиковаться отдельной версией только при фактической необходимости.

Capability declaration SHALL отличать authoring extension от базового `call`.
Author index и extension reference SHALL показывать новый marker, полный
binding/outcome syntax, root scope и минимальные поддержанные profile / runtime
contracts; version выпуска SHALL называться только после его появления.

#### Scenario: Отказ доступен машинному клиенту
- **WHEN** released CLI компилирует fixture с отсутствующим target или не гарантированным export
- **THEN** ненулевой exit завершается Problem с named code/violations для причины и не создаёт child execution

#### Scenario: Host получает задачу внутри child
- **WHEN** native inserted call выдаёт descendant assisted Attempt
- **THEN** host получает обычное session задание этого Attempt в том же Run/claim/candidate контексте; ancestor call не выдаётся за исполняемый worker

#### Scenario: Child отменён
- **WHEN** отмена child оставляет caller waiting по native semantics
- **THEN** read/next показывает recorded child и причину ожидания, а guidance не обещает автоматический новый child или остановку внешнего browser actor

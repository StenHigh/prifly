## MODIFIED Requirements

### Requirement: Compact workflow folder остаётся одним графом
Workflow folder MUST быть единственным внешним Project package authoring source
и содержать root `workflow.yaml`, optional `extend.yaml` и one-document
components only in known schemas/contexts/steps/workflows paths. Directory
name MUST не создавать ref or control flow.

Simple extension SHALL заменять ровно один прямой route в root graph своей
workflow folder вставкой объявленного Project step либо native Call.
`workflow` MUST обозначать short name из ID root WorkflowRevision, а не callee,
filename или произвольный internal graph. `between.from` / `between.to` и все
targets маршрутов MUST быть существующими root stages; отсутствие или
неоднозначность route MUST отказываться до sealing.

Step ветвь MUST сохранять legacy stage ID из `step`, ограничение не более одного
input binding и прежнюю семантику StepResult verdicts / `impossible_verdicts`.
Call ветвь SHALL вставлять stage с ID extension `id`, несколькими typed bindings
и обычным контрактом `kind: call`. Привязки обеих ветвей MUST проверяться теми
же правилами, что привязки авторской стадии: названный выход MUST существовать,
producer MUST исполняться до consumer на каждом соответствующем пути, schema
refs MUST точно совпадать. Вставка MUST NOT вводить альтернативную проверку.

Вставка repeat, parallel, map, join, нескольких routes или рекурсивная правка
internal graphs MUST по-прежнему требовать явного graph. Сложный внутренний
граф callee сам по себе MUST NOT превращать единичную call вставку в patch
этого графа. Extension MUST NOT менять исходные bytes root и sibling workflows.

#### Scenario: Extension пытается изменить parallel join
- **WHEN** author описывает сложную вставку через `extend.yaml`
- **THEN** compiler отказывает и требует явный workflow graph

#### Scenario: Вставленный шаг читает результат предыдущего
- **WHEN** вставка объявляет один вход, связанный с выходом стадии, которая исполняется раньше на том же маршруте
- **THEN** `project compile` запечатывает привязку в граф, и вставленный шаг получает артефакт штатно, а не через пересказ в промпте

#### Scenario: Привязка к тому, чего нет или что ещё не исполнялось
- **WHEN** вставка связывает вход с выходом, которого у названной стадии нет, либо со стадией, не исполняющейся до потребителя
- **THEN** компиляция отказывает теми же отказами, что и для авторской стадии

#### Scenario: Call вставлен только в выбранный root
- **WHEN** `extend.yaml` выбранной folder вставляет call между двумя root stages, а package содержит siblings и child с внутренним call
- **THEN** compiled root содержит одну вставку, исходные bytes и семантика sibling graphs сохраняются, internal graph callee не получает рекурсивных вставок; ordinary `/3` compiled version/ref repinning не считается правкой исходных graphs

## ADDED Requirements

### Requirement: Call extension имеет явную authoring revision и discriminated форму
Новая форма `extend.yaml` SHALL объявлять `authoring: prifly-extension/2`.
Каждая запись `extensions` MUST содержать ровно одно из `step` или `call`,
непустые `id`, `workflow`, `between` и `on`. Отсутствие обеих ветвей или
одновременное указание MUST быть named compile refusal. В call ветви `call`
SHALL быть short name из ID объявленного local WorkflowRevision, а extension
`id` SHALL именовать вставленную root stage независимо от target. Stage ID
collision MUST отказываться, не заменяя существующую stage.

Call SHALL принимать optional `input_bindings` с нулём или несколькими
элементами в пределах обычного call contract и optional `on_error`, адресующий
существующую root stage. Binding expression SHALL иметь форму
`$stages.STAGE.PORT`, доступную в scope root graph. `impossible_verdicts` MUST
быть разрешён только в step ветви. `on` у call MUST проверяться по exact
`allowed_outcomes` callee, не по StepResult verdicts; каждый разрешённый callee
outcome MUST иметь маршрут. Имена вне допустимого набора MUST отказываться.

Немаркированный legacy `extend.yaml` MUST сохранять step-only contract, прежние
defaults, stage IDs, ограничения и sealed/build bytes неизменных входов. Call
без нового marker и неизвестный marker MUST отказываться до sealing. Новый
editor contract SHALL публиковаться отдельно от сохранённого `extension-v1`,
поддерживать explicit version selection и однозначную automatic mapping.
Новый call path SHALL требовать Project profile `/3`; `/2` MUST сохранять
прежний смысл без автоматической миграции.

#### Scenario: Трёхвходовая вставка объявлена явно
- **WHEN** marker `/2` и Project profile `/3` задают `call: project-quality`, три stage-output bindings и все разрешённые outcomes
- **THEN** compiler создаёт native call stage с ID extension `id`, не создавая новые request/schema/finish stages по именам

#### Scenario: Legacy step не мигрирует автоматически
- **WHEN** прежняя немаркированная step fixture компилируется новым CLI без изменения входов
- **THEN** сохраняются stage ID, binding limit, verdict semantics и прежние sealed refs / b1 vectors

#### Scenario: Ветви смешаны или новая форма не выбрана
- **WHEN** запись содержит step и call, ни одной ветви, call с `impossible_verdicts`, call без marker либо call в profile `/2`
- **THEN** компиляция возвращает named refusal без запуска или неявного обновления profile

### Requirement: Call extension разрешает только объявленный exact local callee
Call target SHALL разрешаться среди workflow components одной выбранной folder,
включая `workflows/` и `project/workflows/`, по short name из authored ID.
Resolver MUST получить ровно один WorkflowRevision, проверить kind и запечатать
его exact compiled `id`, `version`, `digest` вместе со всем исполняемым closure.
Все owned refs SHALL связываться с compiled identities по обычным правилам
Project `/3`, а external refs внутри closure SHALL оставаться exact.

Unknown target, wrong-kind target, неоднозначный short name/identity,
несогласованный digest, отсутствующая dependency или call/repeat cycle MUST
давать named refusal до Run admission. Результат MUST NOT зависеть от порядка
YAML filenames или наличия «последней установленной» package edition.
Установленный package или sibling folder MUST NOT становиться целью поиска
необъявленно. Явно вендоренный child bundle SHALL быть достаточным; automatic
cross-package import не требуется.

#### Scenario: Переименование файлов не выбирает другую revision
- **WHEN** declared local child с тем же authored content расположен раньше или позже parent в порядке чтения файлов
- **THEN** компиляция разрешает тот же exact callee и closure; filename не выбирает версию или stage

#### Scenario: Resolver не может доказать единственную цель
- **WHEN** target отсутствует, существует только как step/schema, неоднозначен либо создаёт цикл
- **THEN** public compile отказывает с named diagnostic и не выбирает произвольную цель из registry

### Requirement: Call extension сохраняет typed inputs и гарантии outputs по outcome
Call extension SHALL проходить ordinary native call проверку declared ports,
required inputs, producer reachability и exact schema refs. Compiler MUST
отвергать неизвестный input/output, поздний или недостижимый producer,
незаполненный обязательный input и несовпадающий schema ref, даже когда JSON
shape похож. Existing configuration scope/default rules SHALL сохраняться:
порт с именем `configuration` не становится project-scoped без такого объявления.
Bindings MUST NOT обходить запрет override project-scoped configuration.

Downstream root stages SHALL иметь доступ к declared exports через
`$stages.<extension-id>.<port>` по обычной guaranteed-output проверке каждого
достижимого outcome. Приватные child stages MUST NOT становиться root producers.
Отсутствующий export MUST оставаться отсутствующим; error/cancel path MUST NOT
получать придуманные outputs или defaults вставки.

#### Scenario: Review и UI/UX contracts используют обычные порты
- **WHEN** один child объявляет input `request` и outputs `report/gate`, а другой — inputs `configuration/ui/ux` и outputs `gate/ui_report/ux_report/environment/release`
- **THEN** оба компилируются с exact declared schemas и передают downstream только exports, гарантированные на выбранном outcome; Core не назначает этим именам продуктовую policy

#### Scenario: Partial route читает только success output
- **WHEN** child гарантирует output только на `succeeded`, а downstream после `partial` требует его
- **THEN** компиляция отказывает по ordinary unavailable-output rule; placeholder и пустой success artifact не создаются

### Requirement: Вставленный call исполняется как обычное native включение
Вставленный call SHALL создавать WorkflowInvocation внутри того же Run, с теми
же claim/candidate контекстом и ordinary policy. Он MUST NOT запускать
`project start` через program/assisted step, независимый Run или скрытый bridge.
Declared child outcomes SHALL маршрутизироваться по `on`; child failure SHALL
следовать native `on_error` при его наличии, а cancellation SHALL сохранять
native waiting/settlement semantics. Partial/blocked assessment MUST NOT
превращаться в успех неявно; явный авторский route SHALL проверяться обычным
контрактом callee и parent без product-specific policy Core.

Whole closure SHALL сохранять effect/trust ceilings, child-depth, step,
transition, storage и resource limits, включая descendant calls. Resource,
объявленный для root call stage, SHALL действовать на его descendant Attempts
по existing resource contract. Native cancel MUST NOT объявляться доказательством
остановки внешнего actor. Continue/recover SHALL применять existing effective
accepted-prefix rules: replay root call выполняет весь child заново, а reuse
его accepted exports требует совместимого полного контракта subtree.

#### Scenario: Исходы и технический сбой различимы
- **WHEN** child заканчивается каждым из своих declared outcomes, технически отказывает либо отменяется
- **THEN** parent следует authored outcome/error route либо остаётся waiting по native cancel semantics, и его read state сохраняет child identity и причину

#### Scenario: Policy не допускает глубину или effect
- **WHEN** вставленный child или его descendant требует превышения current policy, ресурса либо бюджета
- **THEN** штатный admission/compile refusal предотвращает недопущенную работу без повышения limits вставкой

#### Scenario: Принятый child изменился перед продолжением
- **WHEN** effective accepted prefix включает child, чей contract/bindings/routes изменены новым package
- **THEN** продолжение отказывает с named причиной либо оператор выбирает допустимую более раннюю root replay point; stale exports не переиспользуются и source Run остаётся неизменным

### Requirement: Project явно связывает программы owned child steps без правки upstream
В marked extension `/2` `execution_bindings.steps` SHALL разрешать существующую
форму binding по short name не только вставленного step, но и объявленного
owned program step, достижимого в closure вставленного call. Legacy unmarked
scope MUST оставаться прежним. Неоднозначный short name, target вне такого
closure, wrong-kind или попытка заменить existing package program binding MUST
отказываться. Само имя callee MUST NOT генерировать executable или binding.

Программы и supporting files SHALL seal-иться и проверяться по existing
execution contract, с ordinary local allowlist и explicit execution consent.
Call stage MUST NOT получать program binding вместо leaf step. Поле MUST NOT
становиться способом заменить команды произвольных upstream/sibling stages.

#### Scenario: Child program доставлен явно
- **WHEN** проект объявляет local child program step и его execution binding в marked extend, а root source не меняется
- **THEN** compilation запечатывает точные program/supporting bytes для этого leaf step; start требует ordinary execution consent и исполняет его внутри native child

#### Scenario: Binding пытается изменить чужую программу
- **WHEN** binding адресует несвязанный sibling step, неоднозначное имя либо уже связанную upstream program definition
- **THEN** compilation отказывает до исполнения, не заменяя package program

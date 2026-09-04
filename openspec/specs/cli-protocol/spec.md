## Purpose

Определяет versioned public command protocol Pri-Fly: DTO, CLI, validation,
executor boundary, errors, preview и compatibility без прямого изменения
authority клиентом.

## Requirements

### Requirement: Все клиенты используют один command protocol

CLI, local UI, assisted host и managed executor MUST применять один versioned
protocol. Только command handler меняет authority; UI, model и skill не пишут
state напрямую. Remote transport требует отдельной qualification security
properties и не является обязательным.

#### Scenario: UI пытается изменить state напрямую

- **WHEN** client обходит command handler
- **THEN** authority не принимает mutation

### Requirement: Project entry points select their host mechanically
`prifly project init` SHALL записывать fixed repository-relative skills roots
для `codex-cli`, `codex-app` и `claude-code` и создавать один `prifly-run`
entry point внутри каждой соответствующей host directory. Каждый entry point
SHALL вызывать Project compilation со своим host identity. Public compile
command MUST требовать host identity и MUST NOT выводить его из существующих
directory. Fresh init MUST отвергать unsafe root или existing runner, не
перезаписывая runner или profile. Для valid tracked profile после clone init
MUST проверить exact runners и создать только отсутствующий ignored `local.yaml`;
он не переписывает shared profile или runner. Эти entry points поддерживают
Codex CLI, Codex app и Claude Code, не делая ни один из них Core dependency.

#### Scenario: Claude Code запускает общий проект
- **WHEN** developer вызывает `prifly-run` из `.claude/skills`
- **THEN** он компилирует с host `claude-code` и никогда не читает Codex skills
  root

#### Scenario: Existing host runner останавливает init
- **WHEN** любой из трёх путей runner уже существует
- **THEN** fresh init возвращает safe diagnostic и не создаёт profile или другой runner

#### Scenario: Clone получает только local authority configuration
- **WHEN** repository уже содержит valid tracked Project profile и exact runners,
  но не содержит ignored `.prifly/local.yaml`
- **THEN** init создаёт только эту local configuration и не меняет shared YAML
  или host runners

### Requirement: Wire framing имеет strict version и limits

Closed DTO MUST различать protocol, resource, semantic, state and read versions.
Wire использует strict UTF-8 JSON, one-object framing, clean JSON stdout and
bounded bytes/depth/nodes before schema validation; unknown fields не расширяют
старый contract.

#### Scenario: Request превышает recursion limit

- **WHEN** JSON выходит за declared depth или node cap
- **THEN** handler отказывает до dispatch без truncation

### Requirement: Shape validation не является admission

Handler MUST последовательно проверить framing, authenticated context, selected
DTO schema, current read access, dedup, exact refs/digests/provenance, semantic
gates, authority/quotas/resources и atomic commit. Old receipt может вернуться
после current read check, но не разрешает new dispatch.

#### Scenario: Receipt запрашивает actor без доступа

- **WHEN** caller потерял право читать protected result
- **THEN** handler не раскрывает receipt через idempotent retry

### Requirement: Canonical identity сохраняет exact bytes

Digest MUST name its bytes/scope, exclude self-reference and use declared
canonicalization. Blob digest covers actual bytes; resource identity is not
silently case- or Unicode-coerced. Timestamps, control keys and numeric data
follow declared exact formats without float or lexical shortcuts.

#### Scenario: Два Unicode-identifiers похожи визуально

- **WHEN** resolver не объявил их equivalence
- **THEN** protocol сохраняет distinct identities

### Requirement: Public DTO имеет named authority происхождения

Definitions, configuration, Run, execution, data, effects, controls,
composition and exchange DTO MUST name which authority creates their facts.
Read model is not universal save command; actor, epoch, received time and state
version are not accepted from caller as evidence.

#### Scenario: Client задаёт control epoch

- **WHEN** mutation payload содержит self-asserted authoritative field
- **THEN** handler derives it from authority rather than trusting payload

### Requirement: StepDefinition описывает contract без self-qualification

StepDefinition MUST declare exact kind, ports, executor/operation, context,
capabilities, effect/retry class and result checks. Declaration of an effect,
sandbox or retry property does not prove it; unknown executor, missing required
context or conflicting capability blocks admission.

#### Scenario: Package заявляет class none и пишет data

- **WHEN** proposed operation exceeds declared capability
- **THEN** admission rejects it

### Requirement: Ports явно описывают required data

Input/output ports MUST use exact typed contracts, required semantics, JSON
schema or blob media/content checks. `skipped`, `waived` and technical failure
are distinct; an absent required producer output cannot be manufactured for a
consumer.

#### Scenario: Required output пропущен

- **WHEN** downstream binding требует output skipped producer
- **THEN** workflow cannot admit consumer without declared optional path

### Requirement: InputBinding выбирает declared provenance

Binding MUST explicitly select workflow input, stage output, literal or allowed
scope with exact producer/port/ref semantics. It does not select a similar or
latest artifact implicitly; unavailable, ambiguous or unsupported projection
fails before execution.

#### Scenario: Два producer имеют совместимый type

- **WHEN** binding does not name one producer
- **THEN** validation reports ambiguity

### Requirement: Workflow graph имеет finite typed transitions

Definition MUST declare valid stage kinds, typed bindings, terminal outcomes and
finite composition. Unsupported control kind fails early; errors, calls, repeat,
parallel, map and waits only exist under their versioned contracts.

#### Scenario: Graph содержит неразрешённый cycle

- **WHEN** resolution detects cycle or bound overflow
- **THEN** Run is not created

### Requirement: Choice использует declared three-valued semantics

Choice MUST evaluate bounded typed predicates with explicit missing, null, type
error and unknown behavior. Exclusive ambiguity and first-match order are
declared; prose or model confidence cannot select branch.

#### Scenario: Predicate возвращает unknown

- **WHEN** required value is unavailable
- **THEN** contract follows declared unknown path, not convenient default

### Requirement: Parallel aggregate declares quorum и remainder

Parallel MUST name membership, join/selection/quorum rule, residual branches
and aggregate outcome. Early winner does not prove loser cancellation or release
of claims/effects.

#### Scenario: Quorum достигнут

- **WHEN** remaining admitted branch has unknown effect
- **THEN** aggregate does not hide its obligation

### Requirement: Map seals collection до children

Map MUST validate complete input collection, item schema, unique stable keys and
max_items before first child admission. Empty collection uses declared `on.empty`;
item provenance and concurrency remain bounded.

#### Scenario: Collection меняется после start

- **WHEN** new member appears outside sealed manifest
- **THEN** existing map does not create hidden child

### Requirement: Repeat сохраняет persistent bounds и decision state

Repeat MUST declare finite iterations, body, exit decision and bindings. Each
accepted iteration and control transition records state; restart, delivery retry
or whole-step retry does not reset repeat counter or become semantic rework.

#### Scenario: Delivery retry произошёл внутри body

- **WHEN** same logical operation is resent safely
- **THEN** repeat iteration count remains unchanged

### Requirement: Wait и schedule имеют durable correlation

Wait MUST register expected signal/timer before producer effect, validate source
identity/schema/correlation and deduplicate delivery. Early, late, cancelled and
rate/byte/TTL constrained events follow declared contract; assisted mode does
not promise wakeup without host.

#### Scenario: Late callback приходит после cancel

- **WHEN** wait уже closed
- **THEN** callback does not reopen Run

### Requirement: Compensation сохраняет original effect history

Compensation MUST be finite typed child work with scoped context, preconditions,
current rights, evidence and budgets. It relates to original operation but does
not delete receipt or invent rollback for unsupported target.

#### Scenario: Compensation не может быть выполнена

- **WHEN** precondition or right is absent
- **THEN** residual effect remains visible

### Requirement: Admission и retry имеют разные identities

ExecutionAdmission, per-action Admission, delivery retry, whole-step retry and
semantic rework MUST have distinct identities and preconditions. A worker has
no arbitrary tool authority; current stop/revocation/budget applies to every
new dispatch.

#### Scenario: Whole-step retry меняет model

- **WHEN** retry would use another provider or model
- **THEN** it requires compatible new revision, not old retry identity

### Requirement: Delivery status не равен effect status

Protocol MUST distinguish preparation, dispatch, response and observation from
`not_started`, `not_applied`, `applied`, `partially_applied` and `unknown`
effect status. Async acknowledgement may retain pending null outcome; final
receipt cannot silently convert unknown to success.

#### Scenario: HTTP accepts operation asynchronously

- **WHEN** adapter returns accepted response without terminal observation
- **THEN** effect remains pending rather than applied

### Requirement: CAS, dedup и stop имеют отдельные semantics

Normal mutation MUST use scoped expected version and principal-aware command
dedup. Relevant input/attempt/resource checks protect parallel results; stale
UI may monotonically restrict through stop, but only exact release and later
resume can remove it.

#### Scenario: Новый stop появился после release

- **WHEN** resume sees applicable current stop
- **THEN** handler refuses continuation

### Requirement: Approval учитывает current authority при consume и dispatch

Approval/Grant MUST bind exact intent, actor, policy, scope, deadlines and
constraints. Consume is atomic with Admission; safe delivery retry does not
reconsume it, but dispatch rechecks stop/revocation/host permission. Redacted
derived output keeps its own provenance.

#### Scenario: Arguments approval изменены

- **WHEN** target, account or protected parameter changes
- **THEN** previous approval cannot be reused

### Requirement: Result intake проверяет exact Attempt и sealed output

SubmitResult MUST verify active identities, envelope digest, ports, seals,
evidence, receipts, relevant claims and current controls. Progress/heartbeat is
not terminal result; late result is evidence with rejection reason, not input
for another Attempt. Отказ intake MUST быть named refusal, называющий предмет:
output port, который обязателен для reported verdict и не отчитан либо не
объявлен step, или identity/digest выхода, не совпавшие с admitted slot.
Для assisted submission эти проверки и result schema шага MUST выполняться
при intake до записи candidate: отклонённая отправка оставляет handoff
awaiting и MUST NOT создавать failed Attempt или terminal Run. Sealing выходов
остаётся частью acceptance.

#### Scenario: Output изменён после hash

- **WHEN** worker submits modified bytes under prior digest
- **THEN** acceptance rejects result

#### Scenario: Обязательный выход не отчитан

- **WHEN** submitted StepResult не содержит output port, обязательный для его
  verdict
- **THEN** intake отказывает stable code, pointer и message называют этот
  port, а Run не меняется

#### Scenario: Неверная отправка не сжигает Attempt

- **WHEN** assisted host присылает StepResult, который не прошёл бы acceptance
  по result schema, портам, identity или digest
- **THEN** submission отклоняется до записи candidate, handoff остаётся
  awaiting, и host может прислать исправленный отчёт под той же envelope

### Requirement: CLI exposes scoped commands without hidden mutation

CLI MUST provide typed installation, authoring, preview/start, observation,
control, correction, decision and export commands through safe JSON/file input.
Read-only `next`, explain and events do not dispatch; init/install/remove and
project-wide control use their own scopes, not Run CAS.

Форма вызова MUST быть читаема из самого инструмента и MUST NOT требовать
authority: запрос справки у любой команды, запрос версии и перечисление
доступных публичных контрактов MUST отвечать своей информацией, а не отказом
про ненайденный объект. Запрос справки по теме MUST возвращать только эту тему.
Имя публичного контракта MUST приниматься и в форме declared reference, под
которой он назван в handed задании; этот ответ MUST оставаться функцией самого
binary. Компонент установленного trusted package MUST читаться по своему
declared ID командой чтения package, без обращения к файлам внутри authority.
Один contract MUST читаться без остального bundle. Установленный путь к binary MUST меняться
объявленной командой, а не ручной правкой machine-only файла.

#### Scenario: User calls next for blocked child

- **WHEN** child cancellation holds caller waiting
- **THEN** CLI reports scoped blocked state without starting a worker

#### Scenario: Пользователь запрашивает форму подкоманды

- **WHEN** к любой подкоманде передан запрос справки
- **THEN** CLI печатает её строку использования и не открывает authority

#### Scenario: Исполнителю нужен список контрактов

- **WHEN** `schema` вызван без имени
- **THEN** CLI перечисляет доступные имена контрактов вместо требования
  назвать точное имя

#### Scenario: Задание называет контракт declared reference-ом

- **WHEN** имя контракта запрошено в той форме, в которой оно названо в
  задании
- **THEN** CLI отдаёт тот же контракт, что и по его имени

#### Scenario: Автору нужна форма authoring-документа

- **WHEN** автор ищет форму YAML-документа, который он пишет сам
- **THEN** её имя перечислено рядом с контрактами обмена и отдаётся той же
  командой

#### Scenario: Оператор заполняет выход по schema пакета

- **WHEN** запрошен компонент установленного trusted package по его declared
  ID
- **THEN** команда чтения package отдаёт его bytes, а команда контрактов
  по-прежнему отвечает только за то, что несёт binary

#### Scenario: Нужна одна форма из большого bundle

- **WHEN** запрошен один contract из bundle
- **THEN** CLI отдаёт только его определение и его закрытие

#### Scenario: Отказ называет неприменимую команду

- **WHEN** проверка графа запрошена для пути вне authority
- **THEN** отказ называет команду, которой проверяется авторская папка без
  создания Run

#### Scenario: Автор называет компонент его полным идентификатором

- **WHEN** расширение ссылается на компонент именем, которого нет среди
  объявленных
- **THEN** отказ перечисляет известные имена, а не только отвергнутое

#### Scenario: Подготовка стадии отказывает до admission

- **WHEN** подготовка стадии отказывает refusal-ом со stable code
- **THEN** diagnostic несёт этот code, а не только фазу подготовки

#### Scenario: Запечатанный package не разрешается при запуске

- **WHEN** launch не находит только что запечатанный package среди доверенных
- **THEN** отказ называет его identity и причину, а не сообщает о ненайденном
  файле

### Requirement: CLI объявляет результат explicit binary update
Public CLI MUST предоставлять `prifly update` как отдельную команду без
`--project` prerequisite. Её structured result MUST различать current version,
installed version и отказ; update не смешивается с command protocol Run и не
создаёт authority mutation. Invalid arguments MUST завершаться existing
`invalid_usage` diagnostic.

#### Scenario: Managed installation получает новую версию
- **WHEN** пользователь запускает `prifly update` и signed compatible Release
  новее installed version
- **THEN** CLI сообщает прежнюю и установленную version только после успешной
  atomic replacement

#### Scenario: Вызов содержит неожиданный аргумент
- **WHEN** пользователь передаёт не поддержанный аргумент команде `update`
- **THEN** CLI возвращает `invalid_usage` и не начинает network operation

### Requirement: Executor interface has no general state.write

Executor MUST receive only bounded attempt/operation DTOs for prepare, propose,
admit, report, result, signal and liveness. It MUST NOT select successor, grant
approval, edit workflow or write authority state; missing host capability MUST
be reported as unsupported rather than prose substitution.

#### Scenario: Helper prints an instruction

- **WHEN** helper cannot provide dispatch receipt
- **THEN** protocol treats it as preview, not started execution

### Requirement: Problem и exit code сохраняют safe meaning

Problem MUST include stable code, message, correlation, violations and safe
next action without secrets or foreign-object detail. `retryable` describes
command/check retry only. CLI exit zero means read or command commit, not Run
success; typed result carries workflow state. Runtime refusal, поднятый со
stable code, MUST доходить до клиента под этим code независимо от того,
сопровождён ли он message; engine-authored detail такого отказа (port, path,
version) MUST сообщаться в `violations`. Только текст без stable code MUST
схлопываться в `invalid_input`, и такой текст MUST NOT попадать в ответ.

Отказ MUST различать классы отсутствия: отсутствующая authority по выбранному
пути, отсутствующий объект внутри существующей authority и существующий объект
без запрошенного состояния MUST иметь разные stable codes. Отказ MUST NOT
утверждать отсутствие объекта, который движок держит. Usage refusal
глобального аргумента MUST повторять полученное значение, чтобы обрезанный
shell-ом путь отличался от дефекта инструмента.

#### Scenario: External effect is unknown

- **WHEN** command reports unknown effect
- **THEN** safe next action is exact reconciliation, not blind retry

#### Scenario: Refusal поднят без сопроводительного message

- **WHEN** runtime отказывает stable code без detail
- **THEN** Problem несёт этот code, а не `invalid_input`

#### Scenario: Refusal несёт engine-authored detail

- **WHEN** runtime отказывает stable code с detail о предмете отказа
- **THEN** Problem несёт этот code и detail в `violations`, без raw parser
  input, argv, environment или foreign payload

#### Scenario: Выбранный путь не содержит authority

- **WHEN** команда выполняется с `--project`, указывающим на каталог без
  authority
- **THEN** отказ называет отсутствие authority по этому пути и отличается от
  отказа про ненайденный Run, definition или artifact

#### Scenario: Run существует, но передачи нет

- **WHEN** host запрашивает удерживаемую передачу Run, который существует и
  не держит ни одной
- **THEN** отказ называет отсутствие активной передачи и предлагает чтение
  состояния и drive, а не поиск Run

#### Scenario: Аргумент обрезан вызывающей стороной

- **WHEN** глобальный аргумент получен в непригодной форме
- **THEN** usage refusal показывает полученное значение

### Requirement: Preview и validation не создают effect

Preview MUST resolve declared refs and display graph, potential effects/resources,
gates and limits without starting code, paid provider, message or approval.
Validation MUST distinguish shape, refs, graph, capability, current authorization
and executability; simulation MUST be fixture-only and not qualification.

#### Scenario: Preview needs external read

- **WHEN** preview would fetch external data
- **THEN** it requires separate declared admission

### Requirement: Extension changes semantics only versionedly

New package/step may use existing protocol, but predicate, stage kind or
authorization primitive MUST receive versioned semantics, compatibility decision
and conformance tests. Unknown retained bytes can export but unsupported reader
does not execute them.

#### Scenario: Package hides control flow in metadata

- **WHEN** extension adds undocumented router behavior
- **THEN** validation rejects it as unsupported semantics

### Requirement: Protocol delivery distinguishes schema evidence from qualification

Delivery MUST include versioned schemas, valid/invalid fixtures, command
inventory and checked links/limits. Shape tests do not prove filesystem,
process, remote effect, human identity or full runtime qualification; report
states this boundary explicitly.

#### Scenario: Schema validates a remote effect DTO

- **WHEN** fixture passes JSON Schema
- **THEN** report does not claim remote effect was executed or authorized

### Requirement: CLI запускает declared Project workflow с explicit workspace mode

CLI MUST provide one typed `project start` command for a declared Project
launch. It MUST require repository, launch ID, host and RunBrief/input sources;
it MUST accept only `worktree` and `checkout` as workspace mode. When invoked
without an interactive host, omitted mode MUST default to `worktree`. Invalid
launch, host, input, repository identity or workspace mode MUST return a stable
diagnostic without partial package registration, claim or Run. The response
MUST name Run and selected Workspace identities.

#### Scenario: CLI starts default isolated workspace
- **WHEN** user starts a valid Project launch without workspace flag
- **THEN** response reports an isolated worktree Workspace and its Run identity

#### Scenario: CLI rejects an unknown workspace mode
- **WHEN** user passes a workspace mode other than `worktree` or `checkout`
- **THEN** CLI returns `invalid_usage` and creates no package, claim or Run

### Requirement: Assisted handoff сообщает versioned declared Workspace tree bindings

Versioned assisted SessionTask MUST сообщать host finite declared Workspace tree
bindings: manifest input/output port names, typed capture policy, expected input
manifest ArtifactRef при его наличии и permitted typed location form for
output-only creation. Host MUST не получать authority handle, artifact-store
path или право выбирать другой Workspace path. Сопоставленный
SessionSubmission MUST использовать version того SessionTask, который был
handed Attempt; старые retained session versions остаются читаемыми и не
получают новую tree-binding семантику.

Runtime MUST capture declared output trees itself before accepting terminal
StepResult при каждой submission шага с declared bindings, независимо от
поддерживающей деревья session version и от наличия `workspace_trees` в
отправке. Host MUST называть capture location только там, где выбирает её
сам: для policy с единственным допустимым значением (`exact_file`) runtime
MUST брать declared path, а отсутствие location MUST NOT быть отказом. Host
MAY report only selected output-only capture location и MAY
повторить declared input location для binding, объявленного и входом, и
выходом, так что форма отправки одинакова для обоих видов binding; путь,
отличный от declared input location, MUST отклоняться named refusal. Host
MUST not подменять WorkspaceTreeManifest, contained ArtifactRef, digest или
capture policy prose-строкой либо arbitrary JSON. Unknown binding field,
version или несовпадение handoff/submission MUST отклоняться до изменения Run.

#### Scenario: Host получает зафиксированную форму Ultra bundle
- **WHEN** assisted workspace-write Attempt имеет output-only direct-child tree
  binding
- **THEN** SessionTask показывает declared parent and typed bundle form, а host
  не может заявить ArtifactRef, entry outside parent или другой output policy
  в result

#### Scenario: Host повторяет declared input location
- **WHEN** binding объявляет одно дерево и входом, и выходом, а submission
  называет для его output port путь, равный declared input location
- **THEN** runtime принимает отправку как при output-only binding и capture-ит
  дерево по declared location

#### Scenario: Host называет другой путь для input binding
- **WHEN** submission называет для такого port путь, отличный от declared
  input location
- **THEN** intake отказывает named refusal до изменения Run

#### Scenario: Exact-file binding без названной location
- **WHEN** submission не называет location для output-only binding с capture
  policy `exact_file`
- **THEN** runtime capture-ит declared path и принимает отправку, не требуя
  повторить единственное допустимое значение

#### Scenario: Submission без workspace_trees для input+output binding
- **WHEN** submission поддерживающей деревья версии не содержит
  `workspace_trees`, а step объявляет binding с входом и выходом
- **THEN** runtime capture-ит дерево по declared input location и заполняет
  output port сам, не требуя от host повторить путь

### Requirement: Project launch принимает typed per-Run decision selection
`project start` MUST accept an explicit package-profile selection and typed
answers only for declared preflight decision IDs. Interactive host launch MUST
ask for an omitted required selection before compilation; non-interactive
launch MUST return a stable missing-decision diagnostic unless a sealed default
rule fills it. Structured launch result MUST name selected profile, decision
catalog digest and decision ledger reference.

#### Scenario: CLI запускает Ultra без изменения проекта
- **WHEN** developer supplies declared `ultra` package-profile to `project start`
- **THEN** CLI creates an Ultra Run and leaves all tracked workflow files
  unchanged

#### Scenario: Non-interactive launch не имеет обязательного выбора
- **WHEN** required preflight decision has neither explicit answer nor allowed
  default
- **THEN** CLI returns a stable diagnostic before compilation and Run creation

#### Scenario: Анкета устарела до запуска
- **WHEN** host передаёт catalog digest из questionnaire, а tracked catalog
  изменился до `project start`
- **THEN** CLI returns `project_start_stale_decision_catalog` before package,
  Workspace claim or Run creation

### Requirement: CLI передаёт lifecycle runtime-решения versionedly
Public CLI MUST expose typed request, read and answer operations for a pending
decision. Compatible executor's request command MUST require its current
Attempt, envelope, declared decision ID and Run generation; CLI derives the
definition digest only from the sealed catalog, never from executor prose.
Answer command MUST require Run ID, decision ID, request/version identity and
typed value; it MUST report accepted, stale, conflict, schema-invalid or
not-pending result without hidden dispatch. Read output MUST be safe to render
by every supported host and contain no secret answer bytes outside authorized
scope.

#### Scenario: Пользователь отвечает из другого host
- **WHEN** second authorized host reads a pending decision and submits its
  current typed answer
- **THEN** CLI accepts it once and the first host can observe the same ledger
  transition after reconnect

#### Scenario: Совместимый adapter отправляет declared request
- **WHEN** adapter вызывает `run decision RUN_ID request` с identity текущего
  SessionTask и declared runtime ID
- **THEN** CLI передаёт ровно sealed definition в Universal Decision Bridge и
  не принимает digest либо вопрос, придуманный adapter-ом

### Requirement: CLI управляет Project workflow folders из репозиториев явными командами
`prifly project workflows` без аргументов MUST по-прежнему перечислять
declared launches. Дополнительно CLI MUST предоставлять
`search [QUERY] [--category ID] [--catalog URL]`,
`add SOURCE [--ref REF] [--path DIR] [--name NAME] [--catalog URL]`,
`update NAME [--ref REF]` и `remove NAME`; `add`, `update` и `remove`
принимают общий `--repository DIR`, `search` не требует repository, и ни одна
из них не требует `--project` и не открывает authority. `SOURCE` MUST толковаться
механически: имя без `/` и `:` — запись каталога; `owner/repo` — GitHub HTTPS
repository; иначе Git URL или абсолютный локальный путь. Опущенный
`--catalog` MUST использовать встроенный официальный каталог
`https://github.com/StenHigh/prifly-workflows.git`; явный `--catalog URL`
переопределяет его для одной команды и MUST проходить те же проверки, что и
`SOURCE`. Сеть MAY выполняться только во время `search`, `add` и
`update`; `init`, `workflows`, `questionnaire`, `compile` и `start` MUST NOT
её использовать. Результаты MUST быть typed JSON с `schema_version`
`project-workflow-catalog/1`, `project-workflow-add/1`,
`project-workflow-update/1` и `project-workflow-remove/1`. Неверные
аргументы MUST давать `invalid_usage` до сети; отказы MUST использовать
stable codes, среди них `project_workflow_source_invalid`,
`project_workflow_repository_unreachable`,
`project_workflow_repository_empty`,
`project_workflow_repository_ambiguous`, `project_workflow_exists`,
`project_workflow_package_conflict`, `project_workflow_origin_missing`,
`project_workflow_modified`, `project_workflow_commit_mismatch`,
`project_workflow_catalog_invalid` и `project_workflow_catalog_entry_unknown`.

#### Scenario: Сценарий установлен по имени каталога
- **WHEN** пользователь вызывает `add NAME --catalog URL`
- **THEN** ответ `project-workflow-add/1` называет package identity, origin и
  launch, а authority не открывается

#### Scenario: Repository неоднозначен
- **WHEN** repository содержит несколько сценариев и `--path` не задан
- **THEN** CLI возвращает `project_workflow_repository_ambiguous` с перечнем
  путей и не меняет `.prifly/`

#### Scenario: Неверный SOURCE
- **WHEN** пользователь передаёт относительный путь, URL с credentials или
  аргумент с ведущим `-`
- **THEN** CLI возвращает stable diagnostic до любой сетевой операции

### Requirement: Host runner предлагает поиск и установку сценария одним вопросом
Runner `prifly-run` MUST содержать инструкции: по явной просьбе разработчика
найти или установить сценарий выполнить `project workflows search --json`,
показать категории и записи одним native finite вопросом, после выбора
вызвать `project workflows add NAME` и предложить разработчику проверить и
закоммитить изменения `.prifly`. Runner MUST NOT устанавливать сценарий без
явного выбора и MUST NOT начинать Run как часть установки.
`project runners update` MUST распознавать прежний exact runner и заменять
его; кастомизированный runner по-прежнему отказывается.

#### Scenario: Разработчик просит установить сценарий
- **WHEN** host получает просьбу найти или установить workflow
- **THEN** он показывает список из `search --json` одним вопросом и вызывает
  `add` только для выбранной записи

#### Scenario: Runner обновлён после изменения
- **WHEN** repository содержит exact runner предыдущей версии
- **THEN** `project runners update` заменяет его новым, не трогая
  кастомизированные файлы

### Requirement: Handoff описывает, что требуется от host

Sealed handoff MUST быть самодостаточным описанием ожидаемого от host: каждая
закреплённая запись контекста MUST быть идентифицируема из самого bundle, без
опоры на порядок перечисления, а выходные слоты MUST быть разделены на те,
которые заполняет host, и те, которые движок закрывает сам объявленным
захватом. Host MUST NOT восстанавливать эту раскладку из содержимого файлов или
из прошлых прогонов.

Форма, в которой host публикует выход, MUST быть записанной, а не свойством,
выводимым из устройства хранилища: инструкции сгенерированного host runner и
authoring reference шага MUST называть, куда пишутся bytes слота и какие поля
несёт соответствующая запись reported result. Host MUST NOT выводить эту форму
из content-addressed storage, чужого прогона или чужой попытки. Baseline
StepResult schema MUST оставаться byte-identical: её digest закреплён в
sealed packages, поэтому аннотация в ней разорвала бы уже подписанные
identity.

#### Scenario: Bundle содержит несколько закреплённых записей контекста

- **WHEN** шагу закреплены skill и его bridge
- **THEN** host определяет, что есть что, по самому bundle, а не по порядку
  ссылок

#### Scenario: Часть выходов закрывается захватом

- **WHEN** шаг объявляет и обычный выход, и выход с привязкой workspace tree
- **THEN** handoff называет, какой слот host заполняет сам, а какой движок
  закрывает захватом

#### Scenario: Host впервые публикует не-древесный выход

- **WHEN** host заполняет слот, который движок не закрывает захватом
- **THEN** форма публикации читается из published contract, без вывода её из
  устройства artifact storage

### Requirement: Read-only виды не занижают то, что движок держит

Read-only вид MUST называть то, что считает, и MUST NOT показывать нулевое
значение для состояния, которое движок держит. Сводка Run MUST различать
выходы Run и запечатанные выходы его шагов. Verdict принятого шага MUST быть
виден из сводки в её обычной форме, а не только в машинной: чтение authority
storage напрямую MUST NOT быть единственным способом узнать исход собственной
работы. Ожидание host MUST быть видно из read-only вида как безопасное
следующее действие. Команда обновления MUST называть адрес, по которому
проверяла release.

#### Scenario: Выход шага запечатан, выходов Run ещё нет

- **WHEN** шаг запечатал выход, а Run ещё не завершил ни одной стадии с
  выходом
- **THEN** сводка не показывает состояние как «ничего не запечатано»

#### Scenario: Задание держит host

- **WHEN** assisted attempt ожидает отчёта host
- **THEN** read-only вид называет чтение задания среди безопасных следующих
  действий, а не только состояние и события

#### Scenario: Обновлений нет

- **WHEN** установленная версия совпадает с последним stable release
- **THEN** ответ называет адрес, по которому проверялся release

#### Scenario: Исполнитель проверяет исход своей работы

- **WHEN** результат шага принят
- **THEN** verdict читается из обычной сводки Run, без machine-readable флага
  и без чтения authority storage

### Requirement: CLI предоставляет явную резолюцию uncertain obligation

Public CLI MUST предоставлять `prifly run resolve RUN_ID (--attempt ID |
--check ID) --outcome not_applied|applied --reason TEXT [--command-id ID]`.
Команда MUST принимать только uncertain attempt или check, MUST требовать
reason и явный outcome, MUST возвращать typed receipt через обычный command
protocol и MUST отказывать с `driver_live`, пока driver этого Run активен.
Её результат не является успехом workflow: `next` после резолюции показывает
освобождённый slot и terminal или следующее honest состояние scope. `run
cancel`, `run resume` и `run drive` MUST NOT выполнять резолюцию неявно.

#### Scenario: Владелец разрешает uncertain attempt
- **WHEN** пользователь вызывает `run resolve` с outcome и reason для
  uncertain attempt без живого driver
- **THEN** CLI возвращает receipt, `capacity show` больше не показывает slot
  этого attempt, а `run next` не предлагает retry

#### Scenario: Резолюция без outcome
- **WHEN** пользователь не указывает `--outcome` или `--reason`
- **THEN** CLI возвращает `invalid_usage` и не меняет authority

## Purpose

Определяет проверяемое управление Pri-Fly: полномочия, security boundaries,
evidence, operator interface и безопасную local эксплуатацию без ложных
гарантий о completion, isolation или recovery.

## Requirements

### Requirement: Core управляет маршрутом, а не текстом модели

Core MUST детерминированно выбирать routes, conditions, limits и transitions.
Ответ модели является typed input step и не заменяет command, approval или
verdict; unknown condition не выбирает удобную ветвь.

#### Scenario: Проверка вернула fail

- **WHEN** tool call технически успешен, но typed verdict равен `fail`
- **THEN** route не объявляет результат `pass`

### Requirement: Pinned contract пересекается с current restrictions

Run MUST сохранять immutable workflow, inputs, policy и package contract.
Current stop, revocation, quarantine, data restriction и authenticated user
restriction могут только сузить admission; их отсутствие нельзя предположить
из недоступного authority source.

#### Scenario: Пользователь запрещает тесты после start

- **WHEN** current authenticated restriction запрещает required test action
- **THEN** новая admission не выдаётся и Run ждёт допустимого решения

### Requirement: Переход принимает только проверяемый result

Core MUST проверять active Attempt, scoped inputs, current restrictions,
ArtifactRevision и required Evidence в одной durable transition. Command
idempotency не смешивает independent heartbeat с устареванием result; late
cancelled result остаётся observation и не переписывает accepted state.

#### Scenario: Worker присылает произвольный successor

- **WHEN** completion command содержит непроверенное имя следующего step
- **THEN** core игнорирует его и вычисляет successor из workflow contract

### Requirement: Роли не имитируют независимость решений

Permissions MUST быть scoped actions над resources, а roles — только удобными
наборами. Profile, требующий separation of duties, различает реальных людей и
service principals, author revision и actor; два account одного человека не
образуют независимый quorum.

#### Scenario: Автор пытается одобрить свою revision

- **WHEN** profile требует независимое approval
- **THEN** quorum не засчитывает автора под другим техническим account

### Requirement: Каждый защищённый effect имеет exact intent

Before dispatch Core MUST создать immutable ActionIntent с operation, owning
Attempt, typed arguments, inputs, resources, outputs, effect class и limits.
ActionIntent сам не запускает effect; display и adapter используют одну
canonical запись. Новый смысловой effect требует нового intent, а retry
delivery сохраняет operation identity.

#### Scenario: Bulk approval не имеет scope

- **WHEN** requester предлагает «все будущие действия» без finite manifest или
  bounded Grant
- **THEN** approval request отклоняется

### Requirement: Approval имеет проверяемый lifecycle

Approval MUST связывать authenticated approvers с exact intent digest, scope,
policy/quorum, expiry и reason. Consume создаёт Admission атомарно и лишь один
раз; reject, revoke и expiry terminal для записи, а changed subject требует
новый intent и approval.

#### Scenario: Consume конкурирует с revoke

- **WHEN** две команды одновременно consume и revoke один approval
- **THEN** journal сохраняет один объяснимый outcome без admission вне срока

### Requirement: Grant делегирует только конечные права

Grant MUST ограничивать subject, operations, resources, data, destinations,
time, count, concurrency и budget. Он не расширяет сам себя, не отменяет
required approval и создаёт отдельный Admission с расходом limits для каждого
intent.

#### Scenario: Grant исчерпан

- **WHEN** следующий intent превысит count или budget Grant
- **THEN** core не создаёт admission

### Requirement: Неотменяемые checks остаются неотменяемыми

Identity, authorization, input integrity, resource boundary, Attempt freshness
и evidence consistency MUST не становиться optional quality check. `skipped` и
`waived` не равны `pass`; waiver фиксирует exact scope, reason и approver и не
создаёт отсутствующий required artifact.

#### Scenario: Обязательный output пропущен

- **WHEN** downstream step требует output skipped producer
- **THEN** UI показывает blocked path, а final report не скрывает waiver

### Requirement: Retry не является тихим fallback

Technical retry MUST иметь qualified finite profile. Unknown effect допускает
лишь exact reconciliation resend с valid target dedup; whole-step retry
требует ledger всех прежних operations. New model, provider, package,
environment или workflow — новая compatible revision либо new work, не retry.

#### Scenario: Первый tool call мог примениться

- **WHEN** worker исчез после первого effect без whole-step checkpoint
- **THEN** runtime не перезапускает worker ради удобного результата

### Requirement: Stop имеет durable и исполнимую границу

Stop MUST atomically запретить future ordinary admissions в exact scope и
вернуть committed generation. Release снимает только named current stops и
проверяет rights/approval; already admitted work, unknown effects и recovery
actions остаются явно наблюдаемыми.

#### Scenario: Stop подтверждён до dispatch

- **WHEN** ordinary intent ещё не пересёк durable dispatch gate
- **THEN** adapter не отправляет его после stop

### Requirement: Resume не обходит uncertainty или stop

Resume MUST revalidate current restrictions, resources, outstanding intents,
approval/Grant срок и budget. Он не снимает stop, не переиспользует consumed
approval и не допускает ordinary work при conflicting unknown effect. Backup
recovery не обнуляет generations или used admissions. Резолюция uncertain
obligation — отдельная аудируемая control operation с собственным permission:
resume, cancel, release и drive MUST NOT выполнять её неявно, а резолюция MUST
NOT объявлять success или снимать stop.

#### Scenario: Новый stop появился между release и resume
- **WHEN** operator последовательно выпускает release и resume
- **THEN** resume отказывает из-за нового применимого stop

#### Scenario: Resume после uncertain attempt
- **WHEN** Run содержит uncertain attempt без резолюции
- **THEN** resume отказывает с `recovery_required`, а safe next action
  называет резолюцию, не retry

### Requirement: Limits различают measured, reserved и estimated

Each execution limit MUST name scope, unit, measurement source, consumption
event and boundary behavior. Hard monetary cap требует enforceable maximum;
unknown usage сохраняет reservation, а estimate не изображается как guarantee.

#### Scenario: Provider не вернул usage

- **WHEN** external request заканчивается неизвестно без cost receipt
- **THEN** budget остаётся reserved и UI не показывает ноль

### Requirement: Оператор видит реального владельца выполнения

Operator surface MUST различать core, host, managed runner, human и external
system. Assisted mode не обещает background wakeup; managed mode обещает лишь
qualified continuation/recovery. Смена mode сверяет active attempts и owners и
не запускает operation второй раз.

#### Scenario: Host закрыт

- **WHEN** assisted host недоступен
- **THEN** интерфейс показывает ожидание владельца, а не бесконечный progress

### Requirement: Threat profile называет enforcement и остаточный риск

Deployment MUST объявлять protected resources, trusted components, threat
model и physical enforcement point для prompt injection, forged result, stale
approval, malicious package, credential loss, resource substitution и late
worker. Cooperative local profile не обещает защиту от malicious OS owner.

#### Scenario: Profile требует untrusted worker isolation

- **WHEN** host не квалифицировал нужное isolation
- **THEN** workflow не запускается под более строгим profile

### Requirement: Access проверяется для каждого объекта и канала

Authorization MUST вычисляться по actor, action, project, Run/Step, resource,
data class и current restrictions for reads, lists, exports, events и receipt
replay as well as writes. Unknown mapping denies access and response не
раскрывает чужой object existence.

#### Scenario: Actor запрашивает чужой artifact

- **WHEN** caller не имеет permission на object scope
- **THEN** system не отдаёт bytes, metadata или existence hint

### Requirement: Identity не требует облачного аккаунта

Local authority MAY использовать owner-bound local identity без внешней
registration и не преувеличивает person proof shared OS account. Shared/remote
profile MUST использовать authenticated protected transport, distinct service
identities, issuer/audience/expiry checks and revocation; sensitive actions
follow the declared fresh-authentication and MFA profile.

#### Scenario: Неизвестный remote issuer

- **WHEN** remote credential имеет unknown issuer или invalid audience
- **THEN** request отклоняется до control action

### Requirement: Host permission сильнее package и prompt

Package capability declaration, task text, model message или internal approval
MUST не расширять host permission. Authenticated user steering is separate from
untrusted document/tool text; denied capability не обходится другим account,
shell, proxy или runtime.

#### Scenario: Issue просит отправить данные

- **WHEN** untrusted issue содержит instruction публикации
- **THEN** он не создаёт recipient permission или approval

### Requirement: Tool calls и network destinations typed

Adapter MUST validate pinned schema and operation contract before execution and
pass structured arguments. Network operation validates final destination,
redirect, DNS and provider account at connection point; private/loopback/
metadata endpoints require an explicit resource profile.

#### Scenario: Allowlisted URL перенаправляет на private address

- **WHEN** redirect меняет final destination за approved scope
- **THEN** adapter blocks request

### Requirement: Package trust ограничивает executable content

Manifest, schemas and dependencies MUST validate before package code loading;
list, doctor and preview do not run install hooks. Pinned revision covers
executable content and dependencies. Revocation blocks new admissions and
trusted acceptance/reuse for affected revisions while preserving history and
physical observations.

#### Scenario: Checker revision отозвана

- **WHEN** cached result зависит от quarantined checker
- **THEN** new trusted reuse is blocked without deleting old evidence

### Requirement: Sandbox обещает только qualified isolation

Profile MUST independently qualify filesystem roots, authority/secrets access,
network, subprocess tree, quotas and cancellation. Directory, worktree,
container name or PID alone не равны sandbox. Unknown owner/generation blocks
cleanup; host refusal не обходится `sudo` или broader mount.

#### Scenario: Container получает host socket

- **WHEN** declared container mount даёт unbounded host authority
- **THEN** profile не маркируется sandboxed

### Requirement: ContextManifest отделяет instructions от data

ContextManifest MUST pin source, digest, version, data class, trust label and
purpose for each fragment. Trusted instructions, current user commands and
external data remain separate; hidden parent history and silent truncation
violate context contract. Fresh context requires proven semantic freshness, not
a new worker ID.

#### Scenario: Обязательный context не помещается

- **WHEN** profile не имеет declared selection strategy
- **THEN** execution waits or refuses instead of silently truncating text

### Requirement: Adapter выдаёт scoped credentials

Secret values MUST not appear in workflow, prompt, ActionIntent, CLI history,
artifact metadata or ordinary logs. Adapter receives minimum credential for an
operation; rotation preserves an intent only when same account/resource and
material rights are proven, otherwise a new qualification and approval are
required.

#### Scenario: Credential rotated to другой account

- **WHEN** rotation changes target authority
- **THEN** old unknown request is not retried under new credential

### Requirement: Data policy контролирует каждый output channel

One data policy MUST apply to context, logs, human notes, exports, previews and
external publication. Writer validates actual bytes, viewer safely handles
untrusted markup and remote content, and retention/erasure follow project
policy without recreating deleted bytes from hidden cache.

#### Scenario: Error содержит secret

- **WHEN** writer detects sensitive value in output
- **THEN** it redacts value and does not echo it in the diagnostic reason

### Requirement: Protected evidence и callback имеют authenticated provenance

Evidence MUST distinguish data author, execution observer and validator; hash
does not by itself prove correctness. Protected receipt issuer is not writable
by worker. Callback validates issuer, audience, intent/attempt correlation,
generation and replay identity; same identity with changed payload is rejected.

#### Scenario: Callback повторяется с новым payload

- **WHEN** provider reuses message identity for different bytes
- **THEN** system rejects conflict without second result

### Requirement: Evidence называет предмет и предел доказательства

Evidence MUST include subject, exact refs, observer/validator, method version,
time, verdict and available bytes. Step contract declares sufficient evidence;
receipt, artifact observation, schema validation, mechanical check, semantic
review and human decision remain distinct kinds. Worker self-attestation does
not become runner observation.

#### Scenario: Файл называется «отчёт»

- **WHEN** step requires checked sources rather than mere artifact existence
- **THEN** filename alone does not satisfy evidence contract

### Requirement: Verification использует первичные записи

Finding count, required outputs and command execution MUST derive from allowed
primary records, not interested worker summary. Artifact acceptance validates
existence, size, schema, digest, producer binding and access policy; evidence
reuse requires exact relevant dependencies and current checker trust.

#### Scenario: Worker сообщает пустой список findings

- **WHEN** observer is absent or incomplete
- **THEN** system records missing evidence, not «no findings»

### Requirement: Authority имеет единую историю control facts

Journal, projections, receipts, telemetry correlation and report MUST preserve
one authority history. Manual side files, stale UI cache, deleted detail or
clock display cannot become parallel source of truth; retention and replay
state their limits explicitly.

#### Scenario: Export потерян

- **WHEN** derived report file is missing
- **THEN** authority history remains intact and report can state unavailable
  evidence without inventing it

### Requirement: Explain и replay остаются read-only

Explain, status, preview and replay MUST reveal route, conditions, reuse,
unavailable inputs and reproducibility limits without dispatching effect,
lifting stop or running collector. A replayed explanation is not a recovery
command.

#### Scenario: Пользователь открывает explain uncertain Run

- **WHEN** Run имеет unresolved effect
- **THEN** tool displays boundary and safe next actions without retrying effect

### Requirement: Incident control не создаёт force success

Incident workflow MUST retain stop, quarantine, receipts, outstanding effects
and recovery conditions. Emergency action stays scoped and audited; insufficient
evidence preserves uncertainty rather than force-unlock, rewrite history or
declare success.

#### Scenario: Authority получает integrity incident

- **WHEN** required blob или evidence повреждён
- **THEN** dependent action is blocked and incident remains visible

### Requirement: Final outcome отражает достигнутую работу

Terminal report MUST distinguish succeeded, rejected, no_work, waived and
permitted partial outcomes together with residual effects and obligations.
Neither skipped work nor unknown effect becomes green completion.

#### Scenario: Workflow завершён с waiver

- **WHEN** allowed quality check is waived
- **THEN** final result and export preserve waiver and its reason

### Requirement: CLI, API и panel используют одни понятия

All control surfaces MUST use one vocabulary and state model for Run, Step,
Attempt, intent, admission, evidence, stop and outcome. Optional GUI adds no
separate authority; machine JSON and CLI preserve the same distinctions.

#### Scenario: Panel показывает unknown effect

- **WHEN** API reports `effect_status=unknown`
- **THEN** panel and CLI do not relabel it as completed

### Requirement: Preview проверяет понимание до expensive effect

Before bulk dispatch and first protected effect, preview MUST show task,
sources, deliverables, workflow, AI role, reads, changes, recipients and cost
boundary from actual inputs/intents. New ambiguity waits for clarification;
corrected goal invalidates previous intents.

#### Scenario: Предмет Pri-Fly истолкован неверно

- **WHEN** preview shows unrelated interpretation
- **THEN** correction stops old scope before bulk delegation

### Requirement: Progress показывает известное, ожидаемое и unknown

Active view MUST identify Attempt, runner, operations/deliveries, last confirmed
event, elapsed time, limit, outputs and wait reason. Queue is not running;
lost heartbeat is not death; graph shows selected branches, loop counters and
safe denominator rather than optimistic percentage. Reconnect rereads authority
state and does not resubmit dangerous command.

#### Scenario: Connection стала stale

- **WHEN** UI loses connection to authority
- **THEN** it marks last snapshot stale and waits for reread before mutation

### Requirement: Human decisions display exact consequences

Approval and waiver view MUST show material intent diff, resources,
destinations, reversibility, count, expiry, budget mode, residual risk and
blocked consumers. Approve/reject are distinct; a select-all action cannot
include hidden pages or future intents without finite manifest.

#### Scenario: Intent изменён после открытия approval

- **WHEN** subject bytes or scope changes
- **THEN** prior decision becomes stale and cannot approve new variant

### Requirement: Error offers only safe next action

Error MUST provide stable machine code, human explanation, correlation ID and
safe continuation for permission, changed intent, missing capability, invalid
evidence, exhausted limit, unknown effect, uncommitted stop or lost worker. It
does not leak secret, private path or foreign payload and distinguishes safe
read retry from dangerous mutation retry.

#### Scenario: Stop receipt не записан

- **WHEN** client loses response before durable stop confirmation
- **THEN** UI asks to inspect status/receipt and does not claim all work stopped

### Requirement: Управление доступно без цветовой или pointer-only зависимости

Critical controls MUST be keyboard-accessible with visible focus, text/icon
meaning, accessible labels and associated errors. GUI claims applicable WCAG
conformance only after qualification; CLI supports no-color, structured output
and stable exit semantics without unescaped worker log pollution.

#### Scenario: Терминал отключает цвет

- **WHEN** user requests no-color output
- **THEN** unknown effect and waiver/pass distinction remain explicit in text

### Requirement: End-to-end profiles не навязывают coding workflow

Research, document, data-operation, coding and no-AI/no-task-source scenarios
MUST each state their required checks, effects and exceptions. Git, issue,
LLM, renderer, test or publication capability is optional unless selected
profile declares it; missing required verification waits or reports limitation.

#### Scenario: Локальный no-AI workflow

- **WHEN** user runs checksum and schema workflow in empty installation
- **THEN** it works without Git, issue source, LLM or development package

### Requirement: Control mechanisms require qualification evidence

Control acceptance MUST exercise forged completion, artifact substitution,
approval race, stop boundary, late result, unknown effect, injection, goal
correction, sensitive output, fresh context, hard budget, keyboard/stale UI and
duplicate action cases. Textual requirements and synthetic happy path do not
claim untested control qualification.

#### Scenario: Adapter не доказывает hard budget

- **WHEN** strict profile requires enforceable cost maximum
- **THEN** adapter cannot mark capability qualified from estimate alone

### Requirement: Local installation is explicit and non-destructive

Operation guide MUST state qualified platform, required build inputs and
checksum verification without installer, background service or automatic update.
Project authority is initialized outside source tree; repeated or interrupted
init does not overwrite discovered configuration.

#### Scenario: Инициализация прервана

- **WHEN** project directory содержит partial initialization
- **THEN** next init preserves it and asks for a new empty directory for retry

### Requirement: Project profile separates versioned packages from local authority

Project profile MUST keep versioned workflow files under repository `.prifly`
while runtime authority remains outside repository. `local.yaml` records exact
binary/state paths without global PATH guessing. Launcher lists declared
launches and inputs before mutation and does not infer workflow from task URL
or chat text.

#### Scenario: Launch не выбран

- **WHEN** task URL is supplied without an explicit launch ID
- **THEN** launcher displays selectable declared launches without starting Run

### Requirement: Package compilation and executor protocol are explicit

Workflow author MUST increment package version and compile YAML to a new output
outside repository/authority; compile validates graph, ports and exact refs but
does not import package or start Run. Executor receives sealed envelope/context
and produces typed result; stdout/stderr and early result do not independently
complete step.

#### Scenario: YAML compilation succeeds

- **WHEN** author runs project compile
- **THEN** package remains unimported and no Run is created

### Requirement: Local failure handling preserves uncertainty

Operational guide MUST forbid editing live SQLite, deleting live locks or
blind PID kill/retry to resolve lost driver, missing blob or IO uncertainty.
Archive copies are read/recovery-only after active obligations settle; removing
`.prifly` destroys history but does not cancel effects or delete unrelated data.

#### Scenario: Foreground driver disappears after dispatch

- **WHEN** process ownership is lost with possible external effect
- **THEN** slot remains uncertain and operator is directed to inspect facts

### Requirement: Operational limits and telemetry remain bounded and truthful

F1 operating guide MUST publish finite limits for driver, slots, steps,
transitions, payloads, artifacts, query and storage budgeting. Telemetry/report
labels quality, coverage and clock basis; query is read-only, bounded and does
not turn unknown into zero or trigger collection.

#### Scenario: Query exceeds its response limit

- **WHEN** telemetry request exceeds declared bound
- **THEN** it returns an explicit error rather than a falsely complete summary

### Requirement: Assisted workspace-write intent сохраняет exact Workspace

New assisted workspace-write Run MUST bind every handoff and protected
workspace-write intent to its exact claimed Workspace identity and generation.
It MUST permit writes only inside that claim, including direct checkout mode;
the mode MUST NOT grant branch switching, reset, clean, deletion or access to
authority data. Existing worktree-only intent and Grant values remain valid
only for their pinned historical Runs and MUST NOT be silently reinterpreted.

#### Scenario: Checkout handoff proposes write
- **WHEN** an assisted host holds a direct checkout Workspace
- **THEN** its handoff and proposed intent name that exact claim and cannot
  address a different repository or authority path

### Requirement: Интерфейс показывает решения до и во время Run
CLI и supported host UI MUST использовать один typed decision model. До first
dispatch они MUST показать requested package profile, known mandatory
decisions, defaults/recommendations, autonomous policy и последствия выбора.
При dynamic request интерфейс MUST показать question, allowed response and
why Run waits; после completion — source каждого выбора. UI MUST NOT label
automatic recommendation как подтверждение человека.

#### Scenario: Владелец выбирает autonomous Run
- **WHEN** preflight показывает entries, которые могут быть answered
  automatically
- **THEN** интерфейс отдельно показывает entries, которые всё равно остановят
  Run при новом или restricted вопросе

#### Scenario: Динамический вопрос требует внешнего действия
- **WHEN** requested value может изменить scope или потребовать Approval
- **THEN** интерфейс не предлагает generic agent recommendation как обход
  человеческого решения

### Requirement: Установка сценария из репозитория не исполняет контент и не расширяет trust
`project workflows add`, `update` и `search` MUST вызывать `git` типизированным
argv без shell, с ограниченным окружением, отключённым terminal prompt и
разрешёнными только протоколами `https`, `ssh` и `file`; они MUST NOT
инициализировать submodules, исполнять hooks, filters или любой файл из
полученного repository. В `.prifly/` копируются только regular files по
`lstat`. URL с userinfo MUST быть отказом до сети, а diagnostics MUST NOT
содержать credentials: аутентификация приходит только из credential helper
или SSH пользователя. Полученные bytes остаются данными: команды MUST NOT
seal-ить, импортировать, доверять или компилировать package против host;
единственное trust decision по-прежнему принимается при `project start`.
Каталог не является trust root, а его необязательный `commit` только
проверяет identity. Сетевые операции MUST иметь ограниченный timeout и
выполняться только внутри этих явных команд.

#### Scenario: URL содержит token
- **WHEN** пользователь передаёт repository вида `https://user:token@host/…`
- **THEN** команда отказывает до сети и не записывает credentials в
  `project.yaml` или diagnostic

#### Scenario: Repository содержит hooks и submodules
- **WHEN** установленный repository объявляет Git hooks, filters или
  submodules
- **THEN** ничего из них не исполняется и не инициализируется, а копируются
  только regular files выбранной папки

#### Scenario: Запрещённый протокол
- **WHEN** SOURCE использует `ext::` или иной неразрешённый Git transport
- **THEN** команда отказывает без выполнения внешней команды

### Requirement: Read-only monitor проверяет origin и не раскрывает внутренности

Loopback monitor MUST принимать запросы только с `Host`, равным его
собственному loopback адресу, MUST отвечать `X-Content-Type-Options: nosniff`,
MUST выдавать ошибки через тот же safe Problem contract, что и CLI, без
внутренних путей и сообщений, и MUST ограничивать отдаваемое содержимое
артефакта объявленным пределом без чтения всего blob в память. Monitor
остаётся окном без команд.

#### Scenario: Запрос с чужим Host
- **WHEN** страница с внешнего домена, разрешённого в loopback, обращается к
  `/api/*`
- **THEN** monitor отказывает и не отдаёт записанные данные

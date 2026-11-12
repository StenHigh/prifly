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

Отдельное решение владельца SHALL разрешать только проверенные schema refs
для точного нового consumer и выбранных доверенных поставщиков. Оно MUST
сохранять terminal revoke и запрет для code, старых pins и непокрытых отзывов.
Reason сам по себе MUST NOT считаться доказательством безопасности или отменой отзыва.

#### Scenario: Checker revision отозвана

- **WHEN** cached result зависит от quarantined checker
- **THEN** new trusted reuse is blocked without deleting old evidence

#### Scenario: Повторный import выглядит как обычное обновление
- **WHEN** пакет или alias снова предоставляет ранее отозванный schema Ref без отдельного решения владельца
- **THEN** security revocation продолжает блокировать admission

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
external publication. Предстартовый итог запуска MUST NOT печатать значения
environment объявленных программ: имена переменных достаточно, чтобы владелец
проверил состав, а значение — то самое место, куда проект кладёт секрет.
Документация и вывод MUST говорить об этом одно и то же. Writer validates actual bytes, viewer safely handles
untrusted markup and remote content, and retention/erasure follow project
policy without recreating deleted bytes from hidden cache.

#### Scenario: Error содержит secret

- **WHEN** writer detects sensitive value in output
- **THEN** it redacts value and does not echo it in the diagnostic reason

#### Scenario: Итог запуска показывает окружение программы
- **WHEN** `project questionnaire --prepare` описывает объявленную программу,
  чьё окружение задано в machine-local конфигурации
- **THEN** итог называет переменные по именам и не печатает их значения, а
  документация не обещает большего, чем вывод делает

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

Project profile MUST хранить версионируемые workflow files под repository
`.prifly`, а runtime authority — вне repository. `local.yaml` SHALL называть
exact binary/state paths без угадывания global PATH. Launcher MUST показывать
объявленные launch и inputs до mutation и MUST NOT автоматически выбирать
исполняемый workflow по task URL или chat text.

Текущая host-сессия SHALL поддерживать условный совет по исходной задаче,
`title` / `description` объявленных launch и применимым критериям `PROJECT.md`
по контракту общего runner. Совет MUST оставаться необязательной помощью при
выборе; его пропуск или проектное отключение MUST NOT мешать ручному выбору.
Явно названный точный ID SHALL считаться уже сделанным выбором. До выбора
host MUST NOT создавать package, claim или Run; совет, предвыбранный вариант
в интерфейсе, молчание и истечение времени MUST NOT считаться ответом.

Конечный выбор SHALL использовать фактически предоставленный и допустимый
для этого вопроса native question tool host, независимо от того, был ли совет,
сохраняя все реальные варианты и страницы при лимите tool. Host MUST учитывать
ограничения режима и при asynchronous tool ждать отдельный фактический ответ,
а не receipt показа. При недоступном или неприменимом tool host MUST ждать
явного текстового ответа без переключения режима ради обхода ограничения.
Подтверждение подготовленного запуска SHALL оставаться отдельным действием.

#### Scenario: Launch не выбран

- **WHEN** пользователь передал task URL без explicit launch ID
- **THEN** launcher показывает selectable declared launches, даёт совет только при применимости и наличии оснований и ждёт явный выбор без запуска Run

#### Scenario: Предвыбранный совет не является ответом

- **WHEN** UI выделил рекомендованный launch, но пользователь не отправил выбор
- **THEN** host остаётся в ожидании без mutation независимо от молчания или прошедшего времени

#### Scenario: Совет пропущен

- **WHEN** пользователь либо `PROJECT.md` отключил совет
- **THEN** все объявленные launch остаются доступными для явного выбора с обычной последующей процедурой запуска

#### Scenario: Асинхронный вопрос ещё не получил ответ

- **WHEN** host показал native выбор, но получил только receipt показа
- **THEN** пользователь видит доступные варианты, а runner не переходит к зависимым действиям до отправленного выбора

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
артефакта объявленным пределом без чтения всего blob в память. Операции чтения monitor не изменяют Run. Отдельный endpoint локального
обслуживания SHALL принимать только подтверждённый same-origin POST с
непредсказуемым token, ограниченным JSON body и повторной проверкой owner,
прав и exact source identity. Остальные методы/пути не получают право записи.

#### Scenario: Запрос с чужим Host
- **WHEN** страница с внешнего домена, разрешённого в loopback, обращается к
  `/api/*`
- **THEN** monitor отказывает и не отдаёт записанные данные

### Requirement: Решение миграции требует точного owner consent
Применение миграции SHALL требовать действующего права менять package trust,
точного digest плана, явной причины и command identity. Право владельца MUST
проверяться при commit. Документ плана MUST NOT сам предоставлять полномочия.
Apply MUST сохранять actor, причину, одобренный scope и исходные lifecycle facts.
Отсутствующий источник отзыва, неизвестное происхождение bytes или неполный
closure MUST сохранять запрет. Миграция MUST NOT переписывать старые trust decisions.

#### Scenario: Сессия потеряла право менять trust
- **WHEN** право действовало при prepare, но отсутствует при apply
- **THEN** authority возвращает именованный отказ без записи решения миграции

#### Scenario: В плане изменили один Ref
- **WHEN** apply получает изменённый план или digest другого плана
- **THEN** authority отказывает и не допускает часть schemas

### Requirement: Apply миграции проверяет свежесть атомарно
Apply SHALL повторно проверять bytes и closure до write-транзакции.
Authority SHALL атомарно проверять package state, control policy, decision state
и влияющие Runs относительно prepare. Изменение проверенных inputs MUST давать
именованный stale либо integrity отказ без частичного применения.
Read-only prepare MUST NOT создавать approvals, менять trust или запускать работу.
Повтор той же принятой command identity и payload MUST возвращать прежний receipt.
Повтор command identity с другим payload MUST давать command conflict.

#### Scenario: Run изменился без изменения общего количества Runs
- **WHEN** влияющий Run изменил состояние после prepare
- **THEN** apply отказывает атомарно; сравнения только количества Runs недостаточно

#### Scenario: Bytes поставщика изменены после prepare
- **WHEN** apply обнаруживает отличие manifest или schema bytes от одобренных digest
- **THEN** authority возвращает именованный integrity отказ и не записывает разрешение

#### Scenario: Ответ apply потерян
- **WHEN** клиент повторяет принятую команду с теми же identity и payload
- **THEN** authority возвращает сохранённый результат без второго решения и без повторной миграции

### Requirement: Новое lifecycle решение сильнее старой миграции
Решение миграции SHALL покрывать конкретные состояния отзыва и доверия, а не
пожизненно разрешать package Ref. Новый revoke покрытого источника, выбранного
поставщика или необходимой зависимости MUST блокировать последующее применение
исключения. Quarantine, remove и утрата разрешаемости выбранного поставщика MUST
также запрещать новые admissions через него. Уже наблюдённые effects MUST сохраняться.
Не связанный с миграцией security revoke MUST сохранять действие.

#### Scenario: После apply источник отозван повторно
- **WHEN** новая принятая lifecycle команда снова отзывает тот же exact package Ref
- **THEN** прежнее решение не покрывает новый отзыв и следующий admission блокируется

#### Scenario: Поставщик отозван после первого шага
- **WHEN** новый Run ожидает следующий admission, а выбранный поставщик либо его dependency теряет доверие
- **THEN** admission блокируется; движок не подбирает другого поставщика и не переписывает effects первого шага

### Requirement: Владелец управляет сохранённым выбором атомарно
Установка, замена и удаление выбора SHALL требовать текущего owner права управления package trust.
Команда MUST сохранять actor, reason и command identity.
Замена и удаление MUST проверять ожидаемую generation выбора, включая отсутствие записи.
Установка MUST проверять текущее решение, точную сборку и authority до commit.
Read-only inspect и prepare MUST NOT создавать запись.
Выбор MUST NOT создавать SchemaMigrationDecision либо менять PackageStatus.
Transform MUST использовать только snapshot и подготовленный payload без чтения bytes или запуска процессов.

#### Scenario: Сессия пытается сохранить выбор
- **WHEN** session principal без owner права отправляет команду установки
- **THEN** authority возвращает object_access_denied без изменения выбора, trust или Run

#### Scenario: Два владельца заменяют одну запись
- **WHEN** две команды используют одну ожидаемую generation
- **THEN** только первая принятая команда меняет запись; вторая получает stale refusal

#### Scenario: Потерян ответ команды
- **WHEN** клиент повторяет принятую command identity с тем же payload
- **THEN** authority возвращает прежний receipt; другой payload той же команды получает command conflict

#### Scenario: Источник отозван повторно
- **WHEN** новый revoke инвалидирует решение, на которое указывает сохранённый выбор
- **THEN** prepare и start отказывают с причиной; запись остаётся доступной для inspect, замены и удаления

### Requirement: Start сохраняет проверенный выбор при конкурирующих командах
Start через сохранённый выбор SHALL проверять его generation и текущие migration preconditions при принятии нового Run.
Изменение выбора после prepare MUST инвалидировать проверенный запуск.
Удаление и повторная установка того же ID MUST NOT восстанавливать действительность прежнего review.
Изменение выбора MUST NOT менять binding уже принятого Run.

#### Scenario: Выбор удалён после prepare
- **WHEN** owner удаляет запись до start с прежним launch digest
- **THEN** start отказывает до создания Run и постоянного claim

#### Scenario: Запись изменилась после последнего чтения start
- **WHEN** другая команда успевает заменить выбор перед принятием Run
- **THEN** authority отказывает атомарно; start не запускает работу с прежней записью

#### Scenario: Запись удалена после успешного start
- **WHEN** owner удаляет выбор после принятия Run
- **THEN** Run сохраняет прежний binding и обычные проверки trust на следующих admissions

### Requirement: Мониторы объясняют препятствие из доступных фактов

Full и Light SHALL показывать в основном блоке карточки и подробностей состояние исполнения, известную причину и безопасный следующий шаг. Эти сведения SHALL происходить из доступного согласованного чтения выбранного объекта. Отсутствующая или неоднозначная причина SHALL обозначаться явно; устаревший отказ другой попытки MUST NOT становиться причиной текущей остановки.

Пояснение SHALL сохранять различия Run status/outcome, состояния QueueTask, фазы задачи и приёмки review. Оно MUST NOT менять категории доски, выбирать единственный текущий Run по времени или скрывать препятствия других активных ветвей. Известный исполняющий субъект SHALL называться только по соответствующей записи; отсутствие identity SHALL оставаться неизвестным.

Следующий шаг SHALL быть пояснением либо переходом к уже доступным данным. Он MUST NOT создавать управляющую команду, обещать автоматическое продолжение, повторять эффект или объявлять допуск полученным. Light MUST NOT предоставлять shell-команды. Переход SHALL сохранять точный source и объект; при неоднозначной принадлежности причины конкретному Run SHALL открываться содержащая её задача, без выбора первого Run.

Общие правила принадлежат `control-security-ux`. Требования Full из `local-run-monitor` и текущий source set Light из `add-prifly-light-task-board` сохраняются. Новые представления MUST NOT изменять публичные versioned identifiers, JSON-формы, закреплённые Run, storage либо replay; нормативное владение не переносится.

#### Scenario: Записана потеря связи с host
- **WHEN** выбранная попытка имеет записанное состояние потери связи или выбранный Run содержит неопределённое обязательство
- **THEN** основной блок называет известную причину либо её предел и необходимость проверить исход; ожидание обычного отчёта не подменяет неопределённость, безопасный повтор не обещается

#### Scenario: Активность драйвера не подтверждена
- **WHEN** чтение не подтверждает активность драйвера, но не содержит записанного отказа или потери связи с host
- **THEN** интерфейс сообщает предел наблюдения и не объявляет host завершившимся или Run остановленным

#### Scenario: Параллельные ветви имеют разные причины
- **WHEN** одна ветвь работает, другая ждёт решения, сигнала, ресурса либо имеет текущее обязательство с потерянной связью
- **THEN** видны работающая ветвь и все доступные причины ожидания с их областью; локальное препятствие не переименовывает весь Run в остановленный

#### Scenario: Ресурс занят
- **WHEN** доступные данные содержат записанную причину ожидания ресурса
- **THEN** карточка выделяет ресурс и ожидание; переход к держателю появляется только при доказанной связи, а освобождение ресурса не выдаётся за обещание автоматического продолжения

#### Scenario: Известный отказ имеет объяснение
- **WHEN** отказ текущего объекта однозначно связан с известным кодом, включая истёкший срок попытки
- **THEN** интерфейс показывает понятное объяснение, доступный переход к диагностике и исходный код; неизвестные подробности и ответственность не выдумываются

#### Scenario: Отказ старой попытки уже обработан
- **WHEN** история содержит уже обработанную ошибку прошлой попытки, а текущая работа продолжается либо завершена с известным исходом
- **THEN** историческая ошибка остаётся в своём scope и не подменяет текущие status/outcome или причину ожидания

#### Scenario: Legacy данные не содержат причины или владельца
- **WHEN** чтение не содержит однозначной причины, нужного поля либо identity исполнителя
- **THEN** интерфейс показывает неизвестное и доступные подробности; название стадии, текст лога и автор Stop не используются для догадки об ответственном

#### Scenario: Частичный результат и отдельное продолжение
- **WHEN** completed Run имеет outcome partial и записанное отдельное продолжение
- **THEN** первый Run остаётся завершённым с частичным результатом; следующее действие открывает его результат или точную связь, а продолжение сохраняет собственное состояние

#### Scenario: Приёмка отличается от исполнения
- **WHEN** usable review report принят с verdict pass, но связанная gate заблокирована или завершилась failed
- **THEN** интерфейс сохраняет отдельные результат проверки и результат приёмки, не создавая общего успешного статуса

### Requirement: Мониторы различают ошибку чтения и состояние исполнения

Full и Light SHALL различать первое чтение, загрузку, пустой успешный результат, ошибку чтения, обновление и ранее полученные устаревшие данные. Обещание сохранённых данных SHALL относиться только к ранее успешному снимку именно текущего запроса и объекта. Успех другого запроса MUST NOT подтверждать наличие такого снимка.

Ошибка транспорта SHALL менять достоверность отображаемого чтения, а не записанное состояние Run, host или review. Успешный HTTP-ответ MUST NOT скрывать stale/unavailable отдельных sources. Доступный ранее снимок SHALL сохраняться с явной отметкой устаревания; подтверждённые сведения других sources SHALL оставаться доступными.

Опоздавший ответ другого запроса MUST NOT заменять текущий экран. Повтор чтения SHALL сохранять выбранные фильтры, объект и доступную позицию, без управляющих эффектов. После восстановления достоверного чтения ошибочная подсказка SHALL исчезать. Изменившийся отображаемый признак наблюдения SHALL обновляться даже при прежней версии Run.

#### Scenario: Первое чтение недоступно
- **WHEN** новый экран ещё не получил ни одного успешного снимка текущего запроса и чтение завершилось ошибкой
- **THEN** экран сообщает об ошибке первого чтения и доступном повторении; он не обещает сохранённые данные и не оставляет загрузку единственным объяснением

#### Scenario: Доска прочитана, новые подробности недоступны
- **WHEN** доска уже была прочитана, пользователь открывает другой объект и его первое чтение завершается ошибкой
- **THEN** успешное чтение доски не выдаётся за сохранённые подробности нового объекта

#### Scenario: Ошибка обновления прежнего объекта
- **WHEN** текущий объект уже успешно показан, а следующее чтение того же объекта завершилось ошибкой
- **THEN** прежний снимок остаётся явно устаревшим; запись о потере связи с host не синтезируется из сетевой ошибки браузера

#### Scenario: Смена фильтра или страницы во время чтения
- **WHEN** пользователь изменил фильтр, вид, порядок, страницу, cursor или объект, а прежний ответ пришёл позже либо новый запрос завершился ошибкой
- **THEN** прежний снимок не выдаётся за результат нового запроса; выбранные фильтры и доступная навигация сохраняются

#### Scenario: Неизменный ответ не создаёт отсутствующий снимок
- **WHEN** ответ сообщает об отсутствии изменений
- **THEN** интерфейс использует только соответствующий ранее полученный снимок; при его отсутствии он повторяет безопасное чтение без выдуманных данных

#### Scenario: Один источник недоступен
- **WHEN** HTTP-чтение успешно, но ответ помечает один source устаревшим или недоступным
- **THEN** свежесть этого source остаётся видимой, а здоровые sources не скрываются и не получают ложную ошибку

#### Scenario: Меняется наблюдение без новой версии Run
- **WHEN** обновление меняет показываемую активность драйвера или свежесть, сохраняя версию Run
- **THEN** текст обновляется без сброса focus и положения; отсутствие нового события не сохраняет прежнюю ложную подсказку

### Requirement: Первый экран монитора сохраняет основное состояние на узком экране

Full и Light SHALL отдавать первый визуальный приоритет названию, состоянию исполнения, краткой причине и доступному следующему переходу. Фаза задачи, приёмка и технические metadata SHALL оставаться отдельными понятиями; они MUST NOT маскировать причину ожидания общим обозначением работы.

При первоначальном открытии на экранах 390×844 и 1440×900 с масштабом 100% первый непустой результат репрезентативного набора SHALL показывать название, состояние и основное объяснение без прокрутки страницы. Подробности SHALL показывать этот блок до развёрнутых технических панелей. Возврат SHALL сохранять прежнее место чтения. Неограниченный текст SHALL иметь доступное раскрытие; сокращение MUST NOT скрывать неопределённость или наличие других препятствий.

На узком экране название и состояние одного результата SHALL читаться вместе без горизонтальной прокрутки. Схемы и дополнительные таблицы SHALL сохранять самостоятельную доступную прокрутку без переполнения всей страницы. Поиск SHALL оставаться доступным; дополнительные фильтры и сведения SHALL раскрываться с клавиатуры и сохранять выбранные значения.

Пустые группы Light при поиске MUST NOT вытеснять найденный результат. Скрытие пустого пространства SHALL сохранять доступные группы фильтра, область и полноту counters, точную историю и самостоятельные исходы Runs. Polling и переходы SHALL сохранять существующие focus, фильтры, страницу и место чтения. Тема, labels, экранирование текста, безопасные source/Run links и режимы доступа сохраняются.

#### Scenario: Первый результат на телефоне
- **WHEN** открыта доска либо список на экране 390×844 с обычным коротким названием и одним найденным результатом
- **THEN** название, состояние, краткая причина или явное неизвестное и доступный основной переход видны на первом экране; состояния не нужно искать правее названия

#### Scenario: Подробности на телефоне
- **WHEN** открыты подробности остановленной или ожидающей работы на экране 390×844
- **THEN** основное объяснение предшествует метрикам, идентификаторам и схеме; причину не вытесняют несколько развёрнутых технических карточек

#### Scenario: Длинное название и несколько препятствий
- **WHEN** карточка содержит длинный текст и несколько причин разных ветвей
- **THEN** страница не переполняется по ширине, доступно раскрытие полного текста и остальных причин, а неизвестность и число скрытых причин остаются заметны

#### Scenario: Найден один результат Light
- **WHEN** поиск оставляет один результат и другие группы не содержат карточек на этой странице
- **THEN** пустые группы не стоят перед найденным результатом как большие пустые блоки; доступные фильтры и честные counters остаются

#### Scenario: Клавиатура и возврат
- **WHEN** пользователь раскрывает дополнительные фильтры, открывает результат клавиатурой и возвращается
- **THEN** labels и focus видимы, выбранные значения и страница сохранены, а скрытые фильтры продолжают действовать

#### Scenario: Переход Light в Full
- **WHEN** пользователь открывает существующую Run-ссылку из Light за proxy prefix
- **THEN** Full получает тот же source/Run и разрешённую вкладку, а исходный контекст Light сохраняется

### Requirement: Light показывает выбранные Run в компактном наблюдении

Доска задач Light SHALL оставаться входным экраном. Оператор SHALL иметь возможность явно выбрать один или два конкретных Run из доступных записей. Задача без Run SHALL оставаться на доске без закрепления Run. Идентичность закрепления SHALL включать источник и Run. Один и тот же Run SHALL занимать одно место. При выборе третьего Run Light SHALL объяснить предел и сохранить текущий выбор. Состав и порядок SHALL сохраняться при возврате и перезагрузке той же вкладки, если доступна её память. Завершение Run SHALL сохранять закрепление до явного снятия.

Наблюдение SHALL показывать компактный список. У каждого Run SHALL быть собственное состояние и исход, известная стадия или причина ожидания либо ошибки, свежесть чтения и ссылка в Full. Если причина или стадия неизвестны, Light SHALL обозначить это. Переход SHALL называть открываемый раздел и сохранять точные источник и Run. Точная запись SHALL использоваться, если её однозначно определяют доступные данные. Состояние или причина другой ветви, другого Run или всей задачи MUST NOT приписываться выбранному Run.

Light SHALL сохранять выбор при неполной странице и обозначать отсутствие свежих сведений. При временном сбое и доступном снимке он SHALL отмечать снимок устаревшим. Явный отказ в доступе MUST NOT раскрывать подробности из сохранённого снимка. Ошибка одного чтения SHALL оставлять доступные строки видимыми. Наблюдение SHALL использовать существующие ограниченные чтения Light и правила доступа. Оно MUST NOT требовать полного обхода истории, чтения артефактов или управляющих операций.

#### Scenario: Два Run одной задачи или разных источников
- **WHEN** оператор закрепляет два Run одной задачи либо два Run из разных источников
- **THEN** строки показывают свои факты и ссылки, сохраняют порядок при обновлении и не объединяются по названию или совпавшему Run ID

#### Scenario: Выбранная запись отсутствует в полученной странице
- **WHEN** выбранный Run отсутствует в ограниченной странице либо источник временно недоступен
- **THEN** его закрепление остаётся на месте с объяснением доступности, без замены другим Run
- **AND** после успешного чтения выбранного Run обновляется только его подтверждённый снимок

#### Scenario: Доступ к источнику отозван
- **WHEN** чтение явно возвращает отказ в доступе
- **THEN** Light сохраняет место закрепления и сообщает об отказе без подробностей из прежнего снимка

#### Scenario: Возврат к доске и переход в Full
- **WHEN** оператор открывает наблюдение из задачи или доски, затем переходит в Full либо возвращается
- **THEN** Full открывает указанный источник, Run и доступную область, а Light сохраняет выбор, фильтры и место чтения

### Requirement: История задачи Light укладывается в узкий экран

Строки наблюдения, сведения задачи и история её Run SHALL читаться на узком экране без горизонтальной прокрутки всей страницы. Длинные названия, строки ветвей, идентификаторы и значения SHALL переноситься либо раскрываться на месте. Полное значение SHALL оставаться доступным для чтения и копирования. Состояние, основная причина и переход SHALL оставаться перед развёрнутыми техническими данными.

#### Scenario: Длинная строка стадии или ветви
- **WHEN** оператор открывает историю задачи с длинной строкой стадии, ветви или идентификатора на экране шириной 320–390 px
- **THEN** ширина страницы остаётся в пределах экрана, полное значение можно прочитать, а переходы доступны клавиатурой и касанием

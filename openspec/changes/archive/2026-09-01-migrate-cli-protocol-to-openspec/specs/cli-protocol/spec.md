## Purpose

Определяет versioned public command protocol Pri-Fly: DTO, CLI, validation,
executor boundary, errors, preview и compatibility без прямого изменения
authority клиентом.

## ADDED Requirements

### Requirement: Все клиенты используют один command protocol

CLI, local UI, assisted host и managed executor MUST применять один versioned
protocol. Только command handler меняет authority; UI, model и skill не пишут
state напрямую. Remote transport требует отдельной qualification security
properties и не является обязательным.

#### Scenario: UI пытается изменить state напрямую

- **WHEN** client обходит command handler
- **THEN** authority не принимает mutation

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
for another Attempt.

#### Scenario: Output изменён после hash

- **WHEN** worker submits modified bytes under prior digest
- **THEN** acceptance rejects result

### Requirement: CLI exposes scoped commands without hidden mutation

CLI MUST provide typed installation, authoring, preview/start, observation,
control, correction, decision and export commands through safe JSON/file input.
Read-only `next`, explain and events do not dispatch; init/install/remove and
project-wide control use their own scopes, not Run CAS.

#### Scenario: User calls next for blocked child

- **WHEN** child cancellation holds caller waiting
- **THEN** CLI reports scoped blocked state without starting a worker

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
success; typed result carries workflow state.

#### Scenario: External effect is unknown

- **WHEN** command reports unknown effect
- **THEN** safe next action is exact reconciliation, not blind retry

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

## Purpose

Определяет проверяемые packages, сценарии, контексты и YAML authoring Pri-Fly,
чтобы удобный исходник не менял граф, полномочия, bytes или history скрытым
образом.

## ADDED Requirements

### Requirement: Пустая установка остаётся рабочим состоянием
Pri-Fly MUST работать без packages, workflows, task sources, Git и LLM
credentials. Diagnostics MUST объяснять недостающую capability только для
нужной операции и MUST не создавать default workflow или auto-install.

#### Scenario: Workflow не установлен
- **WHEN** пользователь запрашивает отсутствующий workflow
- **THEN** он получает объяснимый отказ без Run или установки package

### Requirement: Ответственность частей package разделена
Core MUST управлять protocol и состоянием; package — reusable definitions;
adapter — внешний contract; project configuration — выбранные values/resources.
Прикладное правило MUST не становиться обязательным правилом Core.

#### Scenario: CSV workflow не требует Git review
- **WHEN** package не объявляет Git capability
- **THEN** Core не наследует её из другого установленного package

### Requirement: Executable reference имеет exact identity
Executable reference MUST содержать id, version и digest; alias, branch, URL
или display name MUST разрешаться только до lock. Другие bytes для той же
id/version MUST конфликтовать. Local workflow alias MUST читаться confined,
без сети, symlinks или traversal, и опускаться в exact ref до Run.

#### Scenario: Alias выбран для child workflow
- **WHEN** compiler разрешает author alias
- **THEN** sealed definition содержит exact ref, а не mutable alias

### Requirement: Package inventory проверяет exact bytes
PackageManifest MUST перечислять exports, dependencies и payload files с
digest, размером и назначением. Undeclared file MUST не загружаться как code
или instruction; unpack MUST отвергать path traversal, links и name collisions.
Manifest/signature MUST не входить в собственный inventory.

#### Scenario: Archive содержит post-install script
- **WHEN** package распаковывается
- **THEN** script не исполняется и файл вне выделенного каталога не появляется

### Requirement: Run lock-ит всё исполняемое closure
До Run admission compiler MUST lock-ить workflows, steps, schemas, adapters,
instructions, renderer и used configuration revisions. Incompatible dependency
или call/repeat cycle MUST давать объяснимый отказ до RunStart; latest MUST не
подгружаться между stages.

#### Scenario: Dependency меняется после preview
- **WHEN** package registry содержит новую revision
- **THEN** активный Run продолжает использовать прежнее locked closure

### Requirement: Установка отделена от исполнения
Install MUST разделять obtain, validation и atomic registration. Lifecycle
scripts, README commands и arbitrary post-install MUST не исполняться. Local
offline package MAY быть установлен, когда нужные bytes и trust evidence уже
доступны.

#### Scenario: Registration прерывается
- **WHEN** validation новой package не прошла
- **THEN** прежний package inventory остаётся usable

### Requirement: Доверие package имеет независимое основание
Package MUST хранить provenance, received digest, registrar и trust decision.
Local owner MAY явно доверить свой package; external trust MUST опираться на
independent policy. Signature или provenance MUST не доказывать safety сами по
себе.

#### Scenario: Package приносит собственный ключ
- **WHEN** external package предлагает доверить его подписи
- **THEN** ключ не становится trust root без внешней policy

### Requirement: Requested capability не является разрешением
`required_capabilities` MUST выражать потребность, а действующий operation
MUST лежать в пересечении host, organization, project, Run, Grant и exact
ActionIntent. Package, child workflow или adapter MUST не расширять caller
rights.

#### Scenario: Package запрашивает external write
- **WHEN** host или Grant его не допускают
- **THEN** install consent не разрешает этот effect

### Requirement: Package обновляется без смены pinned history
Package MUST объявлять supported protocol versions и capabilities. Unknown
required capability MUST быть отказом. New revision MUST устанавливаться рядом
с прежней, а config migration MUST быть отдельной проверяемой revision с diff.

#### Scenario: Изменён package default
- **WHEN** новый default установлен после RunStart
- **THEN** действующий lock и его effective configuration не меняются

### Requirement: Uninstall и revocation сохраняют историю
Uninstall MUST закрывать package для новых selection, но не удалять bytes,
pins, receipts, effects или evidence живого/сохраняемого Run. Security
revocation MUST блокировать новые admissions и trusted reuse, сохраняя
наблюдения прошлых effects и честную unavailable history.

#### Scenario: Package отозван во время Run
- **WHEN** следующий admission требует отозванный package
- **THEN** он блокируется, а уже наблюдаемый external effect не исчезает

### Requirement: Пользователь может создать минимальный local step
Пользователь MUST иметь возможность определить local StepDefinition с ports,
executor и result check и вызвать его из minimal workflow без roadmap, MR,
review, Core patch, task source или LLM. Executable code MUST принадлежать
package или pinned adapter, не input shell string.

#### Scenario: CSV normalizer запускается локально
- **WHEN** package описывает deterministic command step
- **THEN** workflow возвращает declared output и report без модели или сети

### Requirement: Configuration меняет values, а не поведение
Config value MUST иметь schema, scope, requiredness и default; unknown value
MUST отклоняться. Effective configuration MUST фиксировать source package
default, project, run или absent. Permissions/limits MUST пересекаться, а
новое behavior MUST требовать StepDefinition или WorkflowRevision.

#### Scenario: Project меняет declared input
- **WHEN** input разрешён project scope
- **THEN** Run lock сохраняет выбранное typed value без подстановки в policy
  или executable arguments

### Requirement: Adapter объявляет проверяемые свойства
Adapter MUST объявлять operations, schemas, effects, modes, platform
constraints и qualified properties fresh context, cancellation, idempotency,
readback и isolation. New Attempt MUST не менять protected target, arguments
или runtime selection без новой revision/intent.

#### Scenario: Provider сменён после approval
- **WHEN** dispatch требует другого adapter или model
- **THEN** исходная Attempt не считается воспроизводимой и требует новый допуск

### Requirement: Интеграции остаются optional consumers
AI Factory, SMSPlace и другие integrations MAY быть packages или project
configuration, но MUST не быть prerequisite Core или независимого workflow.
Удаление integration MUST не ломать unrelated package.

#### Scenario: AIF package удалён
- **WHEN** пользователь запускает local CSV workflow
- **THEN** он остаётся runnable без AIF skills или SMSPlace settings

### Requirement: Package показывает проверяемую пригодность
Package MUST поставлять positive/negative fixtures, contract checks и declared
applicability limits. Catalog MUST показывать identity, provenance, requested
rights, dependencies, unavailable reasons и pinned Runs. Author claim
`tests_passed` MUST не быть independent evidence.

#### Scenario: Executor недоступен
- **WHEN** user просматривает package
- **THEN** diagnostics объясняет unavailable capability и доступный следующий шаг

### Requirement: YAML является lossless authoring frontend
`prifly-workflow/1` и `prifly-step/1` MUST детерминированно опускаться в
canonical JSON definitions до schema, compiler, digest и Run. Runtime MUST не
интерпретировать YAML shortcuts. Full JSON и full YAML without marker MUST
сохранять машинный contract; every rare field MUST иметь полную форму.

#### Scenario: Автор использует полное машинное поле
- **WHEN** compact frontend не сокращает нужную настройку
- **THEN** compiler принимает её как unchanged canonical definition field

### Requirement: YAML defaults ограничены безопасной структурой
Authoring YAML MUST требовать identity, entry/stages, policy и execution
ceilings. Он MAY default title, empty ports/bindings, outcome set и безопасные
structural values, но MUST не угадывать routes, permissions, max iterations,
join, timeout, configuration scope или security semantics.

#### Scenario: Автор опускает join rule
- **WHEN** parallel stage требует join semantics
- **THEN** compiler отклоняет definition вместо скрытого выбора default

### Requirement: Project source компилируется из declared files
Tracked `.prifly/` project source MUST перечислять package identity, external
refs и known component files. Placeholder MUST заменять только whole YAML
scalar exact ref или explicit value; environment, shell, tags, anchors и prose
interpolation MUST быть запрещены. `project compile` MUST создать sealed
package без import, authority mutation или Run.

#### Scenario: Placeholder не найден
- **WHEN** YAML ссылается на undeclared component
- **THEN** compile отказывает без угадывания ref или изменения authority

### Requirement: Compact workflow folder остаётся одним графом
Folder workflow MUST содержать root `workflow.yaml`, optional `extend.yaml` и
one-document components only in known schemas/contexts/steps/workflows paths.
Directory name MUST не создавать ref or control flow. Simple extension MAY
replace exactly one direct route with input-free step; complex repeat, parallel,
map or bindings MUST быть явно записаны в graph.

#### Scenario: Extension пытается изменить parallel join
- **WHEN** author описывает сложную вставку через `extend.yaml`
- **THEN** compiler отказывает и требует явный workflow graph

### Requirement: Workflow отделяет definition, instances и operations
WorkflowRevision MUST declare finite stages, typed bindings, outcomes and
limits; input or SourceSnapshot MUST not modify graph. Workflow, Stage,
StepDefinition, StepInstance, Attempt, Turn and ActionDelivery MUST retain
their distinct identities. Compiler/router/predicates/budgets/transitions MUST
be deterministic code; semantic judgement MUST be a typed worker/check/human
result, not a control command or Approval.

#### Scenario: Worker предлагает маршрут
- **WHEN** worker prose names a new stage
- **THEN** active graph is unchanged until a validated new revision is admitted

### Requirement: Ports и bindings сохраняют exact provenance
Workflow and step ports MUST declare JSON/blob compatibility, requiredness and
required outcomes. Binding MUST name workflow input, declared literal or exact
producer/port, including declared projection schema; it MUST not select a
latest artifact by type/name. Compiler MUST prove required inputs on every
allowed path and distinguish absent from null.

#### Scenario: Только одна branch создаёт required output
- **WHEN** downstream requires that output after a choice
- **THEN** compiler requires an alternative binding or rejects the graph

### Requirement: Workflow composition имеет closed control semantics
Choice predicates MUST be bounded typed ASTs over sealed JSON facts, with
three-valued results, no eval/coercion and durable ordered decision trace.
Exclusive/first-match/default MUST have declared ambiguity and unknown paths.
Parallel and map MUST use finite stable branches/items, shared claims and Run
concurrency; joins MUST preserve outcomes, selection, cancellation/settlement
and MUST not treat satisfied as pass.

#### Scenario: Two exclusive branches are true
- **WHEN** both predicates evaluate true
- **THEN** choice records ambiguity rather than choosing heuristically

### Requirement: Repeat, call, wait и compensation остаются bounded
Repeat MUST have immutable body, initial/next maps, finite limit and declared
completion/unknown/limit routes; iteration state comes from sealed artifacts,
not worker memory. Call MUST be acyclic, keep child invocations in one Run and
export only declared outputs. Wait MUST bind authenticated event generation or
timeout. Compensation MUST be declared, separately admitted and never promise
universal rollback.

#### Scenario: Repeat reaches its limit
- **WHEN** final permitted iteration still requires continuation
- **THEN** declared on-limit route runs without changing max_iterations

### Requirement: Все composition limits учитываются целиком
Static and runtime checks MUST account together for calls, map, iterations,
retries, compensation, control transitions and resource claims. Local profile
limits MUST remain finite policy ceilings, not SLA; counters MUST not reset on
restart or new invocation. Structural validity, resolved refs, graph validity
and executable capability/admission MUST report distinct failure reasons.

#### Scenario: Nested map exceeds global budget
- **WHEN** another child admission would exceed a Run ceiling
- **THEN** it is rejected or waits before creating unaccounted work

### Requirement: Workflow terminal state и evolution имеют explicit contract
Finish MUST validate declared outcome, required outputs/bindings and unsettled
effects; no_work, rejected, partial, waiver and success MUST remain distinct.
Preview MUST be read-only. A protected input, policy or graph change MUST use
a new revision and continuation/fork; replay MUST not repeat external writes
without a separately admitted Run.

#### Scenario: Worker says completed
- **WHEN** required output or effect evidence is missing
- **THEN** Finish does not declare success from the worker message

### Requirement: Context manifest ограничивает exact reading
Each context-using Attempt MUST receive a sealed ContextManifest with exact
refs, digests, size, type, purpose, trust, read mode, order and renderer
revision. Package instructions, user request and untrusted data MUST retain
different roles; renderer MUST deterministically preserve boundaries and not
invent task instructions. Fresh capability MUST require proven isolation of
messages, files, tools and memory.

#### Scenario: Source text asks to ignore checks
- **WHEN** untrusted document appears in context
- **THEN** it remains data and cannot change policy, route or recipient

### Requirement: Mechanical execution и context never grant authority
Command adapter MUST receive typed argv/input channels without shell
concatenation or prompt requirement. Manifest inclusion MUST allow only declared
read; mutation, publish, Grant and next stage require separate exact admission.
Before dispatch, adapter MUST recheck sealed bytes, envelope validity and
confinement; changed bytes require new manifest/admission.

#### Scenario: File changes after preview
- **WHEN** adapter is about to dispatch changed content
- **THEN** it sends sealed bytes or blocks for a new approval path

### Requirement: Dynamic context, limits и secrets остаются explicit
Additional reading/search MUST be a declared bounded tool operation with
provenance. Context bytes, refs and token budget MUST be independently finite;
overflow MUST use declared split/summary/refusal, never silent truncation.
Secrets MUST use restricted runtime channels; redacted material MUST retain
its own digest and MUST not remove evidence while claiming full proof.

#### Scenario: Required source does not fit context budget
- **WHEN** renderer exceeds a mandatory limit
- **THEN** Run follows declared overflow handling or refuses before dispatch

### Requirement: Accepted result требует independent content checks
Artifact descriptor/schema/MIME MUST not prove semantic content. Typed result
MUST be validated for identity, outputs and declared checks; semantic review
MUST preserve its method and limits. Negative/inconclusive reports MUST remain
evidence, not hidden repair, and reuse MUST recheck protected inputs, checker
and current trust.

#### Scenario: PDF has valid descriptor but wrong bytes
- **WHEN** required content checker rejects it
- **THEN** artifact remains unavailable to downstream consumer

### Requirement: Human absence и changed intent не угадываются
Unattended mode MUST not manufacture Approval or widen task scope. A human
response MUST bind authenticated actor, scope, time, digests and operation;
clarification that changes input/graph/policy MUST get new admission, and a
stale worker result MUST not close the new activation.

#### Scenario: Пользователь пишет «да» после scope change
- **WHEN** no exact approval binding exists
- **THEN** Run waits or follows its declared refusal path

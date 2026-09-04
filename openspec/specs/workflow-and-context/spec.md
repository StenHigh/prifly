# workflow-and-context Specification

## Purpose

Определяет проверяемые packages, сценарии, контексты и YAML authoring Pri-Fly,
чтобы удобный исходник не менял граф, полномочия, bytes или history скрытым
образом.

## Requirements

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

### Requirement: YAML authoring имеет локальный editor contract
Repository MUST публиковать versioned local JSON Schema documents и manifest
для Project profiles `/2` и `/3`, workflow folder root, extension list, workflow, step и
context YAML. Contract MUST называть document kind, version marker, known
top-level fields и portable local schema association. Он MUST работать без
сети, AI Factory, credentials или обязательного editor dependency.

#### Scenario: Автор подключает local schema
- **WHEN** автор открывает YAML source или reference в совместимом editor
- **THEN** он может связать document с published local schema и получает
  completion и diagnostics простых полей до compiler

### Requirement: Editor metadata не меняет YAML compiler contract
Portable editor association MUST использовать YAML comments или editor-local
mapping, а не новое data field в authoring document. Such metadata MUST не
входить в parsed YAML value, lowering, exact refs, sealing или Run. JSON Schema
MUST не объявляться semantic authority: compiler MUST продолжать отклонять
недопустимые refs, graph, permissions и limits.

#### Scenario: Reference содержит editor modeline
- **WHEN** authoring reference с local schema modeline проходит existing
  lowering path
- **THEN** compiler принимает тот же authoring document без нового field в
  canonical definition или sealed package

#### Scenario: Schema не может доказать graph
- **WHEN** YAML проходит static editor shape check, но содержит semantic
  нарушение graph или exact ref
- **THEN** `project compile` отказывает до sealing согласно действующему
  compiler contract

### Requirement: Project authoring имеет один YAML route
Project execution profile MUST принимать `prifly-project-profile/2` и `/3`.
Первый срез `/3` меняет compilation identity, сохраняя обязательные Git,
host roots и RunBrief. Переход с `/2` MUST быть явной правкой shared profile,
не побочным эффектом init/start или workflow add/update. Fresh init пока
создаёт `/2`. Каждый declared package MUST ссылаться только на
directory `.prifly/workflows/NAME/` с root `workflow.yaml`, а каждый Project
launch MUST быть `workflow`, ссылающимся на такой root. Profile v1, его
отдельные source roots, `task_recipe`, direct machine workflow и file source
`prifly-package-source/1` MUST быть отклонены до sealing с понятной diagnostic.
Для этих дорелизных authoring forms не создаётся compatibility или migration
obligation.

#### Scenario: Старый authoring source подан compiler
- **WHEN** profile содержит v1, `task_recipe`, direct machine workflow или
  file package source
- **THEN** Pri-Fly отказывает до создания output package, authority mutation
  или Run

#### Scenario: YAML folder подан compiler
- **WHEN** profile v2 называет допустимую workflow folder и workflow launch
- **THEN** `project workflows` объявляет её inputs, а `project compile`
  выпускает тот же sealed package contract

### Requirement: Project YAML authoring имеет независимый corpus
Repository MUST хранить author-visible positive и negative `.prifly` YAML
fixtures вне Go unit tests. Independent verifier MUST копировать каждый case,
инициализировать только local authority когда это необходимо, и проверять
единственный public Project route через `project workflows` или `project
compile`. Corpus MUST подтверждать accepted workflow folder и отказ profile v1,
`task_recipe`, flat package file и unmarked direct workflow. Проверка MUST не
требовать сеть, AI Factory, credentials, package import или execution worker.

#### Scenario: Допустимая YAML folder проверяется вне unit tests
- **WHEN** external verifier получает accepted fixture
- **THEN** `project workflows` объявляет declared inputs, а `project compile`
  создаёт sealed package без authority mutation

#### Scenario: Legacy source попадает в corpus
- **WHEN** external verifier получает каждую legacy fixture
- **THEN** public CLI отказывает с понятной diagnostic до output package или
  Run

### Requirement: Project source компилируется из declared files
Tracked `.prifly/` project source MUST использовать только Project execution
profile `/2` или `/3` и declared workflow folders. Root `workflow.yaml` folder MUST
объявлять package identity, external refs, graph и known component directories;
compiler рекурсивно читает только YAML documents из этих declared locations.
Placeholder MUST заменять только whole YAML scalar exact ref или explicit
value; environment, shell, tags, anchors и prose interpolation MUST быть
запрещены. `project compile` MUST создать sealed package без import, authority
mutation или Run.

#### Scenario: Placeholder не найден
- **WHEN** YAML ссылается на undeclared component
- **THEN** compile отказывает без угадывания ref или изменения authority

### Requirement: Project context resolves the selected host skills root
`prifly-project-profile/2` и первый срез `/3` SHALL объявлять repository-relative skills roots
для `codex-cli`, `codex-app` и `claude-code`; compilation MUST назвать один
из них явно. Context source MAY назвать
regular file относительно skills root явного host compilation через YAML mapping
`{root: host_skills, path: PATH}`. Compiler MUST отвергать неизвестный host,
absolute path, traversal, symlink escape, отсутствующий file или source вне
`.prifly` и declared skills root до sealing. Он MUST закреплять выбранные exact
bytes; host identity MUST NOT менять YAML graph или давать полномочия.

#### Scenario: Claude Code skill закрепляется
- **WHEN** compiler получает `claude-code`, а context называет
  `aif-plan/SKILL.md` относительно выбранного skills root
- **THEN** он закрепляет exact bytes этого файла и не читает Codex root

#### Scenario: Context выходит за свой root
- **WHEN** context source использует `..` или после разрешения находится вне
  `.prifly` и selected host skills root
- **THEN** compiler отказывает до output, authority mutation или Run

#### Scenario: Два host используют один Project workflow
- **WHEN** Codex CLI и Claude Code компилируют одну Project workflow folder
- **THEN** каждый закрепляет bytes своего declared skills root без изменения
  авторского YAML; в `/3` одинаковые bytes дают одинаковую сборку независимо
  от host label, разные bytes дают разные compiled identities

### Requirement: Сборки одного авторского package сосуществуют
Compiler `/3` MUST сохранять авторские IDs, но назначать package и всем owned
components детерминированные compiled versions алгоритма b1. Build key MUST
покрывать effective profile/values/settings/exclude/extensions, normalized
definition closure, exact external refs, context bytes, manifest metadata и
decision catalog. Порядок файлов и absolute source paths MUST NOT влиять на
сборку. Version MUST использовать `0.0.0-b1.` и lower-case base32 полного
SHA-256 без padding: package из build key, component из canonical tuple
`[build key, kind, author ID, author version]`.

Compile и start MUST использовать один resolver и один прочитанный набор
исходников. Owned refs MUST перепривязываться; external refs, literal values,
configuration defaults и instance data внутри schemas MUST NOT переписываться.
`build-provenance.json` MUST быть inert manifest file закрытого формата
`prifly-build-provenance/1`, проверяемого по
[schema](../../../cmd/prifly/project_build.schema.json). Он MUST связывать
author refs с compiled exports/root и не включать собственный package digest.
Перед выбором root CLI MUST проверять schema, полную mapping, derivation
versions и соответствие exports. Эти сведения MUST NOT заменять trust admission.
External consumers MUST получать exact compiled refs, не author alias latest.
`/2` MUST сохранять legacy compilation без provenance, а collision/revocation
и pinned history MUST сохраняться в обоих путях.

#### Scenario: Разные варианты запускаются в одной authority
- **WHEN** пользователь компилирует, импортирует и запускает A, B, A одного package
- **THEN** A и B сосуществуют, повтор A воспроизводит refs, а старый активный
  Run после restart сохраняет свои definitions и context bytes

#### Scenario: Настройка меняет сборку
- **WHEN** меняется только extend setting, exclude или вставка шага
- **THEN** новая сборка устанавливается рядом без переименования авторского package

#### Scenario: Bytes sealed identity подменены
- **WHEN** та же sealed identity подана с другими bytes или revoked build импортирован повторно
- **THEN** import отказывает; collision checks и отзыв не обходятся новой compilation

#### Scenario: Изменился только вопрос или описание
- **WHEN** author меняет только decision catalog либо manifest description
- **THEN** `/3` получает новую identity, не заменяя прежнюю сборку

### Requirement: YAML authoring явно объявляет Workspace artifact tree transform

`prifly-step/1` MUST позволять author-у выразить finite declared Workspace tree
binding только полной проверяемой формой: one manifest output port, optional
compatible manifest input port и bounded capture policy (`exact_file`,
`direct_child_file` или `direct_child_tree`). Compiler MUST reject duplicate
paths/ports, blob or arbitrary JSON ports, absent `workspace_write`, path
outside the claimed repository Workspace и any form that denotes recursive
sync, glob, symlink or implicit file discovery. YAML lowering MUST preserve
this declaration in sealed StepDefinition; compiler MUST not infer it from
skill prose, filename or output name.

#### Scenario: YAML связывает Ultra plan tree
- **WHEN** Project step declares compatible plan-manifest ports and direct
  child tree policy under a relative parent
- **THEN** `project compile` seals one explicit tree binding; a similar
  directory in instructions without that declaration creates no binding

### Requirement: Compact workflow folder остаётся одним графом
Workflow folder MUST быть единственным внешним Project package authoring source
и содержать root `workflow.yaml`, optional `extend.yaml` и one-document
components only in known schemas/contexts/steps/workflows paths. Directory
name MUST не создавать ref or control flow. Simple extension MAY replace
exactly one direct route with input-free step; complex repeat, parallel, map
or bindings MUST быть явно записаны в graph.

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

### Requirement: Project init prepares a context-capable authority

`prifly project init` MUST create its separate authority with the current Core
context configuration required to pin selected host skills and other context
resources. A Project launch MUST reject an older or incompatible authority
before package registration, Workspace claim or Run creation; it MUST NOT
silently reinterpret that authority's existing Runs. When a clone already has
a valid tracked profile and exact host runners, init MUST create only its absent
ignored local authority configuration; shared Project YAML and runners remain
unchanged.

#### Scenario: Старый Core authority выбран для Project launch
- **WHEN** declared Project launch получает authority без current context
  configuration
- **THEN** CLI возвращает stable incompatibility diagnostic без package, claim
  или Run

#### Scenario: Clone получает свою authority
- **WHEN** tracked Project profile and host runners are already present, but the
  machine-local configuration is absent
- **THEN** `project init` creates the local authority configuration without
  replacing the profile or runners

### Requirement: Project launch является единственной исполнимой точкой входа

Public Project launch MUST принимать exact ID объявленного `workflow` launch,
explicit host и typed значения только его объявленных input ports. Он MUST
compile, seal и зарегистрировать exact package before creating Run; source YAML,
host skill bytes и effective inputs MUST become pinned Run inputs. Launch MUST
not выбирать сценарий по тексту задачи, default launch или наличию файлов.
Interactive project host MUST require an explicit `worktree` or `checkout`
selection before it invokes the launch; absence of that answer is a wait, not
a fallback to a different Workspace. A non-interactive CLI invocation MAY use
the declared command default. Запуск Project workflow не запускает
model/provider и не даёт host новых полномочий: assisted handoff остаётся
отдельным existing contract.

#### Scenario: Объявленный launch запускается
- **WHEN** пользователь называет существующий launch, declared host и все
  required inputs
- **THEN** система создаёт Run только из sealed revision этого launch и
  возвращает его identity вместе с выбранным workspace

#### Scenario: Launch не объявлен
- **WHEN** пользователь называет отсутствующий или не-workflow launch
- **THEN** система отказывает до compilation, package registration, claim или
  Run creation

#### Scenario: Host не получил выбор Workspace
- **WHEN** пользователь выбрал launch в диалоге, но не назвал worktree или
  checkout
- **THEN** host задаёт этот единственный вопрос и не создаёт package, claim или
  Run до ответа

### Requirement: Project YAML объявляет решения без скрытого control flow
Project workflow authoring source MUST поддерживать один декларативный каталог
решений и readable tree для его крупных записей. Каждая запись MUST иметь
stable ID и полную машинно валидируемую форму; compact YAML form MAY опускать
только безопасные presentation defaults. Решение может supply только явно
объявленный launch/configuration/step input и MUST NOT добавлять stage, route,
capability, effect или правомочие.

#### Scenario: Большой package раскладывает решения по файлам
- **WHEN** author помещает допустимые YAML decision declarations в разрешённое
  дерево package
- **THEN** compiler читает их как один catalog, а пути улучшают навигацию, но
  не меняют graph или semantic meaning

#### Scenario: Решение пытается включить optional feature
- **WHEN** catalog answer прямо изменяет feature, route или capability, не
  будучи declared configuration input с обычной validation
- **THEN** compiler отказывает до sealing

### Requirement: Условный вопрос зависит только от sealed предшественника
Project YAML MAY ограничить видимость решения выбранным package profile и
exact typed answer previously declared preflight decision. Compiler MUST
reject unknown, forward or cyclic predecessor references before sealing. Such
a predicate MUST NOT add a route, stage, effect, capability or permission.

#### Scenario: Roadmap milestone появляется после linkage
- **WHEN** `roadmap_milestone` declares exact predecessor answer
  `roadmap_linkage = "link"`
- **THEN** host presents it only after that answer and records both values in
  the same immutable Decision Sheet

### Requirement: Project default не маскирует выбор запуска
Project configuration MUST различать reviewed package default и explicit
per-Run launch value. Default MAY заполнить omitted optional launch choice
только по rule sealed package, но host/CLI selection для одного Run MUST иметь
приоритет без записи обратно в tracked authoring files.

#### Scenario: Interactive host выбирает другой profile
- **WHEN** host получает omitted package profile и пользователь выбирает
  допустимый не-default profile
- **THEN** новый Run использует выбор пользователя, а `extend.yaml` остаётся
  byte-for-byte прежним

### Requirement: Workflow repository содержит discoverable Project workflow folders
Workflow repository — любой Git-репозиторий, доступный пользователю, который
содержит одну или несколько Project workflow folders. Pri-Fly MUST находить
такие папки только по root `workflow.yaml` с marker
`authoring: prifly-project-workflow/1`, не спускаясь внутрь найденной папки,
не следуя symlinks и с ограниченной глубиной обхода. Repository MUST NOT
требовать отдельный manifest или registration file: `examples/` самого
Pri-Fly является таким repository без изменений. Если найдена ровно одна
папка, установка MAY обойтись без явного пути; при нескольких папках без
явного `--path` и при отсутствии папок команда MUST отказать с перечнем
найденных путей и без записи в repository проекта. Явный `--path` MUST
указывать на папку с marker; иначе — отказ.

#### Scenario: Repository содержит один сценарий
- **WHEN** пользователь устанавливает repository, в котором найдена одна
  Project workflow folder
- **THEN** именно она копируется без дополнительного выбора

#### Scenario: Repository содержит несколько сценариев
- **WHEN** пользователь устанавливает repository с несколькими папками и не
  задал `--path`
- **THEN** Pri-Fly отказывает, перечисляет найденные пути и не меняет
  `.prifly/`

#### Scenario: Путь указывает на папку без marker
- **WHEN** `--path` называет каталог, чей `workflow.yaml` отсутствует или не
  имеет marker `prifly-project-workflow/1`
- **THEN** установка отказывает до копирования и регистрации

### Requirement: Установка workflow folder разделяет obtain, validate и register
`prifly project workflows add` MUST получить exact commit по запрошенному ref
(tag, branch или commit; по умолчанию remote HEAD), скопировать из выбранной
папки только regular files и структурно проверить копию тем же reader
Project workflow folder, что использует compile, прежде чем атомарно
поместить её в `.prifly/workflows/NAME/` и объявить `packages.NAME` и
`launches.NAME` в tracked `project.yaml`. Symlink, вложенный Git repository
или gitlink, превышение лимитов количества и размера файлов MUST быть
отказом без частичной папки. Имя берётся из `--name` или basename папки и
MUST удовлетворять правилам launch ID; занятое имя папки, package или launch
и другой declared package с тем же `package.id` MUST быть отказом. Установка
MUST NOT seal-ить, импортировать или доверять package, компилировать его
против host, создавать Run или Git commit; ни один полученный файл не
исполняется. Результат MUST называть identity package, declared external
references и записанный origin, а также оставшиеся шаги владельца: review,
commit `.prifly` и обычный `project compile`.

#### Scenario: Сценарий установлен из репозитория
- **WHEN** пользователь выполняет `add` с валидным repository и ref
- **THEN** появляется `.prifly/workflows/NAME/` с declared package и launch, а
  authority, sealed packages и Runs не меняются

#### Scenario: Имя уже занято
- **WHEN** `.prifly/workflows/NAME`, `packages.NAME` или `launches.NAME` уже
  существуют
- **THEN** установка отказывает и не перезаписывает существующую папку или
  запись

#### Scenario: Папка содержит symlink
- **WHEN** выбранная папка в repository содержит symlink или вложенный Git
  repository
- **THEN** установка отказывает до записи в `.prifly/`

### Requirement: Workflow folder origin закрепляет exact commit и digest
Для установленной папки tracked `project.yaml` MUST хранить
`packages.NAME.origin` со строгими полями: `repository` без userinfo, `path`
внутри repository, `ref`, `commit` из 40 hex, `digest` дерева папки без
корневого `extend.yaml`, необязательные `extend_digest` для upstream
`extend.yaml` и `catalog` для установки по имени каталога. Origin — заявление
установившей команды, проверяемое локальным digest; это не TrustDecision и не
sealed `PackageOrigin`. Profile без `origin` остаётся допустимым
`prifly-project-profile/2`; неизвестное поле или невалидное значение внутри
`origin` MUST быть отказом чтения профиля. Написанная вручную папка без
origin не подлежит `update`.

#### Scenario: Профиль без origin читается как прежде
- **WHEN** `project.yaml` объявляет package только с `source`
- **THEN** `project workflows`, `compile` и `start` работают без изменений

#### Scenario: Origin содержит неизвестное поле
- **WHEN** `packages.NAME.origin` содержит поле вне закрытого списка или
  `commit` не из 40 hex
- **THEN** чтение профиля отказывает с понятной diagnostic

### Requirement: Update сохраняет exact identity и правки команды
`prifly project workflows update NAME` MUST требовать записанный origin,
пересчитать digest текущей папки без `extend.yaml` и при расхождении
отказать с перечнем изменённых путей, ничего не перезаписывая. Если удалённый
commit для ref не изменился и digest совпадает, команда MUST завершиться
успешным read-only результатом. Иначе она MUST получить новую папку по тому
же `path`, проверить её как при установке, перенести локальный `extend.yaml`
byte-for-byte, атомарно заменить папку и обновить `origin`. Результат MUST
сообщать, изменился ли upstream `extend.yaml` и остался ли `package.version`
прежним. Sealed packages, locks, Runs и evidence в authority MUST NOT
меняться. `/3` MUST создавать новую exact сборку при изменении исходников
с прежней авторской версией. `/2` MUST сохранять legacy identity conflict и
объяснять явный переход на `/3`; подмена уже sealed identity MUST оставаться
отказом в обоих путях, а не тихой заменой.

#### Scenario: Папка изменена локально
- **WHEN** digest установленной папки без `extend.yaml` отличается от origin
- **THEN** `update` отказывает, перечисляет изменённые пути и не трогает файлы

#### Scenario: Удалённый commit не изменился
- **WHEN** ref указывает на тот же commit, а digest совпадает
- **THEN** `update` сообщает актуальность и ничего не записывает

#### Scenario: Upstream не поднял версию package
- **WHEN** новая папка отличается по bytes, но `package.version` тот же
- **THEN** `update` применяет папку и объясняет: `/3` создаст отдельную сборку,
  а `/2` столкнётся с конфликтом, если прежняя identity уже установлена

### Requirement: Remove убирает folder из tracked profile, а не из authority
`prifly project workflows remove NAME` MUST удалить `.prifly/workflows/NAME`,
запись `packages.NAME` и каждый launch, чей `workflow` лежит в этой папке.
Команда MUST NOT изменять sealed packages, Runs, receipts или evidence в
authority: их история сохраняется по общим правилам uninstall.

#### Scenario: Установленный сценарий удалён
- **WHEN** пользователь выполняет `remove` для объявленного package
- **THEN** папка и её launches исчезают из tracked profile, а authority
  остаётся прежней

### Requirement: Workflow catalog служит только discovery
Workflow catalog — Git-репозиторий с root `catalog.yaml`
`prifly-workflow-catalog/1`: карта `categories` и карта `workflows`, где
каждая запись называет `title`, `description`, `category`, `repository`,
`path`, необязательные `ref`, `commit` и `tags`. Имена записей и категорий
MUST следовать правилам launch ID; неизвестное поле, неизвестная категория,
относительный `repository` или превышение лимитов MUST быть отказом.
`search` MUST возвращать детерминированный список с категориями и
фильтровать по подстроке и категории. `add NAME` MUST разрешить запись
каталога и далее следовать обычной установке из repository; если запись
объявляет `commit`, полученный commit MUST совпасть, иначе — отказ. Каталог
MUST NOT переносить bytes сценариев, ключи или trust; его чтение не делает
запись доверенной.

#### Scenario: Поиск по каталогу
- **WHEN** пользователь выполняет `search` с подстрокой или категорией
- **THEN** он получает отфильтрованный детерминированный список записей без
  изменения repository проекта

#### Scenario: Закреплённый commit не совпал
- **WHEN** запись каталога объявляет `commit`, а repository по `ref` даёт
  другой commit
- **THEN** установка отказывает до копирования

#### Scenario: Запись каталога неизвестна
- **WHEN** `add NAME` не находит запись в выбранном каталоге
- **THEN** команда отказывает и не пытается угадать repository

### Requirement: Репозиторий Pri-Fly не поставляет product workflows
`examples/` репозитория Pri-Fly MUST содержать только authoring references и
нейтральные примеры контракта; product workflow packages, включая AI Factory,
MUST жить во внешних Workflow repositories и попадать в проект через Workflow
catalog или `project workflows add`. Engine-гарантии — declared Workspace tree
bindings, package profiles, decision catalog, `exclude` и `settings` — MUST
проверяться нейтральными fixtures Pri-Fly, а контракт конкретного product
package — проверками его собственного repository против latest stable release
Pri-Fly. Изменение YAML authoring contract Pri-Fly MUST NOT считаться
совместимым с внешним package молча: его repository проверяет совместимость
сам и поднимает свою версию.

#### Scenario: Проекту нужен product workflow
- **WHEN** разработчик хочет сценарий AI Factory
- **THEN** он устанавливает его из каталога или repository, а в `examples/`
  Pri-Fly такой папки нет

#### Scenario: Engine feature меняется
- **WHEN** Pri-Fly меняет compile, profiles или decision catalog
- **THEN** регрессия ловится нейтральным fixture Pri-Fly, а внешний repository
  проверяет свой package против released Pri-Fly отдельно

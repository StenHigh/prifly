## Purpose

Определяет проверяемое исполнение Pri-Fly: от неизменного входа выбранной
работы до завершения Run, его evidence, effects и совместимого изменения
протокола без скрытых полномочий или утраты истории.

## ADDED Requirements

### Requirement: Области исполнения имеют независимую идентичность
Installation, Project, Workspace, Authority и Principal MUST оставаться
разными сущностями. Их roots MUST задаваться независимо, а physical resource
MUST не получать нескольких authority owners из-за разных отображаемых имён.
Remote identity MUST включать provider, account, namespace и resource.

Versioned project execution profile MUST храниться в repository отдельно от
authority root. `project init` MUST создавать profile, local authority root и
ignored local configuration с точными путями к Pri-Fly и `prifly-run`; этот
skill MUST выбирать launch до чтения его inputs. Local state, receipts,
artifacts и claimed worktree MUST оставаться вне repository. Raw `--project`
MUST означать authority root, а не каталог profile.

#### Scenario: Один каталог назван двумя проектами
- **WHEN** два project profiles указывают на один physical workspace
- **THEN** система распознаёт общий resource identity и не создаёт для него
  независимых authority owners

### Requirement: Ресурсы Run имеют разные роли и неизменяемые revisions
RunBrief, TaskInput, SourceSnapshot, WorkflowRevision, WorkflowInvocation,
StageActivation, StepDefinition, StepInstance, Attempt, ContextManifest,
ExecutionEnvelope, ArtifactRevision, Evidence, ActionIntent, Approval, Grant,
Admission, ActionDelivery, ControlIntent, ResourceClaim, RunEvent и
Reconciliation MUST сохранять объявленные разные роли и lifecycle. Immutable
объект MUST не менять bytes или смысл после sealing; versioned объект MUST
нести собственную revision и правила отзыва.

#### Scenario: Принимается результат работы
- **WHEN** worker сообщает результат допущенной попытки
- **THEN** он становится artifact, evidence или Result только через их
  отдельные immutable records, а не заменяет исходные данные Run

### Requirement: Вложенное исполнение сохраняет единый Run
Root workflow и все child invocations MUST принадлежать одному Run с общим
journal, CAS, budget и barriers. Каждый child MUST иметь собственные
invocation, activation и execution identities, exact input/output refs и
локальный outcome; он MUST не быть неявным новым RunStart. Вызовы, repeat,
parallel, map и compensation MUST сохранять parent relationship и не
объединять одинаковые definition references в одну работу.

State/read compatibility MUST сохранять реальные child invocations и их
local ready stages без второй root queue. Repeat body invocations MUST быть
последовательными siblings с 1-based iteration и новыми execution identities.

#### Scenario: Один child workflow вызывается дважды
- **WHEN** parent дважды вызывает одну workflow revision
- **THEN** оба вызова имеют разные invocation, activation, step и attempt
  identities, но учитываются в одном root Run

### Requirement: Run закрепляет полный состав исполнения
До исполнения Run MUST lock-ить brief/input snapshots, workflow closure,
steps, tools, adapters, schemas, instructions, contexts, configuration,
checks и policy revisions с exact refs. Mutable alias MUST разрешаться до
lock и поздняя правка package, config или project setting MUST не менять
активный Run. Недоступные pinned bytes MUST дать
`pinned_resource_unavailable`, а не подменяться latest version.

Required configuration MUST быть valid до RunStart; optional absence MUST
оставаться отсутствием, а не получать скрытый default. Repeat MUST lock-ить
весь body closure и независимо применять initial/next bindings. Emergency
deny и revocation MUST проверяться по текущему состоянию перед эффектом.

#### Scenario: Автор меняет workflow после запуска
- **WHEN** установленный workflow изменён во время активного Run
- **THEN** существующий Run использует прежние pinned bytes и не подхватывает
  новую definition

### Requirement: Машинные definitions имеют однозначную форму
Commands и manifests MUST использовать strict UTF-8 JSON: duplicate keys,
повреждённый Unicode, неизвестные safety-critical fields и неподдерживаемые
versions MUST отклоняться. YAML frontend `prifly-workflow/1` MUST
детерминированно преобразовываться в тот же canonical JSON без executable
tags, anchors, ambiguous numbers и неограниченного expansion; он MUST
оставлять доступным каждый field машинной формы.

Digest и подпись MUST использовать объявленную canonicalization version,
сохранять порядок массивов и не менять identifiers скрытой
Unicode/case-нормализацией. Точные финансовые limits MUST не зависеть от JSON
floating point.

#### Scenario: Author использует неоднозначный YAML
- **WHEN** YAML definition содержит alias, executable tag или неоднозначное
  числовое значение
- **THEN** compiler отклоняет definition до validation и locking

### Requirement: Связи данных объявляются точными портами
StepDefinition MUST объявлять input/output ports, schema refs, requiredness,
media types и constraints. Workflow MUST связывать input только с workflow
input, exact producer output или declared literal; поиск «последнего похожего
файла» MUST быть запрещён. Несовместимый format MUST требовать отдельного
converter с собственным evidence.

JSON outputs MUST проверяться по schema, а binary outputs — по descriptor,
media type, integrity и content checker. Наличие JSON Schema MUST не
доказывать смысл binary content.

#### Scenario: Два producer создают похожие artifacts
- **WHEN** downstream stage требует output одного из них
- **THEN** binding указывает exact producer и port либо workflow не проходит
  validation

### Requirement: Artifact принимается только после sealing
ArtifactRevision MUST хранить producer identity, output port, revision,
digest, byte size, media type, schema, timestamp, classification и
provenance. Import внешнего input MUST иметь собственный producer provenance и
не требовать фиктивного worker. Worker MUST завершить запись, artifact store
MUST проверить bytes и seal, а Core MUST transactionally связать revision с
результатом.

Пустой artifact MAY быть принят только когда это допускает content contract.
Публикация artifact MUST быть отдельным effect с pinned digest, target и
проверяемой expected external version; filename или локальный draft MUST не
считаться разрешением на публикацию.

#### Scenario: Upload прерывается до sealing
- **WHEN** storage содержит незавершённый или пустой upload
- **THEN** Run не считает его accepted output только по имени файла

### Requirement: Evidence подтверждает конкретное утверждение
Evidence MUST связывать subject, claim, method, checker/adapter revision,
inputs, runtime/environment, outcome, timestamps, outputs/logs и limitations.
Process exit, schema validity, passing tests, review approval и remote effect
MUST оставаться разными claims. Worker prose MUST не становиться evidence без
независимой проверки.

Mandatory acceptance checks MUST иметь собственные request/result protocol,
admission и provenance. Pending acceptance MUST удерживать candidate bytes до
positive checks или refusal; поздний intake MUST не переписывать settled
producer result. Semantic or human judgement MUST явно сохранять критерии,
материалы, reviewer и ограничения уверенности.

#### Scenario: Проверка сообщает pass
- **WHEN** check process завершился с кодом 0 и вернул valid JSON
- **THEN** Core отдельно проверяет process, schema и содержательный verdict
  перед принятием claim

### Requirement: Reuse требует совместимых оснований
Reuse MUST быть разрешён только при совпадении значимых inputs, code, tool,
runtime, checker, context, preconditions и срока evidence. Semantic subject
MUST определяться rule, а не автоматически полным repository state. Readback,
human approval и permission MUST сохранять собственные freshness и revocation
semantics.

Cache miss MUST идти обычным execution path и cache MUST не быть источником
полномочий. Retention, удаливший нужное evidence, MUST сделать reuse/replay
недоступным, а не заменяться пересказом worker-а.

#### Scenario: Изменились тесты задачи
- **WHEN** cached pass относится к artifact, но тесты или checker изменились
- **THEN** Core не использует этот pass как совместимое reuse evidence

### Requirement: Core детерминированно управляет control loop
Core MUST кодом, а не LLM, проверять schemas, transitions, attempts, hashes,
permissions, leases и state. Semantic judgement MUST приходить через declared
worker, checker или human step как typed DecisionArtifact. DecisionArtifact
MUST не становиться Grant, Approval или произвольной control command.

#### Scenario: Worker предлагает следующий шаг
- **WHEN** worker присылает поле или prose с другим `next_step`
- **THEN** Core игнорирует его как control command и применяет только
  заранее объявленный transition

### Requirement: Исполнение проходит через проверяемые границы control loop
Router MUST только читать authoritative state и объяснять ready/waiting/
finished/blocked projection. Mutation path MUST проверять gates, immutable
inputs, claims и limits, затем выдавать exact envelope; adapter MUST выполнять
только допущенную работу; один writer MUST принимать outcomes и events
atomically.

Result intake MUST сохранять фактический report, проверять identity, schema,
outputs и receipts и передавать их mandatory acceptance checks. Удобная команда
`run` MAY объединять эти действия, но MUST не убирать проверяемые границы или
recovery path между ними.

#### Scenario: Adapter останавливается после dispatch
- **WHEN** процесс прерывается между двумя соседними стадиями control loop
- **THEN** Run сохраняет проверяемое состояние и объявленный recovery path, а
  не предполагает успешный result

### Requirement: ExecutionEnvelope ограничивает одну допущенную попытку
Envelope MUST содержать run, invocation, activation, step, attempt и
admission identities; pinned workflow/step refs; context/input refs; scoped
grants; output contracts; deadlines; budget reservations и claim generations.
Его digest MUST вычисляться после sealing. Одна worker attempt MAY создать
несколько logical operations, но admission MUST быть выдан по каждой exact
operation, а не как blanket authority для будущих arguments.

Prompt renderer MUST быть детерминированной функцией approved instructions и
envelope, не полного parent transcript. Он MUST не позволять package
instructions отменить ограничения user, host или system.

#### Scenario: Worker пытается вызвать новый tool
- **WHEN** envelope допустил одну operation, а worker выбирает другие
  arguments или target
- **THEN** для нового вызова требуется отдельный exact intent и admission

### Requirement: Конкурентные команды принимаются атомарно
Mutation command MUST нести command ID и expected run version. Exact duplicate
MUST вернуть исходный result, а повтор с иным canonical payload MUST
конфликтовать. Единственный writer MUST принимать current version, добавлять
monotonic event sequence и не позволять обходной direct state rewrite.

Result MUST проверяться против active attempt, relevant input/context/
permission/resource generations. Независимый result MAY быть принят с новым
CAS после reread state, но superseded, cancelled или materially stale attempt
MUST быть отклонён с сохранением late evidence. Stop MUST быть монотонным;
release stop MUST требовать current epoch и отдельный admission.

#### Scenario: Две команды конкурируют за один budget
- **WHEN** обе команды используют одну expected run version
- **THEN** commit получает только одна, а другая не оставляет partial
  reservations, claims или events

### Requirement: Время приходит от доверенного источника
Reducer MUST принимать recorded event time, а не читать wall clock. Trusted
authority or clock adapter MUST назначать admission/deadline time; worker
timestamp MUST не продлевать право. Public timestamps MUST быть UTC с `Z`, а
source offset/timezone MAY сохраняться отдельно.

Live timeout MUST использовать monotonic duration в пределах одного boot,
после restart MUST иметь declared conversion. Clock rollback, skew или
неопределённое current time MUST блокировать чувствительный admission до
восстановления.

#### Scenario: Host clock откатывается
- **WHEN** approval или deadline проверяется после clock rollback
- **THEN** старое client time не продлевает его и Core возвращает безопасный
  отказ при невозможности доверенного сравнения

### Requirement: Run завершает только доказанный lifecycle outcome
Run MUST переходить через created, ready, running, waiting, stopping,
uncertain, completed, failed или cancelled с declared reason. `completed`
MUST иметь allowed outcome; partial, rejected, no_work и waiver MUST не
маскировать missing mandatory evidence, unfinished mandatory branch или
unknown/pending effect. Cancelled child MUST не выдавать workflow outcome и
MUST удерживать caller blocked до required settlement.

Terminal Run MUST быть доступен для чтения/export или нового связанного Run,
но не для тихого переоткрытия прежней истории.

#### Scenario: Внешний effect остаётся неизвестным
- **WHEN** workflow пытается завершить Run с mandatory unknown effect
- **THEN** Core не переводит Run в completed, failed или cancelled как будто
  obligation settled

### Requirement: Step и Attempt различают предметный и технический исход
StepInstance MUST иметь declared lifecycle и присваивать verdict только
accepted Result. `completed + fail` MUST означать известный отрицательный
предметный result, тогда как technical failure MUST оставаться `failed` без
verdict. Unselected branch MUST не создавать фиктивные skipped instances.

Semantic rework MUST создавать новую iteration и StepInstance. Whole-step
retry MUST требовать safety qualification либо checkpoint/replay protocol и
новый ExecutionAdmission; safe transport redelivery MUST создавать новый
ActionDelivery, сохраняя operation, intent, admission и owning Attempt.
Terminal Run MUST не переоткрываться retry.

#### Scenario: Target просит повторить доставку
- **WHEN** exact operation имеет действующую safe retry qualification
- **THEN** Core создаёт новый delivery ordinal, не запускает worker и не
  создаёт новую Step Attempt

### Requirement: Skip, waiver и no_work не равны pass
Skip MUST означать разрешённое непроведение шага, waiver — recorded owner
decision о допустимом отсутствии проверки, а no_work — доказанное отсутствие
нужной работы. Ни один из этих outcomes MUST не считаться pass по умолчанию.
Если downstream требует output такого шага, workflow MUST объявить compatible
default, иной producer или verified reuse; иначе route MUST быть invalid.

Human rejection MUST иметь собственный declared transition и не обязан
совпадать с согласием. Core MUST не задавать один универсальный смысл отказа
для всех workflows.

#### Scenario: Пропущенный шаг должен был дать output
- **WHEN** следующий stage требует output skipped step
- **THEN** compiler отклоняет route без declared compatible alternative

### Requirement: Intent фиксирует effect до approval
ActionIntent MUST существовать до запроса approval и фиксировать exact
operation, tool/adapter revision, arguments, input digests, target/resource,
preconditions, output contracts/destinations, effect class, scope, owning
attempt и dispatch deadline. Изменение защищённого field MUST создавать новый
intent и approval. Несуществующий будущий artifact MUST не получать
выдуманный digest.

Когда exact objects неизвестны заранее, approval MUST охватывать finite
manifest либо строго bounded selector/grant с измеримыми limits; произвольный
`all` MUST не считаться точным scope.

#### Scenario: После approval изменён recipient
- **WHEN** dispatch использует другой target или protected argument
- **THEN** старый approval не допускает effect и требуется новый intent

### Requirement: Approval и Grant имеют ограниченную силу
Approval MUST хранить state, authenticated actor, intent digest, policy,
expiry и required independence. Atomic consume MUST связать его максимум с
одним logical operation admission; safe delivery retry MUST не потреблять
approval повторно и не продлевать deadline. Grant MUST иметь срок, scope,
capabilities и budgets; каждый admission MUST быть отдельным record и MUST
проверять current revocation/stop.

Owner approval MUST не заменять отсутствующее host или external API
authorization. Grant MUST не расширять свои собственные права.

#### Scenario: Два клиента одновременно consume approval
- **WHEN** они пытаются допустить одну protected operation
- **THEN** максимум один logical admission получает consumed approval

### Requirement: Неизвестный внешний effect блокирует обычную работу
Action delivery lifecycle и effect status MUST быть отдельными. Core MUST
различать not_started, not_applied, applied, partially_applied и unknown;
неизвестный остаток MUST оставаться unknown, а штатный in-flight effect MAY
оставаться pending без перевода Run в uncertain. Terminal Run MUST не иметь
pending effects.

В uncertain Run ordinary admissions MUST быть blocked. Qualified recovery MAY
только повторно доставить ту же exact operation с теми же intent, admission,
arguments и target при действующем durable dedup guarantee и новых checks
current controls/budget. Cancellation или локальный rollback MUST не обещать
откат remote effect; compensation MUST быть новой declared operation.

#### Scenario: Ответ потерян после remote write
- **WHEN** adapter не может доказать, был ли применён отправленный effect
- **THEN** Core сохраняет unknown и не допускает новую ordinary operation до
  reconciliation или qualified recovery

### Requirement: Ручной результат сохраняет evidence и границы policy
Manual action MUST импортироваться с actor, scope, artifact/receipt refs,
declared changes и проверками данного step. Он MUST не записывать force
`pass=true` и MUST invalidировать dependent admissions/evidence, когда меняет
authority или execution environment согласно policy. Break-glass MUST быть
заранее declared limited profile, а не универсальная кнопка обхода.

#### Scenario: Человек выполнил работу вне executor
- **WHEN** он импортирует manual result
- **THEN** Core требует его scoped provenance и required checks, не заменяя
  их необоснованным success verdict

### Requirement: Условия графа используют закрытую типизированную семантику
Conditions MUST быть typed AST над declared facts и ports, а не executable
Python, JavaScript, SQL, template expression или natural language. Exclusive
choice MUST получать ровно одну true branch, `first_match` MUST сохранять
pinned order, а default MUST требовать proven false всех alternatives.
Unknown MUST идти через declared `on_unknown` либо durable
`condition_unknown`; schema error MUST не превращаться в false.

#### Scenario: Условие не получает входных данных
- **WHEN** predicate reference отсутствует или имеет неверную schema
- **THEN** workflow не выбирает default branch как будто condition false

### Requirement: Композиция workflow ограничена и учитывается целиком
Repeat MUST иметь finite max iterations, exit predicate и explicit on-limit
route. Fan-out MUST иметь sealed finite item manifest, stable item identity,
max instances/parallelism и result rules. Call graph MUST не иметь hidden
recursion; child depth, steps и control transitions MUST наследовать общий
Run budget, а локальные limits MUST только сужать policy ceiling.

Repeat MUST создать первую body iteration, проверить until после body и
выбирать declared completion, unknown или limit routes без implicit error
fallback. Expansion, restart или одинаковые definitions MUST не обнулять
consumed budget. Смена allowance MUST быть finite, authorized и не превышать
pinned ceiling.

#### Scenario: Вложенный repeat исчерпывает общий budget
- **WHEN** локальные limits допускают больше работ, чем остаток Run
- **THEN** Core оставляет Run waiting для явного решения и не создаёт часть
  новых children незаметно

### Requirement: Изменение definition создаёт новую совместимую работу
Изменение workflow, input, policy, predicate, schema, authorization,
evidence validity или package resolution MUST не менять active lock. Replan
или fork MUST иметь validated revision и explicit карту изменённых inputs,
stages, permissions, evidence, выполненных effects, reusable и повторяемых
работ. Default path MUST быть новым связанным Run с сохранением provenance;
in-place migration MUST требовать отдельного qualified protocol.

#### Scenario: Author меняет workflow активного Run
- **WHEN** требуется изменить stages или permissions уже запущенной работы
- **THEN** Pri-Fly создаёт validated replan/fork или новый связанный Run, не
  редактируя active route напрямую

### Requirement: Эволюция протокола не переписывает историю Run
Изменение schema, predicate semantics, outcome meanings, authorization,
evidence validity или package resolution MUST рассматриваться как protocol
migration, а не как совместимое изменение только потому, что JSON продолжает
парситься. Поддерживаемый старый Run MUST читать pinned interpreter либо
проверенную migration; при невозможности MUST получать объяснимый refusal и
export/recovery path.

Исправленный reducer/checker MUST иметь новую revision. Revised assessment
MUST оставаться отдельной от исходной audit history и указывать affected
earlier decisions.

#### Scenario: Исправлен defect checker-а
- **WHEN** новая revision меняет оценку прежнего результата
- **THEN** исходные events и outputs сохраняются, а revised assessment явно
  ссылается на новую версию и affected Run

### Requirement: Event и schedule имеют явные границы автоматизации
Trigger MUST закреплять workflow revision, input mapper, identity source,
authority/grant и quota. Event MUST иметь stable source/event identity,
schema и receipt; duplicate, late и out-of-order delivery MUST обрабатываться
по declared contract. Wait correlation MUST включать run, step, attempt и
nonce.

Schedule MUST объявлять timezone, calendar/DST, logical slot,
catch-up/skip/coalesce policy и finite burst budget. Пропущенные slots MUST
не запускать опасные historical actions автоматически. Unattended execution
MUST быть explicitly enabled; эта specification MUST не создавать user
automations сама по себе.

#### Scenario: Timer доставлен повторно после restart
- **WHEN** managed host получает один и тот же durable timeout дважды
- **THEN** он принимает максимум один logical transition для его wait
  generation

### Requirement: Внешняя задача проходит через нейтральный immutable intake
`TaskInput/1` MUST быть read-only document между человеком или read-only
source reader и RunBrief; он MUST не быть workflow или командой стартовать
работу. Его exact bytes MUST стать SourceSnapshot, а `task prepare` MUST
валидировать input и source refs, materialize RunBrief в authority intake и
дедуплицировать повтор тех же bytes.

Intake MUST сохранять raw external text, title, desired outcome, scope,
criteria, source provenance, existing snapshot refs, assumptions и owner
confirmation. `unconfirmed` MAY быть подготовлен, но RunStart MUST его
отклонить. Source metadata MUST не быть credentials или permission; live
attachment MUST сначала стать SourceSnapshot. Новый GitLab, GitHub, Jira или
другой reader MUST создавать тот же TaskInput contract и MUST не получать
write rights только от факта чтения.

#### Scenario: Неподтверждённая задача подготовлена
- **WHEN** owner ещё не дал explicit confirmation выбранной работы
- **THEN** `task prepare` может вернуть проверяемый brief, но RunStart не
  начинает её до подтверждения

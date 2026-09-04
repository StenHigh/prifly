## Purpose

Определяет durable runtime Pri-Fly: детерминированное управление, state,
resources, external effects, recovery и storage без скрытой потери history или
authority.

## Requirements

### Requirement: Ядро управляет выполнением детерминированным кодом

Core MUST валидировать definition, вычислять готовность, выбирать разрешённые
переходы, учитывать limits, выдавать admissions и сохранять решения обычным
детерминированным кодом. LLM допускается только внутри объявленного step и
даёт структурированный input следующей проверки; он не выбирает successor,
не авторизует действие и не меняет state. Пустая установка без packages, Git,
sources и LLM credentials остаётся диагностируемой, а unknown workflow MUST
отказать до effects.

#### Scenario: Запускается пустая установка

- **WHEN** пользователь запрашивает doctor, list или unknown workflow без
  установленного package
- **THEN** диагностика и пустые списки доступны, а unknown workflow не создаёт
  effect

### Requirement: Assisted и managed используют единый протокол authority

`assisted` host только доставляет typed commands и исполняет выданные actions;
он не становится authority переходов и не обещает background work без active
host. `managed` executor обслуживает admitted actions, observations и timers
как отдельный процесс. Его capabilities запуска, проверки результата, отмены,
наблюдения после restart и process identity MUST быть объявлены; отсутствующая
capability недоступна. Interaction с человеком — отдельная ось и не расширяет
права.

#### Scenario: Нужное решение ждёт человека

- **WHEN** сценарий требует human decision
- **THEN** Run переходит в видимое ожидание и не получает неявного согласия

### Requirement: Assisted handoff не зависит от worker socket

Runtime MUST создавать owner-only local Unix socket только непосредственно
перед запуском managed local worker, которому нужен publication transport.
Assisted handoff, который передаётся существующему host и не запускает local
process, MUST достигать wait без этой зависимости. Если managed worker требует
socket, но платформа запрещает его создать, Run MUST получить stable
`local_socket_unavailable` refusal before process dispatch; это не является
успешным handoff или скрытым fallback transport. `doctor` MUST предварительно
показывать availability этой возможности; это локальная проверка, не promise,
что permission не изменится позже.

#### Scenario: Assisted Project launch на socket-restricted platform

- **WHEN** declared launch доходит до assisted host handoff без managed worker
- **THEN** он ждёт host без создания Unix socket

### Requirement: Reference profile использует одну локальную authority

Reference deployment MUST состоять из Go CLI/library, одной local state
authority, SQLite journal и файлового immutable artifact store. JSON служит
export/projection, но не authority. SQLite WAL допускает local clients одного
host, но не координацию машин; NFS, SMB и sync folder не квалифицируются для
effects. State directory находится вне mutable project, а workers не получают
прямой writable доступ к journal.

#### Scenario: Journal расположен в сетевой папке

- **WHEN** profile использует NFS или SMB для effect authority
- **THEN** qualification profile отклоняется

### Requirement: Run закрепляет полный исполняемый контракт

До start Run MUST pin-ить WorkflowRevision, StepDefinition, port schemas,
policy, package lock, adapter/tool revisions и initial inputs immutable refs
`{id, version, digest}`. Resolver сохраняет конечное dependency closure;
после start используются pinned bytes, а отсутствие pinned version блокирует
исполнение. Изменение workflow, input или policy создаёт новый либо явно
связанный Run и не переписывает history. Digest доказывает bytes, но не trust
автора или побитовую воспроизводимость неimmutable provider.

#### Scenario: Workflow изменён после start

- **WHEN** installed workflow получает новую revision
- **THEN** открытый Run использует pinned bytes либо останавливается без чтения
  новой revision молча

### Requirement: External snapshot и signal имеют точное происхождение

Необязательный SourceSnapshot MUST хранить source ref, external identity и
version при наличии, schema, content digest, время получения и provenance.
Core принимает typed data либо declared opaque bytes, но не интерпретирует
неизвестный предметный текст как instruction и не выдумывает task id. Incoming
signal имеет stable source identity, schema, payload ref, observation и
authentication provenance; duplicate с тем же digest дедуплицируется, другой
digest — conflict. Receipt означает durable receive, не application; unsuitable
или late signal quarantined и не воскрешает closed wait.

#### Scenario: Повторный event меняет payload

- **WHEN** source присылает прежний event id с другим digest
- **THEN** event quarantined как conflict и Run не продвигается

### Requirement: Входы StepInstance согласованы и sealed

Input manifest MUST называть точные producer instances, ports, artifact
revisions, external snapshots и порядок collection. Неявный выбор «последнего
похожего файла» запрещён. Admission читает один consistent journal snapshot;
inputs разных revisions не смешиваются, а mutable file materialize-ится в
sealed snapshot до execution. Evidence reuse требует совпадения relevant
inputs, checker/tool/environment revisions и policy; heartbeat сам по себе его
не инвалидирует.

#### Scenario: Есть два совместимых producer

- **WHEN** input port имеет несколько кандидатов без binding rule
- **THEN** admission отказывает с ambiguity, а не выбирает один из них

### Requirement: Journal хранит единую проверяемую историю

Authority MUST вести append-only event journal с unique Run sequence,
authenticated command receipts, immutable attempts, action deliveries, claims,
artifacts, waits, timers и budget provenance. Materialized state восстанавливается
из journal и versioned checkpoint; unknown event/schema при replay даёт reader
отказ, а не пропуск. Logical records разделяют Run, workflow invocation, stage
activation, StepInstance, Attempt и operation; logs, artifact bytes и
immutable pinned bytes (definitions, context resources, envelopes) не
дублируются в каждом event или checkpoint: они хранятся один раз по digest, а
checkpoint ссылается на них. Переходы состояний записываются как события, а
не как растущее поле checkpoint. Логическая форма и digest Run от этого не
меняются.

#### Scenario: Reader встречает неизвестное событие
- **WHEN** replay не поддерживает сохранённый event type или schema
- **THEN** authority отказывает без частичного восстановления state

#### Scenario: Run принимает сотни команд
- **WHEN** Run с большим pinned closure выполняет много команд
- **THEN** размер истории растёт с числом событий, а не с числом команд,
  умноженным на размер closure

### Requirement: Управляющая команда коммитится атомарно и идемпотентно

Обычная mutable Run command MUST проверять authenticated principal, namespace,
current permission, expected version или generation, pinned inputs, allowed
transition, stop/revocation, claims и budgets в одной transaction с events,
projections, reservations, telemetry samples и durable receipt. Transform под
write lock MUST быть pure: без компиляции определений, запуска процессов,
чтения файлов и работы, растущей с историей authority; всё это выполняется до
transaction, а transform сравнивает digests. Duplicate того же command id и
digest возвращает stored result после current read authorization и не
повторяет effect; другой digest — conflict. Логически тот же запрос,
повторённый с тем же command id, MUST давать тот же digest: часы и другие
недетерминированные значения не входят в protected payload. Сбой
персистентности authority (lock contention, storage) MUST быть записан как
сбой authority и MUST NOT быть представлен как failure worker или входить в
`on_error`. Stop может монотонно ужесточить scope при stale screen version, но
release и resume требуют своих exact checks и не снимают stop неявно.

#### Scenario: Две команды используют одну Run version
- **WHEN** две ordinary mutable commands имеют одинаковую expected version
- **THEN** коммитится только одна, без частичных events, claims или budgets

#### Scenario: Повтор команды с тем же id
- **WHEN** владелец повторяет `waive` или `approval decide` с тем же command
  id после потерянного ответа
- **THEN** authority возвращает stored receipt как duplicate без conflict

#### Scenario: Write lock занят при наблюдении процесса
- **WHEN** другая транзакция удерживает write lock дольше busy timeout во
  время persist наблюдения worker
- **THEN** settlement называет сбой authority, а workflow не идёт по
  `on_error`

### Requirement: Durable storage принимает только sealed bytes

Reference SQLite profile MUST проверять реально включённые WAL, FULL, foreign
keys, supported SQLite build и bounded busy timeout. Artifact upload резервирует
storage, записывает и sync-ит bytes в staging, проверяет size/digest, seal-ит
их и лишь затем коммитит reference. Crash может оставить orphan, но не
successful step с missing blob; GC уважает active upload pins. Missing
referenced blob — integrity incident, а hash не доказывает автора.

#### Scenario: Сбой между sealing и reference commit

- **WHEN** storage падает после sealed blob, но до journal reference
- **THEN** recovery может зарегистрировать orphan, но не публикует completed
  output

### Requirement: Run завершает только известный aggregate outcome

Runtime MUST различать created, ready, running, waiting, stopping, uncertain
и terminal состояния. `completed`, `failed` и `cancelled` не допускаются при
unresolved effect, pending обязательстве или незавершённом cleanup; partial
outcome возможен только по declared aggregate contract. `uncertain` запрещает
new ordinary admissions до reconciliation, но сохраняет visible facts и
recovery actions. Позднее противоречащее evidence создаёт incident, а не
переписывает history.

#### Scenario: Terminal result имеет unknown effect

- **WHEN** workflow достиг предметного result, но effect остаётся unknown
- **THEN** Run не получает terminal status

### Requirement: StepInstance и Attempt имеют разные lifecycle

Runtime MUST различать semantic StepInstance и technical Attempt. Accepted
`fail` либо `needs_revision` завершает StepInstance и выбирает declared path;
technical `failed` не имеет accepted result и обрабатывается safe retry или
`on_error`. Verdict `pass`, `fail`, `needs_revision`, `no_work`, `skipped` и
`waived` не подменяют друг друга. Attempt владеет всем execution step, а каждая
operation имеет отдельную ActionDelivery; retry delivery не создаёт новую
Attempt.

#### Scenario: Worker возвращает техническую ошибку

- **WHEN** terminal result не принят из-за technical failure
- **THEN** StepInstance не получает предметный verdict и следует declared
  technical-error path

### Requirement: Scheduler разворачивает только конечный и честный граф

Definition validation MUST проверять reachable paths, types, ambiguity, finite
bounds и resource requirements. Fan-out и input-driven map закрепляют ordered
sealed membership до first child admission; repeat, nested invocation,
instances и control transitions расходуют общие persistent budgets. Scheduler
принимает slots, claims и reserve одной transaction, применяет declared policy
с stable tie-break и не допускает starvation. Restart, resume, polling и
receipt replay не обнуляют budgets.

#### Scenario: Коллекция меняется после fan-out

- **WHEN** source collection изменена после membership snapshot
- **THEN** новые members не появляются в уже admitted graph

### Requirement: Turn разделяет routing, admission и effect

Read-only router MUST не вызывать tools, не резервировать resources и не
писать journal. ExecutionAdmission допускает конкретную Attempt и scoped
resources, но не unknown tool calls. Каждая proposed external operation создаёт
отдельный immutable ActionIntent и проходит own admission по exact target,
args, approval/grant, claims, budgets, deadlines и current stop/revocation.
Historical approval не обходит новый запрет.

#### Scenario: Worker предлагает второй tool call

- **WHEN** допущенный worker предлагает неизвестную operation
- **THEN** core требует отдельные intent и admission до dispatch

### Requirement: ExecutionEnvelope ограничивает результат конкретной попытки

Envelope MUST содержать protocol version, authority/Run/instance/attempt
identity, execution admission, pinned input/context refs, grants, executor,
resource generations, output contracts и limits без secret environment copy.
Worker возвращает typed result, output refs, receipts и observation provenance,
но не authoritative successor, policy или approval. Core принимает result
только от active Attempt с relevant input/control generations; duplicate
dedup-ится, conflict сохраняется. Независимое изменение global Run version не
обесценивает добросовестный parallel result само по себе.

#### Scenario: Result прислан чужой Attempt

- **WHEN** worker сообщает terminal result с другой attempt identity
- **THEN** core не завершает Step и сохраняет reason и evidence

### Requirement: Контекст и evidence честно описывают свои границы

Fresh-context profile MUST передавать только declared ContextManifest и
различать подтверждённую свежесть, невозможность её подтвердить и нарушение.
Host не добавляет скрытую цель, данные или права; current system, host и user
restrictions применяются поверх pinned package context. Закрытие step требует
declared artifacts, receipts и checker outcome; prose объясняет result, но не
становится evidence. Semantic review допускается лишь с честно названной
моделью или человеком.

#### Scenario: Adapter наследует parent transcript

- **WHEN** fresh-context adapter не может исключить скрытую историю родителя
- **THEN** profile не получает qualification свежего context

### Requirement: External effect имеет отдельный durable ledger

Journal commit не образует distributed transaction с Git, shell, remote SQL,
SaaS или provider. До dispatch каждой external operation MUST иметь logical
identity, exact intent, admission и durable delivery record. Delivery status и
effect status раздельны: `not_started`, `not_applied`, `applied`,
`partially_applied` и `unknown` имеют доказанные условия, а in-flight и
asynchronous acknowledgement могут ещё не знать effect. Unknown блокирует new
conflicting ordinary admissions и terminal finish до causal reconciliation.

#### Scenario: Ответ target потерян

- **WHEN** dispatch мог завершить effect, но receipt потерян
- **THEN** status становится unknown и core не делает blind retry

### Requirement: Capability adapter доказывается для точной operation

Adapter MUST объявлять и квалифицировать idempotency, conditional write,
readback, cancellation, fencing, compensation и bounded cost для exact target
binding и revision. Preliminary read не заменяет conditional write, gateway
lease не заменяет target fencing, а forecast не считается hard cap. Credential
rotation допустима лишь при доказанной той же authority. Assisted reported cost
хранится как observation, а не provider qualification или monetary cap.

#### Scenario: Adapter не доказывает idempotency

- **WHEN** target не подтверждает scope, key, payload и retention duplicate
- **THEN** runtime не обещает один effect от retry

### Requirement: Retry сохраняет identity и unknown effect

Retry MUST иметь declared safe profile, конечные delivery/attempt/deadline
budgets и current permissions. Transport retry создаёт ActionDelivery той же
operation с exact intent, admission и owning Attempt; whole-step retry требует
qualified checkpoint protocol, учитывающий ledger всех прежних operations.
После unknown допускается только qualified reconciliation resend с действующей
exact dedup guarantee; смена args, target, protected field или expired dedup
запрещает retry. Unknown usage остаётся conservatively reserved.

#### Scenario: Нельзя доказать safety whole-step retry

- **WHEN** worker исчез после partial или unknown effect без checkpoint protocol
- **THEN** runtime не запускает новую Attempt автоматически

### Requirement: Partial failure и компенсация остаются видимыми

Workflow MUST заранее объявлять fail-fast либо collect-results, failure paths
и aggregate outcomes. Compensation — новая typed operation с own intent,
permission, claim, evidence и budget; она связывается с original effect, но не
удаляет history. План компенсации задаёт dependency order, preconditions и
failure policy; невозможная компенсация сохраняет residual effect. Неделимость
multiple targets без capability отклоняется до запуска, а не имитируется
последовательностью вызовов.

#### Scenario: Один target уже изменён

- **WHEN** fan-out частично применил effects и следующий target отказал
- **THEN** aggregate и compensation следуют declared contract, а residual
  effect видим пользователю

### Requirement: Claims используют физическую identity и generation

Resource identity MUST обозначать реальный объект и authority, не human title:
physical workspace/volume, DB server/schema/owner или SaaS scope. Claim называет
owner, scope, mode/capacity, generation и observed lease; adapter задаёт conflict
relation, aliases и parent/child scopes. Local authority выдаёт весь
conflict-aware claim set атомарно с admission. Unknown identity или отсутствие
overlap proof даёт отказ либо явно более широкий claim.

#### Scenario: Два Run запрашивают пересекающийся scope

- **WHEN** concurrent admissions имеют конфликтующие physical identities
- **THEN** только допустимый atomic claim set получает admission

### Requirement: Lease не доказывает прекращение старого владельца

Expiry, heartbeat loss и смерть клиента MUST переводить ownership в
suspected/uncertain и требовать probe; PID сверяется с host, boot и start
identity. Reuse возможен после доказанного прекращения либо target fencing,
отвергающего stale generation. Unfenceable unknown request блокирует новый
conflicting admission. Несколько hosts не ведут independent SQLite authorities
одного mutable scope.

#### Scenario: Старый worker всё ещё может работать

- **WHEN** lease истекла, но process или external request не подтверждённо
  остановлен
- **THEN** новый conflicting admission не выдаётся

### Requirement: Isolation profile объявляет реальные границы исполнения

Profile MUST отдельно объявлять filesystem, process, environment/network,
credential и target-data isolation. Worker получает typed argv, controlled cwd,
allowlisted environment и resource handles; task text не конкатенируется в
shell command. Writable workspace принадлежит Attempt/Run и его output seal-ится
отдельно. Git, container и database profiles закрепляют physical identities и
optional: они не навязываются noncoding workflow.

#### Scenario: Workflow не требует Git

- **WHEN** noncoding scenario запускается в isolated profile
- **THEN** отсутствие Git adapter не делает execution недопустимым

### Requirement: Cleanup проверяет ownership и exact generation

Cleanup MUST быть ограниченной durable operation над точной resource identity
и generation. Он удаляет только созданный и всё ещё принадлежащий owner объект;
claim release и process kill не доказывают cleanup. Unknown owner, changed
identity, active descendant или unavailable target блокируют deletion и reuse.
Late cleanup старого generation не может удалить новый resource, а cleanup
failure не скрывается за основным verdict.

#### Scenario: Resource создан заново под прежним именем

- **WHEN** stale cleanup наблюдает name нового generation
- **THEN** runtime не удаляет новый resource

### Requirement: Stop и cancel сохраняют обязательства

Durable stop acknowledgement MUST запрещать новые ordinary admissions и
перечислять ранее admitted work, cancellation, known и unknown effects.
Pause переходит в waiting после safe stop; cancel становится terminal только
при known судьбе admitted actions, иначе Run uncertain. Probe, cancel и
reconcile действуют по separate recovery permissions. Managed executor
прекращает dequeue и перепроверяет admission перед dispatch; assisted host
честно сообщает, удалось ли остановить worker. Scope cancellation не отменяет
siblings неявно.

#### Scenario: Cancel не может проверить remote target

- **WHEN** target недоступен после cancellation request
- **THEN** пользователь видит uncertain obligation, а не ложный terminal cancel

### Requirement: Recovery классифицирует каждое открытое обязательство

При открытии authority MUST проверить storage/schema/identity и bounded
incremental integrity: события после последнего verified cut проверяются и
отметка сдвигается; полная верификация всей истории выполняется по явной
команде (`doctor`) и никогда не пропускается молча. Затем authority rebuild-ит
materialized state из checkpoint и events и классифицирует каждую open
Attempt, dispatch, upload, claim и budget reservation. Recovery MUST
использовать уже сохранённые доказательства завершения процесса: если journal
содержит наблюдение пустой process group, obligation закрывается как известный
settlement loss с освобождением slot, а не как uncertain; при отсутствии
такого наблюдения после dispatch obligation остаётся uncertain до явной
резолюции. Crash после commit возвращает receipt, crash около external call
требует probe/readback, а crash около sealing/cleanup сохраняет recovery
boundaries. Progress и checkpoint не равны completion; call и repeat replay
используют saved control facts и не создают second child или iteration.

#### Scenario: Crash после effect до local receipt
- **WHEN** target мог применить operation до записи receipt
- **THEN** recovery оставляет effect unknown до target readback или manual
  reconciliation

#### Scenario: Crash после наблюдения пустой process group
- **WHEN** journal содержит `group_empty` для dispatched attempt, а driver
  исчез до settlement
- **THEN** recovery закрывает attempt как settlement loss, освобождает slot и
  не переводит Run в uncertain

#### Scenario: Открытие большой authority
- **WHEN** authority открывается после многих проверенных cuts
- **THEN** проверяются только события после verified cut, а полный скан
  доступен через `doctor`

### Requirement: Resume и fork не переписывают исходный Run

Resume MUST продолжать тот же Run с immutable inputs и definitions после
revalidation current permissions/resources; он не снимает stop и не проходит
completed steps заново. Transport и whole-step retry следуют отдельным
protocols. Fork создаёт новый Run с source provenance и exact source version;
reuse возможен лишь через declared immutable inputs с rechecked port/trust
status. Старые approvals, mutable worker state и safety external effects не
переходят в fork.

#### Scenario: Пользователь пытается resume после stop

- **WHEN** stop действует или появился между release и resume
- **THEN** resume отклоняется и не снимает stop

### Requirement: Checkpoint и reader version сохраняют history

Checkpoint MUST хранить schema/reducer version, `as_of_seq`, state digest,
pinned definitions и replay prerequisites. Он ускоряет чтение, но не создаёт
вторую authority; перед purge проверяется воспроизводимость и нужные dedup
records. Migration выполняется при остановленных admissions, с backup и replay
copy, не запускает user tools и не превращает unknown в success. Unsupported
reader или downgrade на incompatible schema отказывает для всей authority.

#### Scenario: Старый binary читает новую state form

- **WHEN** executable не поддерживает сохранённую event или state version
- **THEN** он отказывает для authority, не пропуская новую history

### Requirement: Restore начинается без права на новые effects

Backup MUST включать согласованный journal snapshot, pinned manifests,
referenced blobs, schema/version inventory и recovery metadata; открытый SQLite
файл нельзя копировать как standalone backup. Restore начинает recovery mode
без new effect admissions и не возобновляет старые dispatches автоматически.
После backup утраченные receipts, claims, approval consumption и settlements
unknown до reconciliation. Backup clone не образует active second authority;
recovery требует actual fencing либо доказанной остановки старых owners.

#### Scenario: Backup потерял поздний receipt

- **WHEN** восстановленный journal не содержит external receipt после snapshot
- **THEN** affected scope остаётся blocked до independent ledger/readback

### Requirement: Время поступает как сохранённое наблюдение

Reducer MUST не читать clock, network или environment напрямую: trusted adapter
передаёт typed durable time observation, а replay использует ту же запись.
Deadline учитывает source, boot/process identity и clock anomaly; uncertainty
отказывает admission с expiry. Monotonic components Go time применимы лишь
внутри одного live clock domain и не сериализуются как durable authority.
Managed timers восстанавливаются из journal, а assisted mode не обещает
background wakeup.

#### Scenario: Системные часы переведены назад

- **WHEN** UTC clock jump или source timestamp противоречит deadline
- **THEN** runtime не продлевает разрешение и сохраняет observation для replay

### Requirement: Budgets и backpressure учитывают весь admission

До admission MUST резервироваться limits slots, attempts, capacity, execution,
context/output/storage и enforceable cost. Reserve, transfer, settlement и
release идемпотентны и имеют exact provenance; child invocation не создаёт
новый budget. Unknown effect/usage сохраняет reserve до known outcome.
Measured, provider-reported, reserved и estimated значения различаются, а
missing value — null с причиной. Queue или disk backpressure останавливает
ordinary admission, оставляя bounded recovery reserve.

#### Scenario: Provider timeout не сообщает стоимость

- **WHEN** external call timed out без usage receipt
- **THEN** cost reserve не освобождается и soft estimate не выдаётся за hard cap

### Requirement: Storage соблюдает privacy, retention и erasure boundary

History MUST хранить нужные metadata/reference с classification и access policy;
raw task, prompt, transcript, output и credentials не попадают в общий audit
автоматически. Retention не удаляет active manifests, unresolved receipts,
claims, grants/approval audit или dedup tombstones. GC и erasure журналируются;
для erasure, переживающего old restore, нужен recovery ledger вне rollback
snapshot. До его replay restored content quarantined.

#### Scenario: Старый backup восстановлен после erasure

- **WHEN** актуальный erasure ledger недоступен во время restore
- **THEN** protected content остаётся quarantined и не выдаётся worker

### Requirement: Telemetry и report выводятся из journal

Runtime MUST вести bounded structured logs, correlation и metrics queue,
active/waiting/uncertain runs, conflicts, attempts, retries, orphans, receipts,
WAL/storage, context, provider usage и rework без raw secret/task data. Report
строится из journal и pinned definitions и показывает expected/factual steps,
skip/waiver, claims, input/output refs, checker, budget, unresolved obligations
и allowed continuation. Missing evidence или metric остаётся visible incident
или null, а не pass или zero; historical telemetry versions не смешиваются.
Объявленные limits запроса MUST быть достижимы внутри объявленного
cooperative deadline на qualified fixture; report сверяет digest сохранённых
publications и не выполняет их повторную валидацию процессами.

#### Scenario: Evidence link повреждён
- **WHEN** report встречает missing required evidence
- **THEN** он сообщает integrity incident и не показывает check как pass

#### Scenario: Запрос на объявленном потолке
- **WHEN** telemetry query охватывает максимально допустимое число Runs и
  records
- **THEN** ответ укладывается в объявленный deadline на qualified fixture, а не
  отказывает по нему

### Requirement: Local-1 profile объявляет qualification envelope

Local-1 MUST публиковать hardware/storage configuration, finite limits runs,
attempts, instances, control transitions, fan-out/map, composition depth,
repeat/retry, command, event и artifact size, а также control load и latency
targets. Workspace может только сузить defaults; повышение требует отдельный
profile, admission validation и repeat qualification. Эти числа не являются
измеренной производительностью, пока workload report не фиксирует configuration,
distribution, IO, checkpoint cadence и results.

#### Scenario: Workspace повышает max items

- **WHEN** workspace запрашивает limit выше qualified Local-1 default
- **THEN** admission требует отдельный qualified profile вместо неявного роста

### Requirement: Failure model честно ограничивает RPO и RTO

Profile MUST разделять process crash, power loss, disk/key loss, malicious
administrator и active backup clone. Acknowledged journal durability и recovery
classification заявляются только для qualified storage/configuration; SQLite
не означает backup или high availability. Backup RPO/RTO относятся к safe
read/recovery mode и declared backup envelope, а не automatic external effect
resume; restore exercise выполняется изолированно без production effects.

#### Scenario: Backup не настроен

- **WHEN** installation не имеет qualified backup
- **THEN** runtime не заявляет backup RPO или RTO

### Requirement: Готовность подтверждается воспроизводимой qualification

Release-ready runtime MUST иметь reproducible contract и fault tests для exact
core build, OS, SQLite, filesystem и adapter profiles. Наличие schema, positive
agent narrative или synthetic check вне заявленного scenario не доказывает
готовность. Документация публикует unsupported profiles и protection limits;
real adapter проверяется на безопасном target с разрешением владельца. History
при stop, resource loss и worker prose не подменяется legacy helper или runner.

#### Scenario: Проверен только synthetic adapter

- **WHEN** profile заявляет qualification real external adapter
- **THEN** он требует отдельную разрешённую проверку этого adapter, а не только
  synthetic test

### Requirement: Workspace claim сохраняет exclusivity в обоих Git режимах

Authority MUST distinguish a disposable `worktree` Workspace from a direct
`checkout` Workspace while using the same canonical physical repository
identity, owner, generation and lease conflict rules. A `worktree` claim MAY
create and later clean up only its own confined directory. A `checkout` claim
MUST refer to the current canonical Git checkout and MUST NOT create, delete,
switch branch, reset or clean that checkout. Both modes MUST block a
conflicting Pri-Fly Workspace claim until it is explicitly settled.

#### Scenario: Checkout mode leaves Git topology unchanged
- **WHEN** authority admits a direct checkout Workspace
- **THEN** no Git worktree is added and no branch, HEAD or tracked file is
  changed by admission itself

#### Scenario: Existing Workspace is held
- **WHEN** another Run requests either mode for the same physical repository
- **THEN** authority rejects the conflicting admission without creating a
  directory or changing the checkout

### Requirement: Runtime confined materialize-ит и capture-ит declared Workspace tree

Runtime MUST materialize и capture-ить declared Workspace tree binding только
в RepositoryWorkspace current WorktreeClaim той Attempt. До записи он MUST
проверить normalized relative root/entry paths, отсутствие traversal,
regular-parent directories и отсутствие symlink escape. Existing bytes каждой
input entry MUST совпадать с pinned ArtifactRevision; output-only target MUST
соответствовать declared capture policy и recorded pre-handoff state. Любое
иное состояние MUST дать stable drift/refusal без автоматической перезаписи
repository file.

Capture MUST enumerate only regular files permitted by declared policy,
recheck confinement и entrypoint, seal every entry before creating the output
WorkspaceTreeManifest и сохранить full input provenance. Crash, missing blob,
non-regular entry, extra out-of-policy file или changed-during-capture tree
MUST не создавать accepted output. Binding не расширяет возможности host:
право изменять claimed Workspace остаётся результатом declared
`workspace_write` effect и existing WorktreeClaim.

#### Scenario: Рабочая копия содержит чужое изменение плана
- **WHEN** до handoff declared input plan entry существует с bytes, отличными
  от pinned ArtifactRevision
- **THEN** runtime отказывает до передачи host и не заменяет файл сохранённой
  копией молча

#### Scenario: Ultra bundle captured после handoff
- **WHEN** host завершил workspace-write Attempt и declared direct-child tree
  содержит valid `index.md` и phase files внутри claimed Workspace
- **THEN** runtime сохраняет все exact raw bytes как один sealed output
  WorkspaceTreeManifest с provenance prior input manifest, не читая другой
  похожий план

### Requirement: Ожидание решения durable и восстанавливаемо
Runtime MUST persist decision request, allowed response contract, originating
Attempt/Step identity, decision ledger transition и wait reason atomically.
Pending decision MUST prevent its step from speculative retry or successor
selection. Recovery, reconnect and host replacement MUST rehydrate the same
pending request; they MUST NOT redispatch a prior external effect merely to
ask the question again.

#### Scenario: Host завершился во время вопроса
- **WHEN** authority restarts with a pending decision
- **THEN** observation возвращает тот же stable decision ID and request digest,
  а Run остаётся waiting до typed answer, stop или declared refusal

#### Scenario: Два клиента отвечают одновременно
- **WHEN** две valid answer commands используют одну expected Run generation
- **THEN** atomically принимается только одна, а вторая получает conflict без
  второй передачи значения worker-у

### Requirement: Uncertain obligation имеет явное owner-attested разрешение

Authority SHALL предоставлять явную команду разрешения для каждого uncertain
Attempt и CheckExecution. Резолюция MUST записывать actor, объявленный исход
(`not_applied` или `applied`), reason и ссылку на последнее наблюдение
процесса; MUST освобождать удерживаемый admission slot в той же transaction;
MUST закрывать obligation как `failed` без входа в `on_error` и без повторного
исполнения; MUST отказывать, пока foreground driver удерживает lock этого Run.
Резолюция — отдельная control operation, а не resume, cancel или retry.
Absence of a resolution MUST NOT освобождать slot по таймауту.

#### Scenario: Driver уничтожен во время шага
- **WHEN** владелец выполняет resolve для uncertain attempt после crash
- **THEN** slot освобождён, attempt закрыт как failed с объявленным исходом, а
  новый Run может быть admitted без повторного запуска старой работы

#### Scenario: Резолюция во время живого driver
- **WHEN** driver lock этого Run удерживается другим процессом
- **THEN** команда отказывает без изменения состояния

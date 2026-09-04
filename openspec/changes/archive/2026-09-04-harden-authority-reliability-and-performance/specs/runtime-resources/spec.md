Authoritative source set: `openspec/specs/runtime-resources/spec.md`
(перенесено). Compatibility path: сохранённые Runs, storage v1–v4 и
опубликованные bundles читаются прежним путём; новые формы (storage v5,
события `attempt.resolved`, `check.resolved`, `state.changed`) имеют
собственную version boundary и отказ старого reader.

## ADDED Requirements

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

## MODIFIED Requirements

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

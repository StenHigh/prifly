## Context

Ревью (см. [proposal.md](proposal.md)) измерило текущую стоимость на demo
проекте `test/e2e/verify-core.py` (169 команд, 20 Runs):

| Метрика | Сейчас | Цель этого change |
|---|---|---|
| `prifly run status` на БД 10,8 МБ | 180–250 мс | ≤ 30 мс |
| `prifly version` (без authority) | 10 мс | без изменений |
| `verify-core.py` wall / user / sys | 38,5 с / 29,4 с / 8,95 с | ≤ 10 с / ≤ 6 с / ≤ 1,5 с |
| Размер БД после `verify-core.py` | 10,8 МБ (94 % — `events.state_after`) | ≤ 2 МБ |
| Рост снапшота одного Run за 30 команд | 30,8 → 80,5 КБ, сумма копий 1,72 МБ | суммарные копии ≤ 3× финального снапшота |
| Валидация значения по схеме | fork, ~16 мс | in-process, ≤ 0,5 мс |
| `File.Sync` (APFS) | 4,0–4,4 мс; 2 на артефакт + 2 транзакции на команду | 1 на новый артефакт, 1 транзакция на команду, `synchronous=NORMAL` |
| Подпроцессы за одну итерацию `Drive` без работы и за один `load()` | ≥ 1 при decision-state или неполном кэше | 0 (проверяется тестом) |
| `go test ./internal/runtime` | 700 с | ≤ 180 с |
| Telemetry на 1000 Runs (объявленный потолок) | экстраполяция ~30 с при дедлайне 5 с | укладывается в объявленный дедлайн на fixture |

Нормативные источники: `runtime-resources`, `cli-protocol`,
`control-security-ux`, `release-distribution`, `architecture-decisions`,
`quality-and-acceptance`, `delivery-roadmap`; все перенесены в OpenSpec, delta
specs лежат в `specs/` этого change.

## Goals / Non-Goals

**Goals:**

- Владелец всегда может закрыть uncertain obligation явной аудируемой командой,
  освобождающей capacity, без повторного исполнения работы.
- Authority никогда не выдаёт собственный сбой (lock contention, диск,
  fork validator) за отказ воркера и не ведёт workflow по `on_error` из-за
  него.
- Transform под write-lock делает только сравнение и мутацию in-memory
  состояния; всё тяжёлое подготовлено до `Apply`.
- Стоимость `load`, `Drive`-итерации, `run status`, `Publish` и `Telemetry`
  ограничена размером Run, а не всей историей authority.
- Старые Runs, storage v1–v4, published bundles и historical evidence читаются
  как прежде; новые формы имеют собственную version boundary.

**Non-Goals:**

- Не менять семантику «никакой автоматический retry»: resolution только
  закрывает обязательство, новая попытка — это новый Run/fork.
- Не вводить фоновый scheduler, daemon или автоматическое пробуждение.
- Не менять YAML authoring contract, wire-формат команд и StepResult.
- Не заменять SQLite-драйвер в этом change: только spike и ADR-решение.
- Не добавлять зависимости.

## Decisions

### 1. Uncertain obligation закрывает владелец явной командой `run resolve`

`prifly run resolve RUN_ID (--attempt ID | --check ID) --outcome
not_applied|applied --reason TEXT [--command-id ID]` — новая ordinary command с
собственной control operation `run.resolve` в owner operation set (reconcile
существующего control plane добавляет её автоматически, как сейчас
`ownerOperations`). Transform: attempt/check переходит в `failed` с failure
`resolved_not_applied` либо `resolved_applied`, activation/step/invocation
получают `failed` без `routeKnownError` (аттестованный неизвестный исход — не
«известный технический failure» и не входит в `on_error`), `ReleaseSlot`
освобождает slot, `HasUnresolvedEffects` пересчитывается по оставшимся
uncertain obligations, `Status` — по `syncInvocationState`. Команда
отказывает, пока `driver.lock` удерживается для этого Run, и требует reason.
Событие `attempt.resolved`/`check.resolved` фиксирует actor, outcome, reason и
ссылку на последнее наблюдение процесса. `MarkSessionDisconnected`
дополнительно переводит attempt в `uncertain`, чтобы resolve видел единый
статус.

Отвергнуто: автоматическое освобождение slot по таймауту (противоречит
`runtime-resources`: lease не доказывает прекращение владельца) и
`run cancel` как способ разрешения (cancel не свидетельствует об исходе).

### 2. Recovery читает уже сохранённые доказательства завершения процесса

В ветке `Drive` «active» при `a.Dispatch != nil`: если journal содержит
`group_empty` (`a.ExecutorEnd != nil`), процесса и его группы больше нет —
recovery закрывает attempt как `failed(executor_settlement_lost)` с
`ReleaseSlot`, а не `uncertain`. Если `Started != nil`, но `ExecutorEnd ==
nil`, либо `Started == nil` при `Dispatch != nil` (spawn мог произойти) —
поведение прежнее: uncertain, слот удерживается до resolve. Аналогично для
check (`check_settlement_lost`). Принятие сохранённого кандидата результата
при recovery (журнал уже содержит candidate) — отдельное решение после этой
фазы: оно требует повторной валидации без владения driver lock и не входит в
change.

### 3. Сбой authority — не отказ воркера

`_busy_timeout` по умолчанию поднимается с 500 мс до 3 с (потолок `OpenStore`
5 с сохраняется). `observe`/`observeCheck` при `SQLITE_BUSY` повторяют попытку
внутри своего 2-секундного контекста. Если персистентность всё же не удалась,
процесс останавливается (правило «нет ненаблюдаемых effects» сохраняется), но
settlement получает код `authority_persistence_failed`, который
`routeKnownError` не считает известным technical failure: workflow не идёт по
`on_error`, а Run остаётся с видимым инцидентом authority. `settleWith`
сохраняет исходную ошибку `resultOutputs`/`decode`/`ValidateJSON` в
диагностике (bounded `Detail`) и отличает infrastructure-класс
(`local.ErrIntegrity`, `os.PathError`, `ErrBlobLimit`, storage budget) от
worker-класса.

### 4. Идемпотентность retry по `--command-id` без часов в payload

`Waive` убирает `expires_at` из payload команды (срок считается в transform по
`obs`); артефакт голоса approval не содержит `ObservedAt` — время фиксируется
в самом Approval record при коммите. Правило закрепляется тестом: каждая
ordinary/authority команда, повторённая с тем же id и логически тем же
запросом, возвращает `Duplicate: true`.

### 5. Transform — только сравнение и мутация

Всё, что читает файлы, компилирует или форкает, выполняется до `Apply`, а в
transform передаются результаты и их digests для сравнения с текущим
состоянием (образец — `settleWith` и `admit`). Затронуты
`actionProposalAttempt` (actions.go), `acceptance.passed` для
`workflow_input`, `acceptArtifactPublicationChecks`, `DecideControlApproval`.
`chargeInvocation` перестаёт вызывать `supportedRun` и заново парсить
workflow на каждом уровне lineage: лимиты lineage считаются один раз на
команду и передаются явно. Бюджеты публикаций считаются по накопительному
счётчику байт в состоянии, а не повторной канонизацией всех публикаций.
`recordCommand` складывает samples в `Change.Samples` той же транзакции;
`AppendSamples` использует `MAX(seq)` вместо `count(*)` и prepared statements.
В тестовой сборке (`testing.Testing()`) обращение к `runSchemaWorker`,
`readLocal`, `BlobStore.*`, `exec.Command` из-под активного transform
проваливает тест через счётчик `local.transformDepth`.

### 6. Схемы валидируются in-process

`santhosh-tekuri/jsonschema/v6` уже подключён и по умолчанию использует
`regexp` Go (RE2, линейное время), поэтому ReDoS-довод в пользу подпроцесса
не действует для валидации значений. Компилированная схема кэшируется в
процессе по digest bytes (LRU ≤ 64 записей / 8 МиБ, как сейчас
`schemaCompileCache`); компиляция ограничена `MaxDocumentBytes` и
`noExternalSchema`. Подпроцесс сохраняется одну версию за флагом
`PRIFLY_SCHEMA_WORKER=1` как fallback и удаляется после релиза с evidence.
Валидация сохранённых публикаций при Telemetry не выполняется повторно: они
были проверены при приёме, telemetry сверяет только digest.

### 7. Открытие authority — bounded, полная проверка — по требованию

`authority` получает столбец `verified_cut` (storage version 5, миграция при
остановленных admissions). `OpenStore` проверяет заголовок, идентичность и
`quick_check` только при `verified_cut == 0`, затем верифицирует события с
`cut > verified_cut` и сдвигает отметку. Полный скан остаётся в
`Store.Verify` и вызывается `doctor`. Это уточнение требования «при открытии
authority MUST проверить storage/schema/integrity»: проверка становится
инкрементальной, а не ослабленной — каждое событие проверено ровно один раз.

### 8. Checkpoint не дублирует неизменяемые байты и журнал переходов

Storage v5 добавляет таблицу `pinned_bytes(digest PRIMARY KEY, bytes)`.
Store при записи снапшота заменяет `definitions[].bytes`,
`context_resources[].bytes` и `attempts[].envelope` ссылкой по digest, при
чтении восстанавливает их и проверяет прежний `snapshot_digest` — логическая
форма Run и его digest не меняются, версия state не растёт, старые
снапшоты читаются без преобразования. `Transitions` переезжают в события
`state.changed` (новая state version), `Timing` и `check_telemetry` читают
переходы из journal через `Store.Read`; старые Runs продолжают читать поле
снапшота. Цель — рост снапшота на команду в пределах нескольких сотен байт.

### 9. Fsync-политика

WAL + `synchronous=NORMAL` (SQLite гарантирует консистентность при падении
процесса; потеря последних транзакций возможна только при потере питания —
это записывается в failure model). `putArtifact` проверяет существующую
identity один раз и не делает `dir.Sync()` на дубликате; `release.replace`
добавляет fsync родительского каталога.

### 10. Горячие циклы

`Engine.plans` — кэш `*flow.Plan` по `(WorkflowRef, resource snapshot digest)`
(планы неизменяемы). Watchdog драйвера и check делят одну реализацию и
опрашивают `SELECT version, event_seq FROM runs` каждые 250 мс, вызывая `load`
только при изменении версии. Linux `readProcessGroup` использует `bytes.*` и
адаптивный интервал (20 мс первые 200 мс, затем 200 мс).
`ProcessExecutableDigest` кэшируется по `(dev, ino, size, mtime, ctime)` на
время процесса; повторный хеш перед exec остаётся при несовпадении stat.
`load` строит индексы (activations по invocation, invocations по caller,
transitions по (kind,id), checks по (invocation, activation)) один раз и
использует их в инвариантах, `Timing`, `stepReadView`, `guardTargetOpen`;
`contextPinnedInvariant` кэширует результат по digest снапшота на время
процесса. `Artifact` и `inventoryResources` получают кэш на время одной CLI
команды/Engine (артефакты неизменяемы по digest).

### 11. Один ранг версий и типизированные ошибки

`stateRank(version) int` и `atLeast(version, minimum)` заменяют 24 предиката;
таблица `versionContracts[rank]` даёт read/next/preview/step-read version;
`Start` выбирает `max` требуемых рангов. Строковая конвенция `"code: msg"`
заменяется типом `runtime.Fault{Code, Message}`, `ProblemFor` перестаёт
разбирать `err.Error()`, CI получает `rg`-ворота на `errors.New("[a-z_]+:`.
`internal/local` экспортирует `IsPersistenceFailure(error)`; runtime не
импортирует `go-sqlite3`; `CGO_ENABLED=0 go vet ./...` входит в `ci-check`.
Spike `modernc.org/sqlite` оформляется отдельным ADR с измерениями, а не
частью этого change.

### 12. Поставка

`install.sh` скачивает `release-manifest.json` из того же Release и сверяет
SHA-256 архива до установки; trust boundary остаётся HTTPS GitHub, что явно
записано. Подпись manifest считается по RFC 8785 каноническим bytes
(`json-canonicalization` уже в зависимостях): два release публикуют и
legacy `release-manifest.sig`, и `release-manifest.jcs.sig`; новый updater
проверяет JCS, старый — legacy; после переходного периода legacy удаляется.

## Risks / Trade-offs

- [In-process валидация недоверенных схем] → RE2 исключает катастрофический
  backtracking; размер схемы/значения ограничен 2 МиБ; fallback-флаг на один
  release; fuzz-тест валидатора на схемах из corpus.
- [`synchronous=NORMAL` теряет последние транзакции при потере питания] →
  записано в failure model; при падении процесса гарантия прежняя; receipts
  остаются идемпотентными, retry безопасен.
- [Storage v5 миграция на больших БД] → выполняется при открытии на запись с
  остановленными admissions, идемпотентна, имеет тест на прерывание; read-only
  открытие старой БД не мигрирует.
- [Resolve используется как «force success»] → outcome только `not_applied`
  или `applied`, статус всегда `failed`, `on_error` не входит; аудит в
  journal; отдельная control operation.
- [Переход подписи manifest] → двойная подпись два release; `verify-release-ci.py`
  проверяет наличие обеих; тест на старый updater против нового manifest.
- [Индексы и кэши расходятся с состоянием] → индексы строятся из загруженного
  снапшота в той же функции и не переживают его; кэши только для immutable
  данных по digest.

## Migration Plan

1. Фаза 1–2 (обязательства, транзакции) — один minor release: новые event
   types, `run resolve`, busy timeout, transform guard; storage/state
   versions не меняются.
2. Фаза 3–4 (валидатор, хранилище) — следующий minor release: storage
   version 5 c миграцией, state version для `state.changed`, fsync-политика;
   rollback — предыдущий binary читает v5 только в read-only после отказа,
   миграция обратима copy-and-replay через backup, как требует
   `runtime-resources`.
3. Фаза 5–6 (циклы, архитектура кода) — без изменения контрактов, поэтапно.
4. Фаза 7–8 (поставка, ворота) — параллельно с любой фазой; двойная подпись
   manifest начинается с первого release этого change.

Каждая фаза закрывается собственным evidence в `tasks.md` без заявления о
закрытии roadmap milestone или product gate.

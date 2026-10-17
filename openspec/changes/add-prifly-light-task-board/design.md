## Context

Мотивация и границы — [proposal.md](proposal.md). Проверенный baseline:

- `cmd/prifly/monitor.go`: `--scan-root`/`--addr`, один embedded frontend; `monitorMux` регистрирует также maintenance endpoints. Light нельзя получить сменой discovery.
- `monitor_list.go`: страница 50 Runs из process catalog, full board группирует состояния Runs. `monitor_catalog.go` обновляет summaries; каждый `MonitorSummary` читает свой cut. Объединение этих summaries с отдельно прочитанной queue не даёт согласованного source response.
- `internal/runtime/task_queue.go`: native `QueueTask`, `Associations`, durable intents, immutable `Run.QueueTask`. Содержание очереди хранится одним authority document; существующий queue list не является bounded Light API.
- `internal/local/monitor_read.go` даёт один Run/admission cut; `ReadAuthorityRuns` — ledger и selected Runs в одной SQLite read transaction. Это исходные primitives, не готовый paginated task/review reader.
- W1/W2/W5 typed schemas/graphs прочитаны на consumer SHA `3499115956903abd264fbde2be2f432d07bc2c43`. Report содержит candidate/round/request digest, а gate не несёт их напрямую: связь восстанавливается только из сохранённых typed inputs/outputs точного producer invocation. Main-schema `internal/flow/protocol.schema.json` и published-contracts запрещают расширение старых closed bundles.

## Goals / Non-Goals

**Goals:** законченный отдельный первый экран, проверяемая read projection и ограниченная стоимость обновления. UI может показывать несколько текущих Runs/ветвей и несколько review statuses, не выдумывая единственный current Run. Конкретные решения — [read-contract.md](read-contract.md), макет — [ui-sketch.md](ui-sketch.md).

**Non-Goals:** новые состояния исполнения, авторизация управления, исполнение producer/adapter кода из браузера, поиск отчётов по диску, самостоятельная multi-user auth, изменение aif packages. Фаза интерфейса не управляет DAG. «Готово» не обещает merge/deploy; native queue complete остаётся явным owner решением.

## Decisions

### 1. Явный режим на отдельном listener

Предлагаемый CLI: `monitor --ui full|light` (default `full`), для Light — `--full-monitor-url`, `--light-projection`, `--base-path`. Синтаксис и routing зафиксированы в CLI delta. Это будущие параметры, их нет в baseline 0.13.79. Light использует отдельные embedded HTML/CSS/JS и allowlist mux. Full mode продолжает обслуживать прежние bytes/routes/health и auto-start на обычном адресе. Light health имеет отдельный service identity, поэтому `monitorReady` не принимает его за full monitor.

Light регистрирует только GET/HEAD health, assets, board/task/review reads. Full maintenance, storage administration, queue mutations и любые command handlers туда не подключаются. Общие Host/loopback/read-access guards переиспользуются. `OPTIONS`, POST/PUT/PATCH/DELETE, неожиданные routes и HTTP-пути к новым authorities не открывают mutation. Отчёты выводятся текстом; произвольные URLs evidence не запрашиваются автоматически.

Альтернатива: спрятать несколько панелей нынешнего frontend. Она оставляет тяжёлую загрузку и maintenance surface, не выполняет E6-A1/A7/A9. Отдельная команда/binary потребовала бы дублировать конфигурацию и поставку; mode достаточно при отдельных mux/assets.

### 2. Task grouping без новой очереди

Ключ native карточки — `(authority_id, project_id, task_id)`. Канонический Run binding и intended Run ID учитываются при отставшем queue index после crash; disagreement — integrity error, без repair из read endpoint. Все recorded associations сохраняются, независимо от title. QueueTask revision и Run binding revision показываются отдельно.

Standalone Run получает ключ `(authority_id, run_id)` и подпись fallback. Recorded forks для него — только ссылки, без автоматического объединения. Удалённый source Run, отсутствующая queue запись и conflicting bindings отображаются как неполная история. Не переписываем source partial после успешного continue.

У native task может быть несколько frontier Runs; frontier вычисляется только по recorded ancestry внутри её associations. Без доказанной связи несколько Runs остаются одновременными источниками, timestamp не выбирает победителя. Terminal task с owner `done` обозначает локальное завершение; terminal succeeded Run при claimed task означает «Run успешен, завершение задачи не подтверждено».

Альтернатива: одна карточка на Run либо группировка по subject. Первая дублирует работу после continue, вторая теряет identity. Ни одна не используется.

### 3. Декларативный mapping вне workflow authoring

Внешний local Light projection file — versioned bounded JSON, загружается и валидируется при старте, фиксируется digest в ответах. Он содержит только exact workflow/schema refs, stage/port selectors, JSON pointers, literal labels и конечные таблицы enum значений. Скриптов, regex по логам, сетевых вызовов и загрузки файлов из Run нет. Изменение файла требует перезапуска; смешение двух mapping digests в одном response запрещено.

Mapping связывает UI phase с exact pinned definition/stage identity и review lens с declared report/gate producers/ports, candidate/request inputs и invocation/iteration relation. Resolution работает только на sealed Run closure и записанных ArtifactRefs, поддерживает старые Runs без изменения state. При отсутствии mapping generic карточка остаётся полезной: source state, активные стадии, причины и «В работе / этап не сопоставлен»; review unknown, round null. Core не содержит AI Factory ветвлений по namespace. Optional mappings/fixtures с namespace живут как данные в examples/test fixtures Core, не в runtime policy.

Отдельно: generic `ProjectionManifest` уже имеет значение data projection. Новый glossary термин «Light projection mapping» не переименовывает его. Конкретные Go bindings добавляются в словарь только вместе с кодом.

Альтернатива: новое authoring поле/новая WorkflowRevision. Для визуализации существующих immutable Runs оно потребовало бы consumer changes и нового seal, поэтому v1 использует observer configuration. Заявление session/model показывается только как producer claim.

### 4. Согласованный read path и process index

Нужен новый driver-neutral runtime reader с реализацией SQLite: queue document/version, Run metadata/bindings, admission facts и selected snapshots читаются на одном фиксированном authority cut. Нельзя просто вызвать queue list и `MonitorSummary` для каждой карточки. SQL/driver типы остаются в `internal/local`; runtime получает собственные DTO. Запросы read-only, без storage migration/queue init/receipts; transform-команды не затрагиваются.

Light использует отдельный process read index. Index — восстановимая копия фактов, не authoritative state, в v1 без записи рядом с consumer. Для каждого source фиксируется cut в одной read-only SQLite snapshot transaction; её metadata pages и queue/admission факты не перечитываются из новых transactions под прежним cut. Bounded batches ограничивают размер обработки, а не разрешают менять снимок между страницами. Changed snapshots обрабатываются streaming по одному внутри той же read-only transaction; typed immutable artifacts читаются по закреплённым refs и проверяются digest. Snapshot bytes/graphs не удерживаются в index. Сначала в bounded batches собираются task identities/Run bindings и компактные summary rows этого cut, затем публикуется immutable source generation. Промежуточная индексация явно incomplete; complete total до окончания не обещается. Предыдущая complete generation остаётся доступной до атомарной замены новой; наличие более свежего cut отмечается как updating. Admission-only/queue-only изменения тоже инвалидируют generation, не только RunVersion.

При cold start полный проход metadata неизбежен. В рамках изменившейся queue version также требуется однократное чтение/декодирование monolithic queue document; это измеряемое ограничение v1, не обещание O(page) для любых mutations. Queue projection кэшируется по version. Run revision metadata перечисляются bounded pages; snapshots декодируются для изменённых Runs, typed report/gate bodies — только для объявленных refs, по digest и с byte budget. Никаких графов, traces или artifact bodies в HTTP board response. Полный storage sizing/scan не стартует в Light. При превышении работы index продолжает фоном с явной неполнотой, ответы не висят.

Board pagination фиксирует vector source generations в короткоживущем page token; внутри каждого source один cut. Уже cached summaries для older cut нельзя пометить новым cut без проверки версии и admitted/resources фактов. Детали другого cut приходят целиком с собственной отметкой и не патчат старую карточку. TTL page generations 5 минут, максимум 8 retained generations; eviction/expiry возвращает `light_page_expired`, UI предлагает обновление с сохранёнными фильтрами. Это не глобальный cut и не полномочие чтения.

Альтернатива: весь `Engine.Runs()`/`MonitorView` при каждом poll. Она не обеспечивает budgets и coherent task snapshot. Новая durable per-task queue storage migration была бы шире E6; допускается только отдельным change, если измерения опровергнут v1 budgets.

### 4.1. Read verification без долгоживущего Engine

Измерение actual browser выявило повторную проверку уже прочитанного журнала при каждом short-lived Open/access. Для Light введён opaque process-local `ReadVerification`: exact database directory/inode, authority ID, epoch, storage version и проверенный prefix cut. Только успешный read-only open выдаёт proof. Следующий Light open проверяет текущие headers/ownership/control, использует прежний immutable prefix и проверяет новые записи. Proof не сериализуется, не пишет durable verified marker, не передаётся на другую database и не даёт доступа. Default Open/full monitor сохраняют прежний verification path. Engine/installation lock закрываются после каждого обращения, cleanup не блокируется idle listener. Tests проверяют другой inode/directory и corrupt suffix.

Индекс держит compact grouping/Run/review summaries и resource candidates. Graphs, definitions, envelopes и предыдущие Attempts освобождаются после streaming projection; artifact cache ограничен 32 MiB/8192 entries, один summary pass — 16 MiB typed bodies, один artifact — 4 MiB. Превышение отображается unavailable с причиной; board не переносит raw bodies.

### 5. Evidence определяет review, gate определяет приёмку

Review row хранит applicability, report check state, gate state и freshness раздельно. У `passed` есть строгие условия: enabled lens, валидный fresh report с той же request/candidate/round и подтверждённая принимающая gate связь. Blocked gate остаётся блокировкой при `StepResult pass` и при passed check; invalid/missing report никогда не зелёный. Gate scope (`lens` или `aggregate`) виден; aggregate blocked не перекрашивает доказанно успешную ветвь в defect, но не разрешает «всё принято».

После записанного repair/candidate change прежние reports становятся historical/stale. До появления нового recorded candidate также ставится invalidated, а не сохраняется PASS. Состояние живых файлов и independence моделей не проверяются. Правила enum и W1/W2/W5 data paths — в read contract. Findings сокращаются с признаком truncation и точными источниками; полный отчёт открывается по explicit action и access check.

### 6. Budgets входят в приёмку

На Linux qualification fixture: не менее 1000 native tasks, 1000 associated/standalone Runs, несколько authorities, ≥1000 событий у части Runs, declared review reports и один недоступный source. Записываются CPU/OS/RAM, binary SHA, данные, cold/warm условия и инструмент измерения.

| Измерение | Начальный обязательный budget |
|---|---|
| Board page | default 25, max 50 карточек; wire JSON ≤128 KiB на default page, без compression |
| Card summary | ≤4 KiB; до 4 review chips и 3 findings previews суммарно, text previews ≤160 символов; дополнительные lens/ветви имеют counts и lazy pages |
| Начальная загрузка браузера | ≤6 requests до первого board render; HTML/CSS/JS суммарно ≤250 KiB gzip |
| Warm board API | p95 ≤250 ms локально, 100 запросов; без повторных snapshot/report decodes на unchanged data |
| Cold fixture index | complete ≤5 s для указанной fixture на зафиксированном стенде; loading/incomplete явно видны, HTTP не ждёт полного обхода |
| Poll | 2 s foreground, ≤1 in-flight request; один conditional board request за tick, unchanged 304 без body |
| Background tab/error | 15 s hidden; backoff 5–30 s при ошибке с видимой свежестью |
| Backend unchanged | 0 Run snapshot decodes, 0 report body reads и 0 filesystem tree scans; лёгкая проверка registry/source generation допустима |
| One changed Run | ≤1 decode его новой snapshot для поколения, без decodes остальных 999; reports cache по digest, queue decode только при queue version change |

Счётчики backend работы тестовые/read diagnostics, не повод менять public Run contracts. Response totals/filter counters считаются по одному index generation, `complete=false` и `total=null` обозначают ещё неизвестный итог. Если budget не достигается, задача A7 остаётся открытой; молчаливое повышение порога или переименование режима не заменяет приёмку.

## Risks / Trade-offs

- [Gate без candidate fields] → доказываем связь через accepted input/output refs producer, иначе unknown; не присваиваем ему candidate по времени.
- [Большая queue переписывает один документ] → decode один раз на version, измеряем отдельно queue churn; durable storage redesign не маскируется как UI работа.
- [Неполный индекс или denied source] → source coverage/error/freshness отдельно, last good data помечены stale, totals имеют область и полноту.
- [Snapshot удерживается слишком долго] → bounded batches/retention и отказ expired token; без write transaction, cancellation и timeout, измеряем WAL/reader lifetime.
- [Частые generations] → ограничиваем retained memory и явно истекаем page token; не обещаем бесконечный historical read.
- [Внедрение под proxy prefix] → один нормализованный base path для assets/API, same-origin relative fetch; full deep link использует отдельный URL и URLSearchParams.
- [UI принят локально, но не в production] → A9 released replay и A10 server rollout остаются отдельными задачами; issue не закрывается synthetic fixture.

## Migration Plan

1. Apply создаёт новые schemas/read DTO, отдельный Light mux/assets, observer mapping examples и проверки. Existing state/read bundles, E1/E2 evidence и consumer workflows остаются byte-identical.
2. Обновить glossary/source map, README и author index/guide в том же implementation change; version/revision указывать по реально выпущенному контракту.
3. Проверить local build, browser и prefix proxy; итоговые ворота перед commit — `make ci-check-once CHECK_CONTEXT=<actual-profile>`. Перед release exact-SHA verify/qualify по правилам AGENTS, затем released replay A9.
4. После готового release владелец отдельно разворачивает Light listener за существующей authentication proxy. Production domain — deployment parameter. Конкретные ожидаемые hosts и evidence — [acceptance.md](acceptance.md).
5. Rollback: убрать только Light proxy mapping/service, сохранить полный monitor, authority и очередь; бинарник с Light не требует Run/storage migration. Не возвращать старый binary на authority с независимой более новой storage version без её compatibility проверки.

На этапе подготовки change никакой listener/domain/service не запускается и не переключается. Способ выполнения следующего apply — обычный OpenSpec без Run, согласно явному указанию владельца в этом запросе.

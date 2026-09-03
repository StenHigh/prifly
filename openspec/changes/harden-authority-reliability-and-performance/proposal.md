## Why

Полное ревью кода (2026-09-03, два прохода по `internal/local`,
`internal/runtime`, `internal/flow`, `cmd/prifly`, `internal/release`,
`scripts/install.sh`) показало три класса проблем, которые противоречат
главному обещанию продукта — «неопределённый исход не повторяем, но и не
теряем управление»:

1. **Корректность.** Run в состоянии `uncertain` навсегда удерживает
   admission slot: ни одна команда не сбрасывает `HasUnresolvedEffects`, а
   `run cancel` на таком Run принимается и тут же завершается
   `recovery_required`. Один `kill -9` драйвера во время шага делает authority
   с capacity 1 непригодной. Recovery игнорирует уже сохранённые
   `group_empty`/exit code и объявляет доказанно завершённый процесс
   неопределённым. Сбой персистентности драйвера (`SQLITE_BUSY` при
   `_busy_timeout=500ms`) записывается как отказ воркера и ведёт workflow по
   `on_error`. Причины `invalid_output` теряются. `run waive` и
   `approval decide` неидемпотентны при повторе с тем же `--command-id`.
2. **Транзакционная дисциплина.** Контракт `Store.Apply` («transform must be
   pure») нарушается: под `BEGIN IMMEDIATE` компилируются планы, форкаются
   процессы валидации схем, читаются файлы артефактов, канонизируются все
   публикации run'а; после каждой команды выполняется вторая write-транзакция
   телеметрии.
3. **Производительность и память.** Полная верификация БД при каждом
   открытии (180 мс на 10 МБ, линейно), квадратичный рост снапшотов
   (94 % БД — копии состояния, 51 % каждой копии — неизменяемые байты и
   журнал переходов, который уже есть в events), fork подпроцесса на каждую
   валидацию значения (16 мс), опросы 20/30 мс с полной перезагрузкой
   состояния, SHA-256 исполняемого файла на каждый dispatch, ~4 мс fsync
   на каждый артефакт и коммит, O(узлов × transitions) в `Timing`,
   O(I² + A·I) инварианты при каждом чтении, отсутствие кэша планов и
   артефактов, telemetry, не укладывающаяся в собственный дедлайн на
   собственном потолке.

Отдельно: семь копий «версионной лестницы» state/read contracts, 24
цепочечных предиката `isXState`, 158 строковых кодов ошибок, сквозной
импорт SQLite-драйвера в runtime (блокирует `CGO_ENABLED=0`), первичная
установка без проверки digest, подпись release manifest в Go-специфичной
форме.

## What Changes

Change затрагивает **product runtime** и публичные CLI/контракты, а не только
процесс репозитория. Работа разбита на восемь фаз в порядке приоритета; каждая
фаза поставляется отдельной серией коммитов с зелёным `make ci-check` и может
быть выделена в собственный capability-sized change при исполнении.

1. **Обязательства и recovery.** Новая явная команда владельца
   `prifly run resolve` для uncertain attempt/check: аттестует исход,
   освобождает slot, не повторяет работу и не входит в `on_error`. Recovery
   использует сохранённые доказательства завершения процесса. `SQLITE_BUSY`
   и другие сбои authority классифицируются как сбой authority, не воркера.
   Причина отказа результата сохраняется в диагностике. Все команды с
   `--command-id` идемпотентны. Детерминированный выбор builtin-схемы
   агрегата.
2. **Чистые транзакции.** Компиляция, валидация схем и чтение файлов
   выносятся из transform'ов; инварианты и бюджеты считаются один раз на
   команду; телеметрия команды пишется в той же транзакции; в тестовой сборке
   нечистый transform проваливает тест.
3. **Валидация схем in-process** на уже подключённом `jsonschema/v6` с
   кэшем скомпилированных схем; подпроцесс остаётся только как явный fallback
   на один release.
4. **Хранилище.** Открытие authority проверяет заголовок и инкрементально
   верифицирует историю от последнего проверенного cut; полная верификация —
   по `doctor`. Неизменяемые pinned bytes хранятся один раз по digest;
   переходы состояний переезжают в журнал событий; WAL с `synchronous=NORMAL`
   и один fsync-путь на артефакт.
5. **Горячие циклы драйвера.** Кэш планов, дешёвый опрос версии вместо полной
   загрузки, адаптивный опрос процесса, кэш digest исполняемых файлов,
   индексы вместо линейных сканов в `Timing`, `stepReadView`,
   `recordDiagnostic`, `Publish`, `FireDueSlots`; кэш `Artifact` и
   `inventoryResources` на время команды.
6. **Архитектура кода.** Один порядковый ранг версий state contract вместо 24
   предикатов и семи копий лестницы; типизированные ошибки вместо строковой
   конвенции; `local.IsPersistenceFailure` вместо импорта `go-sqlite3` в
   runtime; таблица команд CLI; удаление дублей и мёртвых констант.
7. **Безопасность.** Проверка digest архива при первичной установке; подпись
   manifest по RFC 8785 с переходным двойным signing; fsync каталога при
   замене binary; проверка `Host` и очистка ошибок в monitor;
   `--end-of-options` везде; строгая грамматика идентификаторов в receipts.
8. **Ворота качества.** Бенчмарки горячих путей на фиксированной fixture БД,
   регрессионные тесты на каждый найденный дефект, целевое время
   `make test`, обновление словаря и roadmap.

## Capabilities

### New Capabilities

_Нет._

### Modified Capabilities

- `runtime-resources`: явное owner-attested разрешение uncertain obligation с
  освобождением slot; recovery по сохранённым доказательствам завершения;
  bounded incremental integrity check при открытии; чистота transform и
  идемпотентность retry; дедупликация pinned bytes в checkpoint; telemetry
  limits, достижимые в объявленный дедлайн.
- `cli-protocol`: команда `run resolve` и её diagnostics.
- `control-security-ux`: resolution — отдельная аудируемая команда, не resume;
  read-only monitor проверяет `Host` и не раскрывает внутренние ошибки.
- `release-distribution`: bootstrap сверяет digest архива с опубликованным
  manifest; подпись manifest внешне проверяема (RFC 8785) с переходным
  периодом.
- `architecture-decisions`: версии state contract образуют один упорядоченный
  ряд; persistence layer не протекает в runtime.
- `quality-and-acceptance`: benchmark gate горячих путей на fixture БД.
- `delivery-roadmap`: фазы этого change входят в active backlog.

Ownership normative source ни одной capability не меняется: все source sets
уже перенесены в OpenSpec.

## Impact

Затрагиваются `internal/local` (storage version 5, busy timeout, transform
guard), `internal/runtime` (resolve/recovery, кэши, индексы, версии,
ошибки), `internal/flow` (in-process validator), `cmd/prifly` (`run resolve`,
таблица команд, monitor), `internal/release` и `scripts/install.sh`,
generated public schemas (новые event types `attempt.resolved`,
`check.resolved`, `state.changed`), словарь, tests, benchmarks и OpenSpec.
Сохранённые Runs, historical evidence и опубликованные bundles не
переписываются: новые формы получают новые storage/state version boundary, а
старые читаются прежним путём. Новых зависимостей не требуется; `jsonschema/v6`
уже подключён. Работа войдёт в следующие minor releases по фазам; фазы 1–3
образуют первый release candidate этого change.

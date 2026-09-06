Проверки адресные по затронутым packages, с записью команд и счётчиков. Дорогие
ворота один раз на собранном кандидате.

## 1. Время называет, что измеряет

- [x] 1.1 Установить фактом, что измеряет `elapsed` у `run`, `step_instance` и
      `attempt`, и почему у попытки выходит ноль при семнадцати минутах работы.
      Записать места в коде. Гипотезу про управляющий контур проверить, а не
      принять.
- [x] 1.2 Сделать так, чтобы величина рядом с узлом относилась к работе этого
      узла либо называла, к чему относится. Форму выбрать по факту 1.1 и
      обосновать.
- [x] 1.3 Длительность работы исполнителя доступна готовой у своего узла.
      Проверка: потребителю не нужно вычитать два наблюдения и знать, что
      вычитать следует по `utc`, а не по монотонике.
- [x] 1.4 Признак качества различает «измерено и мало» и «измеряется не то».
- [x] 1.5 Если форма контракта изменилась — версия `core-timing` повышена до
      конца, прежние бандлы не сдвинулись; `make schemas-check` зелёный.

Факт 1.1: `elapsed` у любого узла — это `created`/`admitted` → `settled` с
`calendar=true` (`internal/runtime/timing.go:440`). Через границу процесса
`clockComparable` ложно (`timing.go:108`), измеряются только отрезки внутри одной
сессии часов, поэтому у попытки остаётся управляющий префикс ≈0 мс. Гипотеза
пилота подтверждена наполовину: это не «время управляющего контура» по замыслу,
а деградировавший календарный `elapsed`, у которого оценка по `utc` была закрыта
требованием `UTCTrust == "trusted"` (`timing.go:127`), недостижимым для местных
часов (`model.go:223`).

Форма 1.2–1.4: новая величина не заводилась. Календарный интервал в новейшем
калькуляторе публикует `estimate_ms` по настенным часам самой authority — той же
разностью, которой `consumeSessionTime` уже списывает allowance
(`session_timing.go:114`), — с причиной `authority_wall_estimate`, и никогда как
`measured`. `renderTiming` ведёт строкой ту величину, которая покрывает весь
интервал, и называет часы: `~1062000ms by wall clock` вместо `known 0ms`.

Версия 1.5: `core-timing/2` → `core-timing/3` (`timing.go:17`), поведение
изменено только под `isContextState`, поэтому `foundation-timing/1` и
`core-timing/1` выдают прежние числа. Обновлены `test/e2e/verify-context.py:365`
и `scripts/verify-context-upgrade.py:414`. Формы DTO не менялись: бандлы не
сдвинулись.

Проверки: `go test ./internal/runtime ./cmd/prifly -count=1` — ok (218.9s /
30.7s); адресные `TestAssistedAttemptPublishesItsWorkingWindowWithoutSubtraction`,
`TestTimingQualityOverflowAndTrustedEstimate`,
`TestTimingRetainsPiecesAcrossCommandClockSessions`,
`TestTimingSuspendAndSerializedWallTimestamp`,
`TestTimingRecoveryGapKeepsKnownSegments`,
`TestCheckTimingLegacyVersionsIgnoreNewFields`,
`TestRenderTimingNamesTheClockBehindEachNumber` — PASS; `make schemas-check` —
зелёный; `gofmt -l ./cmd ./internal` — пусто.

## 2. Коды возврата

- [x] 2.1 Вывести фактическую таблицу кодов из `exitForCode`
      (`internal/runtime/problem.go:46`): какой код при каких обстоятельствах,
      что стабильный контракт, а что деталь реализации.
- [x] 2.2 Объяснить и записать, почему `run drive` вернул 5 на здоровом
      прогоне.
- [x] 2.3 Описать коды средствами самого инструмента, разделив исправное
      ожидание и отказ. Проверка: скрипт отличает одно от другого, не разбирая
      текст вывода.

Факт 2.1: классификаторов три, не один. `*flow.Problem` (все `usageError`):
`unsupported*` → 5, иначе 2 (`problem.go:114`). `*local.Rejection`: `*forbidden*`
→ 4, `*exhausted*`/`*busy*`/`unsupported*` → 5, иначе 3 (`problem.go:120`).
Остальное (в том числе `runtime.Fault`) — через `exitForCode`
(`problem.go:46`): `unsupported*` → 5, `*conflict*`/`*drift*` → 3, `*recovery*`
→ 6, иначе 2. Типизированные часовые: `context.Canceled` → 7,
`DeadlineExceeded` → 5, `ErrRecoveryRequired`/`ErrIncompatible`/`ErrIntegrity`
и `persistenceFailure` → 6, `ErrReadOnly`/`os.ErrPermission` → 4,
`ErrBlobLimit`/`ErrSampleLimit` → 5, `ErrCommandConflict` → 3. Код 1 не выдаёт
ни одна команда — он принадлежит приватному режиму `flow.SchemaWorker`
(`schema_worker.go:54`), поэтому в справку не внесён.

Стабильны классы, а не маршрут: `budget_exhausted` выходит 2, когда он написан
как `fault(...)` (`driver.go:363`), и 5, когда как `local.Reject(...)`
(`invocation.go:398,412`). Один код, два статуса. Не чинил молча: описание
кодов делает их контрактом, переназначение — решение владельца. **Вынесено
владельцу.**

Факт 2.2: конкретную пятёрку пилота к одному пути привязать нечем — в отчёте
нет ни `code`, ни конверта, а 5 выдают десять разных отказов. Установлено то,
что нужно их скрипту: **ожидание — это 0, а не 5**. `run drive` на
ассистируемой передаче отдаёт конверт и возвращает `nil` (`driver.go:165`),
`nextKind` такую попытку пропускает (`engine.go:456`); это уже закреплено
тестами репозитория (`cmd/prifly/project_mixed_test.go:241,261,286`,
`project_variants_test.go:243`), которые падают на любом ненулевом коде. Значит
их 5 был отказом после допуска, а не передачей. Достижимые из `Drive` пятёрки:
`budget_exhausted`, `state_budget_exhausted` (`engine.go:214`),
`storage_budget_exhausted` (`local/store_usage.go:46`),
`wait_registrations_exhausted` (`wait.go:490`), `unsupported_check_executor`,
`unsupported_evidence` (`driver.go:1315`), `quota_exceeded`,
`deadline_exceeded`.

Форма 2.3: тема `exit-codes` в собственной справке — `prifly help exit-codes` и
`prifly exit-codes` (`main.go`, запись справки). Ожидание и отказ разделены
первой же строкой: 0 — команда выполнена, включая исправное ожидание; любой
ненулевой — отказ с конвертом Problem на stderr. Скрипту разбирать текст не
нужно: класс читается из `$?`, точная причина — из `code` конверта. Тест
`TestExitCodesAreDescribedByTheToolAndMatchTheEngine` держит описание и
`ProblemFor` вместе — описание не может разойтись с движком.

## 3. Отказ по форме команды

- [x] 3.1 `session task RUN_ID` без `--run` называет недостающий флаг и
      показывает принимаемую форму.
- [x] 3.2 Найти и привести к образцу остальные отказы по форме, говорящие общую
      фразу. Перечислить проверенные и намеренно не тронутые.
- [x] 3.3 `safe_next_actions` для отказа по форме относится к этому отказу.

Форма 3.1: `parse()` называет полученное и принимаемое
(`main.go:147-155`), а принимаемую форму читает из того же текста справки
(`acceptedForm`, `main.go:235`), поэтому отказ и справка не могут разойтись.
`session task run:1` → `session task received unexpected arguments "run:1"; the
accepted form is session task --run RUN_ID`.

Отчёт 3.2. Приведены к образцу «назвать полученное, затем принимаемое» все
групповые отказы: `action`, `approval`, `artifact`, `capacity`, `claim`,
`control`, `grant`, `package`, `project`, `run`, `session`. Список принимаемых
операций больше не пишется в коде дважды — `operationForms` (`main.go`) читает
его из той же справки, что печатает `prifly help GROUP`. Это вскрыло две
разошедшиеся копии: `claim` предлагал три операции из пяти (без `create-set` и
`heartbeat`), `package` терял `trust-root`. Ещё две операции работали, но не
были описаны нигде — `run stop` и `project extend`; обе внесены в справку, иначе
корректного перечня не существует.

Отдельно исправлены отказы, отвечавшие не на тот вопрос:
`run show run:1` больше не рассказывает про «automatic retry and terminal
reopening» (`main.go`, ветка неизвестной операции); `run bogus` без RUN_ID
называет неизвестную операцию, а не отсутствующий идентификатор; `action bogus`
и `artifact bogus` проверяются до чтения флагов — прежде им отвечали
`requires --file COMMAND.json` и `requires --ref FILE`, о которых никто не
спрашивал.

Намеренно не тронуты: отказы по недостающим флагам конкретной операции
(`grant issue requires ...` и подобные) — они уже называют точную форму;
`Invalid closed action proposal|admission` и `Invalid closed publication
request` — это отказ разбора запечатанного файла, а не отказ по форме команды;
`unsafe_path` у `step` — там текст уже ведёт к нужной команде.

Проверка 3.3: `safe_next_actions` для `invalid_usage` — `["help"]`
(`problem.go:176`), вместо прежних `doctor`/`run.status`, которые уводили к
диагностике состояния. Тесты:
`TestRefusalByFormNamesTheOffendingPartAndTheAcceptedForm`,
`TestGroupRefusalsOfferExactlyTheOperationsTheGroupAccepts` (проверен и на
отрицание: удаление строки справки `project extend` роняет его сообщением
«accepted but not offered»).

## 4. Завершение

- [x] 4.1 Синхронизировать дельту с основными спеками; проверка:
      `openspec validate --all --strict --no-interactive`, `git diff --check`,
      пустой `git diff --name-only <база> -- openspec/changes/archive`.
- [x] 4.2 Evidence: один раз `make check` и `make e2e` на собранном кандидате,
      результаты записать здесь. Обе зависимые сессии предупредить до
      публикации, если сдвинулась форма контракта. Закрытие roadmap milestone
      или product gate не объявляется.

Проверка 4.1: обе дельты — только ADDED. `cli-protocol` получил «Инструмент
описывает свои коды возврата» и «Отказ по форме команды называет недостающую
часть» рядом с «Problem и exit code сохраняют safe meaning»;
`observability-publication-reactions` — «Измеренная величина называет, что она
измеряет» сразу за «Интервалы имеют разный смысл и не складываются молча», чьё
различение дельта не трогает. Ярлык этапа сценарию не проставлен: закрытие
milestone не объявляется. `openspec validate --all --strict --no-interactive` —
22 passed, 0 failed; `git diff --check` — чисто;
`git diff --name-only 016dc57 -- openspec/changes/archive` — пусто.

Evidence 4.2, один прогон на собранном кандидате:

`make check` — EXIT=0. `go test ./...` — cmd/prifly 35.5s, internal/runtime
222.7s; `go test -race ./...` — cmd/prifly 141.1s, internal/runtime 635.7s;
`go vet ./...` и `CGO_ENABLED=0 go vet ./...` — чисто;
`scripts/check-schema.py` — 47 бандлов совпали побайтно, включая
`workflow-revision-v4` (17091, `sha256:2295aa3d…`) и `contexts` (126383,
`sha256:25973353…`); `verify-release-ci.py` — контракт на месте.

`make e2e` — EXIT=0. `verify-install.sh`; `verify-authoring.py` — passed, 7
случаев + `accept-workspace-modes` + каталог; `verify-cli.py` — 3 случая,
`missing-output` ожидаемо `failed` с `expected_rejection: invalid_output`,
`export_bytes_verified` и `telemetry_population_verified` — true;
`verify-core.py` — passed, 169 команд; `verify-context.py` — passed, 75 команд,
10 случаев (включая обновлённое ожидание `core-timing/3`).

Форма контракта сдвинулась: `core-timing/2` → `core-timing/3` (только под
`isContextState`), плюс изменились тексты отказов по форме и справка CLI.
Обе зависимые сессии предупреждаются **до** публикации релиза; на момент
записи релиз не выпускался. Закрытие roadmap milestone или product gate не
объявляется.

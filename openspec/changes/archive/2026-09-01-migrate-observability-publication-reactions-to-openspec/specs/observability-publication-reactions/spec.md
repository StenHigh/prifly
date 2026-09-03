## Purpose

Определяет проверяемый будущий договор Pri-Fly для наблюдаемости исполнения,
объявленных публикаций шагов и durable реакций на сохранённые наблюдения.

## ADDED Requirements

### Requirement: Измерения принадлежат экземплярам исполнения
Pri-Fly SHALL связывать измерения с Run, WorkflowInvocation, StageActivation,
StepInstance и Attempt, сохраняя identity iteration и item. Control stages MUST
иметь timing activation без выдуманного worker или Attempt; публикации MUST
отдельно показывать readiness, commit и subscriber delivery.

#### Scenario: Identity и control stage без worker
- **Этап:** `P1-06`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Построить timing tree.
- **THEN** Две разные activation/step identities; finish имеет elapsed, executor time неприменим, фиктивной Attempt нет.
- **Контекст:** Один definition использован дважды и достигнут finish.

#### Scenario: Очередь, работа и settlement
- **Этап:** `P1-06`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Принять terminal transition.
- **THEN** Elapsed 15 s, executor 8 s, post-execution settlement 2 s; boundaries ссылаются на реальные события, не текст worker.
- **Контекст:** В тестовых сопоставимых часах queue 5 s, executor occupancy 8 s, затем settlement 2 s.

#### Scenario: Ранний результат не заканчивает время
- **Этап:** `P1-06`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Прочитать отчёт до и после подтверждённого выхода.
- **THEN** Executor interval открыт, post-execution settlement ещё не начался; result-to-acceptance показан отдельно и может перекрываться с исполнением.
- **Контекст:** Процесс выдал валидный result и продолжает жить.

#### Scenario: Параллельное время не суммируется как elapsed
- **Этап:** `P2-10`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Построить отчёт parent и обеих branches.
- **THEN** Sum attempt time 20 s, active union 10 s; родитель не дублирует интервалы через hierarchy.
- **Контекст:** Две сопоставимые Attempts работают одновременно по 10 s.

#### Scenario: Поздний receipt уточняет отчёт
- **Этап:** `P2-13`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Выполнить reconcile и повторно получить timing.
- **THEN** Новый report cut/provenance; исходные observation/decision timestamps не переписаны.
- **Контекст:** Remote exit ранее был неизвестен, пришло доверенное доказательство.

### Requirement: Время сохраняет источник, единицу и качество
Durable time observation SHALL сохранять UTC, clock domain, process/boot
identity и воспроизводимый monotonic segment либо эквивалентный факт. Public
duration MUST быть неотрицательным safe-integer числом миллисекунд с quality,
known/estimated value, `is_open` и причиной отсутствия; сериализованный UTC
MUST NOT выдаваться за monotonic clock.

#### Scenario: UTC rollback и порядок событий
- **Этап:** `P1-07`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Записать observations и replay.
- **THEN** Нет отрицательной duration или перестановки событий; качество календарного elapsed честно снижено, deadline не продлён.
- **Контекст:** Монотонные часы идут, UTC скачет назад.

#### Scenario: Crash и неизвестный интервал
- **Этап:** `P1-07`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Перезапустить и прочитать timing.
- **THEN** Старые monotonic ticks не продолжаются; gap виден, неизвестный exit не заменён нулём/временем рестарта.
- **Контекст:** Driver упал между start и подтверждением выхода.

#### Scenario: Suspend и восстановленные timestamps
- **Этап:** `P1-07`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Возобновить машину и восстановить state.
- **THEN** Process-clock duration не выдана за полный календарный elapsed; сохранённый UTC не объявлен monotonic.
- **Контекст:** Clock profile с исключающим sleep monotonic и сериализованный time.Time.

#### Scenario: Pause и живое исполнение перекрываются
- **Этап:** `P1-07`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Построить интервалы и выполнить resume без release.
- **THEN** Executor time 10 s, union restricted time 5 s, не 15 s elapsed; resume не снимает stops и не продлевает deadline.
- **Контекст:** Executor работает в [2,12), два restrictions действуют в [4,8) и [6,9).

#### Scenario: Рестарт при неопределённых часах
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Planner рассматривает start.
- **THEN** Нет продления TTL старым monotonic tick; требуется новое trusted observation/разрешённое восстановление, ordinary admission запрещён.
- **Контекст:** После reboot нельзя доказать свежесть source/permission.

### Requirement: Интервалы имеют разный смысл и не складываются молча
Pri-Fly SHALL различать elapsed, state time, ready queue, dispatch latency,
executor time, settlement, result acceptance, ожидание по причинам,
restrictions и cancel-to-settlement. Parent rollup MUST отделять сумму attempt
durations от union активного времени и MUST не удваивать child interval.

#### Scenario: Retry и задержка между попытками
- **Этап:** `P2-13`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Допустить retry только после новой положительной проверки guard.
- **THEN** Новые Attempt/Admission без лишней activation; executor sum 10 s, backoff 2 s; прежнее true не обходит текущие stops.
- **Контекст:** Attempts по 3 s и 7 s разделены backoff 2 s; start guard изменяется перед retry.

#### Scenario: Отмена не равна остановке эффекта
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Истекает grace и приходит неполный receipt.
- **THEN** Scope не terminal cancelled, время settlement открыто; нужный claim не освобождён без доказательства.
- **Контекст:** Живой descendant или pending remote effect после cancel request.

### Requirement: Пробелы времени и поздние факты остаются видимыми
Crash, потеря heartbeat или remote receipt SHALL оставлять gap и partial либо
unavailable measurement, а не придумывать terminal boundary. Поздний trusted
fact MUST создавать новую report revision с provenance и MUST NOT переписывать
старый audit, decision time или fixed cut.

#### Scenario: Состояние без живого driver
- **Этап:** `P1-08`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Прочитать из CLI/шага.
- **THEN** Snapshot версионный; executor interval не растянут как measured до as_of, value_ms=null при partial; liveness не объявлена доказанной.
- **Контекст:** Persisted run running, driver heartbeat давно отсутствует.

#### Scenario: Crash и невосстановимые samples
- **Этап:** `P1-08`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Restart и повтор historical read на фиксированных cuts.
- **THEN** Counts фактов восстановимы, samples gap/unknown loss показаны; старый RSS не выдуман из timestamps, future observation не просочилась.
- **Контекст:** Сохранены lifecycle facts и часть basic local/core samples F1, процесс падает до очередного batch; periodic executor RSS не собран.

### Requirement: Отчёт объясняет расход времени и исходы
F1 report SHALL выдавать text и JSON дерево status, verdict, elapsed,
executor time, waits, attempts и свежесть последнего факта. Он MUST отделять
успех чтения от успеха workflow, не смешивать missing/open/failed/cancelled с
успешными запусками и называть population у статистик.

#### Scenario: Read-only timing запрос
- **Этап:** `P1-08`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Выполнить status/timing/events.
- **THEN** Нет refresh, новых admissions, бизнес-событий или изменения leases; missing duration не ноль.
- **Контекст:** Run ждёт, внешний источник недоступен.

#### Scenario: Приватность и неполные статистики
- **Этап:** `P2-16`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Экспортировать metrics/traces и p95 report.
- **THEN** Нет raw secrets/неограниченных ID labels; окно, population, sample size и доля missing показаны, null не нули.
- **Контекст:** Secret payload, множество Run IDs, часть timings отсутствует, есть active/failed runs.

### Requirement: Экспорт не заменяет durable историю
Обязательные lifecycle facts, receipts и control records SHALL оставаться
durable без sampling. Optional OpenTelemetry export MUST быть отключаемым,
bounded и не блокировать stop либо result intake; raw secrets, task и outputs
MUST NOT становиться labels или автоматическим export payload.

#### Scenario: Недоступный exporter и полный durable disk
- **Этап:** `P2-16`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Запросить работу и stop в обоих случаях.
- **THEN** Export не блокирует control; невозможность сохранить обязательные факты блокирует новые effects, не теряет их молча.
- **Контекст:** Отдельно остановлен telemetry exporter; отдельно заполнен durable storage.

### Requirement: Read view имеет точный subject и согласованный cut
Read surface SHALL различать workflow definition и исполняемые Run,
invocation, activation, step и attempt. Один authority snapshot MUST читать
согласованный cut; внешний источник MUST показывать собственные version и
freshness без обещания глобальной атомарности.

#### Scenario: Cross-run cut и независимая authority
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Выполнить query и guard decision.
- **THEN** Первые читаются согласованно; внешний имеет отдельную версию/freshness, права проверены; global atomic snapshot не заявлен.
- **Контекст:** Два subjects одной authority и третий внешний.

### Requirement: Чтение не является управлением
Query, watch и cursor MUST проверять current access при каждом ответе и MUST
NOT refresh external source, продлевать lease, писать business event, запускать
workflow либо расширять Grant. Cooperative F1 MUST честно обозначать, что один
OS UID не является sandbox.

#### Scenario: Aggregate, cache и cursor после revoke
- **Этап:** `P2-15`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Запросить общий count/p95/top errors, затем revoke и следующую страницу.
- **THEN** Access до aggregation/выдачи, B и его counts не раскрыты; cursor/cache не дают прежнее право или новый query.
- **Контекст:** Principal видит A, но не B; есть cached report/cursor и ссылка trace на B.

#### Scenario: Отзыв доступа во время watch
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Доставить следующий event и переиспользовать cursor.
- **THEN** Закрытые данные больше не выдаются; cursor не Grant, core guard следует unknown policy.
- **Контекст:** Worker имеет scoped read, затем право отозвано.

#### Scenario: Control authority отозвана после вычисления
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Попытаться автоматически отменить scope и допустить новую работу.
- **THEN** Нет несанкционированного cancel/восстановления actor Grant; старый stop действует, ordinary admissions блокированы, отсутствие полномочий объяснено.
- **Контекст:** Guard вычислил true, но policy-owned control authority отозвана до control commit; старый stop уже есть.

#### Scenario: Hook не подменяет core authority
- **Этап:** `P1-08`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Подставить sibling ID либо payload status=completed/approved.
- **THEN** Чужая запись запрещена; своё валидное значение остаётся domain state, StepResult/Approval/Grant не возникают.
- **Контекст:** Caller может публиковать своё состояние, но не чужое.

#### Scenario: Тип, evidence и право на артефакт
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Попытаться выдать state.file_ready за publication, читать чужие bytes и принять output без required check.
- **THEN** Ни один обход не даёт пригодный input; core проверяет publication type, actual bytes, нужное evidence и current access отдельно.
- **Контекст:** Hook имеет artifact schema/check policy, subscriber знает digest чужого закрытого объекта.

### Requirement: State, входы и progress остаются разными данными
Pri-Fly SHALL различать authoritative execution state, step-owned state hook,
domain observation и optional worker progress. Dynamic read MUST не менять
sealed input manifest; если оно влияет на результат, result evidence MUST
закреплять прочитанные revision и digest.

#### Scenario: False, null, missing и forbidden
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Вычислить exists/not/all.
- **THEN** Нет implicit cast или fail-open; forbidden не превращается в «нет поля» и не скрывается short-circuit.
- **Контекст:** Четыре разных source cases и stale observation.

#### Scenario: Динамический read не меняет sealed inputs
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Step читает разрешённый observation и сдаёт result.
- **THEN** Input A не переписан; использованный B указан в evidence, live read не даёт control rights.
- **Контекст:** Step admitted с input revision A; source стал B.

### Requirement: Новые read views не изменяют закрытые DTO
Новый timing/read contract SHALL быть явно negotiated и versioned, включая
subject, cut, `as_of`, consistency, freshness и measurement quality. Existing
RunSnapshot, EventEnvelope и старый `run.status` MUST остаться закрытыми;
unsupported future guard fields MUST отклоняться до effect.

#### Scenario: Новый read view и старый JSON
- **Этап:** `P1-08`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Выполнить чтение и validation.
- **THEN** Старый DTO неизменен; новый тип явно negotiated/schema-validated; unsupported guard отвергнут до effect.
- **Контекст:** Старый клиент ожидает RunSnapshot v1, новый просит timing view, F1 получает guard поле.

### Requirement: Сигналы имеют provenance и completeness
Lifecycle facts, diagnostics, measurements, spans и raw diagnostics SHALL
хранить revision, identity, subject, source generation, observed/received
time, trust, classification и coverage. Unsampled mandatory facts MUST брать
происхождение из journal; unavailable meter MUST не выглядеть как zero.

#### Scenario: Успех с warnings и неполное coverage
- **Этап:** `P1-08`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Посчитать warning fraction и показать successful Run с предупреждением.
- **THEN** N=1,D=2,50%; два warned incomplete отдельно, не 150%; warning не меняет verdict/status, unknown coverage не ноль.
- **Контекст:** Два terminal Runs с полным warning coverage: один clean, другой с warning; два неполных также имеют наблюдённые warnings.

#### Scenario: Shared cost ядра и измеренный scope
- **Этап:** `P2-16`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Построить расходы ядра и Run attribution.
- **THEN** Shared bucket сохранён; нет точного деления по elapsed или смешения OS/Go estimates, вложенные spans не удвоены.
- **Контекст:** Два Runs делят core/SQLite/GC; есть process CPU, Go estimated CPU и отдельные operation spans.

#### Scenario: Типы, единицы и неизвестные metrics
- **Этап:** `P1-08`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Передать NaN/overflow, другую unit, decreasing cumulative без reset, неизвестное имя/aggregation и отсутствующий meter.
- **THEN** Некорректное отвергнуто явно, отсутствующее unavailable; нет округления, отрицательного traffic и успешного пустого ответа.
- **Контекст:** Descriptor задаёт counter/bytes с bounds и допустимыми dimensions.

#### Scenario: Cohort window, поздний исход и cut
- **Этап:** `P1-08`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Выполнить три отдельных F1 aggregate query: created-at cohort, explicit event window и completed-within, без view compare.
- **THEN** Cohort включает свою историю до cut после to, не будущий факт; populations/terminal/open и окна раздельны.
- **Контекст:** Run создан перед to, ошибка/settlement после to, но до cut; другой факт принят после cut; остаётся open Run.

#### Scenario: Opaque script и строка ERROR
- **Этап:** `P1-08`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Построить отчёт без declared diagnostic/usage mapping.
- **THEN** Известные core facts есть; текст не становится trusted failure/usage, внутренние calls и warning completeness неизвестны.
- **Контекст:** Неинструментированный script печатает ERROR и token count в stdout/stderr, но имеет известный exit/result.

#### Scenario: Paid deliveries, dedup и unknown usage
- **Этап:** `P2-13`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Посчитать usage/charge/reservation после retry и disconnect.
- **THEN** Две реальные charges, duplicate не добавлен, unknown не ноль и не release reserve; effect count остаётся отдельным.
- **Контекст:** Одна operation имеет две оплаченные deliveries, повтор receipt и ещё один неизвестный charge.

#### Scenario: Flood, exporter failure и mandatory facts
- **Этап:** `P2-16`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Публиковать, sampling/drop и отправить stop/result.
- **THEN** Предел cross-product и loss видны; severity не даёт unlimited audit, accepted facts не потеряны; без commit нет effects/ложного ACK.
- **Контекст:** Worker создаёт много dimensions/diagnostics, exporter down; отдельно исчерпан durable storage.

#### Scenario: Retention, rollup и restore
- **Этап:** `P2-16`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Запросить новый разрез/drilldown и прежний report/cursor.
- **THEN** Retained aggregate с ограничениями либо gap, не exact новые quantiles; current privacy и нужные tombstones сохранены до выдачи.
- **Контекст:** Raw samples удалены по policy, summary сохранён; restore старше erasure/dedup tombstone.

### Requirement: Outcome, error и warning независимы
Report SHALL отдельно хранить lifecycle status, accepted verdict, Run outcome,
effect knowledge и diagnostic severity. Diagnostic occurrence MUST иметь
stable identity, safe code/category, origin и known cause links; duplicate
delivery MUST не создавать вторую occurrence.

#### Scenario: Diagnostic occurrence и причинные ссылки
- **Этап:** `P1-06`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Повторить доставку первого diagnostic и построить сводку/цепочку.
- **THEN** Две occurrences, не число строк/уровней/доставок; неизвестная причина не выдумана, scope/origin сохранены.
- **Контекст:** Одна core failure связана с Problem/log/result, другой occurrence имеет тот же code; traceback содержит много строк.

#### Scenario: Quality, checker revision и waiver
- **Этап:** `P2-15`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Построить quality report и сравнить scores.
- **THEN** Каждый исход/coverage/evaluator различим; schema/waiver не semantic pass, несовместимые scores не объединены.
- **Контекст:** Schema pass, предметный fail, missing required check, waiver и разные evaluator revisions присутствуют в разных Runs.

### Requirement: Каталог метрик задаёт арифметику
Каждый descriptor SHALL закреплять revision, unit, instrument kind, scope,
source, dimensions, aggregation, temporality и availability. Counters, gauges,
distributions и ratios MUST использовать совместимые values, explicit
denominators и declared method; NaN, overflow, incompatible unit и unknown
metric MUST получить явный отказ.

#### Scenario: Children, RSS и sampled maximum
- **Этап:** `P2-09`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Построить process/tree/resource отчёты.
- **THEN** Нет двойного CPU учёта; сумма individual peaks не tree peak, sampled max помечен, scope/coverage/OS method видны.
- **Контекст:** Parent/children имеют пересекающийся accounting, пики памяти в разные моменты; sampler пропускает короткий пик.

#### Scenario: Измерения command и persistence пути
- **Этап:** `P1-06`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Сравнить received/unique/committed/conflict counts и latency.
- **THEN** Transport repeats не новые transitions; rejected/failure не скрыты из counts, counters относятся к правильной authority generation.
- **Контекст:** Один logical command повторён, другой конфликтует; успешный commit и controlled persistence failure наблюдены.

#### Scenario: Cumulative, gauge и duplicate receipt
- **Этап:** `P1-08`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Публиковать, повторять команды, читать summary.
- **THEN** Cumulative total13 и delta3, gauge42, две event occurrences; exact duplicate возвращает receipt без нового measurement.
- **Контекст:** State counter сообщает 10,13,13; gauge повторяет42; event X повторён, event Y имеет тот же payload.

#### Scenario: Retry exposure и reset generation
- **Этап:** `P2-13`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Посчитать first-attempt-pass, Run outcome и новую серию counter.
- **THEN** Обе первые exposure остаются non-pass; retry/waiver не стирают отказ, новый counter не вычитается из старой generation.
- **Контекст:** Первая Attempt failed, qualified retry accepted+pass; другой шаг после первой ошибки явно waived; meter generation сменена.

#### Scenario: Взвешенная агрегация и exact percentile
- **Этап:** `P1-08`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Посчитать общий count/sum/mean/p50/p95 nearest-rank.
- **THEN** Count5,sum110,mean22,p50=3,p95=100; не среднее групповых средних или p95; report называет n/method.
- **Контекст:** Полные значения1,2 и3,4,100 разделены на группы2 и3 элемента, одинаковы method/unit.

#### Scenario: Histogram merge и малая выборка
- **Этап:** `P2-15`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Запросить общий p95 и сравнение.
- **THEN** Нет average(p95) или merge несовместимого; exact либо labelled approximation/отказ, n=1 не уверенный regression.
- **Контекст:** Группы имеют разные counts, p95 и несовместимые histogram boundaries; ещё одна группа содержит один sample.

### Requirement: Надёжность показывает полную population
Report SHALL различать created, admitted, confirmed-started, terminal и open
entities, first-attempt outcome, retry, recovery, rework, cancellation,
refused admission и unresolved obligation. Standard fractions MUST публиковать
точные numerator/denominator и coverage без подмены unknown завершением.

#### Scenario: Lifecycle, verdict и outcome не смешиваются
- **Этап:** `P1-08`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Построить status/outcome/verdict breakdown и стандартные Run fractions.
- **THEN** Terminal=4, succeeded=2/4, failed=1/4, open=1; предметный fail не создаёт дополнительный технический failed Run.
- **Контекст:** Пять F1 Runs: два completed+succeeded, failed, cancelled и running; у одного проверяющего шага принят verdict fail.

### Requirement: Ресурсы измеряются в квалифицированном scope
CPU, memory, I/O, process, network/provider и accelerator data SHALL называть
method, scope, coverage и limits. Pri-Fly MUST не путать PID с process tree,
process elapsed с CPU, sampled maximum с точным peak или opaque executor с
нулевым usage.

#### Scenario: Итоговый OS CPU и отсутствие meter
- **Этап:** `P1-05`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Получить OS accounting, elapsed и exit evidence.
- **THEN** CPU/unit/method/scope отдельно от elapsed; неподдержанное поле unavailable с причиной, не нулевое измерение.
- **Контекст:** Реальный локальный процесс расходует CPU и завершается; второй профиль не поддерживает выбранное поле.

### Requirement: Ядро измеряет собственную нагрузку
Core SHALL учитывать intake, planner, persistence, result/artifact intake,
control/recovery, subscriptions и собственные runtime resources. Shared cost
MUST не распределяться по Run elapsed как measured fact, а read query MUST не
выполнять скрытый checkpoint или иной maintenance effect.

#### Scenario: Read-only query не выполняет maintenance
- **Этап:** `P1-08`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Запросить report несколько раз и попытаться включить refresh/profiling.
- **THEN** Нет checkpoint/provider probe/lease change/workflow event; возвращён сохранённый cut/freshness, unsupported refresh отвергнут.
- **Контекст:** Сохранены WAL/OS observations; новые measurements не собраны, writer работает.

#### Scenario: Предел query и медленный читатель
- **Этап:** `P2-16`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Читать aggregate/top-N, отменить query, продолжить после expiry.
- **THEN** Нет полного вида у partial aggregate; limit/expired явны, writer/control и WAL не удерживаются неограниченно.
- **Контекст:** Cohort превышает scan/group/byte limit, reader остановился между страницами, control command ожидает.

### Requirement: Качество, work units и reuse имеют provenance
Throughput, quality, checked items и reuse SHALL называть domain work unit,
evaluator/check revision, policy, population и provenance. Cache hit MUST не
приписывать source CPU/cost новому consumer, а claimed saving MUST быть только
отдельной labelled estimate.

#### Scenario: Work units, reuse и фактические расходы
- **Этап:** `P2-13`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Посчитать throughput/hit rate/cost per item и first-attempt population.
- **THEN** Знаменатели явны; source CPU/cost не новый расход reuse, savings только estimate; reused-only вне execution exposure.
- **Контекст:** Есть executed и reused-only outputs, eligible hit/miss и item groups разного размера.

### Requirement: Usage и charge не являются одним числом
Instrumented provider usage SHALL отделять reported usage, estimated cost,
provider charge, settled charge и reserved budget. Money MUST использовать
decimal, currency, rate-card revision и provenance; duplicate receipt MUST не
удваивать charge, а unknown charge MUST не освобождать reservation.

#### Scenario: Тариф, валюта и late adjustment
- **Этап:** `P2-16`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Повторить report по старому cut и рассчитать новый.
- **THEN** Историческая estimate pinned, currencies не сложены, adjustment связан без дублирования; source/method/coverage видны.
- **Контекст:** Есть partial usage, rate-card v1/v2, две currencies и поздний provider adjustment.

### Requirement: Шаг публикует метрику только объявленным mapping
StepDefinition SHALL declaratively pin mapping declared state/event hooks в
metric or diagnostic descriptors с schema paths, units, dimensions и bounds.
`step.publish` MUST оставаться единственным intake; worker MUST не менять core
namespace, чужой scope, OS trust или billing proof.

#### Scenario: Собственный metric и защищённый namespace
- **Этап:** `P1-08`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Опубликовать свои значения, затем чужой scope/core counter, неверную schema и forged trust.
- **THEN** Разрешённое видно как worker-reported; подмена отклонена, lifecycle/budget не изменены, descriptor pinned.
- **Контекст:** Шаг объявил progress gauge и diagnostic event hook с pinned mapping.

### Requirement: Historical query фиксирует cohort и метод
Historical report SHALL pin cohort/window/cut, membership, filters,
calculator/descriptor revision, completeness и access scope. It MUST separate
created-at cohort, explicit event window and completed-within view, and MUST
not infer a causal regression from a changed workload mix.

#### Scenario: Изменение состава workload
- **Этап:** `P2-15`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Сравнить общий результат и одинаковые workload strata.
- **THEN** Общие110/200 и54/200 показаны вместе с прежними90%/20% внутри strata; нет вывода о regression кода только из mix.
- **Контекст:** Baseline: small90/100,large20/100 succeeded; candidate: small18/20,large36/180.

### Requirement: Telemetry query остаётся закрытой read-only операцией
`telemetry.query` SHALL принимать closed versioned request с bounded filters,
metrics, dimensions, limits and cursor and return reproducible report with
cuts, values, units, coverage and evidence links. It MUST reject unknown or
incompatible fields before scan and MUST NOT run SQL, arbitrary expressions,
provider refresh or control action.

#### Scenario: Query schema и границы F1
- **Этап:** `P1-08`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Выполнить catalog/records/aggregate, запросить compare/unknown filter и вставить telemetry поля в v1.
- **THEN** F1 reads работают по closed DTO; неподдержанное/unknown/v1 mutation отвергнуто; ни silent ignore, ни placeholder success.
- **Контекст:** Доступны старый RunSnapshot и новые F1 query/report schemas.

### Requirement: Сбор ограничен, приватен и воспроизводим
Release profile SHALL declare collection meters, sampling, cardinality, rate,
bytes, retention and query limits. Sensitive content MUST require explicit
profile, classification, redaction and retention; loss of optional samples MUST
be measured without permitting loss of mandatory history or control reserve.

#### Scenario: Secrets и безопасный export
- **Этап:** `P2-16`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Сохранить/показать/export telemetry в standard и явно разрешённом diagnostic profile.
- **THEN** Действуют minimization/classification/redaction/escaping; нет автоматической утечки raw content или формулы, scope не расширен.
- **Контекст:** Sensitive payload содержит secret в exception/metric attribute/span/profile и HTML/ANSI/formula prefixes.

### Requirement: Анализ не меняет workflow сам по себе
Analytics SHALL produce deterministic reports, explicit trade-offs and
reproducible evidence. AI analysis MAY propose a hypothesis over an allowed
sealed report, but MUST NOT auto-change revision, retry, timeout, grant,
resource limit or active Run; hard control MUST use qualified accepted source.

#### Scenario: Аналитика не меняет управление
- **Этап:** `P2-15`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Выполнить query/AI analysis и доставить accepted publication либо sampled export value.
- **THEN** Query не меняет Run/budget/grants; guard принимает только declared source по policy, не exporter; hard budget не доверяет неподтверждённому meter.
- **Контекст:** Report показывает возможное улучшение от новых limits/checks/model; отдельно guard подписан на разрешённый worker-reported metric.

### Requirement: Телеметрия поставляется по profile phases
F1 SHALL сохранять minimum lifecycle/time/diagnostic facts, declared state and
event hooks and local read-only reporting. F2 MUST add only qualified
operators, comparisons, provider/resource meters, profiles, export and
retention; unavailable capability MUST remain explicit rather than emulated.

#### Scenario: Цена instrumentation и profiling
- **Этап:** `P2-16`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Измерить CPU/memory/bytes/control latency и запросить profiling без разрешения.
- **THEN** Overhead/метод/coverage опубликованы; profiler bounded/authorized, нет public debug endpoint и потери обязательных гарантий ради скорости.
- **Контекст:** Одинаковая workload/build/profile проверяется с disabled optional, standard и diagnostic/profiling.

### Requirement: Шаг объявляет immutable hooks contract
StepDefinition SHALL declare named typed `state`, `event` and `artifact` hooks
with schema, classification, access, availability, payload/rate/count limits
and kind-specific rules. Hook declaration MUST enter the pinned dependency
closure and MUST NOT let worker create unknown hooks, grant authority or
replace inputs/outputs.

#### Scenario: Автор шага объявляет hooks
- **Этап:** `P1-08`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Publish declared hook, затем unknown name/kind и invalid payload.
- **THEN** Корректные данные доступны по контракту; неизвестные/невалидные не применены; для имён не нужен новый код ядра.
- **Контекст:** Два разных StepDefinitions объявили собственные state/event names и schemas.

#### Scenario: Несколько файлов до завершения producer
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** A публикует два разных готовых items и продолжает считать третий.
- **THEN** B получает отдельные закреплённые inputs/admissions и обрабатывает item до terminal A; A не назван completed.
- **Контекст:** A running в parallel branch, hook document_created keyed_many; B — объявленный subscriber.

### Requirement: State and event publication is fenced and idempotent
`step.publish` SHALL authenticate the current unfenced Attempt, validate a
bounded typed payload and preserve receipt idempotency. State MUST use scoped
CAS and full replacement; event MUST use a stable logical occurrence identity;
terminal or revoked namespace MUST not accept new live publication.

#### Scenario: Свой state, CAS и повтор команды
- **Этап:** `P1-08`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Конкурентные full replacements и повтор после потерянного ответа.
- **THEN** Один update commit, другой conflict; exact duplicate не повышает version/TTL, отсутствующие поля не сохраняются implicit merge.
- **Контекст:** Attempt публикует state revision 1; два updates ожидают эту версию.

#### Scenario: Terminal, fencing и ограниченный shutdown state
- **Этап:** `P1-08`; **Статус:** `passed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Публиковать permitted shutting_down и поздние новые updates.
- **THEN** Только разрешённая диагностика активного owner меняется; terminal/fenced state заморожен, existing receipt не создаёт новую запись.
- **Контекст:** Attempt active, затем pause/cancel-pending, terminal или revoked publisher.

#### Scenario: Crash и cancel вокруг publication commit
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Crash до/после commit и повтор команды после изменения исходного пути.
- **THEN** До commit нет visible publication; cancel rechecked; после commit прежний receipt/ArtifactRef, без повторного чтения/дублирования.
- **Контекст:** Скопирован candidate, publisher может быть отменён до commit.

#### Scenario: Несколько независимых subscribers
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Читать/claim публикации и перезапустить consumer driver.
- **THEN** Отдельные cursors/assignment ledger; B не поглощает C событие, повтор не создаёт лишнюю обработку или меняющийся input.
- **Контекст:** B и C подписаны на hook A, B быстрее, доставки повторяются.

#### Scenario: Retry, state generation и logical keys
- **Этап:** `P2-13`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Повторить X/K с теми же/другими bytes, state update и consumer retry.
- **THEN** State не наследует ready; X/K не создают вторую delivery/Trigger, provenance сохранён, разные bytes конфликтуют; consumer привязан к своему assignment.
- **Контекст:** A1 опубликовала item X, event K и ready; A2 — qualified retry той же StepInstance.

### Requirement: Early artifact publication seals bytes before visibility
Artifact publication SHALL validate ownership, path, type, size, digest,
checks and classification, seal its own immutable copy before atomic record
commit, and expose exact ArtifactRef plus PublicationRecord only afterwards.
It MUST not treat a mutable path, file event or `ready=true` state as a ready
artifact.

#### Scenario: Файл меняется вокруг публикации
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Изменить его во время intake, затем отдельно после commit.
- **THEN** Mismatch не публикуется; после commit consumer читает прежние проверенные sealed bytes, не writable original/hardlink.
- **Контекст:** Producer создал файл с ожидаемыми digest/size.

#### Scenario: Explicit close и полный manifest
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Закрыть hook, повторить close и пройти wait/choice/call lowering.
- **THEN** Manifest точен, closure iteration учтена, пустой поток не вызывает consumer; Interrupted не EOF, следующий item не подменяет pending assignment.
- **Контекст:** Несколько items приняты, один обрабатывается; отдельно поток пуст или interrupted.

### Requirement: Subscriptions bind declared producer to declared consumer
Workflow SHALL declare exact producer hook, consumer mapping, mode, initial
read, deadline, limits and failure policy. Each subscriber MUST have its own
durable cursor and assignment ledger; `each_publication` MUST lower to bounded
repeat, wait, choice and call with immutable iteration inputs, not a hidden
stream engine.

#### Scenario: Backpressure, capacity и pins
- **Этап:** `P2-16`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Запросить overlap, публикацию, GC и stop; проверить циклическую композицию и разделённые compute/commit.
- **THEN** Невозможный overlap и явный цикл отвергнуты; finite spool зарезервирован до producer admission, публикация не ждёт consumer settlement; control reserve и pins сохранены.
- **Контекст:** Producer может занять последний slot, subscriber медленный, storage/queue limit достигнут; отдельно A ждёт consumer, который ждёт final A.

### Requirement: Publication lifecycle preserves failure and backpressure truth
Accepted item SHALL not disappear after producer failure or cancellation.
Final-dependent action MUST recheck final evidence; independent consumption,
retries, spool budgets, capacity, pins, closure and backpressure MUST remain
explicit. Timeout, silence, disconnect and interruption MUST not be successful
EOF.

#### Scenario: Producer failure и dependent commit
- **Этап:** `P2-14`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** A падает, B пытается commit, запускается допустимое recovery.
- **THEN** Dependent commit/конечный business success блокированы; independent effect/bytes не исчезают, compensation отдельно допущена; stop/unknown не обходятся.
- **Контекст:** B уже вычислил ранний item, но final-dependent commit ждёт A; отдельно independent B сделал разрешённый effect.

### Requirement: Публичный protocol расширяется по фазам
Pri-Fly SHALL expose one versioned `step.publish` operation with typed state,
event, artifact and close variants. F1 MUST support only declared own
state/event subset; artifact intake, subscriptions, early consumption and full
retention/backpressure MUST remain unsupported until their qualified phase.

#### Scenario: F1 subset и неизменный map
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Запросить ранний consumer в F1 и дописать item в уже принятый map; выполнить bounded repeat/wait subscription в P2.
- **THEN** F1/изменение map отвергнуты; P2 обрабатывает публикации через отдельные immutable iteration inputs без скрытой смены v1 semantics.
- **Контекст:** F1 принимает state/event hooks; P2 имеет существующий sealed map и stream subscription.

### Requirement: Реакции исполняет один durable planner
The existing deterministic planner SHALL persist observations, cursors, timers,
stops and decisions in the authority journal path. Notifications MAY wake it
but MUST not own work; repeated reconciliation over pinned facts MUST not rerun
an effect, and no user callback, second scheduler or general plugin bus is
introduced.

#### Scenario: Изменение между snapshot и watch
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Получить snapshot+cursor и продолжить watch.
- **THEN** Изменение присутствует либо в snapshot, либо после cursor; нет окна потери/двойного control effect.
- **Контекст:** Состояние меняется в окно инициализации подписки.

#### Scenario: Notification не создаёт лишнюю Attempt
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Перезапустить planner до/после initial admission.
- **THEN** Одна activation и одна исходная Attempt; notification не разрешает retry, новые qualified Attempts проверяются отдельно в P2-13.
- **Контекст:** Guard уже true и несколько повторных notifications.

#### Scenario: Отсутствующий владелец wakeup
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Проверить status, включить owner после срока.
- **THEN** Нет обещания невыполненного автозапуска; timers восстановлены с объявленной expiry/misfire policy, worker slot не занят sleep.
- **Контекст:** Run ждёт condition/timer, managed owner выключен.

### Requirement: Binding источника точен и fresh
State binding SHALL record stable identity, owner scope, exact source or
resolution rule, schema, fields, principal, provenance, generation and
freshness. It MUST reject implicit latest/name search and conflicting duplicate
payload; unsupported source capability MUST bound its promise to last value.

#### Scenario: Duplicate, out-of-order и конфликт payload
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Прислать версию 11, duplicate X и другой payload X.
- **THEN** State не откатывается, TTL от duplicate не продлён, конфликт не применяется, второй admission отсутствует.
- **Контекст:** Приняты source version 12 и event ID X.

#### Scenario: Freshness истекла без новых событий
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Наступает durable expiry timer.
- **THEN** Condition становится unknown в принятой time observation, duplicate не освежает факт, реакция не ждёт следующего source event.
- **Контекст:** Последний known value устаревает, источник молчит.

#### Scenario: External state изменился после проверки
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Выполнить action с/без поддержанной target precondition.
- **THEN** CAS-capable target отклоняет mismatch; без него продукт не заявляет атомарность/мгновенную остановку, limitations видны.
- **Контекст:** Прочитана внешняя revision 4; до effect источник перешёл в 5.

### Requirement: Watch начинается без окна потери
Watch SHALL return a consistent snapshot and cursor from the same point, then
deliver ordered changes after that cursor with current access checks. History
gap MUST return `resync_required`, not a false continuous stream; cursors MUST
be opaque, bounded and bound to lineage, scope and view.

#### Scenario: Compaction и обязательный control cursor
- **Этап:** `P2-16`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Выполнить GC и возобновить чтение.
- **THEN** UI получает resync; control facts pinned либо monitoring gap блокирует работу, история true не потеряна молча.
- **Контекст:** UI cursor старый, другой cursor нужен active stop guard.

### Requirement: Guard registration живёт дольше клиента
Durable guard registration SHALL belong to Run, invocation or activation with
pinned condition, dependencies, cursor, deadline, control scope and current
policy-owned authority. Disconnect MUST not remove it; missing wakeup owner or
essential history MUST visibly block ordinary admission rather than pretend an
automatic reaction occurred.

#### Scenario: Stop true до запуска
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Planner рассматривает target.
- **THEN** Durable stop/reason, ноль новых Attempts/effects; stop не проигрывает порядку callbacks.
- **Контекст:** При регистрации одновременно true start и stop.

#### Scenario: Короткое true и согласованные срезы
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Вычислить all(A,B) при backlog/restart.
- **THEN** В первом случае latch на промежуточном true; во втором нет выдуманного cut. Latest B не подставлен к старому A; false не снимает stop.
- **Контекст:** Initial A=false/B=true; отдельные commits A=true, затем B=false до wakeup; отдельно обе перемены в одной transaction.

#### Scenario: Завершённый scope и поздний event
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Читать terminal state и доставить старый event.
- **THEN** Чтение разрешено по current access; поздний event не создаёт worker и не открывает старую activation.
- **Контекст:** Scope settled, registration закрыта.

### Requirement: Guard predicate не исполняет произвольный код
Live condition SHALL use a closed typed AST with defined three-valued logic,
access checks, freshness and durable timers. Missing, null, false, forbidden,
stale and type error MUST remain distinct; `unknown` MUST not become implicit
fail-open or hidden boolean by short-circuit.

#### Scenario: Потеря источника при active работе
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Core и worker теряют read stream.
- **THEN** Core независимо применяет pause/cancel policy; нет synthetic false, новые ordinary actions запрещены.
- **Контекст:** Source disconnected/stale, on_unknown явно задан.

### Requirement: Start guard gates existing graph work only
`start_when` SHALL gate an existing root/child invocation or activation before
its initial admission, without creating arbitrary work or holding worker slot.
Every admission, including qualified retry, MUST recheck current guards, stops,
claims, quotas and inputs; repeated true notification MUST not create another
activation or Attempt.

#### Scenario: Guard изменился в очереди capacity
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Освободить slot.
- **THEN** Admission не использует старое true; scope ждёт без выдуманного pass/skip и лишнего resource reserve.
- **Контекст:** Start был true, slot занят; затем source стал false.

#### Scenario: Цикл ожиданий и пределы
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Validate и довести динамическое ожидание до deadline.
- **THEN** Доказуемый статический deadlock отвергнут; динамический не объявлен доказанно безопасным, ограничен deadline и имеет explain.
- **Контекст:** Stage A ждёт B, который достижим только после A; другой источник создаёт динамическое ожидание.

### Requirement: Stop guard creates durable restrictive control
`stop_when` SHALL register before protected admission and create explicit
pause_scope or cancel_scope record with fact refs, control epoch and exact
target. It MUST require an `on_unknown` restrictive policy, preserve unknown
effects through settlement, and MUST not release or restart a closed scope
automatically.

#### Scenario: Stop конкурирует с terminal result
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Арбитрировать result/stop и закрытие registration в обоих порядках.
- **THEN** Barrier учитывает source/timers до cut, ранний stop не потерян; только ранее committed terminal не переписан.
- **Контекст:** Relevant source true либо freshness expiry предшествуют terminal cut, обработчик отстал; отдельно terminal был раньше source change.

#### Scenario: Parent stop, siblings и compensation
- **Этап:** `P2-14`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Stop child, затем parent, выполнить разрешённое recovery.
- **THEN** Child не отменяет чужой scope; parent не завершается раньше settlement; compensation имеет свои права/лимиты и не обходится callback.
- **Контекст:** Ветви parent имеют effects и declared compensation.

### Requirement: Guard races preserve committed causality
Authority SHALL atomically persist relevant condition facts, decision and
admission/stop boundary. A stop MUST latch a true edge before later false,
terminal settlement MUST process relevant observations/timers before its cut,
and external check MUST not promise atomic control without target-side
precondition.

#### Scenario: Stop на границе commit и spawn
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Commit stop и восстановить после crash.
- **THEN** Проверены актуальные gates; неподтверждённый spawn остаётся неопределённым, blind respawn запрещён.
- **Контекст:** Admission committed, dispatch ещё не подтверждён.

### Requirement: Level, event wait and Trigger stay distinct
Level condition, addressed event wait and Trigger-created Run SHALL retain
separate identities and semantics. A repeated true level MUST not create a
Run; edge-trigger MUST pin source transition identity and use normal quota,
grant and dedup checks.

#### Scenario: Trigger не запускается на каждом true
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Выполнить polling/restart и отдельное новое изменение.
- **THEN** Один Run на принятую identity; следующий только по объявленной новой identity/политике, все обычные grants/quotas применены.
- **Контекст:** Источник час true, delivery повторяется, Trigger имеет одну edge identity.

### Requirement: Waiting is finite and stable intervals are explicit
Registration SHALL have deadlines and bounded evaluations/messages. Optional
`stable_for_ms` MUST be proved by durable timer and reset on unknown/gap;
hysteresis MUST be an explicit source or threshold pair, and safety stop MUST
not wait for debounce.

#### Scenario: Stable interval и safety stop
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Доставить timers/updates.
- **THEN** Стабильность начинается заново после gap; stop не ждёт пользовательского debounce; восстановление false не запускает restart.
- **Контекст:** Start требует stable 5 s; в интервале unknown; затем safety stop.

### Requirement: Worker observation does not create an orchestrator
An ordinary Step MAY use scoped read-only snapshot/watch, but MUST not gain
route/start/stop authority over siblings. Core-owned durable guard MUST be used
for crash-surviving control; blocking worker watch MUST not masquerade as
free wait or prevent qualification of a one-slot profile.

#### Scenario: Worker-наблюдатель не управляет соседями
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Попытаться start/cancel sibling или подменить observation.
- **THEN** Команда/источник отвергнуты; чтение остаётся отдельным правом, managed boundary квалифицирована.
- **Контекст:** Step читает состояние sibling, но control Grant отсутствует.

### Requirement: Reactive load, privacy and lifetime are bounded
Compiler and runtime SHALL bound dependency indexes, registrations, predicates,
inbox, subscribers, evaluations, queues and retention with shared budgets and
control reserve. Slow readers MUST resync/disconnect without blocking authority;
terminal registrations MUST close and late events MUST not resurrect scope.

#### Scenario: Flood и медленный подписчик
- **Этап:** `P2-16`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Применить load envelope и запросить stop.
- **THEN** Буферы ограничены, current access соблюдён, control reserve работает; измеренная latency отделена от remote settlement.
- **Контекст:** Diagnostic flood/slow subscriber и control event одновременно.

### Requirement: Decisions имеют historical explain и safe preview
Pri-Fly SHALL expose target, pinned condition, decision result, authorized fact
references, freshness, waiting reason, stop identity and permitted next action.
Historical explain MUST read the decision facts, while preview MUST have no
registration, refresh, process, grant or effect side effect.

#### Scenario: Preview и историческое explain
- **Этап:** `P2-15`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Выполнить preview и explain того решения.
- **THEN** Preview без effects/refresh; explain называет сохранённые доступные facts, причину и stop identity, не подменяет историю текущими values.
- **Контекст:** Есть guard decision по старой revision и новые факты с ограниченным доступом.

### Requirement: Реакции включаются только по квалифицированным phase
Foundation profile SHALL reject live guards, durable watch, early artifact
consumption, extra predicate sources and condition-driven restart. Their
versioned schemas, policies, capability validation and qualification fixtures
MUST arrive with later profile phases; pinned historical Runs MUST retain their
older semantics.

#### Scenario: Choice не является live guard
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Replay и watch доставляют B.
- **THEN** Route не переизбран; только явно объявленный guard реагирует на B, старые definitions сохраняют смысл.
- **Контекст:** Choice принял route по snapshot A; live source теперь B.

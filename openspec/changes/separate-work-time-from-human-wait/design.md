## Context

Мотивация и границы — в [proposal.md](proposal.md). Решение владельца:
ожидание человека по умолчанию бессрочно, рабочий срок задаётся отдельно.
Разведка выполнена на `f1c9ebd85a971ab0394ea19f49e089a666dc5864`.

- `sessions.go` задаёт `assistedAttemptTimeoutMS = 60 * 60 * 1000`.
  `driver.go` закрепляет этот срок и в Attempt, и в ExecutionEnvelope.
- `decision_bridge.go` при ответе меняет envelope digest/generation, но
  сохраняет старый deadline. Приём ответа не проверяет истечение, а приём
  результата проверяет. Поздний ответ поэтому выглядит как продолжение.
- `engine.go` проверяет singleton PendingDecision прежде cancellation и
  остальной ready работы. Ожидающая Attempt удерживает admission slot;
  простое увеличение срока блокировало бы другие Runs при capacity 1.
- Возврат в общий `settleUnstarted` после cancel assisted работы может
  приписать ей `ProcessOutcome.Started=false`, хотя host уже работал.
- Managed timeout имеет независимый часовой ceiling в configuration,
  execution bindings и `remainingBudget`. Его clock/recovery не равны
  допустимости отчёта host; этот ceiling здесь не повышается.

### Живые наблюдения 2026-09-05

На отдельном source binary без release ldflags (его строка версии
`0.2.0-dev.5` не release version), SHA-256
`fc2fda2fdcba4905a345ad2c8a891cb5c06afc357d3fad0715e36631c6e12366`, выполнен
нейтральный parse → assisted scale → report. Node 22.19.0, Project `/3`,
без Git/Brief/claims; реальный host — этот чат, не provider, запущенный Core.

Первый Run `run:cd6f26c3372c9af939dc15d76236148371497d4992c320a924b06087c1523503`
не смог создать socket в песочнице. После разрешённого запуска вне неё его
10-секундная admission уже истекла: `rejected`, process не был dispatched.
Это не выдаётся за успешное восстановление.

Второй Run `run:dceccd7f783b8dd44f103975244c53656f58d0f35346dda391806b1ad2e001dc`
сохранил вопрос (версия 12), затем ответ `2` (версия 13) в
`2026-09-05T18:27:58.249763Z`, хотя deadline был
`2026-09-05T15:34:19.17549Z`. Та же Attempt получила answer/new envelope;
`session submit` отказал `attempt_deadline_expired`. Итоговых outputs не было.
По просьбе повторить тест выполнены cancel и drive до terminal cancelled
(версия 16); история не удалялась. Успешный cancel после уже принятого ответа
не доказывает исправность cancel при ещё открытом вопросе.

Третий Run `run:13581a2625727d21629a3659b42a12f70c91e4a98976b7b2ab68d3032a7a4ec4`
создан до освобождения старого slot, поэтому Start вернул capacity_conflict.
Run найден по receipt `command:live-neutral-ui-3`, не создан повторно.
После штатной отмены прежнего Run drive исполнил parse; новый request/answer
использовал подтверждённый ранее выбор пользователя. Принят host result,
затем настоящая программа создала report: `Rows: 3`, `Total: 48`, digest
`sha256:534dfc0d3202e8fd64080aaa1d06e8ef9695fabf1201abf602d1fafd485d46ab`.
Reopen подтвердил `completed/succeeded`, 3 Attempt, actor value 2, 0 diagnostics.

Локальные материалы находятся в `/private/tmp/prifly-live-neutral.jj12kx/`
(`TEST-RESULT.md`, `RETEST-RESULT.md`, `retry-*.json`, authority и report).
Это временные материалы, не обязательная вечная ссылка для воспроизведения:
исходный neutral fixture — `cmd/prifly/project_mixed_test.go` и
`examples/workflows/csv-report/`. Повтор подтвердил короткий путь, не недельное
ожидание или native question UI; пользователь отвечал обычным текстом и просил
объяснить контекст вопроса. Планы соседних changes этим не закрыты.

## Goals / Non-Goals

**Goals:** конечный настраиваемый active budget, отдельный optional wait
timeout, durable остаток, безопасное освобождение и повторный допуск capacity,
честные cancellation/expiry и читаемые причины ожидания. Core не знает AIF.

**Non-Goals:** нет CPU accounting чужой сессии, process suspension, heartbeat
сервиса, фонового пробуждения, новой очереди вопросов, automatic retry или
переоткрытия старых Runs. Managed/check executor ceiling остаётся прежним.
Долгое ожидание не обещает автоматический перенос/renewal существующего
Git claim: неподтверждённое владение требует явного recovery, а не смены папки.

## Decisions

### 1. Явная versioned настройка у assisted шага

Добавить `session_limits` в следующую StepDefinition и новый authoring marker
`prifly-step/2`. Marker выбирает новую семантику даже без явной настройки:
`active_timeout_ms` default 3600000, `decision_wait_timeout_ms` default null.
Рабочий срок — положительное целое; ожидание — null либо положительное целое.
Единицы сохраняют используемые Pri-Fly milliseconds; комментарии в полном
YAML reference объясняют человеческие значения. Новый parser durations не нужен.

Для нового assisted contract верхняя граница — представимость целых ms без
переполнения `time.Duration` (9223372036854 ms), а не произвольный час. Это
предел представления, не квалификация многолетней работы. Конкретный конечный
предел закрепляет автор; local policy может только сузить разрешённое время.
Настройка не помещается в ordinary workflow input или environment. Старый
authoring сохраняет прежнюю compilation; явная миграция создаёт новые exact refs.
Для несовместимого executor эта настройка отвергается, не игнорируется.

Альтернатива заменить час на неделю оставляет те же проблемы. Бесконечная
активная работа убирает защиту, а сброс полного бюджета после ответа позволяет
пополнять его вопросами. Ни один из этих вариантов не принимается.

### 2. Сохраняемый остаток и отдельная доставка

Активное время — календарный интервал разрешённой доставки host вне записанного
ожидания, не наблюдённое CPU-время. При новом DecisionRequest текущий interval
закрывается сохранённым Observation, его длительность вычитается из остатка.
Проверка active expiry выполняется до открытия вопроса. Отрицательные интервалы
и противоречивое время отвергаются; округление не возвращает потраченные ms.
Ожидание хранит свой старт и optional deadline. Automatic/preanswered selection
не создаёт waiting interval и не восстанавливает бюджет.

При принятом ответе сохраняется выбранное значение и готовность к повторному
допуску. Только после получения capacity и current admission gate создаётся
новая доставка той же Attempt, с тем же input и оставшимся временем. До этой
доставки активные часы не идут. Повтор команды не вычитает interval второй раз.
Restart/replay читает сохранённые observations, не строит новый срок от now.

Следующая session/state/read edition должна связывать base ExecutionEnvelope,
generation, decision context, timing policy и текущий effective deadline
в проверяемых bytes новой доставки. Первоначальный sealed envelope остаётся
неизменным; одного изменения `Attempt.Deadline` недостаточно. Конкретные DTO
и номера сверить в первом implementation task с опубликованным inventory;
ожидаемые следующие editions — StepDefinition v6, assisted-session/6 и Core
state/read после /26. Не менять schema bytes предыдущих editions.

Использовать текущий authority clock/Observation и чистые reducers. Assisted
UTC остаётся `local_wall_unqualified`, с отказом при обнаруженном rollback;
это не обещание доверенных часов между машинами. Временные сравнения и новый
deadline должны учитывать последнюю принятую границу, не только admission.

### 3. Ожидание передаёт управление, но не уничтожает обязательства

Новый request contract явно подтверждает передачу управления host: после
принятого запроса прежняя delivery не вправе публиковать результат или делать
новую работу. В cooperative local-owner profile это protocol fencing и
обязательство host, не доказательство остановленного OS-процесса.
При известных in-flight/unknown effects, несовместимых с безопасной передачей,
request отказывает до изменения capacity; необходимое settlement не угадывается.

Versioned parked-фаза сохраняет Attempt, pins, outputs и claims, но атомарно
освобождает execution slot и перестаёт расходовать workflow max_parallelism.
Существующая очередь допуска используется для повторной доставки; новая
Attempt или новый scheduler не создаются. Ответ хранится один раз; при занятом
slot состояние остаётся «ответ принят, ожидается допуск», без executable task.
Повторное резервирование учитывает только остаток, не новый полный budget.

Read/next выбирает cancellation/recovery и доступную независимую работу до
idle/waiting_decision. Независимые siblings могут завершиться, join не завершает
припаркованную ветвь вымышленным итогом. Пока сохраняется один pending вопрос
на Run: второй request получает понятный conflict без порчи своей доставки.
Несколько одновременно открытых диалогов — отдельное расширение.

### 4. Ответ, разрешение и recovery — разные факты

До сохранения ответа проверяются identity, value, текущий ACL, срок вопроса
и отсутствие отмены. Допустимый ответ может сохраниться под Pause/Stop,
но новой работы это не разрешает. Повторная доставка требует обычного
admission gate с атомарным ControlPin, current revocation, claims/generations,
resources и capacity. Ни answer, ни elapsed wait не продлевают Approval, Grant,
ActionIntent или lease и не убирают unknown obligation.

Claim сохраняет physical exclusivity. Старый подозреваемый/истёкший lease
нельзя автоматически сделать свежим или освободить по вопросу. UI объясняет
отдельный необходимый recovery; если его нельзя безопасно выполнить нынешним
contract, продолжение остаётся blocked. Полная квалификация недельного
продолжения с Git claim не выдаётся за no-Git тест этой временной модели.

Активная expiry и finite wait expiry закрывают возможность новой доставки,
но не сообщают неизвестному эффекту false outcome. Recovery обслуживает обе
фазы: active host и pending question. Cancel закрывает вопрос с причиной,
не ждёт ответа и не проходит через managed-only `settleUnstarted` для уже
переданной assisted Attempt. При effects:none без иных обязательств допустима
отмена без фиктивного ProcessOutcome; при неизвестном эффекте — обычное
uncertain/resolution. Старые deadlines не продлеваются даже ради исправления
прошлого пилота; late-answer guards и диагностирование его expiry сохраняют
его прежний смысл.

### 5. Проверка короткая, но сквозная

Переиспользовать test clock/Observation и existing mixed fixture. Сначала
воспроизвести нынешний late-answer отказ, затем проверить 10 минут работы →
14 дней ожидания → 50 минут остатка после reopen. Не sleep две недели и не
подменять Run snapshot ради успешного report.
Отдельно проверить capacity 1 с двумя Runs, sibling progress, answer/cancel
CAS, finite deadline boundary, rollback, revoked rights, старые readers и
claimed workspace recovery refusal. Один короткий живой mixed Run — отдельное
наблюдение, не замена этих проверок. Full gates один раз на candidate, а не
на каждой docs-only правке. Точные результаты и exclusions сохранять в tasks.

## Risks / Trade-offs

- Бессрочная пауза держит данные/claims → pins и обязательства сохраняются;
  пользователь видит их, GC/leases не меняются этим изменением.
- Host работает с общим OS UID → не обещать недоступную физическую изоляцию;
  явно показывать cooperative scope и проверять все controlled API boundaries.
- Saved old contracts → явная новая revision и readers, без массового rewrite.
- Разные clocks managed и assisted → не объединять их под обещанием CPU budget.
- Истёкший claim после недели → понятный recovery, не скрытый новый claim;
  этот known limit не маскировать успешным file-only тестом.

## Migration Plan

Сначала refusal regression и inventory, затем новые contracts/authoring,
time transitions, parking/admission/control и UX. Legacy authoring, package
refs, state и frozen runners сохраняются; новые примеры выбирают новый marker
явно. Main specs/glossary синхронизировать с фактической реализацией, не с
одним наличием этого плана. Обновление binary не меняет начатые Runs; rollback
на старый reader не обещается для authority с новой state edition.

Результаты связать с `make-project-launch-workflow-neutral` и незавершёнными
dialog changes без закрытия их других критериев. Release и изменение
пользовательского полигона требуют отдельного решения. По OpenSpec propose
этот planning turn не изменяет код; apply начинается новым запросом владельца.

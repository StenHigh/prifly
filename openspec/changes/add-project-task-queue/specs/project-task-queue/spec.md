## Purpose

Определяет локальную очередь проекта: устойчивую identity задачи до Run, атомарное владение, связь с reviewed execution и восстановление после сбоя без повторного старта или потери истории.

## ADDED Requirements

### Requirement: QueueTask существует до Run и сохраняет исходный запрос

Project queue SHALL быть opt-in и принадлежать одному project/authority. QueueTask SHALL иметь устойчивую identity, исходный закрытый `TaskInput/1` с exact bytes и provenance, монотонную revision редактируемого содержания, priority и устойчивый порядок добавления. Прежние revisions SHALL сохраняться. TaskInput, QueueTask, RunBrief, Run и Git/workspace claim MUST оставаться отдельными понятиями; их соответствия SHALL быть внесены в канонический словарь.

Очередность SHALL сравнивать сначала большую priority, затем меньший порядок добавления, затем identity. Редактирование request/priority SHALL требовать current task revision. Прежняя revision, другая authority либо неподходящий владелец MUST отказывать без частичной записи.

#### Scenario: Одноимённые задачи
- **WHEN** добавлены две задачи с одинаковым title и priority
- **THEN** они сохраняют разные identities и exact requests, а claim-next стабильно выбирает более раннюю ready запись

#### Scenario: Правка после старта
- **WHEN** владелец меняет содержание задачи после admission Run
- **THEN** новая revision не меняет request, binding, typed inputs и evidence уже начатого Run

### Requirement: Захват задачи атомарен и воспроизводим

`ready`, `claimed` и `done` SHALL описывать локальную обработку QueueTask. QueueTaskClaim SHALL закреплять явного owner и монотонное поколение владения; он MUST NOT быть Workspace claim или разрешением исполнения. Claim-next SHALL атомарно выбирать одну ready задачу и закреплять её. Две одновременные команды MUST NOT получить один действующий claim.

Каждая mutation SHALL иметь durable command identity. Точный повтор SHALL возвращать прежнюю квитанцию/результат; другой payload с той же identity MUST отказывать без нового claim, intent или Run. Повтор старой квитанции MUST NOT восстанавливать уже завершённое владение. Изменение владения SHALL инвалидировать прежнее поколение.

#### Scenario: Одновременный next
- **WHEN** два hosts одновременно вызывают claim-next в одной очереди с двумя ready задачами
- **THEN** они получают разные задачи и claims; task revision и исходный request не меняются от захвата

#### Scenario: Повтор и конфликт
- **WHEN** одна command identity повторена сначала с прежним payload, затем с другим owner или request
- **THEN** первый повтор возвращает прежний результат, второй отказывает без mutation

### Requirement: QueueLaunchIntent закрывает crash windows reviewed start

Перед первым потенциальным эффектом queue-bound start SHALL устойчиво записывать QueueLaunchIntent с task revision, owner/поколением claim, exact reviewed selection/digest, command identity и предназначенным Run ID. Reserved intent без начатого dispatch SHALL отличаться от intent с возможным исполнением. Claim сам по себе MUST NOT создавать intent исполнения или Run.

Run admission и association с intent/QueueTask SHALL фиксироваться атомарно. QueueTaskBinding Run SHALL сохранять task identity/revision, request digest, claim generation и intent identity. Accepted association MUST NOT переписываться последующими правками. Повтор после известного admission SHALL находить тот же Run; неизвестный dispatch MUST требовать reconcile и MUST NOT повторять preflight, программы или start молча.

#### Scenario: Сбой после захвата до старта
- **WHEN** host завершился после claim, но до QueueLaunchIntent
- **THEN** claim остаётся записанным; другой host не получает его по таймеру и не запускает работу автоматически

#### Scenario: Сбой между dispatch и admission
- **WHEN** intent уже допускает возможный эффект, но подтверждённого Run нет
- **THEN** retry показывает необходимость reconcile и не создаёт второй intent/Run или повтор программы

#### Scenario: Сбой после admission до ответа CLI
- **WHEN** Run и association записаны, а host не получил ответ start
- **THEN** повтор/reconcile находит exact существующий Run; отдельного окна orphan association нет

### Requirement: Reviewed launch закрепляет revision задачи и typed inputs

Prepare SHALL оставаться read-only и включать выбранную QueueTask revision, request digest и claim generation в exact launch review. Start SHALL повторно проверять их до executable dispatch и в admission. Смена task revision между prepare/start MUST отказывать до исполнения. На время незавершённого start intent редактирование выбранного request/priority SHALL быть запрещено; после admission новая revision допустима без изменения sealed Run.

Original request SHALL сохраняться как exact evidence независимо от typed inputs выбранного launch. Свободный текст MUST NOT молча подставляться в произвольный JSON port. Обязательные inputs, catalog/questionnaire, prepare digest и executable authorization SHALL сохранять existing contract.

#### Scenario: Задача изменена после prepare
- **WHEN** task revision изменена до start с прежним review digest
- **THEN** отказ происходит до dispatch, claim/workspace side effects и создания Run

#### Scenario: Файловый launch требует typed JSON
- **WHEN** claimed задача содержит свободный request, а launch объявляет JSON input
- **THEN** runner сохраняет request отдельно, получает/проверяет объявленный typed input и показывает оба источника в review

### Requirement: Продолжения сохраняют одну задачу и самостоятельные Runs

Continue/recover/fork от queue-bound source SHALL сохранять ту же QueueTask identity. Новая admission SHALL требовать действующий task claim и reviewed current task selection; source и его bindings/outcomes/evidence MUST NOT меняться. Повторное продолжение от того же source SHALL создавать самостоятельные associations только по existing явному разрешению повторного продолжения.

#### Scenario: Partial и два продолжения
- **WHEN** partial Run продолжен, продолжение отменено, затем создан ещё один continue
- **THEN** все Runs относятся к одной QueueTask, имеют разные identities и собственные неизменные outcomes, inputs и evidence

### Requirement: Release и reconcile не перекрывают живую либо неизвестную работу

У QueueTaskClaim MUST NOT быть автоматического timeout requeue. Release/complete SHALL требовать current owner/claim generation/task revision, отсутствие незавершённого intent и доказуемо terminal состояние всех associated Runs без live/pending/uncertain obligations или decisions. Проверка Run revisions и смена queue state SHALL быть атомарны. Старый queue-bound Run MUST NOT снова получить работу после завершения его claim без нового допустимого владения/launch.

Reconcile SHALL находить известный admission, а unresolved dispatch сохранять блокирующим. Разрешение отсутствующего Run SHALL быть отдельным явным решением владельца с записанным основанием и проверкой, что прежний dispatcher уже не работает. Для возможного эффекта SHALL требоваться явное подтверждение установления его исхода; отсутствие Run и истечение времени сами по себе MUST NOT считаться таким подтверждением. Поздний admission от разрешённого/отозванного intent SHALL отказывать атомарно.

#### Scenario: Неопределённый Run
- **WHEN** associated Run uncertain либо имеет pending decision, а host lease истёк
- **THEN** release/reclaim отказывает и оставляет владение и историю прежними

#### Scenario: Явное восстановление после потери dispatch
- **WHEN** прежний dispatcher завершился, owner проверил возможные эффекты и записал допустимое reconcile решение при отсутствии Run
- **THEN** intent закрывается с этим evidence; поздний start не допускается, а дальнейший release/start требует обычных current CAS и review

### Requirement: Done означает только явное локальное завершение

Успех отдельного Run MUST NOT автоматически переводить задачу в done. Явный complete SHALL фиксировать решение владельца о завершении локальной обработки и основание, сохраняя неуспешные Runs и прежние revisions. Done MUST NOT утверждать external acceptance, issue close, publication, push или merge; эти действия требуют собственных механизмов.

#### Scenario: Успешный Run без внешней поставки
- **WHEN** Run завершён успешно, но owner ещё не выполнил complete
- **THEN** задача остаётся claimed; subsequent complete отмечает только локальное решение и не меняет внешнюю систему

### Requirement: Legacy и копии сохраняют границы authority

Проекты и Runs без queue SHALL продолжать работать без автоматического создания queue, переписывания state или миграции. Queue SHALL храниться в local authority, а не shared project YAML/Git. Fresh clone/bootstrap SHALL получать пустую неинициализированную queue; копирование shared profile MUST NOT переносить claims. Скопированная authority SHALL сохранять existing relocation/foreign-authority guards; readonly inspection MUST NOT давать право продолжить её claims или execution.

#### Scenario: Clone общего проекта
- **WHEN** shared profile скопирован на другую машину и выполняется обычный project init
- **THEN** прежние queue/claims/Runs не импортируются, а запуск без queue сохраняет прежний contract

### Requirement: Закрытие capability подтверждается released и live evidence

E2-A1–A9 SHALL проверяться CLI concurrency/crash/recovery fixtures и semantic tests. E2-A10 SHALL отдельно подтверждать те же CLI fixtures на реально released binary с version/digest и живой generated runner на поддерживаемых hosts: явный выбор launch, prepare, ожидание ответа и отсутствие повторного start. Development build, synthetic transcript и schema validation MUST NOT заменять released/live qualification. До этого issue #5 и product gate MUST оставаться открытыми.

#### Scenario: Инженерные tests прошли до выпуска
- **WHEN** код и полный gate прошли на development build, но released/live qualification ещё отсутствует
- **THEN** evidence отдельно называет инженерный PASS и незавершённый E2-A10; capability не объявляется полностью квалифицированной

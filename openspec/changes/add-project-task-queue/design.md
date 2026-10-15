## Context

Мотивация — proposal.md. Queue отсутствует; имеются `TaskInput/1`, read-only task prepare, immutable Run inputs, versioned project questionnaire/launch review, durable authority commands и Start с атомарной записью Run/одного authority control. Workspace claim уже связывается с Run в этой транзакции. Последние public state/read — `/43`; новые поля нельзя вписать в старые closed bundles.

Current source sets: `project-task-queue` создаётся этим change; CLI — `openspec/specs/cli-protocol/spec.md`, contracts — `openspec/specs/published-contracts/spec.md`, canonical shape — `internal/flow/protocol.schema.json`; общая терминология — specification-governance/terms.md. Existing ownership не переносится.

## Goals / Non-Goals

**Goals:** correctness в гонках и crash windows, точное владение и requests, независимые Run associations, понятное ожидание в runner. Queue работает без Git и появляется только от явной mutation queue.

**Non-Goals:** lease-based scheduling, автоматическое исполнение next, provider write-back, UI очереди, изменение исходов Run, новые user-facing YAML сущности workflow. Локальное done — решение владельца о локальной обработке, не внешний product acceptance.

## Decisions

### 1. Один durable authority ledger, отдельный QueueTaskClaim

Queue хранится в отдельном authority key `task-queue` со schema version `project-task-queue/1`, использует existing authority receipts/command identity. Файловый JSON backlog и отдельная БД отвергнуты: они не дают общей транзакции с Run admission. Storage schema мигрировать не требуется ради пустой queue; чтение отсутствующего key возвращает uninitialized/empty без записи.

Канонические понятия: QueueTask (устойчивая задача), QueueTaskClaim (owner + generation), QueueLaunchIntent (один reviewed start), QueueTaskBinding (immutable связь конкретного Run). Их точные Go/JSON соответствия добавляются в словарь с реализацией, без переименования TaskInput или WorkspaceClaim.

Task revision меняется только при редактировании request/priority; claim generation меняется при смене/завершении владения. Это разные CAS: claim-next не делает prepared content новой revision. Priority — целое в явно проверяемом диапазоне JSON safe integers; большее значение раньше, затем монотонный insertion order, затем Task ID. Task identity выводится из authority и command identity добавления, не из title или внешнего issue ID.

Каждая revision хранит exact UTF-8 bytes закрытого TaskInput (lossless JSON string/byte encoding в queue record), digest и priority. Валидация/чтение input файла выполняются до transform; внутри него нет blob/file access. Inline immutable requests позволяют CAS-отказу не создавать побочные artifact records. List отдаёт компактные metadata, show — request/history/claims/intents/associations. Применяются явные существующие document limits с отказом, без обрезания истории.

### 2. Атомарные claim и scoped command replay

Claim-next — один pure authority transform: выбрать ready запись, назначить owner/generation, вернуть квитанцию. Payload включает owner и параметры выбора; CAS owner-sensitive команд проверяет task revision + generation. Store receipts различают exact replay и конфликт той же command identity. Даже поздний replay возвращает исходную квитанцию, но не оживляет старый claim.

`ready` означает отсутствие владельца и незавершённого intent; `claimed` сохраняется после admission и terminal Runs до явного решения; `done` задаётся explicit complete с reason. Release возвращает в ready, сохраняя requests и все associations. Complete закрывает claim и делает done; автоматического перехода от success Run нет. Незавершённый intent отдельно помечает processing/reconcile состояние claimed задачи.

### 3. Intent до эффектов, association вместе с admission

Reviewed selection содержит Task ID/revision, request digest, owner и claim generation. Intent дополнительно закрепляет stable start command identity, ожидаемый Run ID, exact launch digest/summary и dispatcher identity. Intent ID выводится из authority/command, поэтому повтор не создаёт новый.

Перед preflight/programs, workspace claim и Run admission launcher проверяет selection, резервирует intent, затем устойчиво отмечает возможный dispatch. Prepare не резервирует intent и не выполняет эти эффекты. Редактирование request/priority при open intent запрещено, поэтому start не действует от устаревшего request между проверкой и admission.

Run creation атомарно переводит intent в admitted и добавляет association. Для этого local получает runtime-owned pure дополнительную authority mutation с per-task/intent CAS в той же транзакции, где уже меняются Run и Workspace claim/control grant. Не объединять queue с claims key и не делать storage driver частью runtime API. Независимые записи queue не должны теряться от обновления чужой задачи: transform читает текущую queue в write transaction и проверяет свой scoped CAS. Все связанные изменения откатываются вместе при любом отказе; receipt replay не вызывает reducers повторно.

Queue binding попадает в immutable start payload/dedup identity и Run state. При exact retry сначала читается durable intent/результат, а не повторяется preflight. Итог project start сохраняется до выдачи stdout, чтобы известный завершённый вызов воспроизводился. Если ответ потерян до его сохранения, reconcile сообщает известный admission и exact Run; неизвестный ответ не подделывается под прежнюю успешную квитанцию.

| Crash window | Сохранённый факт | Восстановление |
|---|---|---|
| До claim commit | Нет claim | Повтор той же команды выполняет/читает единственный claim |
| После claim до intent | Claimed Task, Run отсутствует | Show/replay возвращает claim; явный release при безопасном состоянии |
| Reserved intent, dispatch не отмечен | Известно отсутствие разрешённого dispatch | Reconcile после завершения dispatcher фиксирует not-started; без автоматического requeue |
| Dispatch отмечен, Run отсутствует | Возможны preflight/claim effects | Reconcile required, без повторного исполнения; owner устанавливает исход возможных эффектов |
| Во время admission | Run + queue + Workspace control одна транзакция | Либо всё записано, либо ни Run, ни association не записаны |
| После admission до stdout | Admitted intent + Run + association | Найти тот же Run; exact известный result воспроизвести, иначе показать восстановленный факт |

Идемпотентность не превращает неизвестный executable dispatch в безопасный retry. При известном отказе до эффектов можно записать отказ intent; возможные эффекты требуют reconcile. Production test backdoors/crash environment flags не вводятся: deterministic unit fault injection остаётся в test adapters, CLI replay использует процессы, SQLite contention и контролируемые declared fixture programs с внешними барьерами.

### 4. Reconcile имеет явную границу знания

Reconcile без resolution сначала читает intent/Run и ничего не запускает. Known admission проверяется по authority, Run ID и immutable binding. При отсутствии Run владелец явно выбирает not-started только для reserved intent либо effects-resolved для возможного dispatch, указывает reason и подтверждение установленного исхода эффектов. Перед разрешением проверяется завершение прежнего dispatcher; неоднозначный/live процесс не считается завершённым. Проверка процесса выполняется до write transaction и фиксируется как evidence, не как lease timeout.

Resolve CAS инвалидирует intent для позднего admission. Живой dispatcher не разрешается чужой сессией; после его завершения возможные дочерние/внешние эффекты не объявляются исчезнувшими только из-за PID. Для dispatching требуется отдельное утверждение владельца об установленном исходе. Reconcile не создаёт Run и не снимает runtime uncertain obligations.

### 5. Release/complete и повторное оживление Run

Снимки всех associated Runs читаются и проверяются до transform, их exact versions пинятся внутри authority transaction. Open intent, нетерминальный Run, pending decision/acceptance или uncertainty запрещают release/complete. Один временно недоступный/повреждённый Run также не считается terminal.

Run reopen и любой путь создания продолжения сохраняют queue guard. Reopen старого Run после окончания его claim отказывает; актуальное новое владение допускает новый reviewed continue/fork с тем же Task ID, не переписывание старого binding. Проверка queue и release Run pins сериализуют гонку reopen/release. Обычные шаги уже активного Run не требуют current content revision: последующая правка task не меняет sealed inputs. Cancellation, result settlement и диагностика должны оставаться доступны.

### 6. Exact prepare, typed inputs и generated runner

CLI добавляет selection flags `--task-id`, `--task-owner`, `--task-revision`, `--task-claim-generation` к questionnaire/prepare/start/continue. Queue selection входит в review digest. Недостающие/устаревшие значения отказывают до dispatch; source-bound continue не может молча потерять или заменить Task identity.

Original TaskInput хранится независимо от declared typed input выбранного workflow. Runner читает show, сохраняет exact request, показывает launch catalog, ждёт фактического выбора/required answers и только затем использует existing typed input validation + prepare + reviewed start. Он не угадывает JSON schema из свободного текста и не превращает claim в execution authorization. При продолжении показываются current queue request и отдельно inherited/explicit launch inputs; совпадение их смысла не выдумывается.

Native finite questions следуют existing host tool contract. Общий runner не получает отраслевые review/commit правила и не запускает следующий next автоматически. Tracked owner runners не перезаписываются: новые templates и exact recognition/update tests выпускаются вместе; qualification создаёт fresh generated runners в isolated projects.

### 7. Opt-in contracts и legacy

Queue records/reads получают собственные closed `/1` definitions. Только Run с `queue_task` запечатывается `core-state/44`, его RunView — `core-read/44`; binding обязателен для этой версии, отсутствие binding не поднимает обычный Run с его прежней версии. Queue review — `project-launch-summary/4`, queue start result — `project-start/4`; обычные `/3` ответы остаются прежними. Writer reachability, old closed schema rejection, real read output и public bundle generation проверяются вместе.

Source map получает новую строку project-task-queue; glossary и TestGlossaryBindings обновляются вместе с кодом. Capability declaration/author index и нужный authoring reference показывают новые queue/selection fields, минимальную версию и примеры отказов. Старые bundles/архивы не меняются.

Queue — machine-local authority state. Shared profile clone/init не переносит её; fresh authority остаётся пустой до queue add. Скопированная authority подчиняется прежним relocation guards и foreign-authority checks: readonly inspection не разрешает её исполнять. Rollback на binary без `/44` не интерпретирует queue Runs как legacy.

## Risks / Trade-offs

- [Unknown pre-Run effects] → durable intent + отказ blind retry, явное owner reconcile с основанием; отсутствие Run не доказательство отсутствия эффектов.
- [Составная admission] → атомарные multiple authority reducers, scoped CAS, rejection/replay/rollback tests и purity guard; no I/O в reducers.
- [Release против reopen] → coupled queue guard и exact Run pins, concurrency regression.
- [Рост inline request history] → existing explicit document limits; компактный list, полный show по запросу, никакого скрытого удаления evidence.
- [Потеря native question wait] → live host qualification, не только сравнение template strings.
- [Неполная release qualification] → инженерная готовность отдельно от E2-A10; issue/archive ждут фактического released/live evidence.

## Migration Plan

1. Commit planning artifacts, затем engineering Run в worktree по всем engineering tasks.
2. Реализовать opt-in queue и schemas без миграции legacy, прогнать targeted CLI/tests и full staged gate, commit/merge в main.
3. Зафиксировать candidate SHA; по существующему release процессу пройти verify/qualify exact SHA. Push/tags принадлежат владельцу; локальные checks их не заменяют.
4. После выпуска выполнить reusable CLI replay на exact released binary и live generated runner на каждом поддерживаемом host (`codex-app`, `codex-cli`, `claude-code`), сохранить version/digest, selection/question wait и admission count evidence.
5. Только после E2-A1–A10 PASS sync/archive с удалением AIF plan и закрытие #5. При откате старый reader сохраняет guards и отказывает `/44`, не меняет историю.

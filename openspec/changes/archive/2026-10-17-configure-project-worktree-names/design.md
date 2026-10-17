## Context

Мотивация и scope — [proposal.md](proposal.md), исходный запрос — [issue #10](https://github.com/StenHigh/prifly/issues/10). Комментариев к issue на момент разбора нет. Это план по текущему checkout, без нового production запуска для воспроизведения.

Проверенные точки реализации:

- `internal/runtime/worktrees.go`: `ClaimWorktreeWithOutcome` выводит claim ID из authority/command, жёстко задаёт hash path/branch, записывает `preparing` в `ApplyAuthority`, затем создаёт Git worktree и активирует claim. Duplicate receipt возвращает записанный claim. `ClaimWorktrees` имеет отдельный batch путь, который не должен получить скрыто иные гарантии.
- `WorktreeClaim` уже хранит `path`, `branch`, `run_id`, generation и physical identity. `ClaimWorkspacePath` и cleanup используют запись; `createWorktree` сейчас создаёт detached tree, затем переключает его на новую ветку. Создание имени, collision cleanup и branch ownership требуют отдельного протокола.
- `cmd/prifly/project.go`: parser принимает только profiles `/2` и `/3`, список верхних полей закрыт; `title` — человекочитаемый текст, а не безопасный project slug.
- `cmd/prifly/project_start.go`: prepare вычисляет digest до claim; continuation/recovery могут передавать старый claim, только иначе создают новый. Existing summary `/3` и queue summary `/4` не имеют naming. `internal/projectwire/queue.go` уже отделяет изменчивый admission preview от review digest; аналогично отделяется availability naming preview.
- `schemas/authoring/project-profile-v3.schema.json` и его manifest — опубликованная закрытая форма. `authority-claims/3`, `foundation-claims/2`, legacy readers и queue-bound outputs требуют проверки совместимости.

Current source sets перечислены в proposal и deltas; их ownership не меняется. Статус roadmap/qualification этим планом не повышается.

## Goals / Non-Goals

**Goals:**

- Дать команде одну tracked политику и явные источники строк без скрытой зависимости от tracker/AI Factory.
- Сделать имя полезным человеку, сохранив claim identity, generation, receipt и Run binding.
- Разрешить одновременные независимые старты одной задачи и сохранять корректность при прерывании между reservation и Git effects.
- Оставить read-only prepare и узкие write-транзакции.

**Non-Goals:**

- Разные templates для directory/branch, произвольные branch prefixes или workspace roots, rename существующего дерева, глобально монотонный task counter.
- Автоматическое заполнение task metadata из queue/issue/brief, tracker integration, новый task entity, новая workflow input schema внутри Core.
- Distributed authorities одного mutable scope, атомарная транзакция SQLite+Git+filesystem, исполнение задачи этим change.

## Decisions

### 1. Общая декларация в явном profile `/4`

Форма общего профиля `/4`:

```yaml
# yaml-language-server: $schema=../schemas/authoring/project-profile-v4.schema.json
schema_version: prifly-project-profile/4
title: SMS Place
worktree_naming:
  template: "{project_name}-{task_source}-{task_id}"
  bindings:
    project_name: {value: smsplace}
    task_source: {input: task, pointer: /source}
    task_id: {input: task, pointer: /id}
# hosts/packages/launches — обычные объявления проекта, как в /3
```

Пример **уже объявленного выбранным WorkflowRevision JSON input** `task`:

```json
{"source":"trucker","id":"default-123"}
```

Его передают существующим `--input task=task.json` либо `--input-ref task=ARTIFACT_REF.json` одинаково в prepare и start, вместе с остальными required аргументами. Core не создаёт port или schema по имени `task`. Пример в authoring guide должен включать полноценный workflow с объявленным typed port и фактические команды. Имена placeholders произвольны: `project_name` тоже явная константа, а не basename repository или `title`.

Binding — closed union ровно `{value: string}` либо `{input: port, pointer: string}`. Источник — effective значение declared root input выбранного launch после штатных default/configuration overrides и чтения ArtifactRef. Только JSON input и конечное string value; pointer по RFC 6901, `""` допустим. Отсутствие отличается от JSON null, оба не являются строкой. Blob читается до authority transform. Launch без нового дерева не разрешает bindings и не требует naming-only inputs; structural validation shared блока выполняется всегда.

Один общий блок означает одну input convention для launches, создающих named worktree. Если такой launch не объявил нужный port, он получает ранний отказ. Per-launch policy и неявный fallback в hash names не вводятся.

Альтернативы: `local.yaml` не передаёт настройку команде; launch-level блок дублирует общую конвенцию; расширение закрытого `/3` меняет прежний контракт. `/4` наследует все ветви `/3` (neutral compilation, hosts, extensions, queue, continue/recover), поэтому equality checks по `/3` нужно заменить осознанной проверкой поддерживаемой семьи, сохранив legacy `/2`.

### 2. Ограниченная grammar, без преобразования данных

- Template: 1..256 ASCII символов; literals `[a-z0-9-]`, placeholders `{[a-z][a-z0-9_]{0,31}}`, до 16 разных bindings. Повтор одного placeholder допустим, literal braces/expressions запрещены. Map bindings точно равна множеству использованных placeholders.
- Каждое разрешённое значение: 1..64 ASCII символа, regex `^[a-z0-9]+(?:-[a-z0-9]+)*$`. Unicode, whitespace, uppercase, separators, dots, control characters и shell/Git metacharacters отклоняются с указанием binding. Это осознанно узкий portable формат; automatic lowercase/slugification отсутствует.
- Разрешённая основа: тот же regex, до 96 bytes; portable reserved device basenames также запрещены. Candidate дополнительно проверяется как branch name до любой Git mutation; suffix добавляется после основы. Без настройки прежний hash naming не прогоняется через эту новую template grammar.
- Namespace: `base`, далее `base-%02d` для 2..9999 включительно; после 99 — `-100`, после 999 — `-1000`. Всего 9999 кандидатов, длина до 101 bytes. Выбирается первый свободный; после полного удаления допустимо повторное использование имени. Это не счётчик запусков.
- Path: только `ClaimRoot/<candidate>`, branch: только `prifly/<candidate>`. Проверяется confined root и отсутствие symlink ancestors/target, существующий любой target считается занятым. Значения не формируют flags, executable или Git revision; Git вызывается argument vector с проверенным ref и обычными end-of-options fences.

Более широкая slugification приятнее при вводе, но скрывает коллизии разных исходных значений и требует отдельного compatibility контракта. Первая версия выбирает явные значения и понятную диагностику.

### 3. Prepare проверяет policy, финальный suffix выделяет start

В новом summary `worktree_naming` содержит reviewed template/bindings, resolved values/base, root/prefix, policy `first-free/1` и `max_candidates=9999`. Отдельный `worktree_name_preview` содержит candidate path/branch и `reserved=false`, исключён из digest так же, как admission preview. Ни обзор, ни проверка доступности не создают lock/reservation.

Digest включает reviewed naming и существующие launch данные; profile/input change даёт `project_start_stale_launch`. При unchanged policy изменившаяся доступность разрешает следующий suffix. Это позволяет двум проверенным стартам успешно создать разные деревья. Альтернатива — закрепить exact suffix в review и требовать re-prepare при каждой коллизии — делает конкурирующие запуски лишний раз зависимыми друг от друга.

Для handover summary показывает recorded claim/path/branch/generation, а naming policy нового дерева не разрешается и не применяется. Если старый claim уже released и continuation законно создаёт новый, используется обычная проверка нового naming. Recover сохраняет штатный `recover_workspace_released`.

### 4. Stable request, durable reservation и effects вне transform

Существующий single-resource claim API получает validated naming request (base + policy); CLI передаёт его только для нового `worktree`. Runtime самостоятельно валидирует request. Batch `ClaimWorktrees` и прямой claim без naming сохраняют существующий hash путь; расширение batch API не входит в срез.

Протокол для named claim:

1. Подготовить canonical repository/base commit и naming request. Semantic payload содержит исходный validated request, **не** динамически выбранный suffix, PID, clock или availability snapshot. Claim ID выводится прежним способом. До нового выделения проверить существующий receipt/claim: тот же semantic request возвращает его, другой payload даёт idempotency conflict. Изменение availability не участвует в identity команды; изменение naming values участвует. Для повтора уже принятого Project Start использовать штатные recorded command/Run semantics, не пытаться создать ещё один claim по новому состоянию namespace.
2. Вне write-транзакции получить bounded inventory занятых targets и Git refs, включая directory/file ref conflicts, и подготовить данные проверки кандидатов. Не сканировать содержимое всех Run. Перед effects повторно проверить selected path/ref. Путь уникален для всей authority, ref — для physical common repository; незанятое имя другого repository всё равно может конфликтовать на общем ClaimRoot.
3. Чистая authority команда сопоставляет inventory со snapshot claims и выбирает первый свободный candidate; атомарно записывает `preparing` claim с фактическими path/branch и naming metadata. Текущие reservations побеждают stale inventory, concurrent claims получают разные names. Generation/ownership rules и capacity остаются прежними. Receipt связывает stable external command с claim ID; allocator subcommands именуются детерминированно внутри него.
4. После commit caller перечитывает выбранный claim. Effects сериализуются per claim посредством process-scoped guard вне transform; guard не заменяет durable ownership и не живёт внутри Git workspace. Git ref создаётся no-overwrite primitive, filesystem target также создаётся без замены чужого объекта. Нельзя использовать force/reset для разрешения коллизии. Записываются доказательства реально выполненных эффектов и directory identity, после чего обычная активация переводит claim в `active`.
5. Если external actor занял candidate, и доказано, что предыдущие собственные effects отсутствуют либо полностью очищены, allocator записывает collision observation и чистым CAS переводит **тот же** preparing claim на следующий свободный candidate. Количество попыток ограничено namespace. Нельзя оставлять собственную ветку предыдущего candidate и считать её внешней коллизией. При неизвестном результате Git/cleanup progression останавливается с диагностируемым preparing claim; автоматический новый suffix запрещён до reconciliation. Public command receipt и claim ID не меняются при внутренних retries.
При named admission устаревшие claims/package/ownership observations до reducer
возвращаются как internal retry без receipt и без Run: до 8 полных повторных
проверок того же request. Семантические отказы, изменившийся source Run и ownership
конфликты не обходятся. Перед Start exact trusted package inventory закрепляется
заново, если его успел импортировать другой процесс. Initial Drive повторяет только
эти transient observation conflicts по уже запечатанному Run; drivers не получают
общей блокировки. Завершённый named Project Start хранит original response в command
receipt над неизменёнными claim bytes, без нового Run поля или отдельного ledger.

6. Duplicate caller перечитывает durable state под тем же claim ID. Active/released даёт прежний результат; preparing даёт existing pending diagnostic или безопасно продолжает доказанный preparation protocol под тем же guard. Он не имеет права создать второе дерево или откатить claim другого caller. Неопределённый dispatch запуска Run обрабатывается штатной сверкой receipt/Run, не освобождением дерева.

Транзакция не делает stat, Git, чтение blobs и cleanup. Внешние observations — inputs, reservation и CAS — решение authority, no-overwrite effects — защита от внешней гонки. Не требуется общая блокировка всех repositories/Run drivers. Reservation не означает доказанную физическую ownership: crash после Git эффекта до записи evidence оставляет unknown, не право удалить чужую ветку по одному совпадению имени.

### 5. Сохранённая запись определяет lifecycle

Path/branch остаются existing полями claim. Naming metadata хранит policy/base, выбранный ordinal и preparation progress/effect evidence; ссылки на пользовательские входы не нужны admission и cleanup. Полные входы задачи не копируются в claim. Внутренние evidence fields выбираются при реализации протокола, но должны различать known-created и unknown branch/tree effects, чтобы не удалять чужой ref.

Проверить все читатели, включая admission, Run claim binding, continuation/recovery, session handoff, heartbeat/release, doctor и rollback start: запрещено восстанавливать path из ID или повторно рендерить template. Handover меняет generation/Run binding, не physical names. Legacy claims остаются без naming metadata. Safe release проверяет recorded path/device/inode и принадлежность удаляемой branch; неизвестный или подменённый ref не удаляется. Ошибка cleanup сохраняет исходную причину start и ссылку на оставшийся claim.

### 6. Отдельные версии без нового Run contract

Планируемые version boundaries (перед реализацией сверить с актуальной лестницей, если появились другие changes):

| Форма | Новая граница | Совместимость |
|---|---|---|
| Tracked profile | `prifly-project-profile/4`, authoring `project-profile-v4` | Existing `/2` и `/3` не меняются; без naming `/4` ведёт себя как `/3` |
| Prepare | `project-launch-summary/5` | Новое naming или handover named claim, включает existing queue selection; для обычного hash пути сохраняются `/3` или `/4` |
| Start result | `project-start/5` | Создание/передача named claim; фактический claim/path/branch и existing queue data |
| Authority claims | `authority-claims/4` | Новый writer при первом named allocation; reader принимает `/1`..`/4`, legacy записи не приобретают naming metadata |
| Claim list view | `foundation-claims/3` | Для нового authority contract; прежняя форма без нового metadata сохраняется для старого contract |

Новые closed claim/review/result schemas публикуются рядом с прежними. Выбор версии учитывает не только текущий shared блок, но и metadata передаваемого claim: удаление policy не позволяет вложить named allocation в старую closed форму. Naming — metadata authority, без нового Run field: `core-state`/`core-read`, StepDefinition/WorkflowRevision/session contracts не повышаются ради имени. Queue launch сохраняет штатную queue state/read boundary. Если реализация неожиданно требует Run field, это требует пересмотра design до кода, не silent расширения historical shape.

`authority-claims/4` обязателен для безопасного отказа старого binary до Git effects. При добавлении обычного hash claim в уже обновлённую authority writer сохраняет `/4`; downgrade/автоматическая обратная миграция отсутствуют. Typed unsupported-contract refusal добавляется новому reader; older binary как минимум отвергает неизвестную версию по existing guard.

### 7. Refusal surface и доставка автору

| Случай | Код |
|---|---|
| Неверная template grammar, поля или mismatch placeholders/bindings | `project_worktree_template_invalid` |
| Binding с неизвестным/non-JSON port или неверным pointer syntax | `project_worktree_binding_invalid` |
| Declared source/pointer отсутствует | `project_worktree_value_missing` |
| Значение не string/unsafe, итоговая основа слишком длинная/reserved | `project_worktree_value_invalid` |
| Все кандидаты заняты | `claim_name_exhausted` |
| Уже записанная preparation не доказала завершение | `claim_preparation_incomplete` |
| Profile/input naming изменились после review | existing `project_start_stale_launch` |

Existing profile/ownership/generation/idempotency/cleanup refusals сохраняются. Runtime коды — typed `Fault`, не строки `errors.New`/`fmt.Errorf`; public Problem сохраняет code и контекст. Ошибка общего validation declared input может появиться раньше naming, если нарушен сам InputPort contract.

Объявить `project_worktree_naming` в existing capability inventory с минимальной поддерживаемой ревизией, добавить ровно соответствующую строку в author index. Расширить `project-profile-authoring-reference.yaml` полноценным `/4` примером и объяснить `/2`/`/3`, policy preview, все границы версий, data sources и validation. Runner/read handoff использует фактический путь, без проекта-специфического переименования. Обновление upstream packages не требуется.

## Risks / Trade-offs

- Prepare не резервирует имя → вывод явно сообщает advisory candidate, digest фиксирует пространство и collision policy, result — окончательный suffix.
- Git/filesystem изменяет внешний actor → no-overwrite primitives и проверка identity; неизвестные effects оставляют owned preparing record и требуют диагностики, не скрытого нового claim.
- Переход после collision требует нескольких authority команд → stable request/claim ID, bounded retries, per-claim effect guard и interruption tests на каждой границе.
- Shared profile с разными input conventions → named worktree launch явно объявляет нужные ports; checkout/без Workspace/handover их не требуют.
- Узкий ASCII формат отвергает привычные task IDs с underscore/uppercase → понятный refusal и явные безопасные значения на стороне declared project input; транслитерация не происходит.
- Старый binary не понимает новый authority contract → explicit upgrade, сохранённый backup и отказ вместо destructive rollback.
- Изменение всех `/3` equality branches может сломать unrelated launches → regression matrix `/2`/`/3`/`/4`, command-only, queue и call extensions.

## Migration Plan

1. Сначала выполнить tasks по contract/negative cases и реализовать opt-in путь с legacy regression. Не менять реальный `.prifly/project.yaml` этого репозитория ради демонстрации.
2. Выпустить новые schemas, reader/writer, capabilities и авторский guide вместе. До первого named claim `/4` без блока не требует нового claim record.
3. Владелец проекта явно меняет shared profile на `/4`, добавляет policy и объявленные inputs своего workflow. Существующие claims/Runs не меняются.
4. После включения можно удалить policy для будущих запусков: hash naming возвращается, уже созданные named claims продолжают жить по записанным полям. Authority contract назад не понижается; старый binary для неё не поддерживается.

## Acceptance Mapping

| Критерий issue #10 | Где проверять |
|---|---|
| 1: shared настройка, явные значения, ранняя диагностика | Tasks 1.1–1.3, 2.1–2.2 |
| 2: первое имя, suffix, refs/path и concurrency | Tasks 3.1–3.3, 5.1 |
| 3: duplicate и handover | Tasks 3.4, 4.1–4.2, 5.2 |
| 4: сохранённые path/branch и cleanup | Tasks 3.2, 4.1–4.3, 5.3 |
| 5: confinement/ref safety | Tasks 1.2–1.3, 3.3, 5.1–5.3 |
| 6: legacy и parallel worktrees | Tasks 1.1, 4.1–4.2, 5.4 |
| 7: тесты, schema, author guide | Tasks 5.1–5.4, 6.1–6.4 |

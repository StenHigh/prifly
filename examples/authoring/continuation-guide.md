# Как сделать workflow продолжаемым

Руководство для автора workflow и для ИИ-агента, который пишет или запускает
его. Поля показаны в
[`workflow-authoring-reference.yaml`](workflow-authoring-reference.yaml); здесь
объясняется, зачем они нужны и как ими пользоваться. Возможность называется
`workflow_continuation` в `prifly capabilities`; поля требуют WorkflowRevision
**7**, которую лестница авторинга выводит сама по наличию полей.

## Главное правило

Движок исполняет, workflow командует. Pri-Fly не знает, что делает ваш
workflow: он не знает имён ваших стадий, не знает, что ваш артефакт содержит
commit, и не проверяет ничего, чего вы не объявили. Всё, что нужно для
продолжения, объявляет workflow:

- **что сохранить** — `checkpoint`: точку, с которой работу можно продолжить;
- **от чего продолжать** — `continuation`: какие Runs и откуда в них брать
  каждый вход;
- **что проверить** — первая стадия workflow продолжения, обычный шаг.

## Checkpoint: что сохранить

```yaml
checkpoint: {schema_ref: resume_schema}   # схема — ваша
stages:
  build:
    kind: step
    step_ref: build_step
    checkpoint: state                     # этот выход шага и есть checkpoint
```

- Содержимое checkpoint — ваше: commit, версия документа, id записи. Движок
  его хранит и передаёт, но не читает.
- Выход, названный в `checkpoint:`, должен быть JSON с той же схемой, что
  `checkpoint.schema_ref`. Иначе компиляция откажет `invalid_checkpoint`.
- Сделайте этот выход обязательным для вердиктов, после которых хотите
  продолжать, **включая `fail` и `blocked`**: `required_for: [pass, fail,
  blocked]`. Иначе шаг может закончиться, не сообщив checkpoint.
- Последний принятый checkpoint Run — выход последнего по времени принятого
  результата стадии, объявившей checkpoint, во всех вызовах Run. `run status`
  показывает его в поле `checkpoint`.
- Вызываемый workflow может объявить checkpoint только той же схемы, что и
  вызывающий: у Run одна форма checkpoint.
- Шаг, убитый без результата, checkpoint не сообщает — действует предыдущий
  принятый. Файлы, которые такой шаг оставил в дереве, не теряются: дерево
  передаётся целиком (см. ниже).

## Continuation: от чего продолжать

Объявляется в workflow, **который продолжает**, а не в исходном. Исходный
workflow ничего не знает о своих продолжателях.

```yaml
continuation:
  from_workflows: [example:workflow/source]   # какие Runs можно продолжать
  from_outcomes: [partial, rejected]          # с каким исходом
  from_cancelled: true                        # и отменённые Runs (исхода у них нет)
  inputs:                                      # откуда взять каждый вход
    task:    {source_input: task}                              # вход исходного Run
    notes:   {stage: prepare, output: notes, verdict: pass}    # принятый выход шага корневого вызова
    finding: {stage: gate, output: finding, outcome: partial}  # выход стадии call — по исходу
    resume:  {checkpoint: true}                                # последний принятый checkpoint
```

- Каждый вход из `inputs` берётся ровно из одного места. Входы, которых нет в
  `inputs`, подаёт launch обычным образом; подать через `--input` вход,
  который объявлен переносимым, нельзя (`project_continue_input_override`).
- Если стадия исполнялась в корневом вызове несколько раз, берётся последний
  принятый результат. Два результата с одинаковым временем — отказ, а не
  догадка.
- Байты проверяются схемой входа **нового** workflow и запечатываются заново;
  исходные ревизии записываются в provenance.
- Отменённый Run исхода не имеет, поэтому его продолжение объявляется
  отдельно: `from_cancelled: true`. Нужно хотя бы одно из `from_outcomes` и
  `from_cancelled`. Типичный случай — хост убил драйвер посреди Run: принятые
  шаги, checkpoint и дерево с оставленными файлами переходят к продолжению.
- Отменённый Run, который держит неразрешённую execution (например, была
  выдана Attempt шага, меняющего дерево), не продолжается:
  `continuation_source_unsettled`. Сначала `run resolve` — владелец говорит,
  применился ли эффект.
- Технический отказ без исхода не продолжают — его восстанавливают
  (`project recover`).

## Первая стадия: что проверить

Движок не проверяет, что рабочее дерево соответствует checkpoint. Если вашему
workflow это важно, первой стадией поставьте шаг, который принимает
checkpoint входом и проверяет дерево (например, что HEAD содержит записанный
commit). Его вердикт `fail` или `blocked` маршрутизируйте, как любой другой.
Так проверка записана в истории Run как результат шага, а не спрятана в CLI.

## Рабочее дерево: что передаётся

- `project continue` по умолчанию **передаёт** новому Run claim исходного Run:
  тот же каталог, та же ветка, все файлы, включая незакоммиченные. Generation
  claim растёт; в `fork.source_run_id` нового Run записан исходный.
- `--workspace-commit SHA` (полный commit, режим `worktree`) вместо этого
  создаёт новое дерево от указанного commit — когда работа доделана вне дерева
  исходного Run. Дерево исходного Run остаётся за ним.
- Если дерево исходного Run уже освобождено, продолжение создаёт новое по
  обычным правилам проекта.
- Деревья Runs, завершившихся не `succeeded` / `completed_with_waivers` /
  `no_work`, больше не освобождаются автоматически следующим запуском — иначе
  незавершённая работа терялась. `claim list` их показывает, `claim release`
  освобождает.

## Возобновить тем же workflow

Часто продолжать нужно не другим workflow, а тем же — с того места, где Run
остановился. Для этого workflow объявляет **ревизию 8** (возможность
`workflow_resume`; лестница выводит её по полю):

```yaml
resumable:
  from_outcomes: [partial, rejected]   # Runs, завершившиеся этими исходами
  from_cancelled: true                 # и отменённые; хотя бы одно из двух
```

Маппинга входов нет: возобновлённый Run получает входы исходного Run как они
были. Ответы анкеты тоже его: если не передать ни одного ответа, профиля
пакета или политики, `project continue` и `project recover` берут те, что
запечатаны в исходном Run. Переданные ответы и профиль модели должны
совпадать с исходными — иначе отказ `recover_context_changed`.

Запуск — тот же `project continue` **с тем launch, которым Run был запущен**.
Если workflow launch тот же, что у исходного Run, это возобновление, иначе —
продолжение по `continuation`.

**Где остановился Run** — движок читает это из записи Run тем же правилом, что
и `arrived_from` в `run next`:

| Исходный Run | Точка возобновления |
|---|---|
| завершился объявленным исходом | стадия корневого workflow, чей принятый результат привёл в finish |
| отменён на стадии | эта стадия |
| отменён между стадиями | стадия, куда вёл последний принятый результат |
| технический отказ (`project recover`) | упавший шаг; объявление не нужно |

`--from-stage STAGE` начинает с более ранней **принятой** стадии корневого
workflow. Если точку нельзя назвать однозначно, будет отказ с просьбой указать
`--from-stage`, а не догадка.

**Что переносится.** Стадии, принятые **до** точки, — шаги (ассистируемые и
программные), вызовы и `repeat` целиком. Переносится только тогда, когда у
стадии в новой редакции пакета не изменились контракт, входы и маршрут.
Точка исполняется заново: `step`, `call` или `repeat`, а вложенный вызов —
целиком. Всё после точки исполняется заново. Если до точки ничего не принято,
будет отказ `resume_prefix_empty`: это новый запуск, а не возобновление.

**Повторное возобновление.** `resumable` по-прежнему требует ревизию 8.
Повторное возобновление исправлено начиная с public stable **0.13.75**
(change `fix-chained-workflow-resume`); 0.13.74 содержит дефект.
Перед живым запуском проверьте installed version: `prifly --version`.

A → B → C переносит унаследованный префикс вместе с собственными принятыми
стадиями B. На следующем переносе source — B, затем C: это текущий владелец
claim. Сначала выполните `project continue --prepare --repository REPOSITORY
--launch ID --source-run B`, проверьте `recovery.reused`, `root_output_refs`,
`frontier_stage_id`, `claim` и **верхний** `review_digest`. Start использует
тот же repository, source и параметры плюс `--expected-launch-digest DIGEST`.

`--from-stage` может выбрать принятую root-стадию, унаследованную от A.
Выбранная стадия, её вложенные исполнения и хвост исключаются из переноса;
имя nested `fix` не становится root-стадией. При выборе entry префикс пуст.
Сохраните dirty work перед рассмотрением нового start; отказ не разрешает reset.

Origin записи в `reused` читается через `recovery.source_run_id` и дальнейшую
source ancestry до Run с указанным `source_stage_activation_id` /
`source_attempt_id`. Эти identities остаются исходными; `source_event_sequence`
сравнивается только внутри origin Run. `root_output_refs` — доступные копии
непосредственного source, с теми же принятыми bytes. Предки и старые schema
editions не переписываются; отсутствующее evidence отказывает явно.

**Дерево** исходного Run передаётся, как при продолжении, вместе с его режимом
(`worktree` или `checkout`): `workspace:` у launch и `--workspace` не нужны, а
`--workspace` при возобновлении — отказ. Если исходный Run дерева не держит, а
workflow его требует, — отказ `recover_workspace_missing`. Исходный Run, его
исход и история не меняются; новый Run помечен `fork.reason:
resume_stopped_run`, а перенесённое видно в `recovery.reused` его `run status`.

Свежесть перенесённого движок не проверяет: если принятое до точки могло
устареть, пока Run стоял, это проверяет шаг workflow — как первая стадия
продолжения.

## Восстановление

`project recover` для технически упавшего Run переносит доказанный префикс и
исполняет упавшую стадию снова **в дереве исходного Run** (оно передаётся так
же); `--from-stage` начинает с более ранней принятой стадии. Дерево освобождено — отказ `recover_workspace_released`. Имена стадий и
содержимое артефактов для выбора дерева не используются; `--prepare`
показывает передаваемое дерево и последний checkpoint.

### Вложенный отказ и отказ до admission

Recovery возвращается к **root stage**. Если preparation отказала до создания
Attempt (например, `workspace_tree_input_drift`), `frontier_action` — `execute`;
`source_frontier_attempt_id` и `candidate_ref` отсутствуют. Отказ внутри `call`
или `repeat` выбирает целый root call/repeat: его subtree, включая прежние
итерации, не переносится. Причину доказывают записанные invocation/caller
связи и решения; независимые отказы дают `recover_frontier_unsupported`.
`--from-stage` не обходит эту проверку. Это исправление
`fix-nested-preparation-recovery`: пока только development CLI, ещё не stable
release. Новых полей YAML нет; `call` требует WorkflowRevision **1**, `repeat`
— **1**, а `resumable` для Run с исходом или отменой — **8**. Технический
recovery не требует декларации `resumable`.

```sh
prifly project recover --prepare --repository . --launch SAME --source-run RUN --host HOST
# Или начать заново с более ранней принятой root stage (например, plan):
prifly project recover --prepare --repository . --launch SAME --source-run RUN --host HOST --from-stage plan
# После просмотра выбранной точки, reused и claim используйте верхний review_digest:
prifly project recover --repository . --launch SAME --source-run RUN --host HOST --from-stage plan \
  --expected-launch-digest DIGEST
```

Prepare читает evidence, не меняя source Run или claim. Start передаёт текущее
дерево без reset: dirty/untracked файлы остаются, claim generation увеличивается.
Inputs и ответы сохраняются. Более ранняя stage должна быть принятой root
stage до точки отказа, в том числе унаследованной; её результат и хвост не
переносятся. При пустом prefix — `resume_prefix_empty`.

Recovery не устраняет drift автоматически. Если заново читать прежний manifest,
посторонняя запись снова даст тот же отказ. Для новой output-only capture
`exact_file` уже существующий unattributed файл даёт `workspace_tree_output_exists`.
Сохраните его; если автор объявил `direct_child_file`, можно явно выбрать новую
разрешённую location. Нельзя удалять неизвестные bytes или расширять capture
policy только ради обхода отказа.

Если стадия упала на проверке своего результата (программа завершилась с кодом
0, а выход не прошёл контракт пакета) и исправленный пакет этот результат
принимает, стадия не исполняется снова: `recovery.frontier_action: revalidate`,
байты берутся из выходов упавшей попытки — только если их digest равен
названному в её запечатанном результате. Иначе (`execute`) исполняется только
упавшая стадия, и `frontier_reason` называет почему. Созданный Run сразу
ведётся, как после `project start`; тот же `--command-id` отвечает тем же Run.

## Прерванная внешняя запись

Шаг, объявивший `effects.class: external_write`, может прерваться, успев
изменить внешнюю систему. Что будет дальше, решает его `retry_class`, а не
workflow:

| `retry_class` шага-программы | Прерванная программа | `project continue` |
|---|---|---|
| `idempotent` | попытка `failed`, как у любой программы | сразу исполняет шаг снова |
| `deduplicated`, `reconcile_required`, `never` | Run `uncertain`, `failure.code: external_write_unreconciled` | отказ `recover_source_unsettled`, пока владелец не скажет `run resolve … --outcome applied\|not_applied`; после этого исполняет шаг снова |

«Прервана» значит: программа запустилась и закончилась без принятого
результата — упала, остановлена по таймауту или отмене, потеряла драйвер или
её результат не прошёл приём. Программа, которая вернула принятый вердикт
(в том числе `fail`), не держится: она сказала, что сделала. Программа, которая
не стартовала вовсе, тоже не держится.

`idempotent` — слово автора, что повтор сам приводит внешнее состояние к
нужному; движок его не проверяет. Если сомневаетесь, объявляйте
`reconcile_required`: лишний `run resolve` дешевле дубля во внешней системе.
Ассистируемый шаг с эффектом при потерянной сессии остаётся `uncertain` при
любом `retry_class`. Подробно — в
[справочнике шага](step-authoring-reference.yaml) и
[troubleshooting](../troubleshooting.md).

## Как запустить (для хоста)

```sh
prifly project continue --prepare --repository . --launch TAIL --source-run RUN --host HOST
# показать владельцу: откуда каждый вход, какое дерево передаётся
prifly project continue --repository . --launch TAIL --source-run RUN --host HOST \
  --expected-launch-digest DIGEST

# возобновить тем же launch, которым RUN был запущен
prifly project continue --prepare --repository . --launch SAME --source-run RUN --host HOST
# показать владельцу recovery: точку, перенесённые стадии, дерево
prifly project continue --repository . --launch SAME --source-run RUN --host HOST \
  --expected-launch-digest DIGEST
```

Не вызывайте `run fork` и не извлекайте refs из JSON вручную: `project
continue` делает это по объявлению и проверяет то, что показал prepare.

`run next` завершённого или отменённого Run называет в `continuations`
установленные workflow, объявившие его продолжение, — и собственный workflow
Run, если тот объявил `resumable` для него, — и тогда предлагает
`project.continue`. Пакет устанавливается при первом запуске его launch, так
что до этого список может быть пуст. Как процесс пришёл к остановке —
`arrived_from` того же ответа; см. [`blocked-guide.md`](blocked-guide.md).

Индекс `continuations` показывает declarations установленных workflow. Только
конкретный prepare проверяет перенос evidence и claim; наличие строки не
означает, что start будет принят. При потерянном ответе start сначала читайте
receipt по `command-id` и созданный дочерний Run; `project_continue_active_child`
не разрешает независимый дубликат для продолжения той же работы.

## Отказы

| Код | Что значит | Что делать |
|---|---|---|
| `project_continue_undeclared` | у workflow launch нет `continuation` | выбрать другой launch или объявить |
| `continuation_source_ineligible` | workflow или исход исходного Run не объявлены | текст называет оба списка |
| `continuation_source_incomplete` | нет объявленного источника (стадия, выход, checkpoint) | текст называет стадию и порт |
| `continuation_source_unsettled` | исходный Run держит активную или неразрешённую execution | `run resolve`, затем продолжить |
| `continuation_source_ambiguous` | два результата с одним временем | продолжение невозможно без решения владельца |
| `continuation_source_incompatible` | байты не проходят схему входа нового workflow | согласовать схемы |
| `continuation_source_changed` | исходный Run или его дерево изменились после prepare | повторить `--prepare` |
| `project_continue_active_child` | у исходного Run уже есть незавершённое продолжение | проверить его; `--allow-duplicate-continuation` для независимой работы |
| `invalid_checkpoint` | компиляция: порт или схема checkpoint не сходятся | см. раздел про checkpoint |
| `recover_source_unsettled` | исходный Run `uncertain`: шаг с внешней записью прервался, или исполнение не завершилось наблюдаемо | `run resolve`, затем повторить |
| `recover_frontier_unsupported` | нет одной доказанной текущей failure chain; отсутствуют caller/decision/pre-admission facts или есть независимые причины | сохранить evidence; `--from-stage` не обходит отказ |
| `recover_workspace_released` | дерево упавшего Run освобождено | восстановить нечем |
| `resume_undeclared` | workflow не объявил `resumable` для этого исхода или отмены | объявить в ревизии 8 или продолжить другим workflow |
| `resume_prefix_empty` | до точки ничего не принято | сохранить dirty work; отдельно рассмотреть новый start |
| `resume_from_stage_invalid` | `--from-stage` не принятая стадия корневого workflow до точки | выбрать принятую стадию раньше точки |
| `resume_frontier_ambiguous` | точку нельзя назвать из записи Run | указать `--from-stage` |
| `resume_frontier_unsupported` | Run остановился на стадии не `step`/`call`/`repeat` | указать `--from-stage` |
| `resume_input_override` | возобновлению передан `--input`, `--workspace` или `--workspace-commit` | не передавать: входы и дерево — исходного Run |
| `project_continue_answers_required` | launch продолжает, а не возобновляет Run, а ответов не передано: чужие ответы продолжению не подставляются | ответить на анкету |
| `project_continue_from_stage_unsupported` | `--from-stage` с launch другого workflow | `--from-stage` только для возобновления |
| `recover_context_changed` | ответы анкеты или профиль модели не те, что у исходного Run | ответить так же |
| `recover_prefix_changed`, `recover_frontier_changed` | в новой редакции изменилась перенесённая стадия или вход/маршрут точки | `recover_prefix_changed` называет первую изменившуюся стадию и подсказывает `--from-stage` с ней: она и всё после неё исполнятся заново |

### Native call из Project extension

`prifly-extension/2` в Project `/3` вставляет обычный Call: descendant задачи
остаются в том же Run/claim. Host обслуживает leaf `session task`; не запускайте
вложенный `project start`. После prepare любое изменение callee/dependency,
binding, outcome/error route, ресурса, limits или supporting bytes требует
нового review; старый digest даёт `project_start_stale_launch` до dispatch.
Continue проверяет effective accepted prefix вместе с subtree. Изменённый
принятый child не переиспользует stale exports: нужен разрешённый root
`--from-stage <extension-id>` (whole child replay) либо named refusal.
Cancel child оставляет native waiting/`blocked_child` до обычного settlement;
он не подтверждает остановку внешнего actor. Source-built harness не заменяет
released CLI и live host qualification.

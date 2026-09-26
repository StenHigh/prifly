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

## Восстановление

`project recover` для технически упавшего Run переносит доказанный префикс и
исполняет упавшую стадию снова **в дереве исходного Run** (оно передаётся так
же). Дерево освобождено — отказ `recover_workspace_released`. Имена стадий и
содержимое артефактов для выбора дерева не используются; `--prepare`
показывает передаваемое дерево и последний checkpoint.

## Как запустить (для хоста)

```sh
prifly project continue --prepare --repository . --launch TAIL --source-run RUN --host HOST
# показать владельцу: откуда каждый вход, какое дерево передаётся
prifly project continue --repository . --launch TAIL --source-run RUN --host HOST \
  --expected-launch-digest DIGEST
```

Не вызывайте `run fork` и не извлекайте refs из JSON вручную: `project
continue` делает это по объявлению и проверяет то, что показал prepare.

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
| `recover_workspace_released` | дерево упавшего Run освобождено | восстановить нечем |

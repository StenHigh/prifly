# Для пакета и пилота: продолжение без знаний движка о пакете

## Что изменилось и зачем

`project continue` и `project recover` работали только для AI Factory: движок
сравнивал workflow ID с `aif:workflow/classic` и
`aif-continuation:workflow/classic-continuation`, искал результаты по стадиям
`warmup`, `implement`, `review`, читал `head_commit` из содержимого
`implementation` и сам считал `changed_files` через Git. Для любого другого
workflow эти команды не работали. Владелец продукта назвал это дефектом
концепции: движок исполняет, workflow командует.

Теперь движок не знает ни одного имени пакета (это проверяет guard-тест), а
всё нужное объявляет workflow (WorkflowRevision 7):

- `checkpoint` — схема точки, с которой можно продолжить, и стадии, которые её
  сообщают (`checkpoint: <выход>` у шаговой стадии);
- `continuation` — в workflow продолжения: от каких workflow и исходов он
  продолжает и откуда берёт каждый вход (вход исходного Run, принятый выход
  стадии корневого вызова с вердиктом или исходом, последний checkpoint).

Второе изменение — рабочее дерево. Продолжение и восстановление получают claim
исходного Run целиком: тот же каталог, ветку и незакоммиченные файлы. Деревья
Runs, завершившихся не `succeeded` / `completed_with_waivers` / `no_work`,
больше не удаляются автоматически следующим запуском — раньше незавершённая
работа терялась.

Полное руководство: `examples/authoring/continuation-guide.md`.

## Что сделать в пакете (`prifly-aif-workflows`)

1. **Checkpoint на шагах, меняющих дерево** (`implement`, `fix` и т. п.):
   объявить `checkpoint: {schema_ref: …}` в workflow и `checkpoint: <выход>` у
   стадий. Выход должен быть обязательным для `pass`, `fail`, `blocked`, если
   после них нужно продолжать. Удобно, если checkpoint содержит commit.
2. **`aif-classic-continuation` и profiled-копия**: объявить
   ```yaml
   continuation:
     from_workflows: [aif:workflow/classic, aif-profiled:workflow/classic]
     from_outcomes: [partial, rejected]
     from_cancelled: true   # продолжение после убитого драйвера
     inputs:
       task:    {source_input: task}
       handoff: {stage: warmup, output: handoff, verdict: pass}
       plan:    {stage: implement, output: plan, verdict: pass}
       previous_implementation: {stage: implement, output: implementation, verdict: pass}
   ```
   (или `{checkpoint: true}`, если пакет перейдёт на checkpoint).
3. **Первый шаг workflow продолжения** — программный шаг, который делал CLI:
   в claimed workspace проверить, что HEAD содержит `base_commit`/`head_commit`
   прежней реализации, посчитать `changed_files` (`git diff` от `base_commit`),
   выдать новый `implementation`. Несоответствие — `fail` или `blocked` по
   маршруту автора.
4. Вход `implementation` больше не подаёт CLI: его выдаёт шаг из п. 3.

## Что исчезло из CLI

- `--implementation-head` → `--workspace-commit` (полный SHA, режим
  `worktree`): новый claim от commit вместо передачи дерева.
- CLI больше не вычисляет `implementation` из Git и не читает `head_commit`.
- `project recover` не требует стадии `review` и не выбирает commit: работает
  в переданном дереве исходного Run.
- Новые отказы: `project_continue_undeclared`, `continuation_source_*`,
  `recover_workspace_released`, `recover_workspace_missing`.
- Инструкция раннера `prifly-run` обновлена: прежний текст распознаётся как
  сгенерированный и заменяется `project runners` как обычно.

## Пилот: как не сломать разработку

- **Обновлять движок только вместе с редакцией пакета**, в которой есть
  объявление продолжения. До её выхода оставаться на текущем релизе движка:
  новый движок откажет в `project continue` для старого workflow продолжения
  (`project_continue_undeclared`).
- Уже идущие Runs продолжения доводятся без изменений.
- Run, отменённый из-за убитого драйвера, теперь продолжается `project
  continue`, если workflow продолжения объявил `from_cancelled: true`; дерево
  с незакоммиченной работой переходит к продолжению. Если отмена оставила
  неразрешённую execution — сначала `run resolve`.
- Деревья незавершённых Runs теперь копятся: смотреть `claim list`,
  освобождать `claim release` после того, как нужное сохранено.
- Восстановление старого failed Run, чьё дерево уже освобождено, даст
  `recover_workspace_released`: восстанавливать его нечем.
- Runs, созданные новым движком для workflow с checkpoint или продолжением,
  имеют состояние `core-state/40`; прежние читаются как раньше.

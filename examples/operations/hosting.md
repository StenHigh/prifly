[Индекс](index.md) · [Новый запуск](launch.md) · [Решения](decisions.md)

# Исполнение Run и отчёт хоста

## Следуйте текущему действию

```sh
"$PRIFLY_BIN" --project "$authority_root" run next "$RUN" --json
```

`action` определяет следующий ход. Для ready stage поле `stage_work` различает `control`, `program` и `assisted_session`.
`awaiting_host` означает уже выданный Attempt; `next_command` указывает получение задания.
`idle` не означает, что хосту нужно создать новую попытку.
`arrived_from` называет принятый результат предыдущего перехода. Причину `blocked` берите из его outputs и task inputs.
`repeats` показывает iteration, limit и on_limit. Не добавляйте свой цикл поверх declared repeat.

```text
run next -> control/program -> run drive -> run next
    |
    +----> assisted -> task -> take -> work -> submit -> run next
    |
    +----> waiting/terminal -> указанная процедура либо остановка
```

`run drive` исполняет программу до завершения внутри вызова.
Если клиент ограничивает длительность вызова, запустите driver фоновым процессом через доступный host механизм и наблюдайте тот же процесс.
Не запускайте второй driver при timeout наблюдения. Сначала проверьте `driver_live`, status и живой process handle.
`--stop-before-program` возвращает управление до допуска программы; `--next` возвращает next view после drive.

## Получите и примите Attempt

```sh
"$PRIFLY_BIN" --project "$authority_root" session task --run "$RUN" --all --json
"$PRIFLY_BIN" --project "$authority_root" session take --run "$RUN" --attempt "$ATTEMPT" --json
```

`--all` перечисляет все outstanding tasks. Каждое задание записывается также в его `workspace/task.json`.
Берите exact `attempt_id`, `envelope_digest`, `step_instance_id`, `repository_workspace` и `permitted_effects` из задания.
`run.attempts` индексируется по ID, а не числовой позиции массива.
Если есть `take_command`, выполните его один раз до работы. Чтение task само по себе не фиксирует pickup.
После take свежий task может иметь новую `run_version`; используйте её для CAS DecisionRequest.

Прочитайте pinned skills из `context/`, входы и `context.json` в Attempt workspace.
Задание может передавать `decision_sheet`, `decision_context` и `model_profile_translation`.
Не заменяйте эти значения собственными ответами и не спрашивайте уже переданные ответы снова.

## Соблюдайте effects и слоты

`permitted_effects` определяет, что текущий Attempt вправе менять.
Наличие `repository_workspace` даёт путь, а не право записи в repository.
Read-only gate использует scratch и output slots; test/build byproducts не должны менять tracked tree.
Workspace write допустим только при соответствующем effect. Diff дерева и HEAD измеряется при submission.
`effect_not_permitted` называет нарушившие пути; не расширяйте effects ради успешного gate.

Host-owned outputs записываются по exact `context.outputs.NAME.path` внутри Attempt workspace.
Report содержит slot `artifact_id`, `revision` и digest фактических bytes. JSON digest считается по canonical JSON; blob — по bytes.
Используйте публичный `ref` для canonical digest, а не хеш pretty-printed JSON с другим порядком ключей.
Не публикуйте незаписанный port и не придумывайте ArtifactRef.

## Получите шаблон и отправьте отчёт

```sh
"$PRIFLY_BIN" --project "$authority_root" session submit --template --run "$RUN" --attempt "$ATTEMPT" --json > submission.json
"$PRIFLY_BIN" schema SessionSubmissionV7 --def runtime_SessionSubmission --json
```

Template фиксирует envelope и IDs. Заполните legal `result.verdict`, `summary` и требуемые host-owned outputs.
`routed_verdicts` в текущем task — допустимые ответы именно этой стадии. Полный enum schema не означает, что любой verdict имеет маршрут.
Схема result принадлежит task `result_schema_ref`; более новый verdict может требовать StepResult v2 и собственную authoring revision.
Shell exit теста или программы не заменяет StepResult и не завершает Attempt.

```json
{
  "schema_version": "assisted-session/7",
  "run_id": "RUN_FROM_TEMPLATE",
  "attempt_id": "ATTEMPT_FROM_TEMPLATE",
  "envelope_digest": "DIGEST_FROM_TEMPLATE",
  "result": {
    "schema_version": "1",
    "run_id": "RUN_FROM_TEMPLATE",
    "attempt_id": "ATTEMPT_FROM_TEMPLATE",
    "step_instance_id": "STEP_FROM_TEMPLATE",
    "envelope_digest": "DIGEST_FROM_TEMPLATE",
    "verdict": "pass",
    "summary": "Краткий фактический результат",
    "outputs": {},
    "evidence_refs": [],
    "effect_receipt_refs": []
  },
  "answered_questions": []
}
```

Это форма для task без host-owned ports. Скопируйте реальные IDs и обязательные ports из своего template.
`question_report: required` требует top-level `answered_questions`, рядом с `result`.
Пустой список означает «шаг не встретил вопросов»; это утверждение, а не удобный default.
Включайте также вопросы pinned skill, решения из decision_context и собственные judgement answers.
Форма entries и основания показаны в [решениях](decisions.md).

```sh
"$PRIFLY_BIN" --project "$authority_root" session submit --file submission.json --json
"$PRIFLY_BIN" --project "$authority_root" run next "$RUN" --json
```

После принятого отчёта всегда читайте следующий ход того же Run.
Ошибка intake не означает принятие результата. Сверьте текущий task и diagnostic перед исправленным report.

## Workspace-tree outputs

Движок сам захватывает файлы repository workspace и создаёт manifest ArtifactRef.
Хост не заполняет этот output внутри `result.outputs`.
Для `direct_child_file` и `direct_child_tree` передайте top-level locations:

```json
{
  "workspace_trees": [
    {"output_port": "plan", "path": ".ai-factory/plans/actual-plan.md"}
  ]
}
```

Этот fragment добавляется в реальный submission, а не отправляется самостоятельно.
Это пример `direct_child_file`. Для `direct_child_tree` path указывает папку с declared entrypoint.
`path` относителен repository workspace и должен попадать в declared capture policy.
`exact_file` имеет фиксированный capture path; для него location можно опустить.
Materialize-only input без output port ничего не захватывает и не требует отчёта об output.
Отсутствующая direct-child location вызывает `workspace_tree_location_missing`.

```sh
"$PRIFLY_BIN" schema WorkspaceTreeLocation --def runtime_WorkspaceTreeLocation --json
```

## Профиль модели

Если task содержит `model_profile`, отчёт обязан указать фактический ответ host.
`honoured` называет реально использованную модель; `unavailable` означает, что host не даёт выбирать модель.
`declined` требует причины осознанного отказа от профиля. Не утверждайте запуск другой модели или subagent, которого не было.
Pri-Fly хранит самоотчёт и не доказывает, какая модель исполнила работу.
При отсутствии отдельного session mechanism выполните Attempt здесь и честно сообщите `unavailable`.
Точную форму `model_profile` берите из актуального schema/template и task translation.

## Программы и контекст

Program executors возвращают результат через declared local protocol, а не через произвольный stdout.
Result channel (fd 3) и progress channel (fd 4) имеют разные контракты.
`program_environment` в next view показывает sealed variables и источники; local changes после start туда не подмешиваются.
`context_request` в отчёте запрашивает контекст по контракту. Он не разрешает произвольный доступ к секретам или эффектам.
`session publish` и `step publish` публикуют declared hooks с typed command; это не замена final report.
`session disconnect` фиксирует разрыв host; он не подтверждает отсутствия внешних эффектов.

См. также [рабочее дерево](workspace.md), [решения](decisions.md), [вывод и прогресс](observability.md).

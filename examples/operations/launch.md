[Индекс](index.md) · [Установка](installation.md) · [Исполнение](hosting.md)

# Новый запуск

## Выберите только объявленный launch

```sh
"$PRIFLY_BIN" project workflows --repository "$repository" --json
```

Ответ перечисляет exact `id`, `title`, `description`, `source` и typed inputs каждого launch.
Если пользователь назвал существующий exact ID, используйте его без повторного выбора.
Не заменяйте неизвестный ID похожим. При пустом списке сообщите отсутствие launches; установка требует отдельного выбора.
При нескольких launches предложение по назначению допустимо, если проект его разрешает и пользователь не отказался от совета.
Совет использует declared title/description и `PROJECT.md`, не имя папки или догадку по ID.
До prepare не обещайте input compatibility или admission.

Для конечного выбора используйте native question tool текущего host, если его контракт допускает такой вопрос.
В Codex учитывайте текущий режим: `request_user_input` либо доступный `request_user_input_async`; Claude использует `AskUserQuestion`.
Покажите реальные ID и последствия, большие наборы разбейте на детерминированные страницы next/back/defer.
Если tool недоступен, задайте обычный текстовый вопрос и дождитесь ответа.
Timeout, dismissal, preselection и tool receipt не являются ответом пользователя.
File paths, task text и произвольный JSON остаются свободным вводом.

## Прочитайте questionnaire

```sh
"$PRIFLY_BIN" project questionnaire --repository "$repository" --launch "$LAUNCH" --host codex-app --json
```

Используйте `project_profile_version`, `profiles`, `preflight`, `runtime`, `decision_states`, `catalog_digest` и `decision_sheet`.
`decision_states` различает applicable, not applicable и conditional. Conditional не превращается в false.
`decision_sheet` показывает источник значения: explicit answer, project default или package default.
Standing answers в `extend.yaml` уже являются ответами. Не спрашивайте их снова без необходимости override.
Selected package profile и decision с destination `package_profile` не передаются одновременно как два разных выбора.

```sh
"$PRIFLY_BIN" project questionnaire --repository "$repository" --launch "$LAUNCH" \
  --package-profile PROFILE --decision-policy attended \
  --preflight-answer 'DECISION=JSON' --runtime-answer 'DECISION=JSON' --json
```

Повторяйте answer flags для разных declared IDs. `autonomous` использует только разрешённые каталогом automatic choices.
Scope-changing decision не становится автоматическим из-за общей autonomous policy.
Runtime preanswer применяется только к объявленному вопросу, когда шаг его запросит.

## Передайте реальные inputs

```sh
# Значение JSON/blob из файла:
"$PRIFLY_BIN" project questionnaire --repository "$repository" --launch "$LAUNCH" \
  --input task=/absolute/path/task.json --host codex-app --prepare --json
# Для уже сохранённого artifact используйте --input-ref PORT=REF.json.
```

Имя `task` здесь пример конкретного port. Другой workflow может не иметь такого входа.
Сохраните исходный текст задачи и URLs; не заменяйте их причиной рекомендации launch.
Input формат и schema принадлежат выбранному workflow. Не добавляйте универсальный RunBrief или task port.
Preflight answer с destination launch input уже связывает этот port; повторная передача конфликтует.
Defaults применяются только по declaration. Missing required input требует значения пользователя.

## Workspace и host

Profile `/3` запрашивает host, Git и brief только по требованиям выбранного workflow.
Program-only workflow может работать без Git и ИИ. Host-bound sources требуют exact `--host`.
Git work требует `--workspace worktree|checkout` либо standing workspace в launch.
Worktree изолирует Run своей веткой; checkout занимает существующее рабочее дерево.
Не задавайте вопрос о workspace для workflow, которому оно не нужно.

Profile `/4` добавляет `worktree_naming`. Prepare показывает candidate с `reserved:false`.
Старт может выбрать следующий свободный suffix. Claim ID и actual path берите из ответа authority.
[Пример naming](../workflows/named-worktree/README.md) показывает bindings; не извлекайте naming values из task prose.

Legacy `/2` сохраняет прежние требования: Git, host roots, explicit host, brief и старую identity semantics.
Checked launch summary требует явной миграции профиля. Не передавайте ему новые review flags вслепую.
Переход на `/3` или `/4` выполняется отдельной явной правкой shared profile и editor schema.

## Prepare, review, start

Обязательные updates выполняются [до этой последовательности](installation.md#обязательная-проверка-перед-новым-run).

```text
questionnaire -> typed answers/inputs -> prepare -> owner review -> start
```

1. Получите актуальный `catalog_digest` из questionnaire.
2. Выполните prepare с теми же selected inputs, host, workspace, profile, policy и answers.
3. Покажите `launch_summary`, включая точный package, inputs, программы и supporting files.
4. Получите подтверждение summary и executable effects в разрешённом host канале.
5. Выполните start с теми же аргументами и `review_digest` из prepare.

```sh
"$PRIFLY_BIN" project questionnaire --repository "$repository" --launch "$LAUNCH" \
  --host codex-app --workspace worktree --input task=task.json \
  --expected-decision-catalog-digest "$CATALOG_DIGEST" --prepare --json

"$PRIFLY_BIN" project start --repository "$repository" --launch "$LAUNCH" \
  --host codex-app --workspace worktree --input task=task.json \
  --expected-decision-catalog-digest "$CATALOG_DIGEST" \
  --expected-launch-digest "$REVIEW_DIGEST" --allow-execution --json
```

Это пример Git+assisted launch. Передавайте только применимые аргументы своего questionnaire.
Prepare компилирует read-only: не импортирует package, не получает claim и не создаёт Run.
`--allow-execution` выражает разрешённые executable effects; он не заменяет review digest и host permissions.
Preflight program, объявленная проектом, выполняется при start до получения ресурсов и может отказать со своим output.
Если source bytes, inputs, answers, bindings или skills изменились, выполните новый prepare и покажите новый summary.
Не удаляйте stale-digest check ради продолжения.

## Как читать итог

`author_package` — версия автора; `package` — exact compiled edition; `build_key` связывает конкретные входы сборки.
Одинаковые inputs сборки `/3` дают одинаковые exact refs. Другие bytes получают другой variant, даже при прежней author version.
Существующие Runs сохраняют свои sealed definitions и inputs.
Summary `/4` может нести queue task, `/5` добавляет naming. Unified composition имеет свои последующие DTO revisions.
Читайте фактический `schema_version`, а не переписывайте его по знакомой старой форме.

Сохраните `run_id`, `workspace_path`, `workspace`, `launch_summary` и `next_command` из start.
Выполнение продолжается через [host protocol](hosting.md). Исход Run проверяйте отдельно от exit команды.

См. также [решения](decisions.md), [workspace](workspace.md), [launch-choice guide](../authoring/launch-choice-guide.md).

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

<a id="project-owned-naming-inputs"></a>
## Project-owned naming inputs

Profile `/5` объявляет `worktree_naming.inputs.NAME.schema` отдельно от workflow inputs.
Questionnaire `/5` показывает `naming_inputs` и условие `required_when: git_workspace_worktree`.
Передайте JSON value через `--naming-input NAME=FILE` в prepare и start.
Binding `{naming_input: NAME, pointer: /id}` выбирает строку; `{input: PORT, pointer: /id}` по-прежнему читает workflow input.

```sh
"$PRIFLY_BIN" project questionnaire --prepare --repository "$repository" --launch "$LAUNCH" \
  --host codex-app --workspace worktree --input task=task.json --naming-input identity=identity.json --json
"$PRIFLY_BIN" project start --repository "$repository" --launch "$LAUNCH" \
  --host codex-app --workspace worktree --input task=task.json --naming-input identity=identity.json \
  --expected-launch-digest DIGEST --command-id issue19:first --json
```

Schema — project-owned JSON Schema 2020-12 без внешних `$ref` и symlinks.
Snapshot содержит declaration, schemas, полные canonical values, sources и digests.
Его общий предел — 128 KiB canonical UTF-8 JSON, включая все служебные поля.
Core не ищет ID в task description и не передаёт naming values шагам.
Изменение любого поля value, schema или declaration делает review устаревшим.
Whitespace JSON не меняет canonical value. Preview не резервирует имя.

```
profile + schema + value
          |
       validate
          |
       review /8
          |
    freshness check
          |
    claim /5 + snapshot
```

Summary/start `/8` показывают snapshot и фактический claim.
`claim list` использует `foundation-claims/4`; persisted record — `authority-claims/5`.
Continue/resume/recover сохраняют source snapshot и отказывают `--naming-input` override.
При новом claim от commit сохраняются исходная основа и snapshot; allocator выбирает свободный suffix.
Checkout и запуск без Git workspace не требуют naming values. Явные неприменимые values получают отказ.
Прежние profiles `/2–/4` и сохранённые Runs сохраняют свои контракты.

[Author reference](../authoring/project-profile-authoring-reference.yaml) показывает сами поля.
[Native probe](../workflows/project-owned-naming/README.md) проверяет контракт без AI host.

<a id="compile-parameters"></a>
## Предел схемы через параметр сборки

Параметр сборки задаёт разрешённый пакетом предел до проверки task. Требуется Project profile `/3` или новее.
Пакет объявляет параметр в `compile-parameters.yaml` версии `prifly-compile-parameters/1`.
[Объявление](../authoring/compile-parameters-authoring-reference.yaml) показывает тип, диапазон, default и точный binding к `maxLength` строковой schema.

1. Проверьте имя и диапазон объявленного параметра.
2. Создайте `.prifly/workflows/NAME/project/compile-values.yaml` по [примеру](../authoring/compile-values-authoring-reference.yaml).
3. Задайте `values.task_description_limit: 32000` для объявленного параметра с таким именем.
4. Выполните обычный `project questionnaire --prepare` с task и сохраните `review_digest`.
5. Проверьте summary и запустите `project start` с теми же аргументами и `--expected-launch-digest`.

```text
compile-parameters.yaml + project/compile-values.yaml
    → проверка диапазона и binding → сборка всех refs
    → prepare → reviewed start → закреплённый Run
```

Compile, prepare и start читают одинаковый owner file. Для default достаточно опустить имя в `values` либо весь owner file.
Изменение значения создаёт новую сборку и делает старый launch review недействительным.
Сводка показывает exact package ref и build key effective контракта.
Ошибка длины показывает JSON pointer, effective limit и фактическое число Unicode code points.
Кириллический символ и emoji считаются каждый одним символом; combining marks считаются отдельно.

Update сохраняет owner file вместе с каталогом `project/`.
После сужения диапазона новая compilation явно отказывает несовместимому значению.
Прежний Run сохраняет свои definitions и полный task.
Continue проверяет carried task по объявленному контракту продолжения и отказывает до эффектов, если данные несовместимы.

`settings` продолжает менять default configuration input.
`compile --value` остаётся разовым legacy input compilation; declared compile parameter задаётся только через owner file.
Подстановка параметра разрешена только в его declared schema bindings.
Package profiles и references не могут объявлять одноимённый параметр.

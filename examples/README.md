# Примеры YAML

Здесь находятся материалы для автора YAML-сценария и указатель по симптомам для
того, кто чужой сценарий запускает. Legacy Python/shell fixtures, F1 demo и
проверки runtime находятся в [`test/`](../test/README.md).

Справочники ниже проиндексированы по полю: ответ находится, если заранее знать
имя поля. Если известен только симптом — отказ, диагностика или поведение
прогона, — начните с [указателя по симптомам](troubleshooting.md).

## Что умеет движок и где это показано

Строка на каждую возможность, которую объявляет `prifly capabilities`. Проверка
`TestEveryDeclaredCapabilityIsInTheAuthorIndex` держит таблицу и этот список
равными: возможность без строки и строка без возможности валят ворота, поэтому
таблица не может отстать молча.

«Справочник» — файл в `authoring/`; `—` значит, что объявлять нечего: это
поведение чтения или команда CLI, а не поле YAML. Где указана ревизия или
контракт шага — это минимум, ниже которого поле не примут.

| Возможность | Что для неё объявляет автор | Справочник |
|---|---|---|
| `step` | `kind: step` в `stages` | workflow |
| `finish` | `kind: finish` с `outcome` | workflow |
| `local_process` | `executor.operation: process` и execution bindings | step, execution-bindings |
| `state_hook` | `hooks.<имя>.kind: state` с `freshness_ms` | step |
| `event_hook` | `hooks.<имя>.kind: event` | step |
| `telemetry.catalog` | `telemetry` у шага; читает `prifly telemetry` | step |
| `telemetry.records` | то же объявление; записи читает `telemetry` | step |
| `telemetry.aggregate` | то же объявление; сводку считает `telemetry` | step |
| `on_error` | `on_error` у стадии | workflow |
| `json_projection` | `pointer` и `projected_schema_ref` в binding | workflow |
| `input_configuration` | `configuration` у входного порта | step |
| `choice` | `kind: choice` с тремя исходами | workflow |
| `call` | `kind: call` с `workflow_ref` и `on` | workflow |
| `repeat` | `kind: repeat` с `max_iterations` | workflow |
| `partial` | `partial` среди `allowed_outcomes` | workflow |
| `local_workflow_aliases` | `refs` — локальные имена точных ссылок | workflow, step |
| `context_resources` | `context_refs` у шага | step, context |
| `full_context` | `instructions_ref` и `context_refs` вместе | step, context |
| `source_import` | `source` у задачи; импорт — `prifly task` | — |
| `context_request` | `context_request` в отчёте хоста | — |
| `automatic_checks` | `result_check_refs` у шага и файлы `checks/` | step, check |
| `assisted_session` | `authoring: prifly-step/2`, `operation: session` | step |
| `quality_waivers` | `waivable_check_refs` в политике; `run waive` | — |
| `parallel` | `kind: parallel` с `branches` и `join` | workflow |
| `map` | `kind: map` с запечатанной коллекцией | workflow |
| `wait` | `kind: wait` с корреляцией | workflow |
| `schedule` | `schedule` у ожидания; `prifly schedule` | workflow |
| `live_guards` | объявляется при создании Run, не в графе | — |
| `reported_cost` | `reported_costs` в отчёте хоста | — |
| `artifact_publication` | `hooks.<имя>.artifact` с `cardinality` | step |
| `artifact_publication_checks` | `content_check_refs` у artifact-хука | step |
| `artifact_close` | `cardinality: keyed_many` и закрытие хука | step |
| `publication_subscription_once` | `from: publication` с `mode: once` | workflow |
| `publication_subscription_each` | `from: subscription` в `repeat` | workflow |
| `publication_subscription_new_only` | `new_only` у источника публикации | workflow |
| `publication_subscription_terminal_failure` | поведение при падении producer'а | — |
| `publication_subscription_blob` | `format: blob` у публикуемого артефакта | step |
| `action_intent_proposal` | `prifly action propose`; в графе не объявляется | — |
| `action_admission` | `prifly action admit` | — |
| `action_grant_admission` | `prifly action admit --grant` | — |
| `action_delivery_prepared` | подготовленная граница; доставки в этой сборке нет | — |
| `run_fork` | `prifly run fork` | — |
| `workspace_modes` | `workspace: worktree|checkout` в профиле проекта | project-profile |
| `workspace_tree_artifacts` | `workspace_trees` с `output_port` | step |
| `decision_catalog` | `decisions` пакета; ответы проекта — `answers` | extension |
| `neutral_start` | `project start` без RunBrief | project-profile |
| `execution_bindings` | `execution_bindings` в корне пакета | execution-bindings |
| `assisted_session_timing` | `session_limits.active_timeout_ms` | step |
| `declared_technical_retries` | `technical_retries` у шаговой стадии — **ревизия 5** | workflow |
| `routed_session_verdicts` | `on` у стадии; задача называет маршрутизируемые | workflow |
| `declared_impossible_verdicts` | `impossible_verdicts` — **ревизия 4** | workflow |
| `assisted_effects_enforced` | `effects.class`; метка дерева сверяется при приёме | step |
| `assisted_session_unbounded_work` | `session_limits.active_timeout_ms: null` — контракт 7 | step |
| `materialize_only_workspace_tree` | `workspace_trees` с `input_port` без `output_port` — контракт 8 | step |
| `run_failure_named` | поведение чтения: `failure` в `run status` | — |
| `stage_work_named` | поведение чтения: `stage_work` в `run next` | — |
| `execution_environment_source` | `environment_from` в execution bindings | execution-bindings |
| `program_environment_named` | поведение чтения: `program_environment` в `run next` | — |
| `model_profile_declared` | `model_profile` у шага — контракт 9 | step |
| `model_profile_translation` | `model_profiles` в профиле проекта | project-profile |
| `project_title` | `title` в профиле проекта | project-profile |
| `failed_stage_recovery` | `prifly run reopen` | — |
| `run_finish_named` | поведение чтения: `finish` в `run explain` | — |
| `blocked_verdict` | `required_for: [..., blocked]` — контракт 10, маршрут при **ревизии 6**, `result_schema_ref` → `core:schema/step-result@2.0.0` | step, workflow |
| `declared_external_write` | `effects.class: external_write` и блок `external_write` — контракт 11, только ассистируемый шаг | step |
| `workflow_continuation` | `checkpoint` в корне и у шаговой стадии, `continuation` в корне — **ревизия 7**; `prifly project continue` | workflow, continuation-guide |

Полный список того, что сборка о себе говорит, включая неподдерживаемое:

```sh
prifly capabilities --json | jq -c '{caps:.profiles[1].capabilities, unsupported}'
```

## Справочники YAML для авторов сценариев

- [`authoring/workflow-authoring-reference.yaml`](authoring/workflow-authoring-reference.yaml) — все
  поля `prifly-workflow/1`, восемь видов stages, bindings, limits и comments, а
  также что открывают ревизии 4, 5 и 6: `impossible_verdicts`,
  `technical_retries` и маршрут для `blocked`.
- [`authoring/continuation-guide.md`](authoring/continuation-guide.md) — как
  сделать workflow продолжаемым: `checkpoint` (что сохранить),
  `continuation` (от каких Runs и откуда брать входы), проверка первым шагом,
  передача рабочего дерева, восстановление и отказы. Ревизия 7.
- [`authoring/step-authoring-reference.yaml`](authoring/step-authoring-reference.yaml) — все поля
  `prifly-step/2` на новейшем контракте шага: ports, hooks, telemetry,
  `session_limits`, `workspace_trees`, `model_profile`, обещание выхода на
  `blocked` и объявленная внешняя запись. Программная форма — тот же файл с
  `authoring: prifly-step/1`.
- [`authoring/extension-authoring-reference.yaml`](authoring/extension-authoring-reference.yaml) —
  tracked `profile`, `answers` (постоянные ответы проекта на объявленные
  решения: `decision_policy`, `preflight`, `runtime` — источник
  `project_default`, флаг перебивает), `execution_bindings` (программы для
  вставных шагов проекта — та же форма и то же доверие, что у пакетных),
  `settings`, `exclude` и простая вставка шага через `extend.yaml`. Свои
  компоненты (steps/contexts/schemas/checks) и программы кладите в
  `<папка пакета>/project/`: `workflows update` переносит это поддерево
  вместе с `extend.yaml` и не считает его правкой upstream. Правка `extend.yaml` — правка команды:
  `origin.extend_digest` в `project.yaml` — отпечаток **upstream**-файла на
  момент установки (по нему `workflows update` видит изменения upstream), к
  локальному файлу он не относится и после правки `answers` не меняется.
- [`authoring/project-profile-authoring-reference.yaml`](authoring/project-profile-authoring-reference.yaml) —
  полный `.prifly/project.yaml` `/3`; hosts необязательны и выбираются явно;
  у launch — постоянный `workspace` (host не спрашивает, `--workspace`
  перебивает на один Run) и `preflight` — программа проекта, которую
  `project start` исполняет до любого захвата и отказывает при ненулевом коде.
- [`authoring/execution-bindings-authoring-reference.yaml`](authoring/execution-bindings-authoring-reference.yaml) —
  все поля локальных execution bindings для steps/checks с comments.
- [`authoring/check-authoring-reference.yaml`](authoring/check-authoring-reference.yaml) —
  полная CheckDefinition для `checks/`, без shorthand и скрытых defaults.
- [`authoring/context-authoring-reference.yaml`](authoring/context-authoring-reference.yaml) —
  обычный текст и явный context source из выбранного host skills root.
- [`authoring/workflow-catalog-authoring-reference.yaml`](authoring/workflow-catalog-authoring-reference.yaml) —
  `catalog.yaml` каталога сценариев: категории и указатели `repository + path + ref [+ commit]`
  для `prifly project workflows search` и `add NAME`.

Для первого запуска без Git и ИИ есть учебный
[CSV → проверка → отчёт](workflows/csv-report/README.md): YAML и обычный Node.js
worker. Это пример общего worker protocol, не отраслевой product workflow.
Сценарии AI Factory
(`aif-classic`, `aif-fanout`) живут в
[`StenHigh/prifly-aif-workflows`](https://github.com/StenHigh/prifly-aif-workflows)
и ставятся из официального каталога `github.com/StenHigh/prifly-workflows`:

```sh
prifly project workflows search
prifly project workflows add aif-classic
```

Локальные JSON Schema для подсказок и диагностики редактора лежат в
[`schemas/authoring/`](../schemas/authoring/README.md). Они не становятся
частью YAML или sealed package: сложную проверку по-прежнему выполняет
`prifly project compile`.

Файлы `authoring/` — **справочники, а не готовые packages**: в них могут стоять
нулевые digest и ссылки на несуществующие иллюстративные components.
Копируй из них только нужную форму и замени refs на exact значения из
`prifly inventory`. Обычный новый проект не обязан перечислять все поля —
безопасные defaults описаны в [workflow and context](../openspec/specs/workflow-and-context/spec.md).

## Проект с Pri-Fly: обычные программы и optional ИИ

Fresh `prifly project init --repository .` создаёт profile `/3` в обычной папке,
без Git и AI directories. `.prifly/local.yaml` связывает этот checkout с
отдельной local authority и exact Pri-Fly executable; её путь не нужно каждый
раз передавать в compile/start. Shared `.prifly/` содержит только общий YAML
и supporting files. После clone/copy тот же init создаёт только отсутствующую
local configuration, не переписывает общий YAML или frozen runners.

Для command-only package host не нужен:

```sh
prifly project compile --repository . --package NAME --output ../NAME.package
prifly project local set --repository . --allow-executable worker=/absolute/path/to/worker
prifly project start --repository . --launch NAME --input source=./input.csv --allow-execution
```

Замените NAME, executable и input на объявления своего package. Compile не
создаёт Run и не исполняет программы. Start требует локальное разрешение и
`--allow-execution`, закрепляет выбранные programs/argv/files отдельно от inputs.
Программа получает чистое окружение (`PATH=/usr/bin:/bin`, `LANG`, `TMPDIR`,
`PRIFLY_*`); что ей нужно сверх этого на **этой машине** — `PATH` до php/composer,
`APP_ENV` — задаётся рядом с бинарём, в ignored `local.yaml`
(пути портов в `context.json` — относительно scratch попытки, cwd программы;
после `cd "$PRIFLY_REPOSITORY_WORKSPACE"` разрешайте их от каталога
`PRIFLY_CONTEXT_FILE`; ссылки на встроенные адаптеры для своих шагов в
`project/` — `references:` в `extend.yaml`, форма `core:adapter/local-process@2.0.0`):
`prifly project local set --env PATH=/opt/homebrew/bin:/usr/bin:/bin --env APP_ENV=testing`;
`--prepare` показывает имена у каждой программы (`execution[].environment_names`)
и учитывает значения в digest'е. `--env` пишет значение в `local.yaml` — это
форма для того, что не секрет. Пароль или токен объявляйте местом, а не
значением: `--env-from DB_PASSWORD=dotenv:/absolute/.env:PASSWORD`,
`--env-from UPSTREAM_TOKEN=env:CI_TOKEN`, `--env-from KEY_FILE=file:/absolute/token`.
Место видно в `execution[].environment_sources` и входит в digest, само
значение читается в момент старта программы и не попадает ни в `local.yaml`,
ни в состояние Run, ни в один документ из него; отсутствующий или пустой
источник — отказ `execution_environment_unavailable` **до** запуска, с именем
переменной и местом. Файл с ключом читается дословно: первая строка `KEY=`,
значение до конца строки, строки с `#` пропускаются, значение в кавычках —
именованный отказ, а не молча снятые кавычки. Программа шага в Run, который держит рабочую копию
(claim), получает её путь в `PRIFLY_REPOSITORY_WORKSPACE` (и `PRIFLY_CLAIM_ID`)
— туда, где ассистируемый шаг читает `repository_workspace`; шаг без
`effects.class: workspace_write` измеряется по той же метке, что и отчёт хоста:
изменил дерево — попытка падает `effect_not_permitted` с именами путей.
Что сейчас исполняется и что шаг выдал — всё в `run status --json`, без
нового поля: исполняющаяся программа — попытка с `process` и без `settled`
(`.run.attempts[] | select(.process != null and .settled == null) |
{step_instance_id, started: .started.utc}`), её стадия — `.run.steps[<step_instance_id>]`;
последний принятый выход шага — `.run.attempts[] | select(.accepted != null)
| .accepted.outputs.<port>` — это `ArtifactRef`, читается `prifly artifact
export --ref REF.json --output FILE`, не нужно искать `work/<attempt>/outputs`
по mtime.
`run drive` исполняет программу шага внутри вызова и возвращается после неё:
хосту с таймаутом на вызов инструмента (обычно 5 минут) такой Run стоит вести
в фоне или с таймаутом больше `timeout_ms` привязки. Бюджет определений
authority (512 записей, `dependency_limit`) виден в `--prepare` как
`registry_budget` — до отказа, а не после; неиспользуемые издания снимаются
`package remove`, а издание, снятое откатом неудавшегося старта, следующий
старт той же сборки берёт снова сам.
RunBrief не создаётся автоматически: если такой документ объявлен required
typed input, передайте его как соответствующий input. История и results
сохраняются вне проекта; scratch — не sandbox для недоверенных программ.

Пошаговая инструкция с реальным результатом — в
[csv-report/README.md](workflows/csv-report/README.md). Путь `/3` доступен
в public stable release начиная с 0.10.0; сборка из checkout ради него не нужна.

Если workflow использует ИИ, подключите только нужные host runners:

```sh
prifly project runners add --repository . --host codex-app
# --host codex-cli и --host claude-code можно добавлять отдельно или вместе.
```

Package с context из host skills компилируется одной командой; `prifly-run`
передаёт один из `--host codex-cli`, `--host codex-app` или `--host
claude-code`, и context YAML закрепляет bytes только из соответствующего
skills root. Компилятор не импортирует package и не запускает Run, а также не
угадывает host по существующим папкам.

```sh
prifly project compile --repository . --package NAME --host codex-cli --output ../NAME.package
```

При использовании Git закоммитьте shared `.prifly/` и только явно добавленные
generated host skills. Посмотрите явные точки запуска:

```sh
prifly project workflows --repository . --json
```

Product workflow не копируется при `project init`: владелец добавляет папку
сознательно, например `prifly project workflows add`. Текущий host runner уже
нейтрален: brief и выбор `worktree`/`checkout` он спрашивает только когда этого
требует Git-работа, и не выдумывает задачу или brief для file-only запуска.
Прежний обязательный диалог Git/brief сохранён лишь в замороженных исторических
шаблонах, по которым `prifly project runners update` распознаёт старые runner
files. Поэтому описанный выше no-Git путь работает и напрямую через CLI, и
через host runner. Для AI/Git workflows host навык читает local authority path
и exact local Pri-Fly executable из ignored `local.yaml`,
предлагает выбрать ID сценария, затем одним диалогом спрашивает `worktree` или
`checkout`, package profile, применимые declared decisions и policy attended
или autonomous. В Codex при доступном `request_user_input` и в Claude Code через
`AskUserQuestion` эти конечные варианты показываются native question UI; если
host не предоставляет tool, навык ждёт явный текстовый ответ. RunBrief, file
path и произвольный input остаются обычным вводом. При большом списке известных
вариантов навык показывает все pages и не выбирает default. До ответа он не
создаёт package, claim или Run. После выбора он
вызывает `project start` с digest этой анкеты, который сверяет current catalog
до создания Run; затем он seal-ит declared package, создаёт Run и
доходит только до первого handoff; Pri-Fly не вызывает модель сам. В `worktree`
код меняется в новом isolated worktree; `checkout` меняет текущий чистый
checkout repository. Sealed context и outputs в обоих случаях остаются в
authority scratch, не в repository.

`project init` намеренно не заменяет tracked `prifly-run/SKILL.md`. После
обновления Pri-Fly владелец проекта обновляет только объявленные generated runner files
отдельным reviewed commit; существующий runner остаётся безопасным и позволяет
clone создать только свой ignored `.prifly/local.yaml`.

```sh
prifly project runners update --repository .
```

Команда заменяет только точные предыдущие generated runners. Если разработчик
изменил хотя бы один из них вручную, она отказывает и не перезаписывает другие
файлы: изменение сначала нужно перенести в reviewed project workflow.
Объявленный host, чей runner в этом clone не лежит (профиль общий, а файл
держат не все), обновление не блокирует: он перечисляется в `missing_hosts`,
остальные обновляются. Отказ `project_runner_missing` остаётся только для
профиля, в котором нет ни одного runner.

Правила проекта для runner'а живут не в `SKILL.md`, а рядом — в
`<skills_root>/prifly-run/PROJECT.md`. Сгенерированный текст называет этот
файл в первых строках: «если лежит рядом — прочитай сразу после этого текста;
при противоречии PROJECT.md сильнее». `runners update` заменяет только
`SKILL.md` и никогда не трогает `PROJECT.md`, поэтому эталон следует за
выпусками, а правки команды — нет. Что PROJECT.md не может: расширить то, что
движок измеряет сам, — объявленные эффекты, захваченные рабочие копии и
выходные слоты. Хост загружает только `SKILL.md`; соседний файл читает
исполнитель по указанию из него, поэтому держите `PROJECT.md` коротким и не
ссылайтесь из него дальше на файлы, которых нет. Хост читает `SKILL.md` при
старте сессии и не перечитывает его посреди неё: обновлённый runner — и
указание на `PROJECT.md` — доходит до исполнителя только в сессии, начатой
после `update` (наблюдено в Claude Code: посреди сессии по Skill выдавался
старый текст, хотя на диске уже лежал эталон). Если `SKILL.md` уже правлен
руками: `diff` против эталона (снять `project init` во временном профиле)
и есть содержимое `PROJECT.md`; после переноса `SKILL.md` возвращается к
эталону и снова принимается `update`.

Существующий `/2` не мигрируется автоматически: он сохраняет Git, все три
host roots, explicit `--host`, обязательный `--brief` и default worktree.
В `/3` Git-запись assisted шага требует explicit workspace choice, а обычным
commands и assisted `effects:none` Git claim не нужен.

Assisted handoff не требует local worker socket. Для
отдельного сценария с managed local worker заранее посмотрите
`prifly --project <authority-root> doctor`: `local_worker_socket: false`
означает, что окружение запрещает локальные Unix sockets. Запустите Pri-Fly в
обычном user environment с разрешённым local IPC либо выберите assisted шаг;
скрытого сетевого fallback нет.
`TaskInput/1` при необходимости готовится явной intake-командой для declared
input workflow; launch не запускает Python recipe и не угадывает источник
задачи. Сам TaskInput реализован; GitLab/GitHub/Jira adapters по-прежнему не
реализованы.

В `assisted-session/2` host может добавить к terminal submission список
`reported_costs`, например
`{"schema_version":"reported-cost/1","source":"claude-code","amount":"0.0125","currency":"USD"}`.
Это готовое число источника для конкретной Attempt. Pri-Fly не вычисляет его из
токенов; несколько источников не примиряются, а отсутствие списка остаётся
«не наблюдалось».

## Проверки для maintainers

Сами проверочные скрипты находятся в [`test/`](../test/README.md), отдельно от
пользовательских примеров. Ниже оставлены только их назначение и команды.

```sh
python3 -B test/e2e/test_examples.py
```

Проверки запускают Python/shell processes либо проверяют static YAML
authoring contract: fd3, actual output bytes, state CAS sequence, warning/event
payloads, editor schema IDs и modelines. Compile-проверки product packages
живут в их собственных repositories, например в `StenHigh/prifly-aif-workflows`.
Это проверки обёртки и source compiler, **не замена**
сквозной квалификации настоящего Pri-Fly runtime. Среда должна разрешать
локальный Unix socket. Результаты полной квалификации и ограничения
фиксируются отдельно в release evidence.

Для сквозной проверки собранного CLI:

```sh
make build
python3 -B test/e2e/verify-cli.py --binary bin/prifly
```

Проверка создаёт отдельный временный проект и выполняет семь случаев: преобразование файла, две успешные проверки, ранний `rejected`, shell без Python, успешный результат с предупреждением, отказ при отсутствии обязательного output и pause → release → resume. Первые шесть Runs образуют telemetry cohort из семи Attempts: records/aggregate сверяются с CPU observations и известными знаменателями ошибок/предупреждений. Отдельный control Run добавляет восьмую Attempt и проверяет точный replay receipt. Экспортированные bytes сверяются с digest и ожидаемым содержимым. В напечатанной директории остаются полные ответы и `verification/summary.json` с SHA-256 проверенного binary. `--target DIR` позволяет явно выбрать пустую директорию. Проверка не заменяет crash/control/concurrency tests ядра.

## Core: конфигурация, проекции и обработка ошибки

```sh
make build
python3 -B test/e2e/verify-core.py --binary bin/prifly
```

[verify-core.py](../test/e2e/verify-core.py) создаёт отдельный проект с явным `init --profile core-workflow/1` и выполняет его через настоящий CLI. Нужны Python stdlib для проверки и JSON-шагов, а также `/bin/sh` для шага с известным отказом. Скрипт проверяет:

- Значение из закреплённого WorkflowRevision, project override и разрешённый run override; ранее принятые Runs не меняются вместе с текущим `prifly.json`.
- JSON Pointer projections в отдельные артефакты с exact schema refs, source provenance и sealed `JSONProjection`; отсутствие значения отличается от `null`.
- Отказ при run override параметра со scope `project`, до admission.
- Настоящий exit code 9: Attempt остаётся failed, `on_error` ведёт к объявленному `rejected`.
- Несоответствие результата projection его schema на `finish`: Run и control stage получают failed с Diagnostic, без StepInstance/Attempt; повторное открытие и Drive не запускают повторную обработку.
- `choice` с правилом `exclusive`: запускается только выбранный producer; следующий control stage читает JSON из его принятого output. Изменение исходного файла после Start не меняет закреплённое условие.
- Правило `first_match`: `unknown` перед истинной ветвью ведёт в `on_unknown`; ветви после первой истинной не вычисляются. Полный trace сохраняет `not_evaluated`.
- Две истинные ветви при `exclusive`: `ambiguous_branch`, failed StageActivation и переход в явный `on_error`, без вымышленной Attempt.
- Общий consumer после развилки: обязательный вход от producer одной ветви отклоняется до admission, даже если текущее значение выбирает эту ветвь; необязательный вход разрешён и остаётся отсутствующим при пропуске producer.
- Отказ для shell, SQL и template expressions в Predicate: schema validation не допускает Run, workspace, артефакт или побочный эффект.
- Два вызова одного child workflow с разными данными: отдельные invocation/activation/step/attempt identities, точные exports и provenance, общий бюджет и сохранённый partial outcome.
- Вложенный Call: parent не завершается вместе с leaf; одинаковые local Stage IDs не смешивают входы и результаты разных invocations.
- Локальные aliases в Registry v2 разрешаются до lock. Настоящий цикл A → B → A отклоняется до Run/worker; смена файла после Start не меняет закреплённый child.
- Repeat с двумя настоящими workers: первый body получает initial input, второй — отдельный next binding; until читает exact `iteration_output`. Решение, номер итерации и новый body фиксируются атомарно; последующий Drive инертен.
- Выход по лимиту, non-continuing outcome без чтения until и `unknown`, который имеет приоритет перед лимитом. Контрольные примеры не создают фиктивных Attempts.
- Отказ при 101 итерации: общая схема допускает это число, но квалифицированный local runtime ограничен 100; новый Run не создаётся.
- Положительные CLI-прогоны `parallel` и `map` с завершёнными child invocations и сводками, а также `wait` с durable registration и завершением по timeout на следующем Drive.

`--target DIR` принимает только пустую или отсутствующую директорию. Определения, входные файлы, экспортированные артефакты и ответы CLI остаются в проекте; `verification/summary.json` содержит hash бинарника, исходника проверки, команды и результаты. `--evidence FILE` задаёт другой новый файл; существующий evidence не перезаписывается. Проверка не заменяет crash/recovery tests или формальный gate F2.

Созданные `workflows/choice-worker.json`, `workflows/choice-unknown-before-true.json`, `workflows/choice-unknown-after-true.json`, `workflows/choice-ambiguous.json` и `workflows/choice-optional-consumer.json` можно изучать и запускать отдельно. `stage.choice_decided` содержит versioned ChoiceDecision с точными source refs и порядком проверки ветвей. `choice` не является подпиской: после фиксации решения повторный Drive не пересчитывает его по изменившимся данным.

`workflows/repeat-worker.json` показывает цикл с явным изменением input следующей итерации. `repeat-limit.json`, `repeat-noncontinuing.json` и `repeat-unknown-at-limit.json` показывают остальные пути без workers. Тело задаётся exact ref или локальным alias до lock; `stage.repeat_decided` содержит RepeatDecision с exact body ID и фактически прочитанными refs. Старые body остаются в истории; их отсутствующие outputs не подставляются вместо результата текущего body.

`make e2e` запускает проверку установщика, проверки обёртки, F1, Core и
полного контекста с checks. Точный статус capability и границы квалификации — в [delivery roadmap](../openspec/specs/delivery-roadmap/spec.md).

Новые Core projects выбирают exact `core:policy/local@2.0.0`, разрешающую глубину до 8 при одном worker, 256 StepInstances и 1024 control transitions. Прежняя policy `1.0.0` остаётся неизменной и не разрешает child depth. Генераторы берут `default_policy_ref` из ProjectConfig; inventory может содержать несколько версий одного ID. Workflows со scoped calls без repeat используют state/read v2; repeat в любом месте закреплённого closure требует v3. Прежние Runs сохраняют свои версии.

## Полный контекст и обязательные проверки

```sh
make build
python3 -B test/e2e/verify-context.py --binary bin/prifly
```

[verify-context.py](../test/e2e/verify-context.py) создаёт независимый проект с явными `core-configuration/2`, Registry3 и local adapter v2. [context-worker.py](../test/fixtures/context-worker.py) — обычный механический Step; [content-checker.py](../test/fixtures/content-checker.py) — отдельный исполнитель `check-request/1`, без StepInstance и publication credentials. Оба используют только Python stdlib, без AI или сети. Прежняя F1-обёртка не меняет протокол.

Сценарии проверяют SourceSnapshot без фиктивного Run, неизменность полученных bytes после изменения source file, instructions/data roles, все пять check boundaries, отрицательные и inconclusive reports, explicit on_error, обязательную инструкцию при overflow и отдельную check telemetry. Проверенный input не получает изменённую metadata. Повторный Drive не запускает завершённые checks. Успешный process и `pass` производителя не заменяют обязательное положительное свидетельство.

PDF fixture имеет корректный descriptor, но заведомо неверные bytes: объявленный checker отклоняет его. Этот пример не является PDF validator: отсутствие header даёт fail, наличие header — только inconclusive. Отдельный пример без content checker показывает ограниченную гарантию local profile: schema/descriptor и sealed bytes, пустой `content_check_evidence`, без утверждения о качестве PDF. Положительные checks распознают только явно описанный fixture JSON format; они не объявляются универсальным semantic review.

`--target DIR` принимает пустую или отсутствующую директорию; `--evidence FILE` — новый файл. Сохраняются hashes binary/scripts, stdout/stderr каждой команды, экспортированные bytes и результаты. `run timing` и telemetry показывают CheckExecution отдельно от producer Attempts. Проверки не квалифицируют sandbox, AI isolation, live retrieval, package trust или полный F2.

## Совместимость настоящих бинарников

[verify-upgrade.py](../scripts/verify-upgrade.py) проверяет F1 → новый Core. Отдельный [verify-choice-upgrade.py](../scripts/verify-choice-upgrade.py) проверяет сохранённый P2-01 → Core с `choice`:

```sh
python3 -B scripts/verify-choice-upgrade.py \
  --old-binary .cache/f2-compatibility/c0b8ef766414689fa6acc3dea39ac350554de1fd407822bcd1ebb89a913bebed/prifly \
  --new-binary bin/prifly \
  --evidence /private/tmp/prifly-choice-upgrade.json
```

Нужен именно сохранённый P2-01 executable с указанным SHA-256, а не пересборка нынешних исходников под старым именем. Проверка откажется принимать другой baseline. Каждый запуск требует нового пути evidence и создаёт свежие authorities; существующие результаты, база и identity не копируются и не изменяются напрямую.

Проверяются прежние Core Runs, принятый output, точный повтор Start и его receipt, telemetry на прежнем cut, pause → release → resume и один настоящий worker. До первого `stage.choice_decided` старый бинарник может читать ready Run, но должен отклонять компиляцию и Drive сценария с `choice`. После нового события старый бинарник отказывает при открытии всего authority, включая совместимый Run рядом с новым. Проверка сверяет неизменность Run, journal, cut и workspaces после таких отказов. Это проверка границы совместимости, а не механизм downgrade или восстановления резервной копии.

Тот же harness отдельно проверяет P2-02 → Call, включая старые choice histories:

```sh
python3 -B scripts/verify-choice-upgrade.py \
  --extension call \
  --old-binary .cache/f2-compatibility/fa421bd4bfa31e7ad4eb46e07c72bb857540cb7d53b3aa30a58b75984dd10e99/prifly \
  --new-binary bin/prifly \
  --evidence /private/tmp/prifly-call-upgrade.json
```

Здесь старый binary уже до первого invocation event отказывает чтению нового `core-state/2`, но ещё может читать совместимый соседний Run. После `invocation.created` он отказывает всей authority с `unsupported_storage_version`. Authority создаётся старым binary с прежним project policy default, чтобы отказ доказывал state/event boundary, а не неизвестную новую конфигурацию. В обоих режимах нужны настоящие сохранённые binaries, новые пути evidence и разрешённые native процессы.

Для P2-03 → Repeat используется отдельный режим того же скрипта:

```sh
python3 -B scripts/verify-choice-upgrade.py \
  --extension repeat \
  --old-binary .cache/f2-compatibility/41e5a84681aff5821ea213d5bb7d33fd5396df159b5bc9e08533fb46dc92dc17/prifly \
  --new-binary bin/prifly \
  --evidence /private/tmp/prifly-repeat-upgrade.json
```

Старый бинарник создаёт завершённый и paused Call со state2, настоящим worker и JSON projection. Проверяются прежние invocation trees, полные event prefixes, owner receipts, artifact/provenance bytes и telemetry на сохранённом cut; paused Call продолжается новой сборкой. До первого repeat event старый reader отказывает выбранному state3 Run, но читает соседний state2 Call. На границе `stage.repeat_entered`, ещё до первого RepeatDecision, проверяется отказ всей authority без изменения state/cut. Для наблюдения этой границы собственный cooperative worker ограниченно ждёт release-файл в своём workspace; тест не меняет SQLite и не создаёт поддельных events. После release две итерации выполняются ровно один раз. Это не разрешение на concurrent mixed-version writers или безопасный downgrade.

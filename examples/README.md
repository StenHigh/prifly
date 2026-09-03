# Примеры YAML

Здесь находятся только материалы для автора YAML-сценария. Legacy Python/shell
fixtures, F1 demo и проверки runtime находятся в [`test/`](../test/README.md).

## Справочники YAML для авторов сценариев

- [`authoring/workflow-authoring-reference.yaml`](authoring/workflow-authoring-reference.yaml) — все
  поля `prifly-workflow/1`, восемь видов stages, bindings, limits и comments.
- [`authoring/step-authoring-reference.yaml`](authoring/step-authoring-reference.yaml) — все поля
  `prifly-step/1`, ports, hooks и telemetry с comments.
- [`authoring/extension-authoring-reference.yaml`](authoring/extension-authoring-reference.yaml) —
  tracked `settings`, `exclude` и простая вставка шага через `extend.yaml`.
- [`authoring/project-profile-authoring-reference.yaml`](authoring/project-profile-authoring-reference.yaml) —
  полный `.prifly/project.yaml` с тремя фиксированными host roots.
- [`authoring/context-authoring-reference.yaml`](authoring/context-authoring-reference.yaml) —
  обычный текст и явный context source из выбранного host skills root.
- [`authoring/workflow-catalog-authoring-reference.yaml`](authoring/workflow-catalog-authoring-reference.yaml) —
  `catalog.yaml` каталога сценариев: категории и указатели `repository + path + ref [+ commit]`
  для `prifly project workflows search` и `add NAME`.

Готовых product workflows здесь нет: Pri-Fly — движок. Сценарии AI Factory
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

Оба файла — **справочники, а не готовые packages**: в них стоят нулевые digest.
Копируй из них только нужную форму и замени refs на exact значения из
`prifly inventory`. Обычный новый проект не обязан перечислять все поля —
безопасные defaults описаны в [workflow and context](../openspec/specs/workflow-and-context/spec.md).

## Проект с Pri-Fly: init, host runner и запуск

Package с context из host skills компилируется одной командой; `prifly-run`
передаёт один из `--host codex-cli`, `--host codex-app` или `--host
claude-code`, и context YAML закрепляет bytes только из соответствующего
skills root. Компилятор не импортирует package и не запускает Run, а также не
угадывает host по существующим папкам.

```sh
prifly project compile --repository . --package NAME --host codex-cli --output ../NAME.package
```

Для нового repository используйте `prifly project init --repository .`,
добавьте собственные `.prifly/` YAML packages и launches, затем закоммитьте
shared `.prifly/` и три generated host skills. После clone каждый разработчик
выполняет ту же команду: она проверяет общие files и создаёт только свой ignored
`.prifly/local.yaml`, не переписывая workflow или skills. Затем посмотрите
явные точки запуска и вызовите `prifly-run` из skills directory своего host:

```sh
bin/prifly project workflows --repository . --json
```

Product workflow не копируется при `project init`: владелец добавляет папку
сознательно, например `prifly project workflows add`. Host навык читает local
authority path и exact local Pri-Fly executable из ignored `local.yaml`,
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
обновления Pri-Fly владелец проекта обновляет три generated runner files
отдельным reviewed commit; существующий runner остаётся безопасным и позволяет
clone создать только свой ignored `.prifly/local.yaml`.

```sh
prifly project runners update --repository .
```

Команда заменяет только точные предыдущие generated runners. Если разработчик
изменил хотя бы один из них вручную, она отказывает и не перезаписывает другие
файлы: изменение сначала нужно перенести в reviewed project workflow.

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

`make e2e` запускает проверки обёртки, F1, Core и полного контекста с checks. Точный статус capability и границы квалификации — в [delivery roadmap](../openspec/specs/delivery-roadmap/spec.md).

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

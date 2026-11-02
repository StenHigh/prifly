# AI Factory classic workflow

## Unified composition 2.0.0

Requires Pri-Fly >=0.13.86 and explicit installation of the
[pinned shared schema dependency](https://github.com/StenHigh/prifly-aif-workflows/blob/v1.57.0/contracts/README.md). New templates use
`prifly-extension/5`; folders use `/3`. Existing installed project files and
sealed Runs are never rewritten automatically. Review the migration before a
new Run. [Complete examples and native checks](https://github.com/StenHigh/prifly-aif-workflows/blob/v1.57.0/examples/unified-composition/README.md).


Catalog installation copies this package folder only. Obtain the exact v1.57.0
source checkout separately. Run its `tools/install_composition_contracts.py`
against the selected authority before compilation. The package installer does
not install this dependency or copy the repository tools automatically.

## Быстрый поиск исходников

Основной путь читается в [workflow.yaml](workflow.yaml). Эта схема показывает
обзор; условия выбора security и все terminal routes остаются в YAML.

```text
warmup → plan → improve → implement → verify → security? → review → commit → done
                                      └─ gate требует участия → partial
```

| Что изменить или понять | Какой файл открыть |
|---|---|
| Entry, переходы и данные между стадиями | [workflow.yaml](workflow.yaml): entry, stages, input_bindings |
| Инструкции verify bridge | [Markdown](instructions/aif-verify-bridge.md), подключённый [context.source](contexts/aif-verify-bridge.yaml) |
| Подключённый upstream aif-verify skill | [contexts/aif-verify.yaml](contexts/aif-verify.yaml) |
| Executor, ports и read-only plan capture verify | [steps/verify.yaml](steps/verify.yaml) |
| Решение владельца и автоматический fix | [workflows/verify-once.yaml](workflows/verify-once.yaml): decide, owner-decides, fix |
| Выключатель и число повторов verify | [workflows/verify-batch.yaml](workflows/verify-batch.yaml): choose, round |
| Настройки конкретного проекта | Его extend.yaml: settings и exclude |

Участок verify:

```text
verify выключен → unchanged → прежняя implementation
verify включён → round → verify
                          ├─ clean → succeeded
                          ├─ owner-only / blocked → standing → partial
                          └─ fix → новая implementation → следующий round
                                                     └─ лимит → exhausted → partial
```

`decide.selection: first_match` проверяет owner-only перед blocking.
`max_iterations: 8` — верхняя граница графа; default `verify_round_limit: 3`
может быть изменён настройкой проекта. Успех fix требует повторной проверки.
Сбой самого шага (`fail`/`no_work`) следует отдельному `stopped` route;
verify-batch возвращает `no_work`, и корень завершает Run как `completed/rejected`.
Этот путь не требует `gate` и не публикует выходы корня.
Он также действует при `fail/no_work` шага fix внутри verify.

```text
verify или вложенный fix: fail/no_work → stopped → unstarted → rejected
                                                               └─ continue → verify целиком
```

Continue сохраняет принятые warmup/plan/improve/implement и рабочее дерево, включая изменения fix.
Новая проверка читает дерево как есть; новый Run начинает счёт раундов заново.
Предыдущий gate остаётся в истории и не становится результатом последней проверки.
`partial` по-прежнему требует пригодный gate; `blocked` с отчётом сохраняет этот маршрут.
Готовые команды и ограничения прежних Runs: [пример](../examples/classic/README.md).


`aif-classic` следует обычному порядку AI Factory: `warmup → plan → improve →
implement → verify → security → review → commit`. Security и review —
необязательные project features после реализации; `aif-qa` и `aif-evolve` не
навязываются каждому development run.

Разогрев отдаёт компактную выжимку выходом `handoff`, и планирование получает
её входом. Каждый шаг здесь работает в своём контексте, поэтому прочитанное
разогревом доходит до планирования только так: сессия, в которой он читал,
закрывается сразу после отчёта. Выжимка не повторяет источники, а называет их
путями — иначе носить её дороже, чем перечитать.

`aif-improve` выполняется последовательно. Первый круг получает plan из
workflow input, каждый следующий — plan предыдущего body из
`iteration_output`. Он применяет только изменения, принятые разработчиком.
Полный план выходит молча. Два bounded repeat limits можно уменьшить в
`extend.yaml`; достижение лимита пачки спрашивает разработчика перед новой
пачкой.

Plan между `aif-plan`, `aif-improve` и `aif-implement` — не JSON-пересказ
`summary/tasks`. Это тот же native файл или bundle AI Factory. Pri-Fly хранит
внутренний sealed `WorkspaceTreeManifest` с exact ArtifactRevision файлов,
materialize-ит их в claimed Workspace перед handoff и seal-ит изменённую
revision после него. Skill видит и редактирует только обычные `.ai-factory/`
файлы.

`extend.yaml.profile` задаёт reviewed team default одного документированного
AIF layout: `fast` для
`.ai-factory/PLAN.md`, `full` для одного `.md` непосредственно в
`.ai-factory/plans`, `ultra` для одного bundle в `.ai-factory/plans` с
`index.md` и phase-файлами. Для конкретного Run host спрашивает и передаёт
`--package-profile fast|full|ultra` до compilation; выбранное значение
попадает в sealed Decision Sheet. При handoff host передаёт `plan_profile` как
leading mode вызову `aif-plan` и все preflight context values как уже принятые
ответы, а не повторяет эту часть диалога. Это package profile, не Core runtime profile. Он
не читает и не угадывает AI Factory config: другой layout нужно сначала явно
описать отдельным profile в reviewed package; неизвестный profile compiler
отклоняет до создания sealed package.

[`decisions/`](decisions/) содержит pinned inventory вопросов upstream
`aif-plan`, `aif-improve`, `aif-implement` и `aif-commit`. YAML описывает
известную preflight-часть, включая ветку `roadmap_linkage → roadmap_milestone`,
а также один runtime выбор `improve_apply`. Primary context каждого изменяемого
AIF шага — package adapter; исходный upstream skill закреплён отдельным
supporting context. Adapter передаёт выбранный profile и preflight values без
повтора диалога, а `improve_apply` отправляет через universal Decision Bridge и
ждёт тот же Attempt. Markdown явно перечисляет остальные поздние native
dialogs: raw `AskUserQuestion` не выдаётся за захваченный Pri-Fly decision,
поэтому этот package пока не обещает полностью autonomous выполнение.

Verify, security и review — read-only gates. Их typed artifact `gate` ведёт
bounded round: пригодный blocking gate запускает отдельный `aif-fix`, затем
тот же gate проверяет исправление снова. Исполнитель гейта возвращает только
свой Attempt и не запускает `/aif-fix`, commit или следующий gate вручную;
`blocking_owner_only` завершает путь с находками для разработчика.
Круг review начинается со **второго читателя** (`review-challenge`): шаг в
своей сессии, по замыслу на другой модели, читает дифф, дерево и недавнюю
историю затронутых путей и отдаёт не gate, а список находок с доказательствами
(`challenge-findings`); шаг review обязан каждую проверить по дереву или
исполнением, выставить severity сам и слить в один документ — `blocking`
остаётся за ним. На живом заходе второй читатель нашёл настоящий блокер круга
и три дефекта в починке и не выдумал ни одной находки из девяти.
Команда может исключить `improve`, `verify`, `security` или `review` через
`extend.yaml`, не редактируя graph. Тем же файлом команда может и добавить
собственный шаг: `extensions` вставляет его в объявленное ребро маршрута.
Форма и её правила — в комментарии рядом с `extensions` в `extend.yaml`.

Каждый skill context использует `source: {root: host_skills, ...}`. Host
launcher передаёт `--host codex-cli`, `codex-app` или `claude-code`; compiler
закрепляет bytes соответствующего local skill и никогда не угадывает host по
папкам repository.

## Читаемые исходники: v1.54.0

Корень и verify-loop используют блочный YAML. Verify bridge подключает Markdown через `context.source`.
Значения графа и decoded bytes инструкции сохранены; author versions повышены.
Готовые task и полные команды: [самостоятельный classic/profiled](../examples/classic/README.md).
[Release guide](../docs/releases/v1.54.0.md) описывает обновление с сохранением `extend.yaml`.
Прежний отказ `verify fail/no_work` отслеживается отдельно в [issue #7](https://github.com/StenHigh/prifly-aif-workflows/issues/7).

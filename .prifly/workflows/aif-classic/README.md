# AI Factory classic workflow

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

Verify, security и review — read-only gates. Blocking result завершает путь
typed artifact `gate` с `suggested_next: /aif-fix`; fix step сам не запускается.
Команда может исключить `improve`, `verify`, `security` или `review` через
`extend.yaml`, не редактируя graph. Тем же файлом команда может и добавить
собственный шаг: `extensions` вставляет его в объявленное ребро маршрута.
Форма и её правила — в комментарии рядом с `extensions` в `extend.yaml`.

Каждый skill context использует `source: {root: host_skills, ...}`. Host
launcher передаёт `--host codex-cli`, `codex-app` или `claude-code`; compiler
закрепляет bytes соответствующего local skill и никогда не угадывает host по
папкам repository.

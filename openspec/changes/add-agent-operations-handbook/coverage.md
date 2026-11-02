# Карта покрытия справочника

Это план покрытия, а не готовое эксплуатационное руководство.
Исходный снимок: `2060fc9286cff2924e198b0294ba9a2814ad0e90`.
Перед реализацией перечень сверяется с текущими dispatch, command help, flags и capabilities.

## Страницы и операции

| Страница | Что обязательно описать | Текущие источники проверки |
|---|---|---|
| `index.md` | Намерение, симптом, команда, capability; как выбрать страницу; версия поставки; поиск без Core checkout | `examples/README.md`, `examples/troubleshooting.md`, CLI help/capabilities |
| `installation.md` | Supported install; managed/source/copied binary; `version`, `update`; stable channel; receipt; exact executable; `project init`, clone/bootstrap; local authority; hosts; `project local set`; `runners add/update/--check`; workflows list/search/add/update/remove; origin/ref/commit; local drift; catalog pins | `README.md`, `internal/release/`, `project.go`, `project_workflows.go`, release-distribution spec |
| `launch.md` | Launch ID/title/description; явный выбор; native question tools; typed inputs/defaults/ref; `/2`, `/3`, `/4`; profiles; standing answers; workspace; preflight program; questionnaire; prepare; catalog digest; summary/review digest; start; author version/build key/compiled refs; legacy migration | project-profile reference, launch-choice guide, `project_questionnaire*`, `project_start*`, `project_build*` |
| `hosting.md` | `run list/status/next/drive`; next action/stage_work; `session task --all`, `take`, `submit --template`, `submit`, `disconnect`; Attempt map by ID; workspace/context/slots; StepResult; artifact digest; questions; model profile/translation; time limits; active vs decision wait; `context_request`; `step publish/status`; program process/result/progress descriptors; program environment | runner template, step/context/execution-bindings references, CLI and published-contracts specs |
| `decisions.md` | Catalog; preflight/runtime/preanswers; applicability true/false/unknown; attended/autonomous; policy boundaries; `run decisions`; `run decision request/answer`; request/envelope digest; expected version; `--yield-execution`; redelivery; native skill questions; `answered_questions` basis | run-decision/extension references, run-decisions spec, runner Decision Bridge |
| `recovery.md` | `project continue` другого и того же workflow; source/current claim holder; checkpoint; accepted prefix; `--from-stage`; duplicate continuation; `project recover`; `run reopen/resume`; pause vs terminal; business fail vs technical failure; writer cancel; unknown effects; `run resolve`; occupied captures; `run fork` отдельно от project protocol | continuation/blocked guides, domain-execution spec, active recovery deltas |
| `workspace.md` | worktree/checkout; no-Git program; read-only repository_workspace; `permitted_effects`; effects.class/external_write/retry_class; claims create/create-set/list/heartbeat/release; lease/generation; naming; workspace-tree input/output; tree drift; dirty repo; parallel runs; merge ownership; byproducts | project-profile/step references, runtime-resources spec, `claims.go` or actual claim handler |
| `authoring.md` | Полный маршрут поля к existing reference; `project compile/extend`; folder revisions; schemas/steps/checks/contexts; input bindings/defaults/projections; choice/call/repeat/parallel/map/wait/finish; error routing/technical retries; blocked/partial; extensions/points/composition; settings/resources; compatible revisions | все `examples/authoring/*`, composition/points examples, workflow-and-context spec |
| `artifacts.md` | `ref`; `artifact import/inspect/export`; `source import`; `task prepare`; JSON/blob/media; canonical bytes vs raw text; schema refs; `schema` list/name/`--def`; `inventory`; package import directory/archive/signature; inspect/verify/list; remove/quarantine/revoke/restore/trust-root; sealed/trusted distinctions; публикации и subscriptions | CLI handlers, core schemas, workflow-and-context and control-security-ux specs |
| `control.md` | `run pause/cancel/stop/release/resume/reopen/resolve/waive/waivers`; `control status/stop/release`; epochs/CAS; reasons/command IDs; `command receipt`; lost response; approval policy/request/decide/revoke/list; grant issue/revoke/list; action propose/admit; session action; proposed/admitted/delivered/effect distinctions | main CLI, cli-protocol/control-security-ux specs |
| `observability.md` | Full monitor/Light; registry/scan roots; loopback/proxy; `run explain/events/timing/output/progress`; pagination/next_after; JSON paths; bounded tail; declared live output; progress freshness; source staleness; telemetry catalog/query; reported costs vs actual provider usage; unknown metrics | monitor handlers, Light guide/active source set, local-run-monitor and observability specs |
| `operations.md` | `project queue add/list/show/update/claim-next/release/complete/reconcile`; explicit intake; owner/generation/revision; selection flags; lost dispatch; no hidden scheduler; capacity show/set; exclusive resources/resource_busy; storage budget show/prepare/set; doctor/doctor --storage/--claim; disk vs budget; reviewed monitor cleanup/dependencies/locks/VACUUM/backup | queue handler/spec, storage-recovery guide, runtime-resources/local-run-monitor specs |
| `troubleshooting.md` | Symptom -> read-only diagnosis -> exact Problem code -> relevant page -> next action/stop; stale review, missing inputs/host, package conflict, modified runner/workflow, unsupported version, timeout, storage, unknown effects, no progress, missing question report | существующий `examples/troubleshooting.md`, refusal contracts, practical fixture cases |

`help`, `--help`, `--version`, `exit-codes`, `--project`, `--json` и `--format` описываются в index.
Разница между repository root, authority root, attempt workspace и executable показывается до первой команды.
Для каждого flag фиксируются тип, default, обязательность, совместимость и источник значения.
JSON-пути берутся из фактического ответа, а не из старого примера.

## Как связать capabilities с процедурами

Полный список берётся из `prifly capabilities`; текущий author index остаётся независимой проверяемой картой.
Группы ниже определяют страницы, но не заменяют проверку каждой записи.

| Группа capabilities | Основная страница | Обязательное взаимодействие |
|---|---|---|
| step, finish, choice, call, repeat, parallel, map, wait, schedule | authoring | Ревизии, bounded loops, три исхода choice, join и late signal |
| on_error, partial, declared_technical_retries, impossible/routed verdicts, blocked | authoring + recovery | Технический retry отличается от бизнес-исхода и нового Run |
| assisted_session, timing/unbounded work, stage_work, next_awaiting_host | hosting | Выданный Attempt, take и выход из ожидания без дубликата |
| context_resources/full_context/context_request/input_configuration | hosting + authoring | Pinned instructions и запрос дополнительного контекста |
| model_profile_declared/model_profile_translation/answered_questions | hosting | Честный результат модели и полный отчёт о вопросах |
| execution_bindings/environment source/program_environment | hosting + installation | Sealed environment и machine-local изменения после запуска |
| local_process/program_external_write/program_live_output/program_progress | hosting + observability + recovery | fd результата отдельно от fd прогресса; retry_class и unknown effects |
| project_call_extensions/project_extension_points/composition/composition_with_points | authoring | opt-in revisions; owner boundaries; carrier bindings; compiled variants |
| workspace_modes/workspace_tree_artifacts/materialize-only/repository read/effects/naming | workspace | Read-only вход, дерево выхода и занятая capture path |
| decision_catalog/neutral_start/workflow_continuation/workflow_resume/next_handoff | launch + decisions + recovery | Defaults, source inputs, prefix и новый review |
| automatic_checks/quality_waivers | hosting + control | Process exit, check verdict и waiver — разные результаты |
| state/event hooks, artifact publications/checks/close, subscriptions once/each/new_only/blob/terminal failure | artifacts + authoring | Producer failure, cardinality, завершение keyed stream и reader binding |
| source_import, local aliases, projections, run_fork | artifacts + recovery | Exact refs, canonical bytes и source provenance |
| action proposal/admission/grant/prepared delivery | control | Prepared delivery не обещает реализованную внешнюю доставку |
| telemetry catalog/records/aggregate, reported_cost, named finish/failure | observability | Unknown values, origin метрик и terminal reason |
| exclusive_resources/storage_budget_management/project_task_queue/light_task_board | operations + observability | Ресурсное ожидание, reviewed mutation, explicit intake и read-only projection |
| live_guards и любые остальные записи текущего capabilities | authoring либо соответствующая операция | Объяснить фактический путь применения и границу публичного интерфейса |

Нельзя писать новую CLI-команду только потому, что её имя встречается в существующем author index.
Например, ссылки на `prifly schedule` требуют сверки с фактическим dispatch установленного binary.
Если публичного handler нет, страница описывает authoring/runtime границу и не публикует выдуманную команду.
Такие расхождения фиксируются как documentation gap до объявления покрытия полным.

## Матрица вариантов

| Измерение | Значения для документации и проверки |
|---|---|
| Host | codex-cli, codex-app, claude-code; отсутствие отдельной session/model selection |
| Установка | managed stable; та же/новая версия; source/copy без receipt; неподдержанная платформа |
| Workflow | tracked branch; fixed tag/commit; direct Git origin; catalog origin с новым ref; local author package |
| Состояние файлов | unchanged; изменённый upstream file; свои extend/project/PROJECT; отсутствующая manual page; custom skill; symlink/conflict |
| Project | clone без local.yaml; profiles /2,/3,/4; один/несколько/no launch; typed defaults; no-Git program |
| Workspace | worktree; checkout; no workspace; repository read-only; output-only scratch; dirty tree; claim conflict |
| Inputs | JSON file; ArtifactRef; default; preflight binding; missing; invalid schema; URL внутри declared task |
| Decisions | standing answers; explicit override; attended/autonomous; applicable/conditional/not applicable; async без ответа |
| Stage | control; program; assisted; nested call/repeat/parallel; resource wait; decision wait; host wait; terminal |
| Result | pass/fail/blocked/partial по контракту; technical failure; uncertain; waived; missing/invalid output |
| Recovery | другой workflow; same-launch resume; recover на новом package; inherited prefix; cancelled writer; duplicate child |
| Updates | успех без изменений; обновление; network/signature failure; partial update; ref policy; concurrent invocation |
| Observability | known/stale/missing/unavailable; pages of events; live output enabled/disabled; Light source fallback |

Полное декартово произведение не требуется.
Проверяются каждое значение, документированные ограничения и сценарии пересечения с существенным риском.

## Практические приёмочные задачи без исходников Core

| ID | Задание агенту | Проверяемый результат |
|---|---|---|
| H01 | Запустить существующий launch с typed task и standing answers | Точный launch; updates до prepare; summary соответствует build; один Run |
| H02 | Выполнить program-only workflow без Git и ИИ | Нет выдуманного host/brief/workspace; корректный результат программы |
| H03 | Забрать outstanding assisted Attempt и отправить результат | task/take/template/output digest/submit; новая выдача не создаётся |
| H04 | Ответить на runtime decision с ожиданием человека | Exact request digest/version; корректный yield/redelivery; timeout не считается ответом |
| H05 | Разобрать blocked/partial через `run next` | Причина берётся из arrived_from и accepted outputs; нет случайного restart |
| H06 | Выбрать continue, resume, reopen или recover | Операция соответствует исходу; prefix и current claim сохранены |
| H07 | Восстановиться после отмены writer | Owner resolution до нового выполнения; partial files не удалены |
| H08 | Диагностировать resource_busy, storage exhaustion и claim conflict | Ожидание отличается от отказа; reviewed budget; без ручной правки authority |
| H09 | Найти причину terminal outcome, прогресс и stderr программы | Публичные explain/progress/output; правильные JSON-пути и pagination |
| H10 | Изменить authoring с retries, input default и extension point | Найдены точные поля и минимальная поддержка; compile без исходников Core |
| H11 | Обновить старый runner с PROJECT и своими настройками workflow | Все три host variants; старый runner узнаётся; пользовательские bytes сохранены |
| H12 | Запустить при network failure либо modified workflow | Новый Run/claim/import отсутствует; точная причина и partial updates в отчёте |
| H13 | Проверить fixed ref и более новый catalog release | Ref не переключается без выбора; ограничения проверки явно показаны |
| H14 | Забрать queue task при потерянном ответе dispatch | receipt/reconcile; одна связь с Run; повторного запуска нет |
| H15 | Найти отсутствующую или несовместимую manual page | Публичные help/schema либо явный пробел; нет угадывания или поиска Core |
| H16 | Продолжить issued Attempt без сети | Pinned instructions исполняются; проверка новой установки не вмешивается |

Для H01–H16 исходники самого целевого fixture-проекта остаются доступны, если задание требует их менять.
Запрещён только доступ к исходникам движка как способу узнать публичную процедуру.
Отчёт указывает доступные измерения токенов; отсутствие измерителя не превращается в обещанный процент экономии.

## Граница готовности

Готовность требует: обе стороны command/capability coverage, работающие ссылки установленной копии, проверенные процедуры и отсутствие регрессий обновления.
Существующие author references не заменяются краткими пересказами; непокрытое поле дополняется в его каноническом reference.
Ни одна галочка этой карты не закрывает F2, release qualification или production rollout.

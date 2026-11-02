# Pri-Fly Light: настройка read-only обзора

Контракты: `prifly-light-board/1`, `prifly-light-task/1`,
`prifly-light-reviews/1`, `prifly-light-report/1`,
`prifly-light-projection/1`. Минимум — сборка с capability
`light_task_board` и этими contracts (эта реализация пока unreleased).
Новый workflow YAML contract или изменение saved Run для Light не нужны.

## Запуск

```sh
prifly monitor --ui light --addr 127.0.0.1:7782 \
  --full-monitor-url http://127.0.0.1:7777/
# Отдельный proxy prefix, assets и API под тем же prefix:
prifly monitor --ui light --addr 127.0.0.1:7782 \
  --base-path /light/ --full-monitor-url https://monitor.example/full/
```

Это отдельный frontend/listener. Без `--ui light` остаётся полный monitor,
включая прежнее обслуживание. Auto-start Run также остаётся full monitor.
Light health: `/api/health` под выбранным prefix, `service=prifly-light/1`.
Для доступа извне сохраняйте свой authenticated reverse proxy, направляющий
запросы к loopback с корректным Host. Forwarded headers не обходят Host guard.
Не публикуйте listener напрямую и не отключайте существующую authentication.
Домены и service configuration не входят в Core. Rollback отключает только
Light service/route; runtime state и queue не мигрируют.

## Что показывает карточка

Native QueueTask видна до первого Run. Её title/revision и recorded Run bindings
определяют группировку; равные titles и разные authorities не объединяются.
Самостоятельный Run получает отдельный fallback. История и несколько текущих
ветвей читаются по страницам. Claimed task после succeeded Run показывает
«Результат получен», а «Готово» задачи требует owner complete. Это локальное
завершение, без обещания merge/deploy/внешней приёмки.

Generic workflow работает без mapping: actual states/stages, причины и «этап не
сопоставлен». Review без достаточных typed данных — «нет данных». Missing,
disabled/N/A, pending, defect, warnings и unavailable не становятся PASS.
`StepResult pass` говорит об исполнении шага, status его gate читается отдельно.
После repair приёмка прошлого candidate остаётся только исторической.

## Exact mapping

`--light-projection FILE` читает JSON один раз до listener, максимум 256 KiB;
после изменения файла нужен restart Light. Структура доступна через
`prifly schema LightProjection`. В файле exact `workflow_ref` и schema refs
с `id/version/digest`, не маски namespaces. Один mapping digest фиксируется
в каждом поколении данных. Никаких скриптов или произвольных файлов в mapping.

[Optional W1/W2/W5 presets](../light/projection-presets.json) — observer data
Core для source SHA `3499115956903abd264fbde2be2f432d07bc2c43`. Они не меняют
classic/profiled, feature defaults, versions или reader composition. Получите
сохранённый `MonitorRunView` через read API полного локального monitor:

```sh
# source — зарегистрированный source key, id — exact Run ID, параметры URL-encoded.
curl --get --fail http://127.0.0.1:7777/api/run \
  --data-urlencode 'source=SOURCE_KEY' --data-urlencode 'id=RUN_ID' > /tmp/run-view.json
python3 scripts/light-projection.py --run-view /tmp/run-view.json \
  --output /tmp/light-projection.json
prifly monitor --ui light --addr 127.0.0.1:7782 \
  --full-monitor-url http://127.0.0.1:7777/ \
  --light-projection /tmp/light-projection.json
```

Resolver читает только указанный JSON и bundled presets, не обращается к сети
и не выбирает newest version. Missing/ambiguous exact refs дают отказ.
У profile `/3` и новее compiled components могут иметь `0.0.0-b1.…` вместо
authored version. Тогда явно передайте `--build-provenance /path/to/build-provenance.json`
из того же проверенного compiled package (`prifly package verify`): resolver
сопоставляет `author_ref` → `compiled_ref` и требует exact совпадение compiled ref
с sealed Run. Без этого файла неизвестная build version не угадывается по ID.
Файл provenance — явно выбранный источник оператора; resolver не сканирует
installation и не выдаёт проверку его структуры за независимую package qualification.

Декларации можно объединить в один файл для нескольких поддержанных Run closures;
разные digests остаются разными workflow mappings. Не меняйте digest вручную.

Для собственного workflow задайте в `workflows[]`:

| Поля | Что объявляют |
|---|---|
| `workflow_ref` | Exact закреплённый WorkflowRevision, включая digest |
| `phases[].stage/key/label` | Exact stage → `plan/implementation/review/work` и понятная подпись; phase не меняет outcome |
| `reviews[].lens` | Название объявленного reader scope, не доказательство отдельной модели |
| `report_stage/report_port/report_schema` | Accepted output точного producer и его typed schema |
| `request_port/candidate_port` | Exact consumed input ports reader; request digest вычислен по этим raw bytes |
| `candidate_stage/candidate_output` | Current capture в basis invocation; пустая пара допускается только при отсутствии такого producer, с консервативной freshness |
| `gate_stage/gate_alternatives/gate_port/gate_schema` | Exact возможные принимающие gate producers/typed output; несколько gates для одного report дают invalid, не выбор по времени |
| `gate_scope/gate_ancestor` | `lens/aggregate` и число parent invocations от reader к gate/basis; 0–8 |
| `gate_report_inputs` | Input ports gate с exact report Ref либо typed bundle refs+raw_base64; bundle bytes/digest проверяются |
| `repair_stages/repair_ancestor` | Exact repair stages и ancestor scope; начало recorded repair инвалидирует прежний PASS |
| `enabled` | `null` либо `{input, pointer, equals}`: scalar equality/`in` по typed invocation input либо bounded `all`/`any` (depth ≤4, ≤64 узлов); unknown не считается false/disabled |
| `disabled_state` | `disabled` либо `not_applicable`, если recorded predicate не выполнен |
| `round_pointer/candidate_pointer/request_pointer/checks_pointer` | Обычно `/round`, `/candidate`, `/request_digest`, `/checks` typed report/request |
| `gate_status_pointer/gate_blocking_pointer/gate_owner_pointer/findings_pointer` | Обычно `/status`, `/blocking`, `/blocking_owner_only`, `/findings`; для typed nested assessment допускается `/gate/status` и остальные `/gate/...` |

Стандартный check composition содержит `id`, `required`, `enabled`, `applicable`;
report checks содержат `id`, `status`, `result`, `reason`. Candidate — typed
объект, совпадающий с consumed capture. Report round/request/candidate обязаны
соответствовать exact consumed request. Другие producer formats требуют
явной compatible declaration; parsing prose не поддерживается. Optional skip
показан coverage/замечаниями, не как выполненная проверка.

У W1 lens определяется preset/request, у report нет собственного lens field.
W2 UI и UX раздельны, gate aggregate; environment/release blockers не доказывают
успешную общую приёмку. W5 sequential и parallel используют разные producer
invocations. Начальный feature round 0 не пересчитывается в report round 1:
читается actual request/report. UI/UX отсутствуют, если producer invocation не
вошёл в backend путь. При недостаточной gate correlation (например, только
outer finish export) summary остаётся неизвестным; дополнительные reviewers
не запускаются. Заявление session/model — наблюдение producer, не независимая
квалификация.

## Чтение и ограничения

Board: `api/light/board`, default 25, максимум 50 карточек; task/reviews
постраничны, report preview до 64 KiB по explicit click. Cursor закрепляет
source generation vector, query и ограниченный retention; просроченный cursor
просит обновить страницу данных с сохранёнными filters. Totals/counters имеют
coverage; недоступный source не считается пустым здоровым проектом.

Фоновое чтение использует согласованный source cut. Разные authorities имеют
свои cuts и свежесть. Неизменившиеся snapshots/reports не декодируются повторно;
queue документ разбирается один раз на queue version. Cold index и изменение
большой monolithic queue требуют полного metadata/queue прохода в фоне. Нет
обязательного disk scan, графов или полных event traces на каждом polling.

Инженерные tests, released replay и production acceptance различаются. Наличие
опций CLI в development build не доказывает server rollout или принятие E6-A10.

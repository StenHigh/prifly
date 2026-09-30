## Context

`GET /api/run` уже отдаёт весь Run, и монитор держит его в `state.run`. Всё, что
нужно блокам, в нём есть (проверено на Run `aif:workflow/classic` пилота):

| Что показать | Где лежит в Run |
|---|---|
| порты входа шага и их артефакты | `attempts[a].context.inputs[port].ref` |
| откуда пришёл вход | `definition.stages[stage].input_bindings[port]` закреплённого workflow: `{from: workflow_input, port}` или `{from: stage_output, stage_id, port}` |
| значения входов запуска и источник | `state.input_values`, `run.effective_configuration.inputs[port].source` |
| переданные агенту ответы | `attempts[a].session.decision_context` (имя назначения → значение; `core:package_profile` — профиль) |
| названия решений | `run.decision_catalog.decisions[*]`: `title`, `choices[].title`, `destination.name`, `when` |
| источник ответа | `run.decision_sheet.records[*].source` (`actor`, `project_default`, `autonomous_policy`) |
| вопросы шага | `run.decision_ledger[*]` с `attempt_id` этого Attempt (`source`, `value`, `observed`, `closure_reason`); ожидающий — `run.pending_decision`; текст и варианты — `decision_catalog.decisions[*].title/description/choices` |
| переданные тексты | `attempts[a].session.skill_refs` (для assisted) → `run.context_resources[*]` по digest; для старых Run — `instructions_ref`/`context_refs` определения шага |
| ожидаемые выходы, эффект | определение шага (`run.definitions[*].bytes` по `steps[s].definition_ref`): `outputs`, `effects`, `external_write`; `session.workspace_trees` |
| вердикты с продолжением | ключи `on` стадии в закреплённом workflow |

Монитор по спецификации только наблюдает; `monitor_ui_test.cjs` уже грузит
`monitor.js` в Node и проверяет чистые функции (`graphData`, `nodeCard`,
`activityRows`).

## Goals / Non-Goals

**Goals:** человек за один взгляд видит, с чем шаг начал работу, и может
раскрыть любой текст целиком; сырые разделы не трогаются.

**Non-Goals:** правка движка или `/api/*`; показ переписки агента, diff,
stdout; перевод текстов промптов; редактирование решений из монитора.

## Decisions

1. **Сборка на клиенте из `state.run`, без нового API.** Все сведения уже в
   ответе `/api/run`; серверная проекция дублировала бы правило и потребовала
   бы версии ответа. Альтернатива — `run explain`-подобная проекция в
   `internal/runtime` — отложена, пока CLI не понадобится то же самое.
2. **Переданные ответы берутся из `session.decision_context`, а не
   пересчитываются из листа.** Это то, что агент реально получил; пересчёт
   повторил бы `decisionSessionContext` второй копией и мог бы разойтись с ней.
   Нет `decision_context` — пишем «не записано в этом Run». Имя назначения
   сопоставляется с решением каталога по `destination.name`; `core:package_profile`
   — по `destination.kind == package_profile`. Источник — из записи листа с тем
   же `definition_id`; ответ runtime-решения из `decision_ledger` помечается
   «ответ во время Run».
3. **Две чистые функции + рендер.** `stepReceipt(run, attempt, workflow)` и
   `launchInputs(run, inputValues)` возвращают простые объекты, рендер их только
   рисует. Функции экспортируются через тот же `module.exports`, что и
   `graphData`, и проверяются в `monitor_ui_test.cjs`.
4. **Тексты и артефакты — через существующие `<details>`/`artifact()`.**
   Содержимое контекста уже лежит в `run.context_resources[*].bytes`; артефакт
   входа грузится лениво существующим `/api/artifact`. Раскрытие переживает
   опрос, так как `preserve()` уже хранит `data-key`.
5. **Место на экране.** Блок стоит первым в карточке Attempt, после карточки
   узла; при выборе стадии без Attempt показывается блок её последнего
   исполнения, если оно есть. Обзор Run получает блок после «Сейчас».
6. **Подписи по-русски, идентификаторы видны.** Название решения — из каталога
   как записано автором (может быть по-английски), рядом id; источник ответа —
   русской фразой из таблицы `labels`.

7. **Вопросы шага честно ограничены объявленными.** Pri-Fly видит только
   runtime-решения каталога пакета. Если навык прочитал заранее запечатанный
   ответ из `decision_context` сам, не отправив запрос, записи с `attempt_id`
   нет — тогда вопрос показывается как «ответ был передан заранее», по
   связке `destination.name` ↔ ключ `decision_context`. Нативные вопросы
   навыка в чате агента (например, уточнение «какие именно» после `select`)
   движок не видит; их запись — отдельное изменение движка и пакета, не этого.

## Risks / Trade-offs

- [Большие тексты контекста (60 KB) в DOM] → текст вставляется только при
  раскрытии `<details>`, как у артефактов.
- [Старые Run без `session.skill_refs`/`decision_context`] → fallback на
  определение шага для текстов и явное «не записано» для ответов; проверяется
  фикстурой.
- [Входной binding из вложенного workflow (`call`)] → стадия ищется в
  workflow своей invocation (`currentWorkflow`/`definitions`), а не только в
  корневом; при неудаче — «происхождение не найдено», без догадки.

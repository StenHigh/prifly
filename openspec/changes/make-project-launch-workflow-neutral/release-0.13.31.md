# Выпуск Pri-Fly 0.13.31

Два OpenSpec-изменения одним тегом на `7233285`:
`add-input-only-workspace-tree-binding` и `report-run-failure-in-the-read-envelope`.

## Что было

- После захвата плана шагом `implement` шаги `verify` и `review` работали в
  claim-worktree без файла плана: upstream `aif-verify` уходил в fallback
  («No plan artifact found»). Единственная форма binding'а дерева требовала
  `workspace_write` и `output_port`; давать read-only gate право записи ради
  чтения — против измеряемых `permitted_effects` (0.13.19). Замер пакетчика
  на 0.13.29: `schema_invalid … requires output_port, capture`.
- Run, остановленный `effect_not_permitted` шага-программы, читался как
  `failed` с `outcome: null`; причина лежала только где-то в `diagnostics[]`
  (заход пилота).
- Текст runner'а говорил «keys `run.attempts` by `attempt_id`», а view
  называет поле `id`.

## Что стало

- **StepDefinition v8**: `workspace_trees[]` с `input_port` и `capture` без
  `output_port` — только на assisted шаге с `effects.class: none`; авторская
  форма `prifly-step/1` lower-ится в v8 сама. Runtime materialize-ит entries
  манифеста в claim того же Run **до** отпечатка рабочей копии, отдаёт шагу
  `repository_workspace`/`workspace_mode`, `materialized_entries` и guide
  `workspace-tree-guide/2`, ничего не захватывает, при отчёте сверяет байты
  materialized entries (git status содержимого untracked не видит) и после
  settle снимает ровно то, что положил, вместе с опустевшими родителями.
  Отказы компиляции: форма без `output_port` на `workspace_write`, с
  `output_port` на read-only шаге, смесь форм, v7 без `output_port`.
- **State/read 30** (`materialized-session` bundle): handoff несёт
  `materialized_entries`; view `failed`/`cancelled` Run несёт `failure
  {code, diagnostic_id, attempt_id, step_instance_id}` — последняя диагностика
  уровня error, выведенная при чтении. /29 и старше — байт в байт прежние,
  старые Run читаются как прежде.
- Текст runner'а: `run.attempts[].id` (SessionTask — `attempt_id`); прежний
  текст заморожен как двенадцатый вариант, `project runners update` заменяет.

## Ворота

`make ci-check` (runtime 240.8 с; staticcheck 9 пакетов без находок; vuln-check
чисто; все bundle'ы совпали, новые `materialized-session` 233 424 байта и
`step-definition-v8` 9 911 байт), `make e2e` (6 наборов), `make race`
(`cmd/prifly` 171.3 с, `internal/runtime` 697.0 с, гонок нет),
`openspec validate --all --strict` 22/22 — всё на `7233285`. Стенд пакетчика на
кандидате: четверо ворот зелены с проводкой плана в verify/review (1.38.0 в
дереве), стенды `1 of 9`/`1 of 10`, совместимость в обе стороны (1.37.0 на
кандидате компилируется; 1.38.0 на 0.13.30 отказывает). Run выпуска
35226471352, все три job'а success.

## Проверено на опубликованном бинаре

После `prifly update` → `0.13.31`: capabilities `core-state/30`,
`core-read/30`, `materialize_only_workspace_tree`, `run_failure_named`;
`aif-classic` 1.37.0 в этом репозитории — `--prepare` с прежним ключом сборки;
`project runners update` заменил оба runner'а. Живой verify с
материализованным планом — первый чужой стенд после `aif-classic` 1.38.0.

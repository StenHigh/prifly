## Why

Run пилота завершился `failed` (шаг-программа `tests` отказан
`effect_not_permitted`: тесты записали два файла в рабочую копию), а
`run status --json` при этом даёт `.run.outcome = null`, поля `failure` у Run
нет вовсе, и причину видно только в `diagnostics[]` — читателю приходится
угадывать, какая из диагностик остановила Run. Рядом второе наблюдение того же
захода: текст runner'а говорит «a Run keys `run.attempts` by `attempt_id`»,
а объект attempt в `run status` несёт `id` (`attempt_id` — поле SessionTask).

Изменение затрагивает опубликованный read contract (`core-read`) и текст
runner'а; сохранённые Run и state contract не меняются. Ownership источников
не меняется.

## What Changes

- **Read envelope `core-read/30`**: у терминального Run со статусом `failed`
  или `cancelled` view несёт `failure` — code остановившей диагностики, её id,
  attempt и step, где она возникла; для `completed` и незавершённого Run поле
  отсутствует. Значение выводится из diagnostics при чтении, state Run не
  меняется; `core-read/29` и старше остаются закрытыми и читаются как прежде.
- **Runner `prifly-run`**: текст называет поле как есть — `run.attempts[].id`
  (SessionTask несёт `attempt_id`, это значение равно `id` попытки); прежний
  текст замораживается для `project runners update`.
- Заметки «`session task --json` даёт `session_limits: null`» и
  «`claim list --json` — массив» в код не сходятся (в SessionTask такого
  ключа нет, `claim list` отдаёт объект `foundation-claims/2` с `claims`);
  у пилота запрошены точные пути — вне этого change.

## Capabilities

### New Capabilities

<!-- нет -->

### Modified Capabilities

- `observability-publication-reactions`: read view терминального Run называет
  причину остановки отдельным versioned полем.

## Impact

- `internal/runtime/model.go` (`RunView`, версия `core-read/30`),
  `compatibility.go` (read versions), место построения view (`View`);
  генерируемый public bundle для нового read contract, `scripts/check-schema.py`.
- `cmd/prifly/project.go`: текущий runner template + замороженная копия
  предыдущего; тест распознавания старого runner'а.
- `examples/troubleshooting.md`: запись «Run `failed`, `outcome` null — где
  причина».

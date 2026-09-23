## Why

Два Run одного Project уже могут держать отдельные worktree claims, но допуск очередной `workspace_write` Attempt ищет единственный активный claim во всём authority. Второй Run получает `claim_ambiguous`, и первый тоже останавливается на следующем шаге.

## What Changes

- При Project Start атомарно привязывать его exact claim к новому Run; при неявном выборе сначала использовать единственный активный claim, уже привязанный к данному Run.
- Для сохранённого Project Run без привязки распознавать только claim с точной парой производных command/claim identities, затем закреплять его обычным admission.
- Сохранить отказ при нескольких claims одного Run и при неоднозначных непривязанных legacy claims.
- Проверить выдачу и повторный выбор claims двум Run одного репозитория при capacity 2.
- Отразить исправление в текущей очереди; отдельные ресурсы тестового стенда остаются задачей scheduling.

## Capabilities

### Modified Capabilities

- `runtime-resources`: claim admission выбирает ресурс по сохранённой привязке к Run.
- `delivery-roadmap`: фиксирует работу над обнаруженным отказом.

## Impact

Сохранённые JSON-поля и Run не меняются. Исправление касается выбора уже записанного claim до admission.

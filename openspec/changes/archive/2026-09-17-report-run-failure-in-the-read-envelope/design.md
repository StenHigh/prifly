## Context

См. proposal.md — Why. `RunView` (`core-read/29`, `internal/runtime/model.go`)
несёт `run` (state `core-state/29`), `timing` и служебные поля; причина
терминального `failed` живёт в `run.diagnostics[]`. Public bundle read
contract'а закреплён по digest (`effects-session.schema.json` для /29).
Текст runner'а — замороженная цепочка шаблонов в `cmd/prifly/project.go`:
текущий текст выводится из предыдущего, `project runners update` распознаёт
каждый прежний байт в байт.

## Goals / Non-Goals

**Goals:**
- Один взгляд на `run status --json` называет, что остановило Run, без
  просмотра всех диагностик.
- Старые reader'ы и saved Runs не меняются.

**Non-Goals:**
- Изменение `run.outcome` для `failed` (outcome — предметный исход workflow,
  у остановленного Run его нет по `domain-execution`).
- Новое состояние (`core-state`) и миграция хранилища.
- `session_limits`/`claim list` — не подтверждены кодом; ждут точных путей.

## Decisions

0. **Граница — `core-read/30` вместе с `core-state/30`**, той же, что вводит
   change `add-input-only-workspace-tree-binding` (state там нужен ради
   `materialized_entries` handoff'а); `failure` по-прежнему выводится при
   чтении и в state не пишется, но bundle `materialized-session` описывает
   обе стороны сразу, чтобы не плодить два номера за один выпуск.
1. **Поле в read envelope, не в state.** `failure` выводится при `View` из
   diagnostics (последняя blocking-диагностика терминального Run по
   `severity`/порядку), поэтому state и его bundle не меняются — только
   `core-read/30` и новый public bundle. Альтернатива — писать `failure` в
   state при остановке — потребовала бы `core-state/30` и миграции ради
   производного значения.
2. **Только `failed`/`cancelled`.** У `completed` есть `outcome`; у
   незавершённого Run причины ещё нет; `uncertain` держит `has_unresolved_
   effects` и `run resolve` — поле там сбивало бы с толку.
3. **Runner text: «`run.attempts[].id`»**, с оговоркой в той же фразе, что
   SessionTask называет ту же попытку `attempt_id`. Прежний текст — новый
   замороженный `projectRunnerSkillTemplateBefore…` по образцу 0.13.24.

## Risks / Trade-offs

- [Несколько blocking-диагностик у одного Run] → берётся та, что связана с
  переходом в терминальный статус (последняя по порядку записи); остальные
  по-прежнему в `diagnostics[]`.
- [Хосты на старом runner-тексте] → `project runners update` заменяет только
  exact прежний текст; PROJECT.md не трогается.

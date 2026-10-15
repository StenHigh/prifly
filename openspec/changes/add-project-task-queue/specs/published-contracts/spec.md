## ADDED Requirements

### Requirement: Queue binding имеет достижимую opt-in compatibility boundary

Queue документы SHALL иметь отдельные closed versioned schemas и authority происхождение. Run с QueueTaskBinding SHALL запечатываться новой границей `core-state/44` и читаться как `core-read/44`; прежние state/read bundles и Runs MUST оставаться byte/meaning compatible. Project queue review/result SHALL использовать `project-launch-summary/4` и `project-start/4` только при queue selection; запуск без неё SHALL сохранять прежние версии.

Новые canonical definitions, generated public bundles, readers, writers, capability declarations, glossary mappings и author index SHALL изменяться вместе. Structural tests SHALL читать реально запечатанный queue Run и его read output, а также проверять legacy отсутствие queue binding; исторические bundles и release evidence MUST NOT переписываться. Отдельная queue state/read форма SHALL явно различать task revision, claim generation, intent и Run association.

#### Scenario: Queue-bound Run реально запечатан
- **WHEN** reviewed queue-bound start допускает Run
- **THEN** сохранённый Run и read output принимаются новым declared bundle и содержат exact binding; прежняя closed версия не получает это поле

#### Scenario: Прежний start без очереди
- **WHEN** обычный project start или legacy reader работает без queue selection
- **THEN** версии/поля прежнего контракта сохраняются, queue не появляется автоматически

## ADDED Requirements

### Requirement: Read-only repository claim передаётся только объявившему его шагу

Run MUST иметь выбранный и связанный с ним Git claim до допуска шага с `repository_workspace: read_only`. Handoff такого шага MUST нести точный путь, identity и mode claim; его `permitted_effects` MUST оставаться следствием класса эффекта и MUST NOT получать workspace-write permission от факта чтения. Для шага без workspace-write effect проверка неизменности дерева при отчёте MUST сохраняться. Сам Pri-Fly MUST NOT исполнять команды, описанные в тексте assisted step.

#### Scenario: Claim отсутствует
- **WHEN** Run содержит step с read-only декларацией, но selected claim отсутствует
- **THEN** Start отказывает до dispatch и не выдаёт task без рабочего каталога

#### Scenario: Агент получает read-only дерево
- **WHEN** Run со связанным claim доходит до объявившего чтение assisted step
- **THEN** task называет этот claim и путь, но не разрешает запись в дерево

#### Scenario: Агент изменил дерево без права
- **WHEN** host read-only step изменил claimed Git tree до отчёта
- **THEN** отчёт получает отказ `effect_not_permitted`, как и у других шагов без workspace-write effect

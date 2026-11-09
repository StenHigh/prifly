[Индекс](index.md) · [Данные и пакеты](artifacts.md) · [Команды](commands.md)

# Общие схемы после ошибочного revoke

Используйте миграцию, если прежний package отозвали ради обычного обновления, а его schemas проверены и безопасны.
Возможность `shared_schema_migration` требует контрактов migration `/1`, package authority `/2` и Run/read `45`.
Минимальная версия Core — `0.13.89`.
[Released acceptance](../../openspec/changes/archive/2026-11-09-migrate-revoked-shared-schemas/evidence/released-v0.13.89/README.md)
подтверждает generic scope. Private authority acceptance не выполнялась и требует отдельной проверки владельца.

Для обычного вывода edition используйте `package remove`. Команда отказывает при живых holders и dependents.
`package quarantine` временно прекращает resolution; `package restore` возвращает доверие после проверки.
`package revoke` означает security withdrawal и остаётся terminal. Миграция не возвращает доверие старому package.
Если содержимое действительно небезопасно, выберите безопасную новую schema revision. Не применяйте исключение ради обхода security revoke.

```
compile/import -> prepare -> review -> apply -> explicit Start
                                                |
                                  каждый admission проверяет binding
```

## Подготовка и review

1. Выполните обычные `project compile` и `package import` для выбранного consumer и его dependencies.
2. Найдите exact package refs через `package list` и `package inspect --ref REF.json`.
3. Создайте `request.json` контракта `SchemaMigrationRequestV1`. Начните с пустого `selections`.
4. Выполните `package schema-migration prepare --request request.json --json > plan.json`.
5. Проверьте все schemas, revoked packages, `status_reason`, `trust_reason`, suppliers и влияющие Runs.
6. Выберите supplier для каждой заблокированной schema. Supplier непосредственно объявляет schema и входит в declared consumer closure.
7. Повторите prepare с полным выбором. Продолжайте только при `complete: true` и `applicable: true`.

`prepare` ничего не импортирует, не создаёт Run/claim и не запускает hooks.
Ограниченный scan не выдаёт применимый план при превышении budget.
План охватывает весь набор заблокированных schemas consumer. Частичное исключение не применяется.
Пример request и сохранённого binding: [справочник](../authoring/schema-migration-guide.md).
Machine shapes доступны через `prifly schema SchemaMigrationRequestV1` и `prifly schema SchemaMigrationPlanV1`.

## Применение и новый Run

1. Скопируйте `review_digest` из проверенного плана.
2. Выполните apply с причиной и стабильным command id.
3. Проверьте сохранённое решение через inspect.
4. Передайте возвращённый decision ID новому Start явно.

```sh
prifly --project AUTHORITY package schema-migration apply --plan plan.json --expected-review-digest DIGEST --reason 'Проверены точные schemas после ошибочного retirement' --command-id command:reviewed-schema-migration
prifly --project AUTHORITY package schema-migration inspect --id DECISION_ID --json
prifly --project AUTHORITY run start --workflow WORKFLOW.json --brief BRIEF.json --schema-migration DECISION_ID
prifly --project AUTHORITY project questionnaire --repository REPOSITORY --launch LAUNCH --host HOST --prepare --schema-migration DECISION_ID --json
prifly --project AUTHORITY project start --repository REPOSITORY --launch LAUNCH --host HOST --schema-migration DECISION_ID --expected-launch-digest LAUNCH_DIGEST
```

Дополните Start обычными inputs и подтверждениями выбранного launch.
Project требует профиль `/3` или новее для выбора миграции. Legacy `/2` отказывает до import, claim и Run.
Project summary/start `/9` сохраняет выбранный ID. Review digest меняется при изменении выбора.
Start проверяет exact consumer и принадлежность workflow. Run связывает только schemas своего настоящего workflow closure.
Первый успешный apply атомарно переводит package authority `/1` в `/2`. Старый binary должен отказать на новом state.
Пакет остаётся revoked. Старые Runs, executable content и trusted reuse старых результатов сохраняют прежний запрет.

## Отказы и повтор

`schema_migration_incomplete` требует полного выбора и нового prepare.
`schema_migration_plan_stale` требует нового prepare и review. Любое изменение общего authority cut может сделать план stale.
Изменение версии влияющего Run считается изменением даже при прежнем количестве Runs.
`schema_migration_integrity` требует проверки sealed bytes через `package verify` и inspection.
`schema_migration_scope_mismatch` требует exact consumer, schema и supplier из scope.
`schema_migration_decision_invalid` требует inspect: lifecycle generation, dependency либо набор revoked suppliers изменился.
`object_access_denied` означает отсутствие права owner на trust operation.

При потерянном ответе повторите тот же apply с теми же plan, reason, digest и command id.
Движок возвращает исходный receipt даже после следующих изменений authority.
Изменённый payload с тем же command id даёт `command_conflict`.
Новый revoke, включая повтор прежнего revoke новой командой, отменяет пригодность старого решения.
Restart перечитывает decision и binding. Он не выбирает нового supplier автоматически.

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
Прежний Project summary/start `/9` сохраняет выбранный ID. Новый `/10` добавляет источник выбора и точный scope; старый bundle остаётся неизменным. Review digest меняется при изменении выбора.
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

<a id="saved-launch-choice"></a>
## Сохранить выбор для launch

Нужна capability `project_launch_schema_migration`, Project profile `/3` или новее и summary/start `/10`.
Это development-возможность после Core `0.13.90`; опубликованный `0.13.90` её не предоставляет.
Сначала проверьте `prifly capabilities --json`. Наличие `shared_schema_migration` само по себе не доказывает поддержку сохранённого выбора.

LaunchSchemaMigrationChoice — сохранённый выбор уже одобренного решения для одной точной сборки launch в одной authority.
Установка и update пакета его не создают. Выбор не меняет исходное решение и terminal revoke.

```text
inspect + owner set -> prepare показывает saved -> start проверяет digest
                             |                            |
                        generation                   Run + Binding
```

1. Выполните inspect с теми же repository, launch, host и package profile, что использует запуск.
2. Проверьте `current.build_key`, exact consumer/workflow refs и существующее решение через `--id`.
3. Выполните set с проверенными build key и generation из inspect.
4. Повторите обычные prepare/start без `--schema-migration`.

```sh
prifly --project AUTHORITY project schema-migration inspect --repository REPOSITORY --launch LAUNCH --host HOST --id DECISION_ID --json
prifly --project AUTHORITY project schema-migration set --repository REPOSITORY --launch LAUNCH --host HOST --id DECISION_ID --expected-build-key BUILD_KEY --expected-generation GENERATION --reason 'Проверен выбор для этого launch' --command-id command:save-launch-choice --json
prifly --project AUTHORITY project questionnaire --repository REPOSITORY --launch LAUNCH --host HOST --input task=TASK.json --prepare --json
prifly --project AUTHORITY project start --repository REPOSITORY --launch LAUNCH --host HOST --input task=TASK.json --expected-launch-digest LAUNCH_DIGEST --command-id command:new-launch --json
```

`task` в примере — порт выбранного workflow; используйте его настоящие inputs и остальные проверенные параметры.
Для каждого нового Run используйте новый command ID. Повтор принятой команды возвращает прежний результат.
Prepare показывает `schema_migration` и `schema_migration_choice` с `source: saved`, scope и generation.
Start через saved требует `--expected-launch-digest`. Он проверяет generation в транзакции принятия Run, включая запуск из очереди.

Непустой разовый `--schema-migration ID` получает приоритет и показывает `source: explicit`.
Этот flag не меняет запись. Неправильный явный ID получает отказ без fallback; пустое значение означает отсутствие разового выбора.
При смене source повторите prepare, даже если ID совпал.

Set заменяет запись при текущей generation. Старый выбор не распространяется на другой launch, repository, authority или build key.
Изменение compile values, host-selected bytes, consumer Ref или workflow Ref требует повторной проверки владельцем.
Prepare/Start сообщают `schema_migration_scope_mismatch`, если сборка отличается, и не выбирают другое решение.
Утрата действительности Decision или supplier сохраняет существующие migration/integrity отказы.

```sh
prifly --project AUTHORITY project schema-migration clear --repository REPOSITORY --launch LAUNCH --expected-generation GENERATION --reason 'Удалён устаревший выбор' --command-id command:clear-launch-choice --json
```

Clear работает без compile, в том числе при повреждённом profile или удалённом launch.
Inspect сохраняет записанную часть и показывает ошибку текущей сборки в `reasons`.
После clear `state.choice` равен null, а generation сохраняет историю изменения.
Clear и повторный set того же ID не восстанавливают старый launch digest.
Удаление настройки не меняет binding уже принятого Run; текущие lifecycle checks продолжают действовать.
При `schema_migration_choice_stale` повторите inspect и проверьте новую generation.
Никогда не снимайте revoke и не редактируйте SQLite ради повторного запуска.

Если start принял Run, но первый Drive завершился ошибкой, повторите start с прежними аргументами и command id.
Повтор возвращает прежний Run и исходный review даже после clear или замены выбора.
Для продолжения выполните `next_command` из ответа; повтор start сам не запускает Drive снова.

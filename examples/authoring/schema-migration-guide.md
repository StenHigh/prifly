# Schema migration: request и Run binding

Возможность `shared_schema_migration` использует public CLI и сохранённый Run.
Она не добавляет YAML-поле workflow или шага.
Project требует профиль `/3` или новее для выбора миграции. Legacy `/2` отказывает до import, claim и Run.
Минимальные контракты: migration `/1`, authority-packages `/2`, `core-state/45`, `core-read/45`; Project summary/start `/9` при явном выборе.
Минимальная версия Core — `0.13.89`.
[Released acceptance](../../openspec/changes/archive/2026-11-09-migrate-revoked-shared-schemas/evidence/released-v0.13.89/README.md)
подтверждает generic scope. Private authority acceptance выполняется отдельно.

Request передаётся в `package schema-migration prepare --request request.json`.
Ниже показана полная форма. Замените refs точными установленными refs из своей authority.
Digest из нулей показывает форму поля и не обозначает проверенный package.

```json
{
  "schema_version": "schema-migration-request/1",
  "consumer_ref": {"id": "example:package/consumer", "version": "1.0.0", "digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"},
  "selections": [
    {
      "schema_ref": {"id": "example:schema/report", "version": "1.0.0", "digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"},
      "supplier_ref": {"id": "example:package/schemas", "version": "2.0.0", "digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"}
    }
  ]
}
```

Для первого диагностического prepare используйте `selections: []`.
Полный выбор покрывает все заблокированные schemas consumer closure.

`run status RUN_ID --json` возвращает binding в `run.schema_migration`.
Следующий фрагмент показывает это поле; остальные поля `core-read/45` опущены.

```json
{
  "run": {
    "schema_migration": {
      "decision_id": "schema-migration:recorded-decision",
      "review_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
      "consumer_ref": {"id": "example:package/consumer", "version": "1.0.0", "digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"},
      "selections": [
        {
          "schema_ref": {"id": "example:schema/report", "version": "1.0.0", "digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"},
          "supplier_ref": {"id": "example:package/schemas", "version": "2.0.0", "digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000"}
        }
      ]
    }
  }
}
```

Binding сохраняет provenance допуска только этого нового Run. Его нельзя переносить в другой Run как разрешение.
Полный операторский маршрут и отказы находятся в [процедуре](../operations/schema-migration.md).

<a id="saved-launch-choice"></a>
## Сохранённый выбор project launch

Это настройка локальной authority, не поле YAML workflow и не новое trust decision.
Требуются `project_launch_schema_migration`, Project profile `/3+`, Choice `/1` и Project summary/start `/10`.
Поддержка находится в development после Core `0.13.90`. Операторский путь: [inspect/set/clear](../operations/schema-migration.md#saved-launch-choice).

Пример полей записи `LaunchSchemaMigrationStateV1` (точные refs и digest берутся из inspect, не составляются вручную):

```json
{
  "schema_version": "launch-schema-migration-choice/1",
  "scope": {"authority_id": "authority:example", "project_id": "project:example", "repository": "/project", "launch": "aif-fix"},
  "generation": 1,
  "choice": {
    "decision_id": "schema-migration:example",
    "review_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
    "build_key": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
    "consumer_ref": {"id": "example:package/fix", "version": "1.0.0", "digest": "sha256:2222222222222222222222222222222222222222222222222222222222222222"},
    "workflow_ref": {"id": "example:workflow/fix", "version": "1.0.0", "digest": "sha256:3333333333333333333333333333333333333333333333333333333333333333"}
  },
  "actor": "local:uid:1001", "reason": "Проверен scope", "command_id": "command:save-choice"
}
```

Этот образец показывает форму, но не предоставляет разрешение. Фактические refs и digest берите из inspect; запись создаёт public set.
Prepare `/10` добавляет `schema_migration_choice.source: saved` либо `explicit`, scope и effective choice.
Для `saved` поле `generation` входит в launch digest. Run закрепляет прежний SchemaMigrationBinding и state/read `/45`.
Clear сохраняет следующую generation с `choice: null`. Замена записи не меняет старый Run.

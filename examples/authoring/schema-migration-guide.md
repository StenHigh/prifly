# Schema migration: request и Run binding

Возможность `shared_schema_migration` использует public CLI и сохранённый Run.
Она не добавляет YAML-поле workflow или шага.
Минимальные контракты: migration `/1`, authority-packages `/2`, `core-state/45`, `core-read/45`; Project summary/start `/9` при явном выборе.
Текущий статус — development change `migrate-revoked-shared-schemas`; release acceptance проверяется отдельно.

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

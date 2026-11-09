# Передача владельцу: source, release и private acceptance

## Фактическая граница

Source implementation содержит reviewed prepare/apply/inspect и explicit Start.
Native source tests проверяют несколько schemas, два последовательных шага, nested admissions, restart и сохранение запрета old Run.
Source tests не являются проверкой выпущенного binary.
Задача 6.2 остаётся открытой до release gates и generic acceptance на фиксированном released binary.
Private authority acceptance не выполнялась. Агент не менял рабочую authority владельца ради ремонта issue #21.
Исходное baseline evidence для 0.13.88 остаётся в `baseline.md` и `baseline-0.13.88.json`.

```
source checks -> owner merge/release -> released generic fixture
                                           |
                                  owner private acceptance
```

## Перед generic acceptance

Владелец выбирает SHA релиза после merge. Push и release выполняет владелец.
Полный release gate относится к этому фиксированному SHA:

```sh
python3 scripts/ensure-release-checks.py --sha RELEASE_SHA --ref main --wait
```

Приёмка использует опубликованный binary этого релиза. Зафиксируйте его version, SHA256 и provenance релиза.
Не подменяйте released binary локальным `go build` и не переносите source PASS в release evidence.
Fixture должна содержать несколько shared schemas, два последовательных шага и old Run без binding.
Пройдите исходный отказ, read-only prepare, owner apply, explicit Start, последующую работу и restart.
Отдельно проверьте old Run refusal, repeat revoke, stale inputs и cleanup временной authority/monitor registration.
Положительный результат относится только к synthetic fixture. Он не подтверждает ремонт private authority.

## Private acceptance: команды владельца

`PRIFLY_BIN` — абсолютный путь проверенного released binary.
`AUTHORITY` — существующая authority владельца из его `.prifly/local.yaml`.
`REPOSITORY`, `LAUNCH` и `HOST` — выбранный проект и launch.
Сначала убедитесь, что revoke был ошибкой ordinary retirement. Для действительно небезопасных schemas исключение не применяется.
Проверьте новую поставку:

```sh
"$PRIFLY_BIN" version --json
sha256sum "$PRIFLY_BIN"
"$PRIFLY_BIN" capabilities --json
"$PRIFLY_BIN" --project "$AUTHORITY" package list --json
```

Capabilities должны объявлять `shared_schema_migration` и state/read `45`. Новые public contracts проверьте командой `schema`.
Подготовьте consumer обычными compile/import. Используйте точные refs из installed package inspection.
Заполните `request.json` контракта `SchemaMigrationRequestV1` сначала с `selections: []`.

```sh
"$PRIFLY_BIN" schema SchemaMigrationRequestV1 --json
"$PRIFLY_BIN" --project "$AUTHORITY" package schema-migration prepare --request request.json --json > diagnostic-plan.json
```

Проверьте все schema refs, всех revoked suppliers, независимые status/trust reasons и влияющие Runs.
Выберите exact supplier для каждой заблокированной schema. Supplier входит в declared consumer closure и непосредственно объявляет schema.
Проверьте consumer, suppliers и зависимости через `package verify --id ID --version VERSION`.
Повторите prepare с полным выбором:

```sh
"$PRIFLY_BIN" --project "$AUTHORITY" package schema-migration prepare --request request.json --json > reviewed-plan.json
```

Продолжайте только при `complete: true` и `applicable: true`.
`REVIEW_DIGEST` — точное поле `review_digest` проверенного файла.
Сохраните plan и явную причину. Примените весь scope одной командой:

```sh
"$PRIFLY_BIN" --project "$AUTHORITY" package schema-migration apply --plan reviewed-plan.json --expected-review-digest "$REVIEW_DIGEST" --reason 'Владелец проверил точные schemas после ошибочного retirement' --command-id command:owner-reviewed-schema-migration --json
"$PRIFLY_BIN" --project "$AUTHORITY" package schema-migration inspect --id "$DECISION_ID" --json
"$PRIFLY_BIN" --project "$AUTHORITY" project questionnaire --repository "$REPOSITORY" --launch "$LAUNCH" --host "$HOST" --prepare --schema-migration "$DECISION_ID" --json
"$PRIFLY_BIN" --project "$AUTHORITY" project start --repository "$REPOSITORY" --launch "$LAUNCH" --host "$HOST" --schema-migration "$DECISION_ID" --expected-launch-digest "$LAUNCH_DIGEST" --json
```

`DECISION_ID` возвращает apply. `LAUNCH_DIGEST` возвращает questionnaire.
Добавьте обычные inputs, naming inputs, standing decisions и approvals программ выбранного launch.
Для raw Run вместо Project Start используйте:

```sh
"$PRIFLY_BIN" --project "$AUTHORITY" run start --workflow WORKFLOW.json --brief BRIEF.json --schema-migration "$DECISION_ID" --json
"$PRIFLY_BIN" --project "$AUTHORITY" run status "$RUN_ID" --json
"$PRIFLY_BIN" --project "$AUTHORITY" run next "$RUN_ID" --json
"$PRIFLY_BIN" --project "$AUTHORITY" run drive "$RUN_ID" --json
```

Проверьте `run.schema_migration`, exact consumer/supplier refs, state/read `/45` и минимум два admissions.
После restart используйте тот же binary и authority. Inspector должен читать сохранённое решение.
Old Runs не получают binding. Их прежние bytes, pins, events, effects и запреты сохраняются.
Новый revoke требует нового review. Изменённый payload с прежним command id не является retry.
При потерянном ответе повторите прежний apply с теми же файлами, digest, reason и command id.

## Evidence, которое возвращает владелец

1. Fixed SHA, released binary version/digest и результаты release gates.
2. Generic fixture report с несколькими schemas, последующей работой, restart и cleanup.
3. Отдельный private report: точные refs, review digest, decision ID, Run ID и результаты admissions.
4. Фактический исход private acceptance: PASS, FAIL либо не выполнена.

Не записывайте private contents в публичное evidence. Не меняйте SQLite, terminal revoke или historical release evidence.
Change нельзя архивировать как полностью принятый, пока задача 6.2 не имеет фактического результата.

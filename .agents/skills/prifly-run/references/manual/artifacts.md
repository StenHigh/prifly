[Индекс](index.md) · [Авторинг](authoring.md) · [Управление](control.md)

# Данные, схемы и доверенные пакеты

## Exact references и digest

Immutable definition ref содержит `id`, `version`, `digest`.
ArtifactRef содержит `artifact_id`, `revision`, `digest`. Эти формы не взаимозаменяемы.
Ссылки из package inspect или task берите целиком; не собирайте их из author version и похожего ID.

```sh
"$PRIFLY_BIN" ref FILE --id ID --version X.Y.Z --json
"$PRIFLY_BIN" ref TEXT.txt --id ID --version X.Y.Z --raw-text --json
```

JSON/YAML канонизируется. `--raw-text` хеширует точные UTF-8 bytes контекста.
Raw `sha256sum` pretty JSON может дать другой digest, чем canonical JSON artifact.
Не меняйте sealed bytes после расчёта digest и не подставляйте digest другого port.

## Найдите форму данных

```sh
"$PRIFLY_BIN" schema --json
"$PRIFLY_BIN" schema SessionSubmissionV7 --def runtime_SessionSubmission --json
"$PRIFLY_BIN" schema WorkspaceTreeLocation --def runtime_WorkspaceTreeLocation --json
"$PRIFLY_BIN" --project "$authority_root" inventory --json
"$PRIFLY_BIN" --project "$authority_root" package inspect --component PACKAGE_SCHEMA_ID --json
```

`schema` читает встроенные wire и authoring contracts по точному имени index.
`--def` возвращает definition вместе с closure её refs, вместо большого bundle.
Schema с package-declared ID читается через package inspect, а не через guessed встроенное имя.
Если несколько editions объявили component, уточните `--package ID@EXACT_COMPILED_VERSION`.
JSON component находится в `component`, prose context — в `component_text`.

## Artifact и source import

```sh
"$PRIFLY_BIN" --project "$authority_root" artifact import --file DATA.json --type json --schema-ref SCHEMA_REF.json --json
"$PRIFLY_BIN" --project "$authority_root" artifact inspect --ref ARTIFACT_REF.json --json
"$PRIFLY_BIN" --project "$authority_root" artifact export --ref ARTIFACT_REF.json --output /absolute/scratch/export.json --json
"$PRIFLY_BIN" --project "$authority_root" source import --file DATA.json --type json \
  --schema-ref SCHEMA_REF.json --media-type application/json --json
"$PRIFLY_BIN" --project "$authority_root" task prepare --input TASK.json --json
```

Blob import использует `--type blob`; JSON проверяется по declared schema.
Source import создаёт SourceSnapshot; `--external-identity`, `--external-version`, `--external-scope` — непроверенные внешние metadata.
Task prepare принимает TaskInput/1, запечатывает источник и создаёт RunBrief.
Это низкоуровневый путь. Project `/3` launch может принимать typed inputs без RunBrief.
Экспорт пишет только явно указанную output path и требует соответствующих host permissions.

## Package lifecycle

```text
authoring -> compile -> sealed bytes -> import/trust -> exact Run pin
```

```sh
"$PRIFLY_BIN" --project "$authority_root" package import --dir SEALED_DIR --reason TEXT --json
"$PRIFLY_BIN" --project "$authority_root" package import --archive PACKAGE.tar --signature PACKAGE.sig --reason TEXT --json
"$PRIFLY_BIN" --project "$authority_root" package list --json
"$PRIFLY_BIN" --project "$authority_root" package verify --id ID --version EXACT_VERSION --json
"$PRIFLY_BIN" --project "$authority_root" package inspect --ref REF.json --json
```

Import seal/verify/trust выполняет явное локальное решение. Ничего из package при import не исполняется.
Ключ внутри package не назначает себя trusted root.

```sh
"$PRIFLY_BIN" --project "$authority_root" package trust-root --id KEY_ID --public-key HEX --reason TEXT --json
"$PRIFLY_BIN" --project "$authority_root" package trust-root --id KEY_ID --remove --reason TEXT --json
```

| Команда | Действие и ограничение |
|---|---|
| `package quarantine` | Ограничивает использование указанной edition по явной причине |
| `package remove` | Снимает trust и освобождает inventory pins; отказывает при удерживающем Run |
| `package restore` | Возвращает removed edition с теми же ID/version |
| `package revoke` | Terminal запрет edition; не отменяется restore/remove |
| `package verify` | Перечитывает exact sealed bytes; несовпадение сообщает, не ремонтирует |

Для lifecycle mutation передайте `--id ID --version EXACT_VERSION --reason TEXT`.
Remove не удаляет физически все bytes; это отдельная операция от reviewed storage cleanup.
Отзыв может запретить прежний pin; не переписывайте Run ради снятия такого запрета.

## Publications и контекст

`session publish --file COMMAND.json` и `step publish --file COMMAND.json` используют declared hook, artifact и cardinality.
`step status` читает состояние из admitted local step environment.
`session action --file COMMAND.json` относится к declared effect protocol, не к публикации произвольного ответа.
Subscriptions связывают producer и consumer через contract. Once/each/new_only не являются общей внешней event bus.
Точные формы requests получайте через schema index; inputs/outputs/hook IDs берите из pinned task/step.

См. также [host outputs](hosting.md), [effects и trust](control.md), [авторинг](authoring.md).

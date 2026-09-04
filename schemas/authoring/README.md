# YAML editor contract

Эта папка — локальный, versioned contract для подсказок YAML-редактора. Она не
заменяет `prifly project compile`: только compiler проверяет exact refs,
bindings, limits, permissions и graph целиком.

При работе из checkout используйте пути из [`manifest.json`](manifest.json).
В распакованном release они лежат в `source/schemas/authoring/`. Ничего из этой
папки не нужно копировать в `.prifly/`: это не часть project source и не
попадает в sealed package.

## Portable modeline

Для yaml-language-server и совместимых editor добавьте первой строкой нужного
YAML-файла comment (путь должен быть доступен на вашей машине):

```yaml
# yaml-language-server: $schema=/absolute/path/to/prifly/schemas/authoring/step-v1.schema.json
```

Comment не является YAML field и игнорируется Pri-Fly. Full authoring
references уже показывают вариант с относительным путём. JetBrains IDE также
можно связать с тем же local JSON Schema через **Settings → Languages &
Frameworks → Schemas and DTDs → JSON Schema Mappings**.

## Mappings

| Файл | Schema |
|---|---|
| `.prifly/project.yaml` | `project-profile.schema.json` (markers `/2` и `/3`) |
| `.prifly/workflows/NAME/workflow.yaml` | `project-workflow-folder-v1.schema.json` |
| `.prifly/workflows/NAME/extend.yaml` | `extension-v1.schema.json` |
| `.prifly/workflows/NAME/decisions/**/*.yaml` | `run-decision-v1.schema.json` |
| `.prifly/workflows/NAME/workflows/**/*.yaml` | `workflow-v1.schema.json` |
| `.prifly/workflows/NAME/steps/**/*.yaml` | `step-v1.schema.json` |
| `.prifly/workflows/NAME/checks/**/*.yaml` | `check-v1.schema.json` (full `check-definition/1`, profile `/3`) |
| `.prifly/workflows/NAME/contexts/**/*.yaml` | `context-v1.schema.json` |
| `catalog.yaml` в корне репозитория каталога сценариев | `workflow-catalog-v1.schema.json` |

Schema ловит форму документа и опечатки в известных полях. После изменения
сценария выполните обычную проверку:

```sh
prifly project compile --repository . --package NAME --output ../NAME.package
```

Для привязки строго к одной версии используйте `project-profile-v2.schema.json`
или `project-profile-v3.schema.json` в modeline. Единственный автоматический
pattern у общей schema: редактор не должен одновременно требовать оба marker.
Переход `/2` → `/3` выполняется явной правкой `schema_version` в shared
`project.yaml`, с обновлением version-specific modeline при его наличии.

Fresh init создаёт `/3` без hosts и Git. В `/3` `hosts` можно опустить или
перечислить только нужные; `project runners add --host HOST` подключает
выбранный runner. `/2` сохраняет все три roots: `codex-cli`, `codex-app` и
`claude-code`. Для context source `{root: host_skills, path: SKILL/PATH}`
добавьте `--host HOST` к compile: compiler не угадывает host по папкам и не
читает skills другого host. Exact bytes попадают в sealed package.

Profile `/3` назначает package и owned components детерминированные compiled
versions `0.0.0-b1.…`, поэтому разные profiles/settings/context bytes могут
сосуществовать в одной authority. Авторские версии YAML не меняются; соответствие
хранится в `build-provenance.json` sealed package. Внешняя ссылка использует
exact compiled ref, не author `ID@version` как alias «последней» сборки.
`/2` сохраняет прежние refs и отказы конфликтующих вариантов. `/3` не требует
host, Git или RunBrief, когда выбранный workflow от них не зависит.

Root `workflow.yaml` может объявить `execution_bindings.steps` и
`execution_bindings.checks`: ключ — полный ID owned definition, значение —
логическое имя executable, argv, supporting files и execution limits.
[Аннотированный reference](../../examples/authoring/execution-bindings-authoring-reference.yaml)
показывает все поля. Machine paths задаются только в ignored `local.yaml`
через `project local set --allow-executable NAME=/absolute/path`, а выбранный
запуск требует `--allow-execution`. Compilation и установка ничего не исполняют.

`checks/` принимает полные CheckDefinition без shorthand/defaults: все семь
полей обязательны, `content` допускает `content_valid`, а `result` —
`check_passed` или `semantic_review`. [Reference](../../examples/authoring/check-authoring-reference.yaml)
показывает exact adapter ref и поддержанный placeholder. Package с owned check
получает manifest `/2`; остальные сохраняют manifest `/1`. Это не новый
executor: check program использует существующий `check-request/1` contract.

`extend.yaml` может содержать три независимые части: `settings` задаёт значения
уже объявленных project-scoped inputs, `exclude` выключает named optional
features, а `extensions` вставляет простой no-input step. `exclude` не удаляет
stage: author объявляет feature в `workflow.yaml`, связывает его с boolean
project input и сам описывает оба route через обычный graph. Compiler проверит
значения, graph и refs перед sealing.

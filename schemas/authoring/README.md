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
| `.prifly/workflows/NAME/contexts/**/*.yaml` | `context-v1.schema.json` |
| `catalog.yaml` в корне репозитория каталога сценариев | `workflow-catalog-v1.schema.json` |

Schema ловит форму документа и опечатки в известных полях. После изменения
сценария выполните обычную проверку:

```sh
prifly project compile --repository . --package NAME --host codex-cli --output ../NAME.package
```

Для привязки строго к одной версии используйте `project-profile-v2.schema.json`
или `project-profile-v3.schema.json` в modeline. Единственный автоматический
pattern у общей schema: редактор не должен одновременно требовать оба marker.
Переход `/2` → `/3` выполняется явной правкой `schema_version` в shared
`project.yaml`, с обновлением version-specific modeline при его наличии.

В первом срезе оба profile фиксируют три skills roots: `codex-cli`, `codex-app` и
`claude-code`. Выберите тот host, из которого запускается сценарий: compiler
не угадывает его по папкам. Context может взять AI Factory skill только через
`source: {root: host_skills, path: SKILL/PATH}` из выбранного root; exact bytes
попадают в sealed package.

Profile `/3` назначает package и owned components детерминированные compiled
versions `0.0.0-b1.…`, поэтому разные profiles/settings/context bytes могут
сосуществовать в одной authority. Авторские версии YAML не меняются; соответствие
хранится в `build-provenance.json` sealed package. Внешняя ссылка использует
exact compiled ref, не author `ID@version` как alias «последней» сборки.
`/2` сохраняет прежние refs и отказы конфликтующих вариантов. Запуск без Git,
host или RunBrief этим первым срезом ещё не вводится.

`extend.yaml` может содержать три независимые части: `settings` задаёт значения
уже объявленных project-scoped inputs, `exclude` выключает named optional
features, а `extensions` вставляет простой no-input step. `exclude` не удаляет
stage: author объявляет feature в `workflow.yaml`, связывает его с boolean
project input и сам описывает оба route через обычный graph. Compiler проверит
значения, graph и refs перед sealing.

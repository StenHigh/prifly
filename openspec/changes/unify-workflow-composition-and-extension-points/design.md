## Context

Статус: проект контракта; реализация и опубликованная поддержка пока отсутствуют.

Причина изменения описана в [proposal.md](proposal.md). База исследования — commit `30131326`; опубликованный `/4` — v0.13.83.

Текущий `/3` объединяет исходники. `/4` выбирает реализацию package-owned точки. Обе формы используют native Step, Call, Choice и Parallel.
Текущий запрет совместного применения защищает ownership до появления положительной карты происхождения. Простое удаление guard эту защиту уничтожит.

Основные источники:

- `openspec/SOURCE-OF-TRUTH.md` и main specs четырёх затронутых capabilities.
- Active changes `add-workflow-component-imports-and-layers` и `add-nested-project-extension-points`.
- `cmd/prifly/project_composition*.go`, `project_extension_points.go`, `project_compile.go`, `project_build.go`.
- `internal/flow/extensions.go`: Parallel предоставляет aggregate summary, а не произвольный output выбранной ветви.
- `schemas/authoring/extension-v4.schema.json` и `extension-points-v1.schema.json`: опубликованные closed forms.

## Goals / Non-Goals

**Goals:**

- Один opt-in контракт поддерживает composition и точки одновременно.
- Compiler доказывает ownership до объединения inventories.
- Автор видит место подключения, выбранную реализацию, её источник и exact compiled refs.
- Разные настройки одного import не меняют другие использования этого источника.
- Неизменный `/4` сохраняет прежние bytes, поведение и путь исполнения.

**Non-Goals:**

- Runtime plugins, новые stage kinds, сетевой fetch и автоматический trust.
- Произвольное изменение чужих graph, policy, effects или программ.
- Неявный перенос данных между параллельными branches.
- Новый output-aggregate protocol или nested-stage recovery.
- Автоматическая миграция `/3` или `/4`, release и продуктовая приёмка smsplace.

## Decisions

### 1. Новая authoring boundary вместо расширения `/4`

Единый extend объявляет `authoring: prifly-extension/5`. Source folder `/3` добавляет явный экспорт точек поверх возможностей folder `/2`.
`extends` остаётся полем `workflow.yaml`; extend `/5` не дублирует выбор parents.
Sidecar остаётся `prifly-extension-points/1`: его поля и смысл не меняются. Новую версию sidecar этот срез не вводит.

| Документ | Область |
|---|---|
| `prifly-extension/4` | Опубликованные local point bindings и прежние root entries |
| `prifly-extension/5` | Imports, layers, root/point contributions, arrange и imported implementations |
| `prifly-project-workflow/2` | Существующие source exports и parents |
| `prifly-project-workflow/3` | Folder `/2` плюс `exports.points` |
| `prifly-extension-points/1` | Прежние owned anchors, inputs, routes и consumers |

Для нового compiler нужен явный opt-in выбранного root: extend `/5` либо root folder `/3`.
Импорт нового provider не повышает старый root до нового режима молча.

Альтернатива — добавить поля в `/4`. Она нарушает closed schema и делает одинаковые исходники зависимыми от версии binary.
Другой вариант — снять sidecar guard в `/3`. Он не решает ownership и создаёт незаявленную миграцию старой формы.

### 2. Один pipeline с сохранённым legacy front end

При отсутствии нового root opt-in прежние readers, version selection и b1 path остаются действующими.
После opt-in compiler разбирает каждый документ по его собственному marker. Затем он переводит допустимые записи в общую внутреннюю модель.
Old `/4` entries внутри нового наследования сохраняют local target rules и typed consumer semantics. Они не получают новые поля `/5`.

```text
source + parent graph + layers
              |
              v
versioned readers -> owner map -> exported targets
                                      |
                                      v
                     scoped variants + contributions
                                      |
                                      v
                       typed native graph -> seal
                                      |
                                      v
                        provenance -> review -> start
```

Внутренняя модель хранит source owner, author identity, declaring file, target scope, point declaration и implementation origin.
Compiled workflow variant — неизменяемый вариант сборки workflow с определённой point policy и settings.
Это compiler metadata, не новый runtime объект. Код не определяет владельца по расположению элемента в flattened inventory.

### 3. Положительное владение до flattening

Point target обязан принадлежать package, объявившему sidecar. Сначала compiler проверяет его исходную inventory и exact author workflow.
Project-owned definitions не становятся package-owned. Импортированный definition не становится local только из-за совпавшего ID.

Local point implementation сохраняет правило `/4`: source расположен в разрешённой `project/` inventory объявляющего consumer.
Qualified implementation в `/5` разрешён только через explicit component export выбранного import.
Supporting files разрешаются относительно объявления программы или слоя. Этот каталог не определяет владельца definition.

Наследуемый point сохраняет owner общего ancestor. Если target — root основы, compiler явно связывает его с производным root до sealing.
Inherited child target сохраняет собственную author identity. Неоднозначность владельца или target даёт отказ.

Альтернатива — повторно применить отрицательную проверку `/4` к объединённому списку. Она уже не различает чужие и собственные definitions.

### 4. Чужая точка становится публичной только через export

В folder `/3` появляется отдельная таблица `exports.points`. Значение называет public workflow export и local point из sidecar.

```yaml
authoring: prifly-project-workflow/3
# Остальные обязательные package/root поля опущены только в этом фрагменте.
exports:
  workflows:
    review: "{{workflow_review}}"
  points:
    before-read:
      workflow: review
      point: before-read-local
```

`workflow` здесь — имя export, а не произвольный ID. Target local point обязан входить в reachable closure этого workflow.
Это допускает явно опубликованный hook внутри native repeat body. Сам private body не получает отдельного component export.
Root launch не становится экспортируемым component через этот механизм. Export names остаются уникальными между группами exports.

Импорт workflow не публикует все его points. Все прочитанные sidecar declarations проверяются в исходном owner scope, включая невыбранные private points.
Standalone `extend.yaml`, layers, launch и ответы provider не применяются через import.

### 5. Selector явно задаёт пространство имён

`point` в `/5` принимает одну из трёх closed форм:

```yaml
point: before-commit
# Локальная либо единственная видимая inherited точка.

point: {parent: classic, name: before-commit}
# classic явно назван непосредственным parent. Его visible point сохраняет owner.

point: {import: ui, export: before-read}
# ui объявлен в imports. before-read находится в exports.points provider.
```

Compiler не ищет совпадение в Registry или среди всех прочитанных folders.
Неполная либо неоднозначная форма отказывает. Новые object selectors не допускаются под `/4`.

Два selector одного common-ancestor point разрешаются в одну canonical identity.
Разные owners с одинаковым именем остаются разными points. Они могут конфликтовать по physical anchor, но не объединяются по имени.

Альтернатива — перегрузить dotted string для parents, imports, workflow names и point names. Closed object forms устраняют зависимость от порядка поиска.

### 6. Point entry сохраняет границу authority владельца

Leaf entry `/5` использует знакомую форму `/4`:

```yaml
authoring: prifly-extension/5
imports: {ui: aif-ui-ux}
extensions:
  - id: interface-review
    point: before-commit
    call: ui.review
    output_bindings: {}
  - id: measure-before-read
    point: {import: ui, export: before-read}
    step: measure
    output_bindings: {reader: carrier, repair: carrier, clean: carrier}
```

Это иллюстрация при условии соответствующих declarations. Точный runnable example входит в задачи реализации.
`step` и `call` взаимоисключающие. Unqualified implementation разрешается в собственном authoring scope; qualified target требует import/export.

В point entry запрещены `workflow`, `between`, `input_bindings`, `on`, `on_error` и произвольные consumers.
Их задаёт sidecar. Каждый declared consumer обязан получить explicit output с exact compatible schema и доказанной availability.
`enabled: false` сохраняет passthrough, не создаёт executable closure реализации и не требует execution consent её программы.
Declaration и shape при этом проверяются по прежним правилам `/4`.

### 7. Root entries остаются ограниченным совместимым механизмом

В `/5` можно использовать root entry формы `/3`: `id`, `workflow: root`, `between`, `step|call`, bindings и routes.
Он адресует только effective root своего authoring scope. Он не открывает произвольные internals импортированного workflow.

Одна запись содержит либо root attachment, либо point selector. Смешанная форма отказывает.
Root entry и point entry, занимающие один anchor или consumer в одном variant, конфликтуют. Порядок файлов не разрешает конфликт.

Новые справочники рекомендуют declared points для повторно используемых контрактов. Legacy root insertion сохраняется для существующих источников и локальной адаптации.

### 8. Parents и layers сохраняют identity declarations

Общий exact ancestor применяется один раз. Независимые contributions не исчезают при совпадении имени или bytes.
Собственные option declarations потомка заменяют ancestor declarations. Разные значения независимых parents требуют явного consumer resolution.
Opaque settings objects, answers, resources и model records заменяются целиком. Отсутствующий `exclude` отличается от явного пустого списка.

Layer `/5` допускает settings, imports и extensions, включая point entries. Он не объявляет `extends`, `layers` или `arrange`.
Пути нормализуются относительно declaring document внутри repository. Повторное явное подключение одного layer даёт отказ между любыми declarations.
Cached common ancestor не создаёт повторной декларации слоя. Library source mode не читает standalone layers provider.

Наследуемые `/4` implementations удерживают ownership исходного project subtree. Final consumer не получает права на provider program override.
При конфликте нескольких активных contributions одной точки нужен допустимый `arrange`, а не выбор первого parent.

### 9. Группы не расширяют native transport

Root `arrange` сохраняет sequence, parallel и choice из `/3`.
Point `arrange` использует тот же выбор members, но задаёт `point` вместо `between` и не задаёт собственные routes.

```yaml
arrange:
  - id: quality
    point: before-commit
    parallel: [classic-ui.interface-review, classic-audit.audit]
```

Каждый active member одной точки учитывается ровно один раз. Идентичный inherited group остаётся целым при следующем наследовании.
Disabled member нельзя назвать active member группы. Неизвестный, потерянный либо повторный selector даёт отказ.

**Первая `/5` разрешает point groups только при пустом `consumers`.**
Для такого point нужна `routes.call` с `succeeded` и `rejected` и объявленным `on_error`.
Success и rejection destinations должны различаться.
Step members дополнительно требуют `routes.step`: pass ведёт туда же, что call succeeded; остальные достижимые verdicts ведут туда же, что call rejected.
Step `on_error` должен совпадать с Call `on_error`; authored impossible_verdicts сохраняются.
Call member допускает succeeded на success route; любой другой declared outcome должен вести на rejection route.
Distinct failure destinations или несовместимые маршруты дают named refusal. Compiler не сворачивает их молча в rejected.

Sequence требует успеха всех members в указанном порядке. Parallel использует all/all/wait и settled branches.
Choice имеет explicit default и `on_unknown: rejected`; predicates используют native typed operands.
Технический failure остаётся failure без business outcome. Outer point Call использует declared `on_error` владельца.
Все members получают разрешённые inputs точки. Последовательный порядок не означает неявной передачи новых outputs следующему member.

Точка с consumers принимает один Step либо один typed Call по правилу `/4`.
Её сложную реализацию автор объявляет отдельным workflow с explicit inputs/outputs и обычным native graph.
Автоматический point group с consumer ports даёт `project_composition_point_group_outputs_unsupported` до effects.
Автоматическое объединение конфликтующих parent implementations в такой точке не выполняется; автор меняет composition sources явно.

Причина ограничения: native Parallel экспортирует summary с ArtifactRef, а не произвольный carrier branch.
Добавление неявного dereference или нового aggregate protocol расширило бы runtime scope этого change.

### 10. Imported point bindings создают изолированные compiled variants

Binding imported point относится к паре import alias и workflow export из `exports.points`.
Он действует на использования этого export в данном import scope. Другой alias или другой workflow export сохраняет собственную selection policy.
Общий nested child клонируется в compiled closure только там, где политика различается. Исходные bytes provider не меняются.

Повторные Calls одного export в одном scope используют одну policy и один exact compiled variant.
Для двух policies автор объявляет два import aliases. Их source owner остаётся одним; scope не выдаёт новых прав.
Unchanged exact variant учитывается однократно. Configured и unconfigured variants получают разные compiled refs.

Если shared child входит в два exports, binding одного export не изменяет второй export.
Compiler разделяет dependent variants и заново seal-ит child refs снизу вверх.
Definition/variant dependency cycles, неоднозначные targets и попытка выйти за public point export дают отказ.
Owner map не является execution DAG: injected consumer step может зависеть от owner consumer без цикла definitions.
Compiler отдельно проверяет source imports и actual definition/variant dependencies.

Альтернатива — изменить provider definition глобально. Она меняет невыбранных consumers и нарушает isolation.
Вариант с единственной policy на source проще, но отказывает на легитимных независимых uses. Поэтому выбран versioned variant identity.

### 11. Algorithm b2 имеет отдельную boundary

`/5` использует `prifly-build-provenance/3` и algorithm `b2`. Старые paths продолжают b1 без новых inputs или изменения tuple.

Внутренний `variant_key` — SHA-256 canonical записи со следующими значениями:

- Exact source owner с relative source identity и content digest, author component identity и вид root/imported scope.
- Нормализованные selected point contracts, implementation targets, output bindings и effective settings.
- Canonical dependency variant keys, включая implementations и generated groups.

Variant key использует author/source targets и dependency keys до sealing. Compiled refs и final package digest в него не входят.
Это исключает цикл между variant key, build key и compiled refs.
Alias spelling и абсолютный machine path не определяют owner. Независимые source origins не объединяются только по одинаковому содержимому.
Root package build key учитывает также selected profile, values, declaration provenance и hashes всех прочитанных inputs.
Переименование alias или изменение authored parent order может изменить общий build key через source bytes.
Неизменность package identity при таких правках не обещается. Порядок parents не разрешает conflicts и не задаёт неявный execution order.

Compiled package version: `0.0.0-b2.` плюс lower-case base32 полного build-key digest без padding.
Compiled component version: такой же prefix от hash tuple `[build_key, kind, author_id, author_version, variant_key]`.
Source author IDs не меняются. Несовпадающие variants различаются version и digest; aliases указывают на exact variant Ref.

Provenance `/3` содержит mapping author/source/variant к compiled refs. Package digest не входит в собственный build key.
Общий candidate budget — не более 256 compiled Workflow variants, включая generated wrappers. Другие file/byte/native limits действуют независимо.

### 12. Programs, effects и source budgets проверяются по всей closure

Provider programs связываются только с собственными selected definition variants. Private unselected program не читает supporting files.
Shared binding закрепляется один раз на exact Ref. Provenance сохраняет каждый реально прочитанный supporting path.
Consumer не заменяет provider program. Local project program сохраняет standard allowlist, review и execution consent.

Новый scope не сбрасывает budgets: folders 32, dependency depth 8, layers 32, group depth 8, members 32.
Point manifest сохраняет предел 256 declarations. Все clones учитываются в общих package payload file/byte limits и native cost.
Runtime limits не повышаются ради wrappers. Source resolution, variant dependencies и graph cycles проверяются до effects.
Compiler не читает файлы и не запускает процессы внутри transform write transaction.

### 13. CLI и capability имеют отдельные closed формы

| Новый результат | Marker |
|---|---|
| Compile | `project-compile/4` |
| Prepare / launch summary | `project-launch-summary/7` |
| Start / continue result | `project-start/7` |
| Build provenance | `prifly-build-provenance/3`, algorithm b2 |
| Composition review | `workflow-composition/2` |

Новый additive bundle — `workflow-composition-points.schema.json`.
Review показывает source owners, variant mapping, exports points, active/disabled choices, groups, consumers и exact refs.
`--explain` описывает эти же решения и ничего не исполняет. Он не подменяет digest review.
Изменение sidecar, export, implementation, layer, settings либо supporting bytes после prepare отменяет прежний review digest.

Capability token — `workflow_composition_with_points`. Два прежних tokens вместе не доказывают новую поддержку: текущий commit имеет оба и всё ещё отказывает на mixed use.
Новый token получает строку author index, reference с реально показанными полями и runnable example.

Новая metadata находится в package provenance и новых Project DTO, а не в полях Run.
Native definitions и existing state/read contracts остаются прежними. Если реализация потребует новый persisted факт, она выходит за выбранный scope и требует отдельной version boundary.

### 14. Совместимость проверяется отдельными путями

| Источник | Новый binary | Identity / views |
|---|---|---|
| Неизменный `/4` на прежнем root | Прежний compiler path | Прежние b1 bytes, schema versions и refusal codes |
| Прежний `/3` без нового opt-in | Прежний composition path | Прежние b1 и mixed-form refusal |
| Root opt-in `/5` с parent `/4` | Legacy parse, затем общий pipeline | b2 и новые views; исходники parent неизменны |
| Root opt-in `/5` с прежними `/3` contributions | Versioned parse, затем общий pipeline | b2 и новые views |
| Imported folder `/3` под старым root | Named refusal | Нет неявного upgrade |
| Новый `/5` на старом binary | Refusal по неизвестной версии | Нет fallback на `/4` |

При новом root opt-in прежние документы всё ещё проходят собственные closed schemas.
Совместное чтение sidecar и `/3`-среза разрешено только этой новой boundary. Отдельный вызов старой `/3` сохраняет исходный refusal.
`/4` не получает imported targets или object selectors только из-за расположения внутри нового package.

Pure `/4` baseline сравнивает refs, package/build digest, native definitions, DTO versions, disabled behavior, execution consent и typed refusal codes.
Normalization legacy paths не может заменить baseline проверкой только успешного exit code.

## Risks / Trade-offs

- **Скрытое ослабление ownership** → positive owner map до flattening, export gating и adversarial fixtures с совпавшими IDs.
- **Variant explosion** → общий предел 256 workflow variants и независимые source/native budgets.
- **Потеря mandatory parent contribution** → origin identity, exact coverage и сохранение inherited groups.
- **Carrier из неверной ветви** → consumer-bearing points используют один typed Call; автоматический group получает отказ.
- **Изменение legacy bytes при общей нормализации** → прежний default path, exact `/4` differential vectors и frozen schema checks.
- **Расхождение двух active source sets** → `/5` владеет только новый change; `/3` и `/4` остаются baseline соответствующих форм.
- **Невозможность rollback новым источником** → хранить прежний source commit и принимать source downgrade только явной правкой владельца.

## Migration Plan

1. Зафиксировать exact `/4` baseline на v0.13.83 и новые additive contract identifiers.
2. Добавить versioned readers и typed owner/variant model за новым root opt-in.
3. Реализовать b2, public point exports и isolated variants с негативными проверками до effects.
4. Добавить inheritance, layers и допустимые point groups без расширения runtime transport.
5. Выполнить differential `/4` и полный совместный CLI harness на одном candidate.
6. Обновить author index, references, glossary и source-of-truth map вместе с реализацией.
7. Выпускать capability после exact-SHA release gates и replay опубликованного binary.

```text
v0.13.83 /4 baseline -> opt-in /5 implementation -> mixed acceptance
        |                                           |
        +----------- unchanged /4 checks -----------+
                                                    |
                                                    v
                                              release replay
```

Автоматического rewrite `extend.yaml` нет. Новые проекты выбирают `/5` явно; действующие `/4` могут продолжать прежний путь без срока отключения.
Rollback возвращает authoring commit `/4` и прежние choices. Он не переписывает accepted Runs или их pinned packages.
Совместимость уже созданного native Run определяется его native contracts; номер authoring сам по себе не доказывает поддержку старым binary.

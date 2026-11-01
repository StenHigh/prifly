## ADDED Requirements

### Requirement: Unified composition requires explicit root opt-in
Единая authoring форма SHALL выбираться через `prifly-extension/5` выбранного root либо root folder `prifly-project-workflow/3`. Она SHALL объединять source composition и declared extension points в одном compiler path. Каждый документ MUST проверяться своей closed schema.

Root без нового opt-in MUST сохранять прежние readers, meaning, native definitions и build identity. Наличие нового provider MUST NOT молча менять режим старого root. Existing `/4` entries внутри нового inheritance SHALL сохранять собственные local resolution и consumer rules.

Авторитетные source sets остаются в main `workflow-and-context`, `cli-protocol`, `published-contracts` и `specification-governance` по `SOURCE-OF-TRUTH.md`. Этот delta задаёт только новый opt-in. Active changes `/3` и `/4` остаются baseline прежних форм.

#### Scenario: Existing point project remains unchanged
- **WHEN** прежний root и `prifly-extension/4` компилируются binary с поддержкой `/5`
- **THEN** compiler сохраняет прежние sealed definitions, b1 identity, output versions и refusal codes без source rewrite

#### Scenario: New provider does not upgrade old root
- **WHEN** старый root импортирует source, которому нужен unified authoring
- **THEN** compiler возвращает named version refusal до effects, а не включает новый режим автоматически

### Requirement: Imported point visibility is an explicit package export
Folder `/3` SHALL добавлять `exports.points`. Каждый public point export MUST называть public workflow export и local point declaration из неизменённого sidecar `/1`. Target point MUST принадлежать declaring package и находиться в reachable closure выбранного workflow export.

Экспорт workflow MUST NOT автоматически публиковать его points или private components. Point export вложенного child либо repeat body SHALL давать право выбрать hook, а не прямой доступ к private workflow. Все прочитанные sidecar declarations MUST валидироваться в исходной owner inventory.

#### Scenario: Consumer selects a published nested point
- **WHEN** provider экспортирует hook внутри closure public workflow, а consumer явно импортирует provider и выбирает этот point export
- **THEN** compiler применяет разрешённую реализацию к scoped variant public workflow, не создавая public export private body

#### Scenario: Consumer guesses a private point name
- **WHEN** имя есть в sidecar, но отсутствует в `exports.points`
- **THEN** compiler отказывает до создания package и не ищет target в flattened inventory

### Requirement: Point selection retains positive source ownership
`/5` SHALL разрешать scalar `point` для local либо однозначного inherited point. Object `{parent, name}` SHALL выбирать visible point непосредственного parent. Object `{import, export}` SHALL выбирать public point export explicit import. Смешанные, неполные и неоднозначные selectors MUST отказываться.

Point target MUST проверяться по положительной inventory declaring owner до flattening. Local point implementation MUST сохранять project-owned scope `/4`. Qualified implementation SHALL разрешаться только через explicit component export. Source folder программы MUST NOT считаться доказательством ownership definition.

При наследовании root point compiler SHALL сохранять original owner и явно связывать target с effective derived root. Совпавший ID чужого definition MUST NOT давать права владельца.

#### Scenario: Foreign definition collides with an owned ID
- **WHEN** imported definition имеет такой же ID, как объявленный target либо program binding другого source
- **THEN** compiler проверяет owner, kind и exact variant, а неподтверждённую подмену отвергает

#### Scenario: Parent root receives a derived identity
- **WHEN** child наследует root graph и point общего ancestor
- **THEN** point адресует corresponding derived root, сохраняя original owner и неизменные source bytes ancestor

### Requirement: Point implementations preserve declared inputs routes and consumers
Point leaf `/5` SHALL содержать `id`, `point`, ровно один `step|call` и `output_bindings`. Inputs, anchors, routes и разрешённые consumers SHALL определяться только sidecar владельца.

Каждый consumer MUST иметь explicit producer output с exact compatible schema и доказанной native availability. Client routes, arbitrary consumer keys и input sources внутри point entry MUST отказываться. Missing output MUST NOT заменяться прежним carrier или считаться success.

`enabled: false` SHALL сохранять original passthrough. Disabled implementation MUST NOT расширять executable closure или execution consent. Static declaration validation SHALL сохраняться.

#### Scenario: Repair needs a fresh carrier
- **WHEN** выбранный point связывает reader, finish либо следующую iteration с output measurement
- **THEN** соответствующий consumer получает именно новый output; вложенные ArtifactRef сохраняют свою identity

#### Scenario: Implementation is disabled
- **WHEN** entry имеет enabled false
- **THEN** прежний маршрут и bindings сохраняются, а supporting program implementation не читается для исполнения

### Requirement: Unified inheritance and layers retain declaration identity
Единый путь SHALL поддерживать parents, imports и layers без изменения их source semantics. Общий ancestor SHALL применяться один раз; независимые contributions MUST NOT исчезать по совпавшим именам или bytes.

Настройки SHALL объединяться по immutable declarations и ancestry. Потомок SHALL перекрывать своего ancestor; конфликт независимых declarations SHALL требовать explicit consumer resolution. Opaque values и списки SHALL заменяться целиком.

Layer `/5` SHALL разрешать settings, imports и root/point entries. Nested layers, extends и arrange в слое MUST отказываться. Повторное explicit подключение normalized layer path MUST отказываться между любыми declarations. Inherited read общего cached ancestor MUST NOT считаться новым explicit подключением.

#### Scenario: Parents add different checks at one point
- **WHEN** два parents наследуют общую exact основу и добавляют разные active contributions одной точки
- **THEN** compiler сохраняет оба origins и требует допустимую explicit composition вместо выбора порядка parents

#### Scenario: Two parents include the same layer explicitly
- **WHEN** declarations используют разные написания одного normalized layer path
- **THEN** compiler возвращает duplicate-layer refusal до merge; общий ancestor с одной declaration остаётся допустимым

### Requirement: Point policies create isolated compiled workflow variants
Unified compiler SHALL создавать отдельные immutable compiled variants для разных effective point policies либо settings одного source workflow. Point binding import SHALL относиться к workflow export, указанному point export, в scope выбранного import alias.

Другой alias либо другой workflow export MUST сохранять собственную policy. Shared child с различающимися policies MUST получать отдельные compiled variants. Их author IDs и source bytes MUST сохраняться; compiled refs MUST различаться.

Повторные uses одной effective policy SHALL переиспользовать один exact variant. Независимые source owners MUST NOT объединяться по одному ID или hash содержимого. Variant dependencies MUST быть конечными и проходить общие compile budgets.

#### Scenario: Same provider is used with and without measurement
- **WHEN** consumer импортирует один provider двумя aliases и выбирает point только для первого workflow export
- **THEN** первая compiled closure содержит measurement, вторая сохраняет passthrough; source provider не изменяется

#### Scenario: Exported workflows share a private child
- **WHEN** два exports используют один child, а point policy меняется только для одного export
- **THEN** resealing создаёт нужные child variants и не переносит изменение во второй export

### Requirement: Point groups stay within native output and route contracts
Point arrange SHALL явно перечислять active contributions одной canonical point и одного variant. Каждый member SHALL учитываться ровно один раз. Unknown, disabled, duplicated и потерянные members MUST вызывать отказ. Inherited group SHALL сохраняться как целое contribution.

Автоматическая point group SHALL разрешаться только при пустом `consumers`. Она MUST иметь совместимый sidecar call contract `succeeded/rejected/on_error`. Success и rejection destinations MUST различаться. Step pass и Call succeeded SHALL соответствовать success route. Остальные достижимые step verdicts и declared Call outcomes SHALL соответствовать rejection route. Step on_error MUST совпадать с Call on_error; authored impossible_verdicts SHALL сохраняться. Разные failure destinations MUST NOT объединяться молча.

Sequence SHALL требовать успеха всех members в порядке автора. Parallel SHALL использовать native all/all/wait. Choice SHALL иметь explicit default и `on_unknown: rejected`. Technical failure SHALL оставаться failure без business outcome и достигать declared outer `on_error`.

Автоматическая point group с consumer ports MUST давать `project_composition_point_group_outputs_unsupported`. Такая точка SHALL принимать один Step либо typed Call с обычными native inputs/outputs. Неявный branch-output transport и dereference AggregateManifest MUST NOT добавляться.

#### Scenario: Two acceptance checks use an output-free point
- **WHEN** point имеет пустой consumers и compatible routes, а consumer явно группирует UI и audit параллельно
- **THEN** обе branches исполняются и settle до success; техническая ошибка не превращается в rejected outcome

#### Scenario: Parallel point group would supply a carrier
- **WHEN** arrange пытается автоматически объединить members точки с carrier consumers
- **THEN** compile возвращает explicit unsupported refusal до effects и требует отдельную typed Call реализацию

### Requirement: Root contributions and points have disjoint authority
Unified root entry SHALL сохранять ограничение direct route собственного effective root. Он MUST NOT адресовать internals чужого imported workflow.

Одна запись MUST выбирать root attachment либо point selector. Две независимые записи, занимающие один physical anchor либо consumer одного variant, MUST конфликтовать. Порядок файлов MUST NOT разрешать конфликт. Допустимый point arrange SHALL заменять свои members одной planned insertion.

#### Scenario: Root insertion overlaps a selected point
- **WHEN** обе записи занимают один anchor в одном compiled variant
- **THEN** compiler возвращает named conflict, не меняя порядок либо подавляя запись

### Requirement: Unified preparation preserves programs effects and bounded source reads
Programs SHALL связываться только с собственными selected definition variants. Provider program override MUST отказываться. Unselected private program MUST пропускаться до supporting file reads. Provenance SHALL сохранять каждый реально прочитанный path, даже при dedup identical binding.

Общие пределы SHALL сохранять folders 32, dependency depth 8, layers 32, group depth 8 и members 32. Point sidecar SHALL сохранять 256 declarations. Unified compile SHALL иметь общий предел 256 Workflow variants, включая generated wrappers. File/byte и native limits SHALL действовать дополнительно и не сбрасываться на import либо clone.

Compiler MUST NOT выполнять fetch, trust, programs или authority mutation. Existing effects, execution consent и root recovery frontier SHALL сохраняться. Source composition MUST NOT добавлять filesystem/process effects внутрь transform write transaction.

#### Scenario: Aliases attempt to reset a variant budget
- **WHEN** aliases и nested imports создают более 256 workflow variants либо превышают другой общий предел
- **THEN** compiler возвращает named limit refusal до effects без повышения runtime budgets

#### Scenario: Continue follows a repaired nested implementation
- **WHEN** reviewed continue повторяет root Call/Repeat frontier
- **THEN** native recovery rules остаются прежними; новая composition не создаёт nested-stage resume или stale carrier reuse

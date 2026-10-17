# Published Contracts Specification

## Purpose

Определяет правила публикации, совместимости и проверки machine-readable
contracts Pri-Fly, не подменяя сами JSON Schema и Go types Markdown-копией.

## Requirements

### Requirement: Семантика и machine shape имеют разные, явные источники
Published-contracts capability SHALL отделять semantic rule от machine shape.
OpenSpec MUST быть current source для смысла publication, compatibility,
validation boundary и ownership. Versioned JSON Schema, generated public
bundles и corresponding Go types MUST оставаться exact product artifacts;
OpenSpec MUST NOT дублировать их поля или bytes.

#### Scenario: Автор меняет публичный contract
- **WHEN** изменение затрагивает observable JSON DTO или definition
- **THEN** author изменяет semantic capability и exact product artifact вместе,
  а crosswalk/verification показывает их declared relationship

### Requirement: Опубликованный contract сохраняет compatibility boundary
A published contract MUST иметь явную version/identity boundary. Изменение
MUST NOT silently расширять closed historical shape, менять meaning accepted
Run или считать новый Go field частью старого public contract. Новый behavior
MUST использовать declared compatible version/bundle либо быть rejected before
use.

Версия состояния, объявленная ради записываемого факта, MUST быть достижимой:
Run, который этот факт несёт, MUST запечатываться именно ею. Объявить версию,
опубликовать её бандл и перечислить её в capability document, но записывать
факт под прежней версией — то же silently расширение закрытой формы, только
совершённое с другой стороны: поле появляется в Run, чей контракт его не
описывает, и бандл этой версии собственный Run отвергает.

Проверка достижимости MUST читать запечатанный Run, а не объявление: наличие
строки в лестнице, бандла в каталоге и capability в документе не является
доказательством того, что этой версией что-либо запечатано.

#### Scenario: Новое поле нужно новому reader
- **WHEN** новая capability требует дополнительное public поле
- **THEN** historical reader and saved Run сохраняют прежний shape и meaning,
  а новый contract объявляет собственную versioned boundary

#### Scenario: Объявленная версия состояния ничем не запечатана
- **WHEN** версия состояния объявлена ради записываемого факта, но ни один Run
  ею не запечатывается
- **THEN** это дефект контракта, а не экономия: Run несёт поле под версией,
  чей опубликованный контракт его не описывает, и проверка обязана падать

#### Scenario: Run несёт факт, ради которого версия объявлена
- **WHEN** Run запечатывает перевод профилей модели либо его план объявляет
  профиль модели
- **THEN** Run запечатан версией, объявленной ради этого факта, и бандл этой
  версии принимает этот Run

### Requirement: Fixtures и schema checks не являются qualification evidence
Contract documentation SHALL различать structural schema validation, semantic
admission, runtime execution и release qualification. Positive fixture,
generated-schema match или successful document check MUST NOT обещать
installed adapter, permissions, external effect или qualified profile.

#### Scenario: Baseline fixture проходит shape validation
- **WHEN** fixture проходит declared schema check
- **THEN** result говорит только о форме и явно сохраняет отдельный execution
  status вместо заявления о runtime qualification

### Requirement: Contract publication проверяется против canonical artifacts
Published contract surface MUST иметь reproducible verification, которая
обнаруживает расхождение generated schema, embedded/runtime representation,
fixture registry и declared compatibility boundary. Documentation count or
inventory claim MUST быть проверен against canonical artifact before он
становится current fact.

#### Scenario: Inventory расходится с artifact
- **WHEN** документированное количество contracts отличается от actual
  machine-readable inventory
- **THEN** migration records the discrepancy, determines its source and fixes
  only the supported current claim with an explicit verification; it does not
  relabel historical evidence or silently normalize bytes

### Requirement: Legacy contract material остаётся историческим до final cleanup
Migration SHALL сохранять byte-identical legacy contract guide, fixtures,
schemas, Go definitions, evidence и manifests до общего final cleanup. Archive
crosswalk MUST record legacy inventory/status vocabulary and map it to the
permanent contract rules without exposing old record IDs as a new authoring API.

#### Scenario: Постоянная спецификация ссылается на legacy baseline
- **WHEN** reader needs provenance старого baseline contract
- **THEN** он находит exact historical record в archive or Git, а permanent
  spec остаётся readable current rule without a second editable baseline

### Requirement: Direct checkout state имеет отдельную compatible version boundary

Published machine contracts for direct checkout Workspace SHALL use a new
declared state/read compatibility boundary. Previous worktree-only schemas,
bundles and saved Runs MUST retain their original shape and meaning; a reader
without the new boundary MUST refuse new state rather than treating checkout as
a disposable worktree.

#### Scenario: Старый reader встречает новый Workspace mode
- **WHEN** binary without the declared compatible state reader opens a Run with
  direct checkout Workspace data
- **THEN** it refuses that authority state without changing or cleaning the
  repository

### Requirement: Queue binding имеет достижимую opt-in compatibility boundary

Queue документы SHALL иметь отдельные closed versioned schemas и authority происхождение. Run с QueueTaskBinding SHALL запечатываться новой границей `core-state/44` и читаться как `core-read/44`; прежние state/read bundles и Runs MUST оставаться byte/meaning compatible. Project queue review/result SHALL использовать `project-launch-summary/4` и `project-start/4` только при queue selection; запуск без неё SHALL сохранять прежние версии.

Новые canonical definitions, generated public bundles, readers, writers, capability declarations, glossary mappings и author index SHALL изменяться вместе. Structural tests SHALL читать реально запечатанный queue Run и его read output, а также проверять legacy отсутствие queue binding; исторические bundles и release evidence MUST NOT переписываться. Отдельная queue state/read форма SHALL явно различать task revision, claim generation, intent и Run association.

#### Scenario: Queue-bound Run реально запечатан
- **WHEN** reviewed queue-bound start допускает Run
- **THEN** сохранённый Run и read output принимаются новым declared bundle и содержат exact binding; прежняя closed версия не получает это поле

#### Scenario: Прежний start без очереди
- **WHEN** обычный project start или legacy reader работает без queue selection
- **THEN** версии/поля прежнего контракта сохраняются, queue не появляется автоматически

### Requirement: Durable storage budget имеет явную writer compatibility boundary
Persisted configurable storage ceiling SHALL поставляться в новой storage
version после baseline storage 9 с атомарной supported migration. Migration
MUST сохранять прежний effective default, authority identity, Run/event bytes,
pins, claims и receipts; read-only show/prepare MUST NOT мигрировать storage.
Устаревший writer, открытый до migration, MUST отказываться до записи после
изменения storage header; обновлённый writer SHALL читать current persisted
setting при каждом budget decision. Downgrade binary MUST NOT обещать
read/write совместимость с новой storage version.

Новые status/review/result и authority budget control SHALL иметь отдельные
closed versioned schemas и реально emitted fixtures. Existing Run state/read,
Problem и plain doctor contracts MUST NOT получать новые fields/meaning.
Historic schemas/bundles/evidence SHALL сохранять bytes. Storage-only migration
MUST NOT трактоваться как новая WorkflowRevision или пересчёт принятых Run.

#### Scenario: Старый process продолжает запись после migration
- **WHEN** старый writer держал соединение до перехода storage на новую version
- **THEN** его следующая write transaction отказывается как incompatible до mutation, без обхода нового ceiling

#### Scenario: Reader выполняет prepare на storage 9
- **WHEN** новый CLI читает budget или готовит повышение на прежней authority
- **THEN** storage header и история остаются прежними, review явно показывает требуемый upgrade, а setting не повышается автоматически

### Requirement: Project worktree naming имеет opt-in version boundary

Новые naming declaration, allocation metadata и review/result fields SHALL публиковаться как новые closed versioned authoring и CLI/authority формы. Профили `/2`, `/3`, старые launch summary/start/claim views и historical bundles MUST сохранять bytes и meaning. Явный переход shared profile на `/4` SHALL быть достаточен для включения naming declaration; init/start/runner upgrade MUST NOT автоматически менять существующий shared profile.

Новый reader SHALL поддерживать legacy hash claims без naming metadata. Старый reader MUST отвергать новый authority claim contract до любых ownership effects; отсутствие понимания named allocation MUST NOT превращаться в удаление/переименование дерева. Saved Run contracts, WorkflowRevision и StepDefinition MUST NOT получать новые версии только из-за внешнего имени рабочего дерева; при отсутствии нового Run field прежние state/read contracts SHALL сохраняться.

Новые review/result формы SHALL одновременно поддерживать naming и существующий queue-bound launch; queue identity, intent и idempotency semantics MUST сохраняться. Structural проверки SHALL валидировать фактические outputs и persisted claim bytes, проверять отказ старой closed schema для новых полей и отсутствие новых полей в legacy outputs.

Выбор wire version SHALL учитывать metadata передаваемого claim даже после удаления policy из shared profile: named handover MUST NOT расширять старую closed форму. Отсутствие настройки SHALL сохранять старые output versions для обычного hash пути без inherited naming metadata.

Текущий source set — `openspec/specs/published-contracts/spec.md` и canonical JSON/Go artifacts; ownership не переносится. Historical evidence и archived changes MUST оставаться неизменными.

#### Scenario: Named launch вместе с queue selection
- **WHEN** профиль `/4` создаёт named worktree для queue-bound launch
- **THEN** новые output schemas принимают реальные naming и queue данные, Run сохраняет штатную queue association и прежнюю границу state/read для queue

#### Scenario: Новый claim прочитан старым reader
- **WHEN** authority содержит новый allocation contract, который reader не поддерживает
- **THEN** reader возвращает unsupported contract до Git/filesystem/claim mutation

#### Scenario: Профиль не включает настройку
- **WHEN** `/2`, `/3` или `/4` без `worktree_naming` запускает прежний workflow без передачи named claim
- **THEN** naming fields отсутствуют, имена и ранее применимые output/state versions сохраняются

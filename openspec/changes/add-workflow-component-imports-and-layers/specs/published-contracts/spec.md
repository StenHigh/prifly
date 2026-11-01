## ADDED Requirements

### Requirement: Композиция workflow имеет opt-in authoring boundary
Exports, root imports и производная folder SHALL использовать `prifly-project-workflow/2`. `extends` SHALL поддерживать один parent и несколько parents с общей базой. Extension imports, layers, composition groups, квалифицированные targets и новые extension binding expressions SHALL использовать `prifly-extension/3`. Старые authoring schemas SHALL сохранять bytes и прежний смысл.

Поэтапная реализация MAY временно отказывать на ещё неподдержанных формах. Она MUST NOT игнорировать parents/groups или заявлять полную capability до applicable gates. Опубликованная неполная новая форма MUST получить совместимую границу при последующем расширении.

Новые embedded/public schemas и editor mappings SHALL публиковаться вместе. Старый CLI MUST отвергать новые markers до effects. Обычные `/1`, legacy extend и `/2` extensions MUST NOT получать новую семантику автоматически.

#### Scenario: Прежний проект компилируется без перехода
- **WHEN** неизменённый legacy source компилируется новым binary
- **THEN** fixed compiled vectors, public versions и поведение остаются прежними

#### Scenario: Старый CLI читает новую композицию
- **WHEN** прежний binary встречает `/2` folder либо `/3` extension
- **THEN** он явно отказывает до исполнения, а не игнорирует imports/layers

### Requirement: Новые composition views не расширяют historical closed forms
Новые compile/explain/prepare JSON fields и build provenance SHALL иметь новые closed versioned forms. Implementation SHALL объявить точные версии до записи fixtures. Проверки SHALL читать реально emitted outputs и проверять их опубликованными schemas.

Композиция SHALL понижаться в обычные native definitions и PackageComponent зависимости. Sequence SHALL использовать обычные стадии, parallel — native branches/join, choice — native predicates/routes. Новый runtime operator или state version MUST NOT добавляться только ради authoring inheritance. Если реализация потребует нового persisted факта, отдельная совместимая граница SHALL быть согласована до его записи.

#### Scenario: Новый JSON показывает происхождение слоя
- **WHEN** CLI выдаёт composition-aware compile или prepare
- **THEN** новый bundle принимает output, а старая closed форма не используется с дополнительными полями

### Requirement: Возможность импорта доходит до автора в том же выпуске
Релиз composition capability SHALL включать строку capability в `examples/README.md`, authoring reference и полный пример без копирования provider исходников. Справочник SHALL показывать markers, minimum Core version, scoped targets, layers, bindings, diamond inheritance, группы и update/refusal поведение.

Проверки SHALL отличать compilation, native execution, host observations и опубликованный пилот. Synthetic applicability reader MUST NOT называться доказательством качества UI/UX или реальной browser qualification.

#### Scenario: Автор открывает индекс возможностей
- **WHEN** release заявляет imports и layers
- **THEN** индекс ведёт к примеру, который показывает реальные поля и минимальные совместимые contracts

### Requirement: Composition сохраняет границу выпущенных extension points
Composition `/3` и extension points `/4` SHALL сохранять разные closed authoring forms. Effective folder `/2` либо composition `/3` MUST отвергать effective `/4`, point entries и наличие `extension-points.yaml` в любом участвующем source package с typed `project_composition_extension_points_unsupported` до flattening, lowering и effects. Это ограничение SHALL включать невыбранный sidecar и inherited effective parents. Поддержка импортируемых point declarations требует отдельного совместимого контракта владения.

Library import SHALL игнорировать standalone provider `extend.yaml`, включая `/4`, если source не содержит point sidecar. Такой import MUST NOT применять provider extensions или расширять execution consent. Presence sidecar MUST NOT молча исчезать из identity: текущий срез явно отказывает всему сочетанию.

Legacy points `/4` без composition SHALL сохранять package-owned target, project-owned implementation, reviewed identity, disabled passthrough и native sealing order. Опубликованные `/4` и point `/1` schemas MUST сохранять bytes.

#### Scenario: Composition встречает point declaration без выбора
- **WHEN** root, parent либо imported provider содержит `extension-points.yaml`
- **THEN** reader возвращает typed unsupported refusal до объединения документов и не теряет declaration молча

#### Scenario: Effective parent выбирает /4 без sidecar
- **WHEN** composition наследует parent с effective `/4` либо читает `/4` layer
- **THEN** reader явно отказывает до graph mutation

#### Scenario: Library import не запускает standalone extensions
- **WHEN** consumer импортирует export provider со standalone `/4` без point sidecar
- **THEN** compiler использует только source export и не применяет provider extend

#### Scenario: Legacy point остаётся независимой возможностью
- **WHEN** folder `/1` использует `/4` без composition
- **THEN** прежние ownership, consent, freshness и native runtime checks сохраняются

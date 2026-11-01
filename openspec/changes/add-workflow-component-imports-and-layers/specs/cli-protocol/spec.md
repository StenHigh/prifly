## ADDED Requirements

### Requirement: Project показывает читаемую композицию до запуска
Для нового authoring Project compile и prepare SHALL показывать parents, общую базу, порядок layers, imports и добавленные stages. Обзор SHALL показывать однократное включение общего ancestor и явные sequence/parallel/choice группы. Краткий вывод SHALL отделять обязательный вызов проверки применимости от условной browser-проверки. Он MUST NOT изображать ещё не вычисленную применимость как пропуск.

Подробный explain SHALL показывать source file/key для effective settings, замещённые значения, export identity, dependency digest и итоговые routes. Обычный вывод MUST NOT требовать чтения полного compiled JSON. Public JSON SHALL иметь явный versioned contract.

#### Scenario: Пользователь читает конфигурацию smsplace-подобного сценария
- **WHEN** пользователь выполняет compile explain или prepare reference scenario
- **THEN** он видит classic базу, delivery layer, обязательный applicability child и условную UI/UX-ветвь без чтения provider internals

#### Scenario: Поздний слой заменил настройку
- **WHEN** собственный extend изменяет значение общего layer
- **THEN** explain показывает итоговое значение и оба source keys

#### Scenario: Пользователь проверяет нескольких родителей
- **WHEN** пользователь открывает explain workflow с двумя производными parents
- **THEN** он видит общую базу один раз, уникальные дополнения и способ их объединения после tests

### Requirement: Ошибка композиции указывает место исправления
Project CLI SHALL различать неизвестный package/export, неверный kind, цикл, несовместимую общую базу, ports, отсутствующий route и конфликт layers/contributions. Problem SHALL содержать стабильный code и контекст источника; новый отказ MUST использовать typed Fault.

Ошибка SHALL показывать consumer source, import alias или layer entry и relevant provider export. Она MUST NOT предлагать копировать upstream graph либо молча отключать обязательную проверку.

#### Scenario: Пакет больше не экспортирует прежнее имя
- **WHEN** compile читает старую import-ссылку после изменения provider
- **THEN** отказ называет alias, export и source key, а также доступные exports нужного kind

### Requirement: Prepare закрепляет всю композицию без выполнения providers
Reviewed prepare SHALL учитывать ancestry graph, общую базу, effective настройки, layers, contributions, группы, predicates, выбранные exports и bindings. Execution files, host contexts и разрешённые параметры SHALL оставаться частью review. Изменение любого effective входа MUST инвалидировать прежний review digest.

Prepare MUST NOT создавать Run, claim или trust и MUST NOT запускать programs, models либо network fetch. Чтение host skills SHALL следовать existing explicit host policy. Start SHALL повторно проверять входы до effects.

#### Scenario: Layer изменён между prepare и start
- **WHEN** пользователь запускает start со старым review digest после effective изменения layer
- **THEN** CLI отказывает до admission и требует новый обзор

### Requirement: Update и continuation сохраняют локальные расширения и принятые версии
Workflow update SHALL сохранять существующий project-owned extend и subtree по прежнему контракту. Shared layers вне provider folder SHALL оставаться нетронутыми. Update одного provider MUST NOT автоматически обновлять другие packages либо предоставлять им trust.

Новая compilation SHALL читать обновлённый provider, но старый Run SHALL использовать свои pinned bytes. Continue/recover SHALL применять existing effective accepted-prefix rules ко всему импортированному subtree. Несовместимый prefix MUST NOT переносить прежний PASS.

#### Scenario: Пользователь обновляет UI/UX provider
- **WHEN** update меняет только его upstream source
- **THEN** shared layer и consumer source сохраняются, а следующий prepare показывает новую зависимость

#### Scenario: Принятый imported child изменился
- **WHEN** reviewed continuation пытается перенести accepted prefix с другим effective child contract
- **THEN** existing recovery check отказывает либо допускает явно выбранный законный whole-child replay, без изменения source Run

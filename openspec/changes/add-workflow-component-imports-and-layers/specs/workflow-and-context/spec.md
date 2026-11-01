## MODIFIED Requirements

### Requirement: Call extension разрешает только объявленный exact local callee
В `prifly-extension/2` Call target SHALL разрешаться среди workflow components одной выбранной folder. Поиск SHALL включать `workflows/` и `project/workflows/` и использовать short name authored ID.

В новой форме `prifly-extension/3` Call target SHALL дополнительно принимать квалифицированное имя явно импортированного workflow export. Неквалифицированное имя SHALL сохранять прежнее локальное разрешение. Resolver MUST получить ровно один WorkflowRevision и проверить kind.

Resolver SHALL закреплять exact compiled `id`, `version`, `digest` цели и полную исполняемую зависимость. Owned refs SHALL связываться по обычным правилам compilation; external exact refs SHALL сохранять свой смысл.

Unknown target, wrong-kind, неоднозначность, несогласованный digest, отсутствующая dependency и call/repeat cycle MUST давать named refusal до admission. Выбор MUST NOT зависеть от порядка filenames или «последней установленной» редакции.

Установленный sibling package MUST NOT становиться целью поиска без явного импорта. В `/2` явно вендоренный child bundle SHALL оставаться достаточным; cross-package imports SHALL требовать `/3`.

#### Scenario: Переименование файлов не выбирает другую revision
- **WHEN** автор меняет порядок файлов неизменённого local child
- **THEN** compilation разрешает тот же exact callee и dependency closure

#### Scenario: Импортированная цель объявлена явно
- **WHEN** marked `/3` call называет workflow export через объявленный import alias
- **THEN** compilation связывает exact export и проверяет его обычный native Call contract

#### Scenario: Sibling существует без импорта
- **WHEN** расширение называет компонент соседнего установленного package без import declaration
- **THEN** compilation отказывает и не ищет замену в Registry

#### Scenario: Resolver не может доказать единственную цель
- **WHEN** target отсутствует, имеет другой kind, неоднозначен либо создаёт цикл
- **THEN** compilation отказывает до исполнения и указывает источник ссылки

## ADDED Requirements

### Requirement: Project folder явно экспортирует переиспользуемые компоненты
Новая форма `prifly-project-workflow/2` SHALL поддерживать именованные exports workflow и step. Export SHALL ссылаться на собственный объявленный компонент folder. Private компоненты MUST NOT становиться публичными из-за совпадения имени.

Supporting schemas, contexts, checks, instructions и execution files SHALL включаться через проверенные зависимости export. Входы и выходы SHALL сохранять exact schema identity. Непересекающиеся private компоненты MUST NOT попадать в исполняемую closure. Консервативное обновление build identity provider SHALL оставаться допустимым.

#### Scenario: Workflow экспортирован с supporting program
- **WHEN** автор импортирует workflow export с вложенным program step
- **THEN** compilation закрепляет его граф, контексты, schemas, execution bindings и supporting bytes без ручной копии исходников

#### Scenario: Private компонент пытаются вызвать напрямую
- **WHEN** consumer называет внутренний workflow, которого нет в exports
- **THEN** compilation отказывает и перечисляет доступные exports нужного kind

### Requirement: Импорт читает только объявленные источники и не наследует чужую настройку молча
`prifly-project-workflow/2` и `prifly-extension/3` SHALL объявлять imports через уникальный alias и имя package из выбранного `project.yaml`. Root imports SHALL описывать зависимости source exports; extension imports SHALL описывать зависимости consumer composition. Compilation MUST читать зарегистрированный tracked source и проверять его declared origin, если origin указан. Compilation MUST NOT выполнять fetch, install, update, trust или код package.

Импорт SHALL выбирать provider source exports, а не его launch, `extend.yaml` или локальные проектные слои. Consumer SHALL передавать public inputs и явно задавать разрешённые project-scoped settings экспортированного workflow. Импорт private internals, изменение producer effects и замена уже связанной provider программы MUST отказываться.

Aliases SHALL изолировать имена разных imports. Одна exact dependency SHALL учитываться однократно; конфликт identity с разными bytes SHALL отказываться. Все зависимости SHALL использовать общие source/size/depth и runtime limits; импорт MUST NOT обнулять budgets.

#### Scenario: Provider имеет собственные проектные overrides
- **WHEN** standalone provider настроен своим extend, а consumer импортирует его export
- **THEN** consumer получает объявленный source export и свои явные bindings/settings, без скрытого применения provider extend

#### Scenario: Provider обновился после prepare
- **WHEN** bytes выбранного export или его зависимости изменились после reviewed prepare
- **THEN** start со старым review digest отказывает до claim или dispatch

#### Scenario: Import пытается скачать отсутствующий package
- **WHEN** имя отсутствует в project packages либо source недоступен
- **THEN** compilation отказывает с предложением отдельно установить или исправить источник

### Requirement: Производный workflow объединяет родителей с общей закреплённой базой
`prifly-project-workflow/2` SHALL принимать `extends` с одним package либо непустым списком родителей выбранного проекта. Производный workflow SHALL иметь собственные package и root workflow identities. Он SHALL наследовать маршрут, публичные contracts и объявленные расширения родителей без копирования source graph.

Inheritance graph SHALL быть конечным и ацикличным. Для нескольких родителей resolver SHALL находить единственную ближайшую общую базу с одинаковой exact source identity. Несвязанные основы, разные закреплённые редакции общего предка и self-reference MUST отказываться.

Общий ancestor и одинаковые унаследованные contributions SHALL включаться однократно по source provenance и совпадающему effective contract. Одинаковое имя или одинаковые bytes независимых contributions MUST NOT означать общность происхождения. Разные bindings/settings общего contribution MUST NOT исчезать при dedup. Несовместимые изменения SHALL отказываться либо разрешаться явно в допустимом authoring scope.

Производный workflow SHALL явно объявлять limits либо наследовать согласованные limits. Compilation MUST NOT автоматически суммировать budgets или повышать policy ceilings. Source graphs родителей SHALL оставаться неизменными.

#### Scenario: Проект использует общий маршрут без копирования
- **WHEN** производная folder объявляет classic базой и добавляет свой extension
- **THEN** compiled root содержит inherited маршрут и extension, а исходники classic не изменяются

#### Scenario: Обновление базы удалило точку вставки
- **WHEN** новая база больше не содержит точный переход, указанный extension
- **THEN** compilation отказывает и показывает слой, запись и отсутствующий переход

#### Scenario: Ромб наследования объединяет общий classic
- **WHEN** два производных workflow наследуют одну exact classic-базу и проект наследует оба
- **THEN** общий маршрут и его tests/commit исполняются один раз, а уникальные дополнения обоих родителей сохраняются

#### Scenario: Родители закрепили разные общие основы
- **WHEN** родители ссылаются на несовместимые редакции общего ancestor либо не имеют единственной общей базы
- **THEN** compilation отказывает и показывает происхождение конфликта, не выбирая «первого» родителя

### Requirement: Автор явно задаёт композицию унаследованных дополнений
Новая authoring composition SHALL поддерживать sequence, parallel и choice для contributions общей базы. Contribution SHALL означать добавление с source identity, объявленными входами/исходами и ограниченной точкой подключения. Оно SHALL допускать отдельный step/Call либо структурированный child graph, включая развилку.

Дополнения разных непересекающихся переходов SHALL сохранять свои места. Несколько contributions на одном переходе SHALL требовать явной группы. Порядок `extends` MUST NOT означать порядок исполнения, parallelism или подавление дополнения.

Sequence SHALL исполнять указанные contributions в авторском порядке. Parallel SHALL использовать native branches и all/wait join с проверкой всех обязательных результатов. Choice SHALL использовать обычные typed predicates с явными default/unknown routes. Автор SHALL явно указывать, какие дополнения обязательны и какие являются альтернативами.

Группа SHALL охватывать каждое contribution своей точки один раз. Потерянное, повторённое, неизвестное или silently skipped обязательное contribution MUST вызывать отказ. Native validation SHALL проверять bindings, branch output availability, budgets, effects и resources всего результата.

#### Scenario: Две проверки нужны после tests
- **WHEN** два родителя добавляют проверки на `tests → commit`, а consumer явно задаёт sequence
- **THEN** tests исполняется один раз, затем обе проверки в указанном порядке, затем commit только после их успешной приёмки

#### Scenario: Все дополнительные проверки нужны параллельно
- **WHEN** consumer явно объединяет независимые проверки через parallel
- **THEN** native join ждёт обе settled ветви и не допускает commit по одному положительному результату

#### Scenario: После tests выбирается альтернативный маршрут
- **WHEN** родитель добавляет child Choice и consumer сохраняет или явно объединяет это contribution
- **THEN** развилка исполняется по объявленным predicates без копии классического графа

#### Scenario: Пересечение оставлено без порядка
- **WHEN** родители добавили разные contributions на один переход без явной группы
- **THEN** compilation называет contributions и предлагает выбрать sequence, parallel или choice вместо скрытого порядка

#### Scenario: Выход одной parallel ветви читается в соседней
- **WHEN** composition требует результат ещё не settled sibling branch
- **THEN** ordinary native binding validation отказывает до исполнения

### Requirement: Слои расширения применяются в явном порядке без произвольного YAML merge
`prifly-extension/3` SHALL поддерживать упорядоченный список локальных layers. Слой SHALL содержать те же ограниченные настройки и extensions, но MUST NOT подключать другие layers или задавать базу. Файл SHALL разрешаться относительно объявляющего документа внутри выбранного repository без symlinks.

Порядок для одной ветви SHALL быть: effective база, перечисленные layers, собственный extend, разрешённые параметры Run. Позднее значение SHALL заменять одноимённую конечную настройку; массив SHALL заменяться целиком. Общие ancestors SHALL учитываться один раз до уникальных parent deltas.

Разные значения одного ключа в независимых parents SHALL требовать явного решения consumer. Порядок списка parents MUST NOT выбирать победителя. Одинаковое effective значение SHALL оставаться допустимым с сохранением обоих источников. Родственные overrides одной ветви SHALL сохранять обычный порядок слоёв.

Extensions SHALL сохранять source identity и unique effective stage IDs. Повторный layer из общего ancestor SHALL применяться один раз. Повторное явное подключение того же layer либо конфликт структурных вставок без разрешения MUST отказываться.

`workflow: root` SHALL адресовать effective root только в `/3`. В `/3` явный extension `id` SHALL именовать stage независимо от qualified step/Call target. Исходные v1/v2 selectors и step IDs SHALL сохранять прежний смысл. Слой MUST NOT расширять разрешения policy или обходить native validation.

#### Scenario: Проект переопределяет одну общую настройку
- **WHEN** общий layer задаёт два settings, а собственный extend меняет одно
- **THEN** первое значение сохраняется, второе заменяется, а обзор показывает оба источника

#### Scenario: Два слоя вставляют одинаковую stage
- **WHEN** extensions двух layers используют один stage ID
- **THEN** compilation отказывает с обоими source paths вместо скрытой замены

#### Scenario: Два родителя меняют одну настройку
- **WHEN** независимые parents задают разные значения одного setting без явного значения consumer
- **THEN** compilation показывает оба источника и отказывает; перестановка parents не меняет результат

### Requirement: Импортированный вызов сохраняет обычные bindings и единый Run
В `/3` imported workflow SHALL вызываться native Call, а imported step — обычной step stage. Call bindings SHALL поддерживать `$inputs.PORT` и `$stages.STAGE.PORT`, включая обычные typed projections, допустимые native contract.

Compilation SHALL проверять required inputs, exact schemas, project-scoped configuration и гарантированность outputs на каждом пути. Child SHALL исполняться в одном Run с общими claim, effects, resources и budgets. Отдельный `project start` вместо Call MUST NOT использоваться.

#### Scenario: Child получает задачу и результат реализации
- **WHEN** extension связывает root task и гарантированный implementation output с compatible child ports
- **THEN** child получает исходные ArtifactRefs с provenance, без пересказа или поиска «последнего» artifact

#### Scenario: Success-only output требуется после partial
- **WHEN** downstream требует export, который child не гарантирует на partial route
- **THEN** compilation отказывает до исполнения

### Requirement: Условное выполнение остаётся авторским графом с обязательной проверкой применимости
Композиция SHALL позволять подключить exported workflow, который обязательно определяет применимость и затем выбирает собственную ветвь исполнения. Core MUST NOT вводить предметную UI/UX-эвристику или неявный `when changed` для импорта.

Универсальный reference scenario SHALL передавать исходную задачу и актуальный candidate в такой child при каждом достижении приёмки. Его типизированный результат SHALL различать required, not-required с причиной и unknown. Required SHALL вести к проверке; not-required — к записанному пропуску; unknown — к несостоявшейся приёмке. Недоступная обязательная проверка MUST NOT становиться успехом.

После repair либо изменения candidate прежние applicability и acceptance evidence MUST NOT использоваться для нового состояния без ordinary freshness/replay checks. Native cancel MUST NOT объявляться доказательством cleanup внешней среды.

#### Scenario: Интерфейс изменён через PHP
- **WHEN** reference fixture содержит задачу об интерфейсе и изменение PHP-описания admin UI
- **THEN** обязательный applicability child вызывается и required route выполняет UI/UX reader, без встроенного в Core списка frontend extensions

#### Scenario: Изменения не требуют UI/UX
- **WHEN** applicability child установил not-required для актуальной задачи и candidate
- **THEN** Run сохраняет причину пропуска и не создаёт browser reader Attempt

#### Scenario: Применимость установить нельзя
- **WHEN** applicability child сообщает unknown либо обязательные данные отсутствуют
- **THEN** родитель не достигает successful acceptance, commit или publication по этому пути

#### Scenario: После проверки произошла починка
- **WHEN** candidate изменился после принятого applicability/acceptance результата
- **THEN** следующий проход reference scenario повторяет проверку применимости и нужную приёмку для нового candidate

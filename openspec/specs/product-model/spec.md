# product-model Specification

## Purpose

Определяет назначение Pri-Fly и границы универсального продукта, чтобы
сценарии, исполнители и подключённые системы не подменяли управление работой,
доказательства результата и решение владельца.

## Requirements

### Requirement: Pri-Fly управляет проверяемым выполнением работы
Pri-Fly MUST управлять сценарием от принятого задания до проверенного
результата: закреплять предмет, входы, исполнителя, полномочия, историю,
evidence, точки решения человека и безопасное продолжение после остановки.
Подключённые исполнители выполняют предметную работу, но не получают
ответственность Core за admission, журнал и честный status.

#### Scenario: Исполнитель сообщает о завершении
- **WHEN** worker возвращает результат допущенной работы
- **THEN** Pri-Fly различает сообщение worker-а, его evidence и принятое
  состояние сценария, прежде чем продолжить маршрут

### Requirement: Новый сценарий выражается существующими контрактами
Новый сценарий MUST добавляться без изменения Core, если он выражается
существующими step, data и capability contracts. Core MUST не закреплять
предметные имена вроде plan, merge, GitLab, Laravel или конкретной модели ИИ;
неподдержанная capability MUST дать `unsupported_capability` с названием
недостающего контракта, а не скрытый fallback.

#### Scenario: Проект добавляет собственную работу
- **WHEN** author описывает сценарий из поддерживаемых contracts
- **THEN** он может быть проверен без изменения Core, а недостающая capability
  объясняется явным отказом

### Requirement: Пустое ядро остаётся допустимой установкой
Установка без packages, сценариев, task source, Git и AI credentials MUST быть
допустимой: version, diagnostics, пустой inventory, project creation и
проверка собственного step definition доступны без network intake или скрытого
standard workflow. Минимальный разрешённый command step MUST работать без
AI Factory, repository и модели.

#### Scenario: Пользователь открывает пустую установку
- **WHEN** пользователь запрашивает диагностику или отсутствующий workflow
- **THEN** он получает доступные пустые представления либо понятный отказ без
создания Run, установки package или обращения к модели

### Requirement: Ответственность частей продукта разделена
Core, package, adapter, project configuration, host/platform и owner MUST
иметь явные логические границы. Core управляет состоянием и admission; package
описывает работу; adapter реализует свой протокол; configuration выбирает
ресурсы и policies; host применяет реальные ограничения; owner определяет
цель и неделегируемый риск. Core MUST не получать project paths, названия
тестов, формат tracker task или конкретную модель; package MUST не расширять
себе полномочия; adapter MUST не объявлять чужой effect состоявшимся;
configuration MUST не менять files установленного общего package; host MUST
не доверять запросу вне собственных разрешений; owner MUST не быть обязанным
разбирать каждую техническую Attempt вручную. Это разделение MUST не требовать
микросервисов.

#### Scenario: Подключается новый внешний инструмент
- **WHEN** проекту требуется инструмент или предметное правило
- **THEN** оно остаётся в package/adapter/configuration и не становится
обязательным бизнес-объектом Core

### Requirement: Базовый продукт не подменяет внешние системы
Pri-Fly MUST не объявлять себя Git host, editor, model, secret store, CRM,
billing system, operating system, general-purpose language или distributed
cluster. Он MUST не создавать marketplace с автоматическим выполнением
установочных scripts. Возможность вызвать внешнюю систему через adapter MUST
не означать, что Pri-Fly владеет её business state или что завершение local
step доказывает внешний effect.

#### Scenario: Local step предлагает публикацию
- **WHEN** step завершает локальную подготовку материала
- **THEN** его completion не выдаётся за доставку получателю, merge ветки или
другое внешнее последствие без отдельного контракта и evidence

### Requirement: Роли и полномочия не выводятся из текста
Owner, scenario author, operator, worker, reviewer, administrator и auditor
MUST различаться по ответственности. Один человек MAY совмещать роли, если
policy не требует независимости; имя процесса, prompt или незащищённый payload
MUST не назначать роль и не повышать права.

#### Scenario: Worker называет себя администратором
- **WHEN** worker или command payload заявляет другую роль
- **THEN** Pri-Fly использует удостоверенную policy и object access, а не это
самоописание

### Requirement: Одна модель поддерживает разные предметные сценарии
Pri-Fly MUST позволять packages выражать разработку, research, документы,
данные, controlled import, audit, incident work, release, human process и
finite parallel work без обязательного Git, LLM, конкретного hosting provider
или отраслевого package. Матрица таких применений MUST не обещать поставку
каждой интеграции. В частности, Core MUST не требовать repository/commits для
research, documents или data work; не выдавать import за безусловный bulk
write; не требовать исправления после audit; не отправлять production command
из incident log; не навязывать CI/CD для release; не публиковать текст только
потому, что он готов; не создавать бесконечный shell process для periodic
check; не требовать модель для human process и не давать параллельным workers
необъявленный общий изменяемый file.

#### Scenario: Проект описывает исследование без репозитория
- **WHEN** package задаёт источники, worker и проверку исследования
- **THEN** Core не требует Git, coding workflow или вызов модели только из-за
выбранного класса работы

### Requirement: Предмет и границы закреплены до выполнения
До значимой работы Run MUST иметь RunBrief с предметом, ожидаемым результатом,
scope, criteria, разрешёнными ресурсами и основанием старта. `TaskInput/1`
MUST сохранять исходный текст и provenance, а local preparation MUST выводить
из него RunBrief и immutable SourceSnapshot без изменения scope. Существенная
неоднозначность MUST потребовать конкретного уточнения или подтверждения.
Подтверждённая project/resource identity MUST иметь приоритет над соседним
контекстом разговора. Прямое указание owner-а exact task/workflow MAY быть
основанием старта в рамках существующих полномочий; уже разрешённые безопасные
действия MUST не требовать повторного подтверждения без новой причины.

#### Scenario: Предложенный brief неверно понял предмет
- **WHEN** owner отклоняет preview RunBrief
- **THEN** массовая работа не начинается, пока предмет и границы не будут
исправлены и подтверждены

### Requirement: Изменение намерения не продолжает прежний объём
Новые сообщения owner-а MUST различаться как clarification, scope change,
status question, pause, cancel или новый запрос. Status question MUST не
отменять исходную работу. Существенная смена предмета MUST остановить новые
admissions прежнего scope и создать новую связанную brief/revision либо Run;
старые outputs и effects MUST не переименовываться под новую цель.
Классификация, способная изменить scope или полномочия, MUST сохраняться и
показываться owner-у; clarification MUST не теряться при compaction или смене
session, а owner MAY отклонить предложенную классификацию.

#### Scenario: Owner меняет задачу после начала работы
- **WHEN** у Run уже есть results или pending intents и owner меняет scope
- **THEN** Pri-Fly сохраняет provenance прежней работы и не допускает её как
результат новой цели без явной связанной revision/fork

### Requirement: Сценарий отделён от предметного плана
Workflow MUST объявлять допустимые stages и transitions; предметный plan MAY
быть одним из его artifacts, но не является обязательным для всех работ. AI
MAY предложить plan или новый workflow, однако предложение MUST пройти тот же
validation и admission, что и человеческое определение. Принятый Run MUST не
получать скрытую замену graph, instructions или criteria.
Новый вид работы MUST использовать принятую revision или связанный Run; child
workflow MUST вызываться только по заранее pinned reference. Произвольная
команда в commentary MUST не менять graph.

#### Scenario: Модель предлагает другой маршрут
- **WHEN** в активном Run появляется новый список stages
- **THEN** он не изменяет текущие bindings и transitions; применение требует
отдельной принятой revision или связанного Run

### Requirement: Взаимодействие с человеком отделено от исполнения
`interaction_mode=with_human|unattended` MUST описывать доступность решения
человека, а `execution_mode=assisted|managed` — способ запуска worker-а. В
assisted Pri-Fly MUST не обещать пробуждение завершённой host session; в
unattended заранее утверждённые limits и grants MUST не превращать отсутствие
человека в согласие. Managed execution MUST квалифицироваться только там, где
adapter умеет start, status, cancel и recovery процесса; иначе это свойство
остаётся неподдержанным.

#### Scenario: Assisted host недоступен в точке решения
- **WHEN** scenario требует участия host-а или человека, но они недоступны
- **THEN** Run остаётся честно waiting либо сообщает неподдержанную capability,
а не изображает фоновое выполнение или approval

### Requirement: Параллельная работа ограничена зависимостями и ресурсами
Одновременное исполнение MUST быть разрешено только независимым по data и
resource StepInstances. Общие limits распространяются на весь Run, children,
loops и retries; join MUST объявлять нужные results и обработку неполных,
failed, skipped, waived или cancelled branches. Human UI grouping MUST не
отменять обязательные producer dependencies.

#### Scenario: Пользователь отключает необходимую ветвь
- **WHEN** отключён step, output которого требуется дальше
- **THEN** compiler требует явный совместимый input/reuse либо отклоняет
вариант, не создавая отсутствующий artifact

### Requirement: Результат и evidence имеют разные уровни достоверности
Process exit, полученный artifact, passing check и внешний effect MUST
наблюдаться как разные факты. Acceptance MUST опираться на объявленные criteria
и evidence; prose worker-а не является самостоятельным доказательством.
`no_work`, `skipped`, `waived`, `rejected`, допустимый partial и success MUST
оставаться различимыми, а unknown external effect MUST не выглядеть успехом.
Нетестируемая semantic оценка MUST хранить reviewer, материалы, criteria,
вывод и ограничения; независимость reviewers MUST определяться context,
roles и access, а не только разными именами моделей. Процент готовности MUST
не объявляться полным до completion выбранного scenario.

#### Scenario: Проверка не подтверждает внешний effect
- **WHEN** worker завершился и artifact прошёл проверку, но effect не
подтверждён
- **THEN** Run не сообщает неподтверждённый effect как completed success

### Requirement: Проверки выбираются явно и не обходят integrity guards
Package MUST объявлять meaningful checks и выбор их объёма для конкретной
работы. Размер diff, уверенность модели или стоимость MUST не давать право
пропустить требуемую проверку. Разрешённое исключение MUST быть явным owner
decision и recorded waiver; Core integrity, identity, authorization и
confinement checks MUST не допускать waiver.
Package-level policy MAY требовать review, protected branches или
неизменность спецификации, но эти правила MUST не становиться скрытой
зависимостью empty Core.

#### Scenario: Автор предлагает не запускать проверку из-за малого изменения
- **WHEN** package требует проверку, а worker предлагает её пропустить
- **THEN** Pri-Fly сохраняет required check либо применяет только явно
разрешённый waiver вне integrity guards

### Requirement: Остановка и восстановление сохраняют границы работы
Owner MUST иметь возможность остановить новые действия, увидеть in-flight
work и получить безопасный следующий шаг. Stop MUST не обещать откат внешних
effects. Resume MUST сохранять принятые definition и history; retry MUST быть
новой Attempt только когда это разрешено; изменение workflow, input или policy
MUST создавать связанную revision/fork с анализом reuse evidence.

#### Scenario: Run восстанавливается после остановки
- **WHEN** Run получает resume после durable stop/release
- **THEN** он не повторяет подтверждённый effect и не скрывает неизвестный
исход прежней Attempt

### Requirement: Packages выбираются явно и проверяются до запуска
Установка MUST позволять выбрать empty Core и конкретные packages, показывая
dependencies, capabilities, install-time effects и compatibility. Отсутствие
нужных данных или executor-а MUST дать совместимую зависимость либо понятный
conflict до запуска. Optional AI Factory package MUST не быть обязательным для
Core и его удаление MUST не ломать независимые scenarios.

#### Scenario: Выбранный package требует отсутствующую capability
- **WHEN** owner выбирает package без необходимого executor-а или input
- **THEN** установка объясняет dependency/conflict до Run, не устанавливая
полный чужой workflow или AI Factory по умолчанию

### Requirement: Стоимость и качество процесса наблюдаются честно
Pri-Fly MUST различать measured, estimated и unavailable сведения о времени,
rounds, context, model telemetry и расходах внешних tools. Сокращение затрат
MAY происходить через меньший context, reuse и подходящий executor, но MUST не
достигаться сокрытием stop, проверки или качества. Объявление unlimited tokens
MUST не становиться бессрочным financial grant для будущих Runs.

#### Scenario: Источник стоимости не предоставил данных
- **WHEN** assisted host не сообщает usage или cost
- **THEN** Pri-Fly показывает это как unavailable, а не вычисляет или выдаёт
нулевую стоимость без evidence

### Requirement: Заявленная capability не равна qualification
Каждая capability MUST различать `specified`, `implemented`, `qualified`,
`enabled` и `suspended`. Qualification MUST опираться на конкретный protocol
испытаний для runtime/package/adapter/deployment profile; schema validation,
самодекларация или один demo MUST не выдаваться за qualification. Неизвестное
или недоказанное свойство MUST оставаться явно ограниченным.

#### Scenario: Workflow требует неподтверждённое свойство adapter-а
- **WHEN** выбранный adapter не квалифицирован для нужного свойства
- **THEN** Pri-Fly объясняет отсутствующую capability и не выбирает другой
provider или неподтверждённый режим

### Requirement: Owner сохраняет переносимый доступ к работе
Owner MUST получать документированные переносимые formats workflow/package
lock, brief, history, artifacts, approvals и evidence с manifest/checksums и
redaction. Local mode MUST не зависеть от cloud account. Offboarding MUST
останавливать новые admissions и сохранять либо явно учитывать active pins,
unknown obligations и нужные данные до согласованного retention решения.
Uninstall MUST не удалять user work, active pins, unmerged changes или evidence
без отдельного согласованного retention решения.

#### Scenario: Внешний provider недоступен
- **WHEN** owner читает остановленный local Run без доступа к provider-облаку
- **THEN** он получает разрешённые history и pinned data, а отсутствие облака
не закрывает ему доступ к собственному состоянию

### Requirement: Независимость продукта проверяется обязательными конфигурациями
Product acceptance MUST проверять чистый каталог без `.ai-factory`, собственный
command step, scenario без task/Git/LLM, отдельный optional AIF package в чужом
project, project-specific configuration без изменения package, stop/recovery,
unsupported capability и export с воспроизводимым чтением. Архитектурная
полнота MUST оцениваться этими закрытыми contracts и явными границами, а не
числом абстракций или обещанием бесконечных возможностей.

#### Scenario: Выпускается универсальный build
- **WHEN** candidate заявляет универсальность Pri-Fly
- **THEN** applicable independent configurations и их evidence проверены, а
отсутствующие возможности остаются явно исключёнными из заявления

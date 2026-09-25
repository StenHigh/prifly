## Purpose

Определяет versioned public command protocol Pri-Fly: DTO, CLI, validation,
executor boundary, errors, preview и compatibility без прямого изменения
authority клиентом.

## Requirements

### Requirement: Все клиенты используют один command protocol

CLI, local UI, assisted host и managed executor MUST применять один versioned
protocol. Только command handler меняет authority; UI, model и skill не пишут
state напрямую. Remote transport требует отдельной qualification security
properties и не является обязательным.

#### Scenario: UI пытается изменить state напрямую

- **WHEN** client обходит command handler
- **THEN** authority не принимает mutation

### Requirement: Project entry points select their host mechanically
`project init` SHALL создавать нейтральный profile `/3` в обычной папке без
обязательного Git и без AI skills. Host entry points SHALL добавляться только
явно выбранным поддержанным hosts; каждый передаёт свой identity, не угадывает
его по directory. Compile `/3` MUST требовать host лишь при чтении host-bound
source; `/2` сохраняет explicit host. Fresh init MUST отвергать unsafe root или конфликт runner без
перезаписи. Для valid existing profile после clone/copy init MUST создавать
только отсутствующую local configuration, сохраняя shared YAML и exact runners.
Названный `--host`, который профиль **уже объявляет**, MUST NOT быть отказом
такого init: это заявление «я работаю отсюда», а не попытка присоединить
второй хост. Отказ `project_profile_conflict` MUST оставаться ровно для
хоста, которого профиль не объявляет.
Отсутствие runner-файла объявленного host MUST NOT быть отказом такого init:
профиль общий, а runner держат не все clone. Init MUST требовать присутствия
только у host, названного `--host`, MUST называть остальных отсутствующих в
ответе так же, как это делает `project runners update`, и MUST NOT требовать
их наличия у команд, которые их создают: `project runners add --host NAME`
MUST отказывать лишь из-за конфликта самого named runner, а не из-за
отсутствия чужого. Отказ, который всё же случается, MUST называть
исполнимый выход в `safe_next_actions`, а не только справку.
Квитанция machine-local настройки MUST описывать файл, а не аргументы вызова:
разрешённые программы и окружение читаются из `local.yaml` одинаково, чтобы
вызов, изменивший одно, не читался как отменивший другое.
Чтение `/2` и распознавание опубликованных frozen runners MUST сохраняться.

Для конечного developer decision entry point MUST использовать нативный
question tool своего host, когда этот tool предоставлен runtime: Codex runner
вызывает `request_user_input`, а Claude Code runner — `AskUserQuestion`.
Новый runner для Codex CLI и Codex app MAY разделять один текст template с
подставленным fixed host ID; Claude Code MUST получать отдельный template.
Вопрос содержит только реальные взаимоисключающие варианты, short label и
последствие выбора; рекомендуемый вариант обозначается явно. Runner MUST NOT
синтезировать Markdown-псевдокнопки, скрывать варианты или выбирать default.
Если native tool не предоставлен, runner MUST ждать явный текстовый ответ и
MUST NOT начинать mutation. Existing tracked runner остаётся reviewed source:
новая версия `project init` не перезаписывает его; owner обновляет его отдельным
commit.

#### Scenario: Claude Code запускает общий проект
- **WHEN** developer вызывает установленный `.claude/skills/prifly-run`
- **THEN** он передаёт `claude-code` и не читает Codex root

#### Scenario: Codex показывает конечный выбор нативно
- **WHEN** Codex runner получил несколько допустимых launch или Workspace
  вариантов и `request_user_input` предоставлен runtime
- **THEN** runner вызывает этот tool до создания package, claim или Run и ждёт
  returned selection

#### Scenario: Native tool недоступен
- **WHEN** host runner обязан получить конечное решение, но runtime не
  предоставляет его native question tool
- **THEN** runner запрашивает один явный текстовый ответ и не выбирает вариант
  или не меняет authority до ответа

#### Scenario: Existing host runner останавливает init
- **WHEN** создание выбранного runner конфликтует с существующим файлом
- **THEN** init возвращает diagnostic без частичной перезаписи profile/runners

#### Scenario: Clone получает только local authority configuration
- **WHEN** shared profile и его runners уже есть, а local configuration отсутствует
- **THEN** init создаёт только machine-local configuration

#### Scenario: Пользователь не использует ИИ
- **WHEN** init выполняется без host в папке без `.git`
- **THEN** Project готов к managed workflow, AI directories и Git не создаются

#### Scenario: Clone без runner-ов чужих хостов подключается
- **WHEN** профиль объявляет несколько hosts, а в этом clone лежит runner
  только одного из них, и разработчик вызывает `project init` с этим host
- **THEN** init создаёт local configuration и authority, называет отсутствующие
  runner-ы в ответе и ничего не переписывает

#### Scenario: Команда, создающая runner, не требует его наличия
- **WHEN** разработчик вызывает `project runners add --host NAME` в clone, где
  runner другого объявленного host отсутствует
- **THEN** команда создаёт названный runner; отсутствие чужого отказом не
  является

#### Scenario: Init называет хост, с которого работают
- **WHEN** профиль уже объявляет названный `--host`, а local configuration
  в этом clone отсутствует
- **THEN** init создаёт её и authority, не переписывая общий YAML и runner-ы;
  отказ остаётся только для хоста, которого профиль не объявляет

#### Scenario: Квитанция machine-local настройки после частичного изменения
- **WHEN** `project local set` меняет одну часть настройки, а другая уже
  записана в `local.yaml`
- **THEN** квитанция печатает обе из файла, и владелец не читает её как отмену
  того, чего вызов не касался
### Requirement: Общий runner не содержит правил отраслевого процесса
Текущий `prifly-run` SHALL исполнять только общий protocol выбранного launch:
объявленные inputs, ready tasks, effects, typed decisions, control и outputs.
Он MUST NOT добавлять improve/review/fix, число рецензентов, обязательный commit
или правило завершения отраслевого цикла. Такие правила MUST принадлежать
workflow package. Frozen исторические templates остаются только для exact
recognition и безопасного upgrade, не как default инструкции нового runner.

#### Scenario: Package использует один шаг без planning/review
- **WHEN** host запускает такой package
- **THEN** runner не добавляет рецензента, цикл улучшений или commit

### Requirement: Project результат различает авторскую версию и сборку
При compilation profile `/3` CLI MUST выдавать `project-compile/2`, а при
start — `project-start/3`. Оба результата MUST содержать `author_package`
с авторскими `id`/`version`, `build_key` и exact compiled ref в `package`.
Результат start MUST сохранять Run, Workspace и применимые Decision Sheet и
autonomy summary; compile и start одного входа MUST согласовывать root и
сборку. Legacy `/2` MUST сохранять существующие response versions и поля.

#### Scenario: Команда запускает другой вариант
- **WHEN** один author package компилируется и запускается с profile `/3`
- **THEN** оба ответа показывают понятную авторскую версию и одну exact сборку,
  а потребитель не принимает author version за alias последнего варианта

### Requirement: Wire framing имеет strict version и limits

Closed DTO MUST различать protocol, resource, semantic, state and read versions.
Wire использует strict UTF-8 JSON, one-object framing, clean JSON stdout and
bounded bytes/depth/nodes before schema validation; unknown fields не расширяют
старый contract.

#### Scenario: Request превышает recursion limit

- **WHEN** JSON выходит за declared depth или node cap
- **THEN** handler отказывает до dispatch без truncation

### Requirement: Shape validation не является admission

Handler MUST последовательно проверить framing, authenticated context, selected
DTO schema, current read access, dedup, exact refs/digests/provenance, semantic
gates, authority/quotas/resources и atomic commit. Old receipt может вернуться
после current read check, но не разрешает new dispatch.

#### Scenario: Receipt запрашивает actor без доступа

- **WHEN** caller потерял право читать protected result
- **THEN** handler не раскрывает receipt через idempotent retry

### Requirement: Canonical identity сохраняет exact bytes

Digest MUST name its bytes/scope, exclude self-reference and use declared
canonicalization. Blob digest covers actual bytes; resource identity is not
silently case- or Unicode-coerced. Timestamps, control keys and numeric data
follow declared exact formats without float or lexical shortcuts.

#### Scenario: Два Unicode-identifiers похожи визуально

- **WHEN** resolver не объявил их equivalence
- **THEN** protocol сохраняет distinct identities

### Requirement: Public DTO имеет named authority происхождения

Definitions, configuration, Run, execution, data, effects, controls,
composition and exchange DTO MUST name which authority creates their facts.
Read model is not universal save command; actor, epoch, received time and state
version are not accepted from caller as evidence.

#### Scenario: Client задаёт control epoch

- **WHEN** mutation payload содержит self-asserted authoritative field
- **THEN** handler derives it from authority rather than trusting payload

### Requirement: StepDefinition описывает contract без self-qualification

StepDefinition MUST declare exact kind, ports, executor/operation, context,
capabilities, effect/retry class and result checks. Declaration of an effect,
sandbox or retry property does not prove it; unknown executor, missing required
context or conflicting capability blocks admission.

#### Scenario: Package заявляет class none и пишет data

- **WHEN** proposed operation exceeds declared capability
- **THEN** admission rejects it

### Requirement: Ports явно описывают required data

Input/output ports MUST use exact typed contracts, required semantics, JSON
schema or blob media/content checks. `skipped`, `waived` and technical failure
are distinct; an absent required producer output cannot be manufactured for a
consumer.

#### Scenario: Required output пропущен

- **WHEN** downstream binding требует output skipped producer
- **THEN** workflow cannot admit consumer without declared optional path

### Requirement: InputBinding выбирает declared provenance

Binding MUST explicitly select workflow input, stage output, literal or allowed
scope with exact producer/port/ref semantics. It does not select a similar or
latest artifact implicitly; unavailable, ambiguous or unsupported projection
fails before execution.

#### Scenario: Два producer имеют совместимый type

- **WHEN** binding does not name one producer
- **THEN** validation reports ambiguity

### Requirement: Workflow graph имеет finite typed transitions

Definition MUST declare valid stage kinds, typed bindings, terminal outcomes and
finite composition. Unsupported control kind fails early; errors, calls, repeat,
parallel, map and waits only exist under their versioned contracts.

#### Scenario: Graph содержит неразрешённый cycle

- **WHEN** resolution detects cycle or bound overflow
- **THEN** Run is not created

### Requirement: Choice использует declared three-valued semantics

Choice MUST evaluate bounded typed predicates with explicit missing, null, type
error and unknown behavior. Exclusive ambiguity and first-match order are
declared; prose or model confidence cannot select branch.

#### Scenario: Predicate возвращает unknown

- **WHEN** required value is unavailable
- **THEN** contract follows declared unknown path, not convenient default

### Requirement: Parallel aggregate declares quorum и remainder

Parallel MUST name membership, join/selection/quorum rule, residual branches
and aggregate outcome. Early winner does not prove loser cancellation or release
of claims/effects.

#### Scenario: Quorum достигнут

- **WHEN** remaining admitted branch has unknown effect
- **THEN** aggregate does not hide its obligation

### Requirement: Map seals collection до children

Map MUST validate complete input collection, item schema, unique stable keys and
max_items before first child admission. Empty collection uses declared `on.empty`;
item provenance and concurrency remain bounded.

#### Scenario: Collection меняется после start

- **WHEN** new member appears outside sealed manifest
- **THEN** existing map does not create hidden child

### Requirement: Repeat сохраняет persistent bounds и decision state

Repeat MUST declare finite iterations, body, exit decision and bindings. Each
accepted iteration and control transition records state; restart, delivery retry
or whole-step retry does not reset repeat counter or become semantic rework.

#### Scenario: Delivery retry произошёл внутри body

- **WHEN** same logical operation is resent safely
- **THEN** repeat iteration count remains unchanged

### Requirement: Wait и schedule имеют durable correlation

Wait MUST register expected signal/timer before producer effect, validate source
identity/schema/correlation and deduplicate delivery. Early, late, cancelled and
rate/byte/TTL constrained events follow declared contract; assisted mode does
not promise wakeup without host.

#### Scenario: Late callback приходит после cancel

- **WHEN** wait уже closed
- **THEN** callback does not reopen Run

### Requirement: Compensation сохраняет original effect history

Compensation MUST be finite typed child work with scoped context, preconditions,
current rights, evidence and budgets. It relates to original operation but does
not delete receipt or invent rollback for unsupported target.

#### Scenario: Compensation не может быть выполнена

- **WHEN** precondition or right is absent
- **THEN** residual effect remains visible

### Requirement: Admission и retry имеют разные identities

ExecutionAdmission, per-action Admission, delivery retry, whole-step retry and
semantic rework MUST have distinct identities and preconditions. A worker has
no arbitrary tool authority; current stop/revocation/budget applies to every
new dispatch.

#### Scenario: Whole-step retry меняет model

- **WHEN** retry would use another provider or model
- **THEN** it requires compatible new revision, not old retry identity

### Requirement: Delivery status не равен effect status

Protocol MUST distinguish preparation, dispatch, response and observation from
`not_started`, `not_applied`, `applied`, `partially_applied` and `unknown`
effect status. Async acknowledgement may retain pending null outcome; final
receipt cannot silently convert unknown to success.

#### Scenario: HTTP accepts operation asynchronously

- **WHEN** adapter returns accepted response without terminal observation
- **THEN** effect remains pending rather than applied

### Requirement: CAS, dedup и stop имеют отдельные semantics

Normal mutation MUST use scoped expected version and principal-aware command
dedup. Relevant input/attempt/resource checks protect parallel results; stale
UI may monotonically restrict through stop, but only exact release and later
resume can remove it.

#### Scenario: Новый stop появился после release

- **WHEN** resume sees applicable current stop
- **THEN** handler refuses continuation

### Requirement: Approval учитывает current authority при consume и dispatch

Approval/Grant MUST bind exact intent, actor, policy, scope, deadlines and
constraints. Consume is atomic with Admission; safe delivery retry does not
reconsume it, but dispatch rechecks stop/revocation/host permission. Redacted
derived output keeps its own provenance.

#### Scenario: Arguments approval изменены

- **WHEN** target, account or protected parameter changes
- **THEN** previous approval cannot be reused

### Requirement: Result intake проверяет exact Attempt и sealed output

SubmitResult MUST verify active identities, envelope digest, ports, seals,
evidence, receipts, relevant claims and current controls. Progress/heartbeat is
not terminal result; late result is evidence with rejection reason, not input
for another Attempt. Отказ intake MUST быть named refusal, называющий предмет:
output port, который обязателен для reported verdict и не отчитан либо не
объявлен step, или identity/digest выхода, не совпавшие с admitted slot.
Для assisted submission эти проверки и result schema шага MUST выполняться
при intake до записи candidate: отклонённая отправка оставляет handoff
awaiting и MUST NOT создавать failed Attempt или terminal Run. Sealing выходов
остаётся частью acceptance.

#### Scenario: Output изменён после hash

- **WHEN** worker submits modified bytes under prior digest
- **THEN** acceptance rejects result

#### Scenario: Обязательный выход не отчитан

- **WHEN** submitted StepResult не содержит output port, обязательный для его
  verdict
- **THEN** intake отказывает stable code, pointer и message называют этот
  port, а Run не меняется

#### Scenario: Неверная отправка не сжигает Attempt

- **WHEN** assisted host присылает StepResult, который не прошёл бы acceptance
  по result schema, портам, identity или digest
- **THEN** submission отклоняется до записи candidate, handoff остаётся
  awaiting, и host может прислать исправленный отчёт под той же envelope

### Requirement: CLI exposes scoped commands without hidden mutation

CLI MUST provide typed installation, authoring, preview/start, observation,
control, correction, decision and export commands through safe JSON/file input.
Read-only `next`, explain and events do not dispatch; init/install/remove and
project-wide control use their own scopes, not Run CAS.

Каждая объявленная операция MUST открывать authority в режиме,
соответствующем её фактическому эффекту: операция, создающая или меняющая
authority state, MUST открываться на запись, а операция только чтения — в
read-only. Состав таких операций — список фактов рядом с их обработчиками, и
машинная проверка MUST покрывать каждую операцию в обоих направлениях:
мутирующая операция не работает в read-only authority, а read-only не
держит writer.

Форма вызова MUST быть читаема из самого инструмента и MUST NOT требовать
authority: запрос справки у любой команды, запрос версии и перечисление
доступных публичных контрактов MUST отвечать своей информацией, а не отказом
про ненайденный объект. Запрос справки по теме MUST возвращать только эту тему.
Имя публичного контракта MUST приниматься и в форме declared reference, под
которой он назван в handed задании; этот ответ MUST оставаться функцией самого
binary. Компонент установленного trusted package MUST читаться по своему
declared ID командой чтения package, без обращения к файлам внутри authority.
Один contract MUST читаться без остального bundle. Установленный путь к binary MUST меняться
объявленной командой, а не ручной правкой machine-only файла.

#### Scenario: User calls next for blocked child

- **WHEN** child cancellation holds caller waiting
- **THEN** CLI reports scoped blocked state without starting a worker

#### Scenario: Пользователь запрашивает форму подкоманды

- **WHEN** к любой подкоманде передан запрос справки
- **THEN** CLI печатает её строку использования и не открывает authority

#### Scenario: Исполнителю нужен список контрактов

- **WHEN** `schema` вызван без имени
- **THEN** CLI перечисляет доступные имена контрактов вместо требования
  назвать точное имя

#### Scenario: Задание называет контракт declared reference-ом

- **WHEN** имя контракта запрошено в той форме, в которой оно названо в
  задании
- **THEN** CLI отдаёт тот же контракт, что и по его имени

#### Scenario: Автору нужна форма authoring-документа

- **WHEN** автор ищет форму YAML-документа, который он пишет сам
- **THEN** её имя перечислено рядом с контрактами обмена и отдаётся той же
  командой

#### Scenario: Оператор заполняет выход по schema пакета

- **WHEN** запрошен компонент установленного trusted package по его declared
  ID
- **THEN** команда чтения package отдаёт его bytes, а команда контрактов
  по-прежнему отвечает только за то, что несёт binary

#### Scenario: Нужна одна форма из большого bundle

- **WHEN** запрошен один contract из bundle
- **THEN** CLI отдаёт только его определение и его закрытие

#### Scenario: Отказ называет неприменимую команду

- **WHEN** проверка графа запрошена для пути вне authority
- **THEN** отказ называет команду, которой проверяется авторская папка без
  создания Run

#### Scenario: Автор называет компонент его полным идентификатором

- **WHEN** расширение ссылается на компонент именем, которого нет среди
  объявленных
- **THEN** отказ перечисляет известные имена, а не только отвергнутое

#### Scenario: Подготовка стадии отказывает до admission

- **WHEN** подготовка стадии отказывает refusal-ом со stable code
- **THEN** diagnostic несёт этот code, а не только фазу подготовки

#### Scenario: Запечатанный package не разрешается при запуске

- **WHEN** launch не находит только что запечатанный package среди доверенных
- **THEN** отказ называет его identity и причину, а не сообщает о ненайденном
  файле

#### Scenario: Множественный claim выполняется

- **WHEN** владелец запрашивает атомарный набор из нескольких repository
  claims объявленной командой
- **THEN** authority открыта на запись, набор создаётся целиком или
  отказывает целиком, и reject read-only открытия называет режим, а не
  ненайденный объект

#### Scenario: Проверка режима покрывает всю поверхность

- **WHEN** машинная проверка режима открытия перебирает каждую объявленную
  операцию
- **THEN** каждая мутирующая операция присутствует в списке записи, и
  отсутствие операции в нём — отказ проверки, а не read-only выполнение

### Requirement: CLI объявляет результат explicit binary update
Public CLI MUST предоставлять `prifly update` как отдельную команду без
`--project` prerequisite. Её structured result MUST различать current version,
installed version и отказ; update не смешивается с command protocol Run и не
создаёт authority mutation. Invalid arguments MUST завершаться existing
`invalid_usage` diagnostic.

#### Scenario: Managed installation получает новую версию
- **WHEN** пользователь запускает `prifly update` и signed compatible Release
  новее installed version
- **THEN** CLI сообщает прежнюю и установленную version только после успешной
  atomic replacement

#### Scenario: Вызов содержит неожиданный аргумент
- **WHEN** пользователь передаёт не поддержанный аргумент команде `update`
- **THEN** CLI возвращает `invalid_usage` и не начинает network operation

### Requirement: Executor interface has no general state.write

Executor MUST receive only bounded attempt/operation DTOs for prepare, propose,
admit, report, result, signal and liveness. It MUST NOT select successor, grant
approval, edit workflow or write authority state; missing host capability MUST
be reported as unsupported rather than prose substitution.

#### Scenario: Helper prints an instruction

- **WHEN** helper cannot provide dispatch receipt
- **THEN** protocol treats it as preview, not started execution

### Requirement: Problem и exit code сохраняют safe meaning

Problem MUST include stable code, message, correlation, violations and safe
next action without secrets or foreign-object detail. `retryable` describes
command/check retry only. CLI exit zero means read or command commit, not Run
success; typed result carries workflow state. Runtime refusal, поднятый со
stable code, MUST доходить до клиента под этим code независимо от того,
сопровождён ли он message; engine-authored detail такого отказа (port, path,
version) MUST сообщаться в `violations`. Только текст без stable code MUST
схлопываться в `invalid_input`, и такой текст MUST NOT попадать в ответ.

Stable code MUST жить в поле `code`, а не внутри собственного предложения
отказа. Отказ, чей код читается только разбором `message`, MUST считаться
отказом без stable code: он называет один и тот же код для всей поверхности и
лишает читателя различения, ради которого код объявлен. Машинная проверка
MUST покрывать каждый конструктор ошибок, которым отказы создаются, а не
выбранное подмножество.

Exit class MUST следовать смыслу отказа, а не написанию его имени. Отказ,
означающий, что два объявления автора не сходятся между собой, относится к
классу формы и входных данных; класс состояния authority MUST оставаться за
отказами, где не совпали version, epoch, claim, slot, admission или access.

Отказ MUST различать классы отсутствия: отсутствующая authority по выбранному
пути, отсутствующий объект внутри существующей authority и существующий объект
без запрошенного состояния MUST иметь разные stable codes. Отказ MUST NOT
утверждать отсутствие объекта, который движок держит. Usage refusal
глобального аргумента MUST повторять полученное значение, чтобы обрезанный
shell-ом путь отличался от дефекта инструмента.

#### Scenario: External effect is unknown

- **WHEN** command reports unknown effect
- **THEN** safe next action is exact reconciliation, not blind retry

#### Scenario: Refusal поднят без сопроводительного message

- **WHEN** runtime отказывает stable code без detail
- **THEN** Problem несёт этот code, а не `invalid_input`

#### Scenario: Refusal несёт engine-authored detail

- **WHEN** runtime отказывает stable code с detail о предмете отказа
- **THEN** Problem несёт этот code и detail в `violations`, без raw parser
  input, argv, environment или foreign payload

#### Scenario: Выбранный путь не содержит authority

- **WHEN** команда выполняется с `--project`, указывающим на каталог без
  authority
- **THEN** отказ называет отсутствие authority по этому пути и отличается от
  отказа про ненайденный Run, definition или artifact

#### Scenario: Run существует, но передачи нет

- **WHEN** host запрашивает удерживаемую передачу Run, который существует и
  не держит ни одной
- **THEN** отказ называет отсутствие активной передачи и предлагает чтение
  состояния и drive, а не поиск Run

#### Scenario: Аргумент обрезан вызывающей стороной

- **WHEN** глобальный аргумент получен в непригодной форме
- **THEN** usage refusal показывает полученное значение

#### Scenario: Отказ проектной поверхности называет свой код

- **WHEN** объявленная операция проекта отказывает по объявленной причине
- **THEN** `code` несёт именно этот код, `message` несёт причину без него, и
  машинная проверка конструкторов отказа валит сборку, если код снова оказался
  внутри текста

#### Scenario: Расхождение двух объявлений не выдаётся за состояние authority

- **WHEN** отказ означает, что две строки авторского объявления противоречат
  друг другу, и его имя содержит слово, которым назван класс состояния
- **THEN** exit class остаётся классом формы и входных данных

### Requirement: Инструмент описывает свои коды возврата

CLI MUST описывать коды возврата собственными средствами: перечень, значение
каждого и то, какие из них означают исправное ожидание, а какие отказ.
Автономный драйвер, читающий только код завершения, MUST мочь отличить
«работа передана и прогон ждёт исполнителя» от «прогон отказал», не разбирая
текст вывода.

Код, возвращаемый в исправном ходе работы, MUST быть описан наравне с кодами
отказов: необъяснённый ненулевой код на здоровом прогоне читается как поломка.

#### Scenario: Скрипт читает код завершения

- **WHEN** автономный драйвер получает ненулевой код после команды, которая
  отработала штатно
- **THEN** он находит значение этого кода средствами самого инструмента и
  отличает ожидание от отказа, не полагаясь на разбор текста

### Requirement: Отказ по форме команды называет недостающую часть

Отказ, вызванный формой вызова, MUST называть, чего не хватает или что лишнее,
и показывать принимаемую форму. Общая формулировка о неожиданных аргументах без
указания недостающего флага MUST NOT оставаться единственным содержанием
отказа.

Предлагаемые следующие действия MUST относиться к этому отказу: команды
диагностики состояния MUST NOT предлагаться там, где ошибка в форме вызова.

#### Scenario: Пользователь передал идентификатор позиционно

- **WHEN** он вызывает команду, ожидающую значение под флагом, и передаёт его
  как позиционный аргумент
- **THEN** отказ называет нужный флаг и показывает принимаемую форму, а не
  сообщает лишь о неожиданных аргументах

### Requirement: Preview и validation не создают effect

Preview MUST resolve declared refs and display graph, potential effects/resources,
gates and limits without starting code, paid provider, message or approval.
Validation MUST distinguish shape, refs, graph, capability, current authorization
and executability; simulation MUST be fixture-only and not qualification.

#### Scenario: Preview needs external read

- **WHEN** preview would fetch external data
- **THEN** it requires separate declared admission

### Requirement: Extension changes semantics only versionedly

New package/step may use existing protocol, but predicate, stage kind or
authorization primitive MUST receive versioned semantics, compatibility decision
and conformance tests. Unknown retained bytes can export but unsupported reader
does not execute them.

#### Scenario: Package hides control flow in metadata

- **WHEN** extension adds undocumented router behavior
- **THEN** validation rejects it as unsupported semantics

### Requirement: Protocol delivery distinguishes schema evidence from qualification

Delivery MUST include versioned schemas, valid/invalid fixtures, command
inventory and checked links/limits. Shape tests do not prove filesystem,
process, remote effect, human identity or full runtime qualification; report
states this boundary explicitly.

#### Scenario: Schema validates a remote effect DTO

- **WHEN** fixture passes JSON Schema
- **THEN** report does not claim remote effect was executed or authorized

### Requirement: CLI запускает declared Project workflow с explicit workspace mode
CLI SHALL предоставлять один `project start` для declared launch. В profile
`/3` путь проекта MUST не подразумевать Git; нужны только declared typed inputs,
а RunBrief MUST требоваться лишь как объявленный вход. Host MUST требоваться
для assisted launch/host-bound source, Git — для заявленной Git Workspace.
Без Git работы workspace mode MUST не запрашиваться; её результат MUST
явно отличаться от worktree/checkout. При Git-записи `/3` MUST требовать explicit
`worktree` или `checkout`, без неявного изменения текущего checkout. `/2` MUST
сохранять прежний default `worktree`. Invalid launch, host, inputs, bindings и
workspace MUST давать stable diagnostic до registration, claim или Run.
Ответ MUST называть Run и фактически используемые ресурсы без фиктивного claim.

#### Scenario: CLI starts default isolated workspace
- **WHEN** пользователь запускает valid `/2` launch без workspace flag
- **THEN** результат сообщает isolated worktree и Run identity

#### Scenario: CLI rejects an unknown workspace mode
- **WHEN** пользователь передаёт несуществующий workspace mode
- **THEN** CLI возвращает typed отказ без package, claim или Run

#### Scenario: Managed launch имеет только файловый вход
- **WHEN** `/3` launch получает declared input file и разрешённые executable bindings
- **THEN** он возвращает Run без требования host, brief или Git

### Requirement: Assisted handoff сообщает versioned declared Workspace tree bindings

Versioned assisted SessionTask MUST сообщать host finite declared Workspace tree
bindings: manifest input/output port names, typed capture policy, expected input
manifest ArtifactRef при его наличии и permitted typed location form for
output-only creation. Materialize-only binding MUST сообщаться без output port
и с declared location materialized entries; guide рядом с манифестом
(`workspace-tree-guide/2`) MUST говорить, что такой port host не объявляет.
Read-only step с materialize-only binding MUST получать `repository_workspace`
и `workspace_mode` той же WorktreeClaim, что и workspace-write step Run. Host MUST не получать authority handle, artifact-store
path или право выбирать другой Workspace path. Сопоставленный
SessionSubmission MUST использовать version того SessionTask, который был
handed Attempt; старые retained session versions остаются читаемыми и не
получают новую tree-binding семантику.

Runtime MUST capture declared output trees itself before accepting terminal
StepResult при каждой submission шага с declared bindings, независимо от
поддерживающей деревья session version и от наличия `workspace_trees` в
отправке. Host MUST называть capture location только там, где выбирает её
сам: для policy с единственным допустимым значением (`exact_file`) runtime
MUST брать declared path, а отсутствие location MUST NOT быть отказом. Host
MAY report only selected output-only capture location и MAY
повторить declared input location для binding, объявленного и входом, и
выходом, так что форма отправки одинакова для обоих видов binding; путь,
отличный от declared input location, MUST отклоняться named refusal. Host
MUST not подменять WorkspaceTreeManifest, contained ArtifactRef, digest или
capture policy prose-строкой либо arbitrary JSON. Unknown binding field,
version или несовпадение handoff/submission MUST отклоняться до изменения Run.

#### Scenario: Host получает зафиксированную форму Ultra bundle
- **WHEN** assisted workspace-write Attempt имеет output-only direct-child tree
  binding
- **THEN** SessionTask показывает declared parent and typed bundle form, а host
  не может заявить ArtifactRef, entry outside parent или другой output policy
  в result

#### Scenario: Host повторяет declared input location
- **WHEN** binding объявляет одно дерево и входом, и выходом, а submission
  называет для его output port путь, равный declared input location
- **THEN** runtime принимает отправку как при output-only binding и capture-ит
  дерево по declared location

#### Scenario: Host называет другой путь для input binding
- **WHEN** submission называет для такого port путь, отличный от declared
  input location
- **THEN** intake отказывает named refusal до изменения Run

#### Scenario: Exact-file binding без названной location
- **WHEN** submission не называет location для output-only binding с capture
  policy `exact_file`
- **THEN** runtime capture-ит declared path и принимает отправку, не требуя
  повторить единственное допустимое значение

#### Scenario: Submission без workspace_trees для input+output binding
- **WHEN** submission поддерживающей деревья версии не содержит
  `workspace_trees`, а step объявляет binding с входом и выходом
- **THEN** runtime capture-ит дерево по declared input location и заполняет
  output port сам, не требуя от host повторить путь

#### Scenario: Read-only step получает materialized план и путь рабочей копии
- **WHEN** assisted step с `effects.class: none` объявляет materialize-only
  binding
- **THEN** SessionTask называет `repository_workspace`, binding без output port
  и location materialized entries, а submission, объявляющая порт этого
  binding'а, отказывается named refusal до изменения Run

### Requirement: Project launch принимает typed per-Run decision selection
`project start` MUST accept an explicit package-profile selection, typed
answers for declared preflight decision IDs, and, under a separate flag so the
phase is readable in the command itself, typed answers for declared runtime
decision IDs. Interactive host launch MUST
ask for an omitted required selection before compilation; non-interactive
launch MUST return a stable missing-decision diagnostic unless a sealed default
rule fills it. Structured launch result MUST name selected profile, decision
catalog digest and decision ledger reference. Под autonomous policy тот же
результат MUST дополнительно нести перечень применимых runtime-решений, которые
эта политика взять не сможет, с ID и причиной каждого; перечень MUST
присутствовать и пустым, чтобы его отсутствие не читалось как «нечего
сообщать».

#### Scenario: CLI запускает Ultra без изменения проекта
- **WHEN** developer supplies declared `ultra` package-profile to `project start`
- **THEN** CLI creates an Ultra Run and leaves all tracked workflow files
  unchanged

#### Scenario: Non-interactive launch не имеет обязательного выбора
- **WHEN** required preflight decision has neither explicit answer nor allowed
  default
- **THEN** CLI returns a stable diagnostic before compilation and Run creation

#### Scenario: Анкета устарела до запуска
- **WHEN** host передаёт catalog digest из questionnaire, а tracked catalog
  изменился до `project start`
- **THEN** CLI returns `project_start_stale_decision_catalog` before package,
  Workspace claim or Run creation

#### Scenario: Autonomous launch называет решения, которых политика не возьмёт
- **WHEN** владелец запускает Run под autonomous policy, а catalog объявляет
  применимое runtime-решение без разрешённого automatic selection
- **THEN** typed результат launch содержит его ID и причину, а Run создаётся

#### Scenario: Владелец предответил runtime-решение
- **WHEN** он передаёт `project start` typed ответ на объявленный runtime
  decision ID
- **THEN** CLI проверяет значение до создания Run, запечатывает его в decision
  sheet и не называет это решение среди тех, которых политика не возьмёт

### Requirement: CLI передаёт lifecycle runtime-решения versionedly
Public CLI MUST expose typed request, read and answer operations for a pending
decision. Compatible executor's request command MUST require its current
Attempt, envelope, declared decision ID and Run generation; CLI derives the
definition digest only from the sealed catalog, never from executor prose.
Answer command MUST require Run ID, decision ID, request/version identity and
typed value; it MUST report accepted, stale, conflict, schema-invalid or
not-pending result without hidden dispatch. Read output MUST be safe to render
by every supported host and contain no secret answer bytes outside authorized
scope.

#### Scenario: Пользователь отвечает из другого host
- **WHEN** second authorized host reads a pending decision and submits its
  current typed answer
- **THEN** CLI accepts it once and the first host can observe the same ledger
  transition after reconnect

#### Scenario: Совместимый adapter отправляет declared request
- **WHEN** adapter вызывает `run decision RUN_ID request` с identity текущего
  SessionTask и declared runtime ID
- **THEN** CLI передаёт ровно sealed definition в Universal Decision Bridge и
  не принимает digest либо вопрос, придуманный adapter-ом

### Requirement: CLI управляет Project workflow folders из репозиториев явными командами
`prifly project workflows` без аргументов MUST по-прежнему перечислять
declared launches. Дополнительно CLI MUST предоставлять
`search [QUERY] [--category ID] [--catalog URL]`,
`add SOURCE [--ref REF] [--path DIR] [--name NAME] [--catalog URL]`,
`update NAME [--ref REF]` и `remove NAME`; `add`, `update` и `remove`
принимают общий `--repository DIR`, `search` не требует repository, и ни одна
из них не требует `--project` и не открывает authority. `SOURCE` MUST толковаться
механически: имя без `/` и `:` — запись каталога; `owner/repo` — GitHub HTTPS
repository; иначе Git URL или абсолютный локальный путь. Опущенный
`--catalog` MUST использовать встроенный официальный каталог
`https://github.com/StenHigh/prifly-workflows.git`; явный `--catalog URL`
переопределяет его для одной команды и MUST проходить те же проверки, что и
`SOURCE`. Сеть MAY выполняться только во время `search`, `add` и
`update`; `init`, `workflows`, `questionnaire`, `compile` и `start` MUST NOT
её использовать. Результаты MUST быть typed JSON с `schema_version`
`project-workflow-catalog/1`, `project-workflow-add/1`,
`project-workflow-update/1` и `project-workflow-remove/1`. Неверные
аргументы MUST давать `invalid_usage` до сети; отказы MUST использовать
stable codes, среди них `project_workflow_source_invalid`,
`project_workflow_repository_unreachable`,
`project_workflow_repository_empty`,
`project_workflow_repository_ambiguous`, `project_workflow_exists`,
`project_workflow_package_conflict`, `project_workflow_origin_missing`,
`project_workflow_modified`, `project_workflow_commit_mismatch`,
`project_workflow_catalog_invalid` и `project_workflow_catalog_entry_unknown`.

#### Scenario: Сценарий установлен по имени каталога
- **WHEN** пользователь вызывает `add NAME --catalog URL`
- **THEN** ответ `project-workflow-add/1` называет package identity, origin и
  launch, а authority не открывается

#### Scenario: Repository неоднозначен
- **WHEN** repository содержит несколько сценариев и `--path` не задан
- **THEN** CLI возвращает `project_workflow_repository_ambiguous` с перечнем
  путей и не меняет `.prifly/`

#### Scenario: Неверный SOURCE
- **WHEN** пользователь передаёт относительный путь, URL с credentials или
  аргумент с ведущим `-`
- **THEN** CLI возвращает stable diagnostic до любой сетевой операции

### Requirement: Host runner предлагает поиск и установку сценария одним вопросом
Runner `prifly-run` MUST содержать инструкции: по явной просьбе разработчика
найти или установить сценарий выполнить `project workflows search --json`,
показать категории и записи одним native finite вопросом, после выбора
вызвать `project workflows add NAME` и предложить разработчику проверить и
закоммитить изменения `.prifly`. Runner MUST NOT устанавливать сценарий без
явного выбора и MUST NOT начинать Run как часть установки.
`project runners update` MUST распознавать прежний exact runner и заменять
его; кастомизированный runner по-прежнему отказывается.

#### Scenario: Разработчик просит установить сценарий
- **WHEN** host получает просьбу найти или установить workflow
- **THEN** он показывает список из `search --json` одним вопросом и вызывает
  `add` только для выбранной записи

#### Scenario: Runner обновлён после изменения
- **WHEN** repository содержит exact runner предыдущей версии
- **THEN** `project runners update` заменяет его новым, не трогая
  кастомизированные файлы

#### Scenario: Постоянный workspace и предстартовая программа запуска
- **WHEN** `project.yaml` объявляет у launch `workspace: worktree|checkout`
  и/или `preflight: {executable, args, timeout_ms}`
- **THEN** `project start` без `--workspace` берёт постоянный выбор там, где
  workflow требует рабочую копию (и молча не применяет там, где флаг был бы
  отказан как лишний), анкета называет его в `workspace`, host не спрашивает;
  предстартовая программа (бинарь из `local.yaml`, без `--allow-execution`)
  исполняется в корне repository до компиляции и любого захвата, ненулевой
  код или таймаут — отказ `project_start_preflight_failed` /
  `project_start_preflight_timeout` с хвостом вывода; `--prepare` её не
  исполняет

#### Scenario: Программа шага получает рабочую копию и окружение машины
- **WHEN** Run держит ровно одну активную claim и шаг с `operation: process`
  запускается, а `local.yaml` содержит `environment` (задано `project local
  set --env NAME=VALUE`; имена `PRIFLY_*` — отказ)
- **THEN** программа получает `PRIFLY_REPOSITORY_WORKSPACE` и
  `PRIFLY_CLAIM_ID` рядом с `PRIFLY_SOCKET`/`PRIFLY_CONTEXT_FILE`, окружение
  машины поверх чистого (`context.json` не меняется), `--prepare` показывает
  окружение у каждой программы и учитывает его в `configuration_digest`; шаг
  без `effects.class: workspace_write` под состоянием с проверкой эффектов
  измеряется по метке рабочей копии до и после программы — изменившееся
  дерево завершает попытку `effect_not_permitted` с именами путей

#### Scenario: Проектная надстройка над runner
- **WHEN** рядом с сгенерированным `prifly-run/SKILL.md` лежит `PROJECT.md`
- **THEN** сгенерированный текст в первых строках велит прочитать его после
  себя, если он существует, и объявляет, что при противоречии `PROJECT.md`
  сильнее, а границы движка (эффекты, рабочие копии, слоты) им не
  расширяются; `project runners update` заменяет только `SKILL.md`, и
  `PROJECT.md` остаётся байт-в-байт прежним

#### Scenario: Объявленный host без runner в этом clone
- **WHEN** профиль объявляет host, чей `prifly-run/SKILL.md` в repository
  отсутствует, и хотя бы один другой объявленный runner присутствует
- **THEN** `project runners update` обновляет присутствующие runner'ы и
  перечисляет отсутствующие в `missing_hosts`, ничего для них не создавая;
  профиль без единого runner по-прежнему получает `project_runner_missing`

### Requirement: Handoff описывает, что требуется от host

Sealed handoff MUST быть самодостаточным описанием ожидаемого от host: каждая
закреплённая запись контекста MUST быть идентифицируема из самого bundle, без
опоры на порядок перечисления, а выходные слоты MUST быть разделены на те,
которые заполняет host, и те, которые движок закрывает сам объявленным
захватом. Host MUST NOT восстанавливать эту раскладку из содержимого файлов или
из прошлых прогонов. Рабочая копия, которую держит Run, MUST называться так,
чтобы её не пришлось искать: путь в ответе запуска и в задаче MUST быть
абсолютным либо MUST нести имя корня, относительно которого он записан. Host
MUST NOT определять её перебором каталогов или средствами Git.

Форма, в которой host публикует выход, MUST быть записанной, а не свойством,
выводимым из устройства хранилища: инструкции сгенерированного host runner и
authoring reference шага MUST называть, куда пишутся bytes слота и какие поля
несёт соответствующая запись reported result. Host MUST NOT выводить эту форму
из content-addressed storage, чужого прогона или чужой попытки. Baseline
StepResult schema MUST оставаться byte-identical: её digest закреплён в
sealed packages, поэтому аннотация в ней разорвала бы уже подписанные
identity.

#### Scenario: Bundle содержит несколько закреплённых записей контекста

- **WHEN** шагу закреплены skill и его bridge
- **THEN** host определяет, что есть что, по самому bundle, а не по порядку
  ссылок

#### Scenario: Часть выходов закрывается захватом

- **WHEN** шаг объявляет и обычный выход, и выход с привязкой workspace tree
- **THEN** handoff называет, какой слот host заполняет сам, а какой движок
  закрывает захватом

#### Scenario: Host впервые публикует не-древесный выход

- **WHEN** host заполняет слот, который движок не закрывает захватом
- **THEN** форма публикации читается из published contract, без вывода её из
  устройства artifact storage

#### Scenario: Хост ищет рабочую копию захода
- **WHEN** запуск создал claim и вернул его путь
- **THEN** путь читается однозначно: он абсолютный или сопровождён именем
  корня, от которого записан, и хост не обращается к `git worktree list`

#### Scenario: Хост перечисляет выданные задания
- **WHEN** хост запрашивает все выданные задания Run одной командой
- **THEN** каждая перечисленная попытка вручена так же, как при запросе одной:
  документ задачи лежит в её рабочей папке, и справка не обещает файла там,
  где его не будет

#### Scenario: Хост ведёт Run, но не исполняет программу сам
- **WHEN** следующая работа Run — шаг, исполняемый программой, возможно за
  одной или несколькими управляющими стадиями, и хост ведёт Run с объявленной
  границей
- **THEN** драйвер проходит управляющие стадии и вручения, останавливается до
  запуска программы, возвращает управление и называет предстоящую работу; без
  объявленной границы поведение прежнее — программа исполняется внутри вызова

### Requirement: Read-only виды не занижают то, что движок держит

Read-only вид MUST называть то, что считает, и MUST NOT показывать нулевое
значение для состояния, которое движок держит. Сводка Run MUST различать
выходы Run и запечатанные выходы его шагов. Verdict принятого шага MUST быть
виден из сводки в её обычной форме, а не только в машинной: чтение authority
storage напрямую MUST NOT быть единственным способом узнать исход собственной
работы. Ожидание host MUST быть видно из read-only вида как безопасное
следующее действие. Команда обновления MUST называть адрес, по которому
проверяла release.

#### Scenario: Выход шага запечатан, выходов Run ещё нет

- **WHEN** шаг запечатал выход, а Run ещё не завершил ни одной стадии с
  выходом
- **THEN** сводка не показывает состояние как «ничего не запечатано»

#### Scenario: Задание держит host

- **WHEN** assisted attempt ожидает отчёта host
- **THEN** read-only вид называет чтение задания среди безопасных следующих
  действий, а не только состояние и события

#### Scenario: Обновлений нет

- **WHEN** установленная версия совпадает с последним stable release
- **THEN** ответ называет адрес, по которому проверялся release

#### Scenario: Исполнитель проверяет исход своей работы

- **WHEN** результат шага принят
- **THEN** verdict читается из обычной сводки Run, без machine-readable флага
  и без чтения authority storage

### Requirement: Анкета объясняет запуск до первого эффекта
`project questionnaire` SHALL давать read-only представление selected profile,
applicable preflight и runtime decisions, typed предответов и политики участия.
Он MUST показывать причины потенциального ожидания до первого dispatch, не
извлекать вопросы из текста skills и не объявлять все runtime decisions
обязательными. CLI и host MUST использовать одну validation и проверку stale
catalog. Изменение исходников или ответов MUST требовать пересчёта итогов до
исполнения. Final launch result сохраняет ledger и known unanswered summary.

#### Scenario: Пользователь собирается отойти
- **WHEN** он готовит autonomous launch с applicable runtime-решением без
  разрешённого automatic selection
- **THEN** до запуска он видит вопрос, возможность предответа и причину
  возможного ожидания; сама анкета не создаёт package, claim, Run или worker

### Requirement: Анкета называет решение одним именем во всех списках
Ответ `project questionnaire` SHALL содержать три списка объявленных решений:
`preflight` и `runtime` перечисляют только применимые к выбранному package
profile решения своей фазы, а `decision_states` — все объявленные решения
каталога с применимостью, фактом ответа и причиной ожидания. Идентификатор
решения во всех трёх списках MUST называться `id` и совпадать с `id`
объявленного решения; ни один список MUST NOT называть его иначе. То же
правило действует для `decision_states` внутри launch summary. Изменение
формы этих ответов MUST повышать версию контракта; прежние версии остаются
историческими и не переиспользуются.

#### Scenario: Клиент читает идентификаторы одним выражением
- **WHEN** JSON-клиент берёт `id` из `preflight`, `runtime` и
  `decision_states` одного ответа анкеты
- **THEN** он получает непустой идентификатор в каждой записи всех трёх
  списков, а не тихий `null` из-за другого имени поля

#### Scenario: Поле переименовано
- **WHEN** имя или состав поля в этих списках меняется
- **THEN** ответ выходит под новой версией контракта
  (`project-questionnaire/4`, `project-launch-summary/3`), а прежняя версия
  не переиспользуется под новой формой

### Requirement: CLI предоставляет явную резолюцию uncertain obligation

Public CLI MUST предоставлять `prifly run resolve RUN_ID (--attempt ID |
--check ID) --outcome not_applied|applied --reason TEXT [--command-id ID]`.
Команда MUST принимать только uncertain attempt или check, MUST требовать
reason и явный outcome, MUST возвращать typed receipt через обычный command
protocol и MUST отказывать с `driver_live`, пока driver этого Run активен.
Её результат не является успехом workflow: `next` после резолюции показывает
освобождённый slot и terminal или следующее honest состояние scope. `run
cancel`, `run resume` и `run drive` MUST NOT выполнять резолюцию неявно.

#### Scenario: Владелец разрешает uncertain attempt
- **WHEN** пользователь вызывает `run resolve` с outcome и reason для
  uncertain attempt без живого driver
- **THEN** CLI возвращает receipt, `capacity show` больше не показывает slot
  этого attempt, а `run next` не предлагает retry

#### Scenario: Резолюция без outcome
- **WHEN** пользователь не указывает `--outcome` или `--reason`
- **THEN** CLI возвращает `invalid_usage` и не меняет authority

### Requirement: Пользователь видит причины вопроса и временные ограничения
Questionnaire и launch summary нового timed contract MUST показывать
конечный рабочий срок отдельно от срока ожидания, называя отсутствие последнего
«без ограничения времени». Host MUST объяснять контекст объявленного вопроса
и последствия вариантов по закреплённым данным, не выдумывая рекомендацию
или новый scope. Уже выбранное пользователем значение MUST не переспрашиваться.

После ответа CLI/host MUST различать «ответ сохранён» и «работу можно
продолжить». При capacity, control, expired delivery или resource recovery
представление MUST называть конкретное препятствие и допустимую команду,
не предлагать повторный ответ или молчаливый новый Run. Временные правила
legacy Run MUST быть показаны как legacy, не как новый default.
Нормативный source set — `openspec/specs/cli-protocol/`.

#### Scenario: Человек возвращается через несколько дней
- **WHEN** он читает ожидающий вопрос без срока и отвечает
- **THEN** host объясняет задачу и затем сообщает либо разрешённое продолжение,
  либо конкретную причину ожидания при уже сохранённом ответе

#### Scenario: Для продолжения нужен recovery владения
- **WHEN** ответ записан, но прежний claim нельзя подтвердить автоматически
- **THEN** CLI показывает именно это ограничение без запуска в другом checkout
  и без объявления ответа потерянным

### Requirement: Отказ во владении рабочей копией называет путь восстановления
Отказ, вызванный состоянием владения claimed рабочей копией, MUST называть
действие, которым владелец выходит из этого состояния, а не только то, что
именно не сделано. Это относится к отказу в допуске по истёкшему lease, к
отказу в продлении и к отказу в освобождении неулаженного Run.

Команда продления lease MUST быть исполнима владельцем из обычного вызова CLI.
Команда, которая по построению отказывает любому вызову, MUST быть либо
исправлена, либо удалена: неисполнимая команда хуже отсутствующей, потому что
выглядит путём восстановления и им не является.

#### Scenario: Владелец упирается в истёкший lease
- **WHEN** он получает отказ в допуске работы по владению рабочей копией
- **THEN** текст отказа называет, чем это лечится, и владелец не ищет выход
  перебором команд

#### Scenario: Продление вызвано из новой команды
- **WHEN** владелец продлевает lease обычным вызовом CLI, а не из процесса,
  создавшего claim
- **THEN** продление проходит; отказ по идентификатору процесса или сессии
  часов MUST NOT возвращаться

### Requirement: Срок в задании исполнителя проверяем получателем
Задание, выдаваемое ассистируемому исполнителю, MUST нести срок в форме,
которую получатель может сверить в своём процессе, либо не нести срока вовсе.
Величина, которую сама система объявляет неквалифицированной, MUST NOT
предъявляться исполнителю как единственное основание для проверки «успеваю ли
я». Если срок передаётся, задание MUST нести и признак доверия к нему.

Отсутствие срока в задании MUST означать «проверяемого срока не дано», а не
«срока нет»: система может продолжать проверять собственный срок при приёме
отчёта. Исполнитель MUST NOT читать пустой срок как разрешение работать
неограниченно.

#### Scenario: Исполнитель проверяет, укладывается ли он
- **WHEN** он читает срок из полученного задания
- **THEN** он либо может сверить его собственными часами, либо видит, что
  система не даёт проверяемого срока, и не строит решение на величине, которой
  сама система не доверяет

#### Scenario: Задание не даёт проверяемого срока
- **WHEN** шаг не объявил ограничений сессии, и система не может дать срок,
  который получатель сверит
- **THEN** задание не несёт срока вовсе, а не несёт неквалифицированную
  величину под видом ограничения

### Requirement: Задание называет то, из чего исполнитель выбирает
Задание, выдаваемое ассистируемому исполнителю, MUST называть вердикты,
маршрутизируемые на этом узле. Исполнитель MUST NOT быть вынужден выбирать из
общего перечня возможных вердиктов, не зная, какие из них объявлены. Перечень
MUST принадлежать узлу: цели переходов и форма графа не раскрываются.

Задание MUST называть действующий срок работы, в том числе когда он взят из
умолчания, а не из объявления шага. Ограничение, известное только коду, MUST
NOT применяться молча.

Отсутствующая величина MUST отсутствовать, а не приходить пустым значением:
пустое поле читается как данное и превращает отказ в тихую пустоту.

#### Scenario: Исполнитель выбирает вердикт
- **WHEN** он завершает работу и выбирает вердикт для отчёта
- **THEN** он видит в задании, какие вердикты этот узел принимает, и не может
  выбрать законный по форме вердикт, который узел не маршрутизирует

#### Scenario: Шаг не объявил ограничений времени
- **WHEN** срок берётся из умолчания движка
- **THEN** задание называет действующий срок, а автор пакета узнаёт об
  ограничении из задания, а не из исходников движка

### Requirement: Текстовое и машинное представление состояния согласованы
Текстовое представление состояния Run MUST называть срок работы, идущей в
данный момент, если его называет машинное представление. Расхождение между
ними MUST NOT существовать: читатель, проверивший одно, делает вывод обо всей
системе.

#### Scenario: Читатель проверяет по текстовому выводу
- **WHEN** он смотрит состояние Run без `--json`
- **THEN** он видит срок действующей работы и не заключает, что срока нет

### Requirement: CLI запускает общий локальный монитор после создания Run
Успешное создание Run через CLI SHALL регистрировать authority и обеспечивать один фоновый monitor текущего пользователя ОС до длительного drive. Это относится к обычному start, project start, fork и созданию через scheduler. Monitor SHALL быть доступен по локальному адресу независимо от браузера и не открывать браузер автоматически. Ошибка регистрации, занятый порт либо невозможность запуска monitor MUST NOT отменять созданный Run или препятствовать его исполнению; CLI сообщает предупреждение отдельно от machine-readable результата. Read-only команды сами не создают Run и не запускают driver. Явный monitor SHALL позволять открыть историю без нового Run и задать дополнительные области обнаружения. Существующий --project остаётся явным источником, а не границей общего списка.

#### Scenario: Два запуска одновременно
- **WHEN** два CLI создают Run одновременно
- **THEN** оба источника регистрируются и доступен один monitor без остановки исполнителей из-за конкуренции за порт

#### Scenario: Порт занят другой программой
- **WHEN** Run создан, но адрес monitor недоступен
- **THEN** Run продолжает обычное исполнение, machine-readable результат сохраняет контракт, а ошибка monitor сообщается отдельно

#### Scenario: Пользователь открывает историю
- **WHEN** пользователь запускает monitor без нового Run
- **THEN** он получает общий обзор сохранившейся истории без запуска либо возобновления исполнителей

### Requirement: Ответ о следующем действии называет род предстоящей работы
Versioned next-action view MUST при `action: stage` называть род работы, которую
драйвер выполнил бы для этой стадии: assisted-шаг, который будет выдан хосту;
программа, которая исполняется внутри вызова драйвера; либо управляющая стадия
без внешней работы. Род MUST выводиться из закреплённого определения шага этой
стадии, а не из имени, текста или догадки; если сборка не может его определить,
поле MUST отсутствовать, а не называть род по умолчанию. Read-only чтение
MUST оставаться read-only: называя программу, оно её не запускает. Прежние
read versions MUST сохранять свою форму без этого поля.

#### Scenario: Хост узнаёт про программу до запуска драйвера
- **WHEN** следующая стадия Run — шаг, исполняемый программой, и хост читает
  ответ о следующем действии
- **THEN** ответ называет `action: stage` и род работы «программа», ничего не
  исполняя, так что хост запускает драйвер тем способом, который выдержит её
  длительность

#### Scenario: Следующая работа — ассистируемый шаг
- **WHEN** следующая стадия — assisted шаг
- **THEN** ответ называет род «ассистируемый шаг», и после вызова драйвера
  тот же ответ несёт `session.task` среди безопасных действий

#### Scenario: Прежний reader читает тот же Run
- **WHEN** Run читается reader-ом read version без этого поля
- **THEN** он получает прежний shape ответа, а сохранённое состояние Run не
  изменено

### Requirement: Ответ о следующем действии называет место остановки Run

Versioned next-action view MUST при `action: terminal` называть место, где Run
остановился: invocation и finish-стадию, которой он завершён, вместе с её
объявленным исходом. Эти факты уже есть в состоянии Run; читателю, который
хочет узнать причину исхода, MUST NOT быть нужно упорядочивать активации
самому.

Ответ MAY дополнительно называть объявленное ребро, которым эта finish-стадия
достигнута: стадию-предшественницу и её вердикт. Ребро MUST выводиться
маршрутизацией запечатанного плана этого Run — той же, которой шёл драйвер, —
а не вторым описанием правил и не порядком записей. Если такой вывод
неоднозначен (к finish-стадии ведёт объявленное ребро более чем от одной
исполненной стадии) либо стадия-предшественница не маршрутизируется вердиктом
шага, обе части ребра MUST отсутствовать. Отсутствие MUST читаться как «этой
сборке ребро не назвать», а не как «ребра не было».

Run, остановленный отказом или отменой, называет причину остановки прежним
способом; это требование MUST NOT подменять его собой.

Новое поле MUST NOT требовать новой версии состояния Run: ответ ничего не
записывает, и форма состояния от него не меняется. Прежние next versions MUST
сохранять свою форму без этого поля.

#### Scenario: Хост узнаёт, где заход свернул к исходу
- **WHEN** Run завершён finish-стадией, и хост читает ответ о следующем
  действии
- **THEN** ответ называет `action: terminal`, invocation, finish-стадию и её
  исход, и хост не открывает ни состояние authority, ни исходник workflow

#### Scenario: Ребро выводится однозначно
- **WHEN** ровно одна исполненная стадия объявляет ребро своим вердиктом в эту
  finish-стадию
- **THEN** ответ называет стадию-предшественницу и её вердикт

#### Scenario: Ребро вывести нельзя
- **WHEN** в эту finish-стадию ведут объявленные рёбра более чем от одной
  исполненной стадии
- **THEN** ответ называет место остановки без ребра, и ни одна из версий не
  выдаётся за состоявшуюся

#### Scenario: Прежний reader читает тот же Run
- **WHEN** Run читается reader-ом next version без этого поля
- **THEN** он получает прежний shape ответа, а сохранённое состояние Run не
  изменено

### Requirement: Обзор перед стартом называет отказ, следующий из его же чисел

Read-only обзор запуска MUST называть отказ, который следует из показанных им
самим величин, тем же сравнением, которым откажет старт. Показать количество и
границу и промолчать о том, что первое превысило второе, — значит оставить
вывод читателю в единственном месте, которое существует, чтобы вывод был
сделан за него.

Названный отказ MUST совпадать со отказом старта по коду и по границе
включительно/исключительно: обзор, объявляющий отказ там, где старт пройдёт,
лжёт ровно так же, как молчащий.

Обзор MUST оставаться read-only: называя отказ, он не отказывает и ничего не
меняет.

#### Scenario: Бюджет определений уже превышен
- **WHEN** количество определений, которое оставит этот запуск, превышает
  границу, и читатель запрашивает обзор
- **THEN** обзор называет `dependency_limit` как отказ, которым ответит старт,
  и сам ничего не отказывает

#### Scenario: Счёт равен границе
- **WHEN** количество равно границе, на которой старт ещё проходит
- **THEN** обзор не называет отказа

### Requirement: Отказ по source продолжения называет допустимый source

Отказ продолжения от Run, который сам является продолжением, MUST называть Run,
с которого продолжение допустимо, когда происхождение это позволяет вычислить.
Происхождение Run записано, поэтому «не тот source» без указания того MUST NOT
быть окончательным ответом.

Обход цепочки происхождения MUST быть ограничен по числу шагов: цепочка коротка,
но чтение не может опираться на это как на инвариант.

Код отказа MUST оставаться прежним и жить в типизированном отказе; называние
допустимого source — деталь отказа, а не новый код.

#### Scenario: Продолжение от продолжения
- **WHEN** source продолжения — сам Run-продолжение
- **THEN** отказ называет исходный Run в конце цепочки происхождения

#### Scenario: Происхождение не ведёт к допустимому source
- **WHEN** цепочка происхождения оборвана либо длиннее допустимого обхода
- **THEN** отказ сохраняет свою причину без названного source и MUST NOT
  называть догадку

### Requirement: Полнота маршрутов спрашивает набор исходов у своей ревизии

Требование полноты маршрутов MUST проверять набор исходов, за который отвечает
ревизия проверяемого документа, а не общий список законных исходов. Расширение
общего списка MUST NOT делать уже запечатанный граф неполным: план
восстанавливается из запечатанных байтов при каждом чтении Run, поэтому такая
неполнота означала бы, что Run, читавшийся вчера, перестал читаться.

Новая ревизия MUST требовать маршрут либо объявленную невозможность для каждого
исхода своего набора, включая добавленные. Прежние ревизии MUST сохранять свой
набор без изменений.

Запечатанный граф, получивший законный исход, для которого его ревизия маршрута
не требовала и автор его не объявил, MUST вести себя как при любом неотвеченном
исходе: принятый результат сохраняется, Run останавливается с точным
диагнозом — и MUST NOT уводиться на маршрут, объявленный для другого исхода.

#### Scenario: Запечатанный граф прежней ревизии после расширения словаря
- **WHEN** Run, запечатанный до расширения словаря исходов, читается и драйвится
- **THEN** он компилируется и исполняется как прежде

#### Scenario: Новая ревизия умолчала о добавленном исходе
- **WHEN** граф новой ревизии не объявил для добавленного исхода ни маршрута, ни
  невозможности
- **THEN** компиляция отказывает и называет стадию и исход

#### Scenario: Прежняя ревизия встретила добавленный исход
- **WHEN** шаг запечатанного графа прежней ревизии возвращает добавленный исход
- **THEN** принятый результат сохранён, Run останавливается с точным диагнозом,
  и маршрут другого исхода не используется

### Requirement: Вставка поднимает ревизию документа и не понижает её

Вставка проектного шага, объявляющая невозможные вердикты, MUST поднимать
ревизию документа до наименьшей, которая такое объявление допускает, и MUST NOT
понижать документ, уже написанный на более поздней ревизии. Присваивание вместо
подъёма означает, что проект переписывает контракт автора пакета: маршруты,
добавленные поздней ревизией, после вставки отказываются как неподдерживаемые.

Ревизию, которую сборка не знает, вставка MUST оставлять как есть: документ,
чью ревизию нельзя расположить в порядке, нельзя и переписать безопасно.

Невозможным вставка MAY объявлять только вердикт, за который отвечает ревизия
результирующего документа. Отказ MUST называть эту ревизию и её набор: enum в
сыром отказе схемы не говорит автору ни того, на какой ревизии его граф, ни
того, чем это исправить.

#### Scenario: Вставка в граф поздней ревизии
- **WHEN** проектная вставка с объявленными невозможными вердиктами
  применяется к графу ревизии более поздней, чем минимально требуемая
- **THEN** ревизия документа не меняется, и маршруты, которые она допускает,
  остаются допустимыми

#### Scenario: Вставка называет вердикт, за который ревизия не отвечает
- **WHEN** вставка объявляет невозможным вердикт, которого набор
  результирующей ревизии не содержит
- **THEN** отказ называет ревизию документа и вердикты, за которые она отвечает

#### Scenario: Ревизия документа сборке неизвестна
- **WHEN** документ объявляет ревизию, которой эта сборка не знает
- **THEN** вставка её не переписывает

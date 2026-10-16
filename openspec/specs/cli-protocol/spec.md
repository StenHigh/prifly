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
question tool своего host, когда этот tool предоставлен runtime и его контракт
допускает данный вопрос в текущем режиме: Codex runner вызывает применимый
`request_user_input` либо `request_user_input_async`, а Claude Code runner —
`AskUserQuestion`. Правило MUST действовать и для предстартового выбора launch,
Workspace, profile, policy и finite preflight answer, независимо от совета.
При асинхронном tool entry point MUST ждать фактический ответ до зависимого
действия; receipt показа и предвыбранный вариант MUST NOT считаться выбором.
Новый runner для Codex CLI и Codex app MAY разделять один текст template с
подставленным fixed host ID; Claude Code MUST получать отдельный template.
Вопрос содержит только реальные взаимоисключающие варианты, short label и
последствие выбора; рекомендуемый вариант обозначается явно. Runner MUST NOT
синтезировать Markdown-псевдокнопки, скрывать варианты или выбирать default.
Если native tool не предоставлен или неприменим, runner MUST ждать явный текстовый ответ и
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
До явного выбора runner SHALL поддерживать условный совет по назначению
объявленных launch, не добавляя обязательный этап к каждому workflow.
Он MUST NOT добавлять improve/review/fix, число рецензентов, обязательный commit
или правило завершения отраслевого цикла. Такие правила MUST принадлежать
workflow package. Frozen исторические templates остаются только для exact
recognition и безопасного upgrade, не как default инструкции нового runner.

#### Scenario: Package использует один шаг без planning/review

- **WHEN** host запускает такой package
- **THEN** runner не добавляет рецензента, цикл улучшений или commit

#### Scenario: Workflow не нуждается в совете

- **WHEN** launch уже назван пользователем или проектные инструкции отключают совет для него
- **THEN** runner переходит к существующей процедуре выбранного launch без отдельной стадии подбора, нового Run или новых настроек workflow

### Requirement: Project результат различает авторскую версию и сборку
При compilation profile `/3` CLI MUST выдавать `project-compile/2`, а при
start без queue — `project-start/3`. Queue-bound start MUST использовать
`project-start/4` с exact queue binding и `project-launch-summary/4`;
prepare без queue сохраняет `project-launch-summary/3`. Оба результата MUST содержать `author_package`
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

### Requirement: Project continuation и recovery принимают любой объявивший launch
`project continue` SHALL принимать любой declared launch, чей скомпилированный
workflow объявил продолжение, а `project recover` — любой launch по общим
правилам восстановления. Launch без объявления продолжения MUST отказывать
`project_continue_undeclared` до claim и Run, называя launch и недостающее
объявление. `--prepare` MUST показывать для каждого перенесённого входа его
источник в исходном Run и точный ArtifactRef, а также claim, который будет
передан, или указание на новый claim. `--workspace-commit` с полным
идентификатором commit MUST создавать новый claim от этого commit вместо
передачи; `project recover` его не принимает. CLI MUST NOT вычислять входы
из Git и MUST NOT читать содержимое артефактов за workflow. `run status`
MUST показывать последний принятый checkpoint Run.

#### Scenario: Launch другого пакета объявил продолжение
- **WHEN** оператор вызывает `project continue --prepare` для launch
  пакета, не связанного с AI Factory, и исходного Run допустимого workflow
- **THEN** prepare возвращает источники и refs всех перенесённых входов,
  передаваемый claim и review digest, а start с этим digest создаёт Run
  продолжения

#### Scenario: Launch без объявления
- **WHEN** оператор вызывает `project continue` для launch, workflow которого
  не объявил продолжение
- **THEN** CLI отказывает `project_continue_undeclared` без claim и Run

#### Scenario: Работа сделана на другом commit
- **WHEN** оператор передаёт `--workspace-commit` с полным существующим
  commit в режиме `worktree`
- **THEN** создаётся новый claim от этого commit, claim исходного Run
  остаётся за ним, а проверку связи commit с checkpoint делает первый шаг
  workflow продолжения

### Requirement: run next передаёт новому исполнителю, как процесс пришёл к действию
Ответ `run next` SHALL в новой версии содержать всё, что исполнителю с чистым
контекстом нужно, чтобы продолжить без истории беседы и без чтения
хранилища:

- `arrived_from` — стадию, чей принятый результат привёл к текущему действию
  или к finish, то, по чему она маршрутизировала (вердикт шага, исход
  дочернего вызова или маршрут решения `repeat`, например `on_limit`), и ссылки
  на её принятые выходы (у `repeat` — выходы тела, по которому принято
  решение);
  если к стадии ведут несколько принятых рёбер и однозначно назвать одно
  нельзя, поле отсутствует, а не угадывается;
- `checkpoint` — последний принятый checkpoint Run, если workflow его объявил;
- `repeats` — для каждого `repeat`, внутри которого находится текущее
  действие: стадию, текущую итерацию, действующий лимит с учётом сужения
  проектом и маршрут `on_limit`;
- для завершённого или отменённого Run — `continuations`: workflow
  установленных пакетов, чьё объявление продолжения допускает этот Run, и
  `project.continue` в `safe_next_actions`, если список не пуст. Пустой
  список MUST означать, что установленные пакеты не объявили продолжение
  этого Run, а не что продолжение невозможно вообще.

Список `continuations` MUST строиться из установленных и доверенных пакетов;
пакет, чей launch ещё ни разу не запускался, в authority не установлен, и
окончательную проверку MUST делать `project continue --prepare`.

Run, созданный проектным запуском (`project start`, `project continue`,
`project recover`), MUST запечатываться не ниже первого состояния, которое
отвечает этой версией `run next`, даже если его возможности требуют более
раннего; Runs, запечатанные раньше, MUST сохранять своё состояние и ответ.

Все поля MUST выводиться при чтении из того, что Run и установленные пакеты
уже хранят; содержимое артефактов MUST NOT читаться. Прежние версии ответа
MUST оставаться прежними байт в байт.

#### Scenario: Препятствие передано следующему шагу
- **WHEN** шаг вернул `blocked` с обещанным выходом, и `on.blocked` ведёт к
  шагу устранения
- **THEN** `run next` для шага устранения называет в `arrived_from` исходную
  стадию, вердикт `blocked` и ссылку на её выход

#### Scenario: Ограниченный цикл устранения
- **WHEN** текущее действие находится во второй итерации `repeat` с
  `max_iterations: 3`, суженным проектом до 2
- **THEN** `repeats` называет итерацию 2, лимит 2 и маршрут `on_limit`

#### Scenario: Завершённый через blocked Run можно продолжить
- **WHEN** Run завершился исходом, в который вёл `blocked`, а установленный
  пакет содержит workflow, объявивший продолжение этого workflow и исхода
- **THEN** `run next` называет этот workflow в `continuations` и
  `project.continue` в `safe_next_actions`

#### Scenario: Программный Run проекта без Git
- **WHEN** проект запускает workflow только из программных шагов без рабочего
  дерева
- **THEN** Run запечатан не ниже состояния, которое отвечает новой версией
  `run next`, и после остановки по `on_limit` `arrived_from` называет стадию
  `repeat`, маршрут `on_limit` и выходы последнего тела

#### Scenario: Продолжать нечем
- **WHEN** ни один установленный workflow не объявил продолжение этого Run
- **THEN** `continuations` пуст, а `safe_next_actions` не предлагает
  `project.continue`

### Requirement: project continue возобновляет Run тем же launch
`project continue --source-run RUN` SHALL возобновлять исходный Run, если
выбранный launch запускает тот же workflow, что и исходный Run; с launch
другого workflow он MUST продолжать по объявлению `continuation`, как прежде.
`--from-stage STAGE` SHALL выбирать более раннюю завершённую стадию корневого
вызова как точку возобновления. `--prepare` MUST показывать точку
возобновления, перенесённые стадии и передаваемое дерево. `run next`
завершённого или отменённого Run MUST включать в `continuations` его
собственный workflow, если тот объявил `resumable` и допускает этот Run, и
тогда предлагать `project.continue`. Возобновление и `project recover` MUST
брать режим дерева из claim исходного Run, а не из `workspace:` launch или
`--workspace`. Если не передано ни одного ответа анкеты, профиля пакета или
политики, они MUST брать ответы, запечатанные в исходном Run; переданные
ответы MUST сверяться с исходными. Продолжению другим workflow ответы
исходного Run MUST NOT подставляться.

#### Scenario: Пилот продолжает свой Run тем же launch
- **WHEN** оператор вызывает `project continue --source-run RUN --launch L`,
  где L — launch, которым RUN был запущен, и его workflow объявил `resumable`
- **THEN** prepare показывает точку возобновления и перенос, а start создаёт
  возобновлённый Run в дереве исходного

#### Scenario: Точка возобновления указана явно
- **WHEN** оператор передаёт `--from-stage plan`
- **THEN** возобновлённый Run начинает с `plan`, и prepare это показывает

#### Scenario: Возобновление без workspace у launch и без ответов
- **WHEN** у launch нет `workspace:`, а оператор не передаёт ни `--workspace`,
  ни ответов анкеты
- **THEN** возобновление берёт режим дерева и ответы исходного Run и
  проходит; другой ответ даёт отказ `recover_context_changed`

### Requirement: Автоматическая очистка Registry обозрима до и после запуска

Read-only подготовка установки SHALL показывать текущий и прогнозный бюджет Registry, exact package refs кандидатов, число освобождаемых entries, причину выбора и защищённые editions. Review digest MUST покрывать этот план. Подтверждённая команда SHALL повторно проверять его и возвращать фактически изменённые `PackageStatus` и итоговый бюджет; скрытое удаление при `--prepare` запрещено. Старая версия публичного ответа MUST не получать новые поля с изменённым смыслом: новая форма ответа версионируется.

#### Scenario: Оператор видит предстоящую отставку
- **WHEN** подготовка Project launch обнаруживает нехватку места и безопасную старую edition
- **THEN** ответ перечисляет её exact ref и освобождаемые entries, но package list и authority остаются неизменными

#### Scenario: Между подготовкой и запуском статус изменился
- **WHEN** оператор подтверждает прежний review digest после новой установки или изменения держателей
- **THEN** запуск отказывает с требованием повторного preview, не заменяя кандидата молча

#### Scenario: Запуск завершил освобождение места
- **WHEN** подтверждённая команда установила новую edition и вывела старую
- **THEN** ответ и package list показывают обе exact identities, новый статус старой и итоговый бюджет

### Requirement: Чтение Run показывает объявленный эффект каждого шага

`run status` в текстовом виде MUST выводить для каждого шага класс эффекта и
`retry_class`, а для `external_write` ещё и систему, операции и цель границы.
Ответ `run next` / `run explain` (одно семейство ответов) MUST в новой версии
`core-next/42` нести то же для каждого экземпляра шага Run (с его стадией) и для
стадии текущего действия. `run status --json` (RunView) MUST NOT меняться: его
версии закреплены состоянием Run. Прежние опубликованные версии MUST NOT
меняться.

#### Scenario: Автор проверяет, что граница записана
- **WHEN** оператор читает `run status RUN` после Run с шагом `external_write`
- **THEN** строка шага называет `external_write`, `retry_class` и границу

### Requirement: project start отказывает до предстартовой сводки

`project start` MUST выполнять все проверки, которые могут отказать до создания
Run, включая бюджет Registry, до того, как пишет предстартовую сводку. Если
отказ случается до сводки, вывод MUST содержать только документ отказа. Отказ,
который может случиться только после сводки (например, изменилась локальная
конфигурация после её публикации), MUST быть последним документом потока, а
код возврата MUST быть ненулевым, как и сейчас.
Справка `project start` MUST говорить, что сводка и отказ идут в stderr, а
итог запуска — в stdout.

#### Scenario: Registry переполнен
- **WHEN** `project start --json` не может установить редакцию пакета из-за
  предела Registry
- **THEN** stderr несёт единственный документ — отказ `dependency_limit`,
  предстартовой сводки перед ним нет, stdout пуст

### Requirement: run next называет ожидание хоста

Новая версия ответа `run next` MUST иметь действие `awaiting_host`, когда
драйверу нечего делать, а выданное задание ждёт хоста. `idle` MUST оставаться
только для состояния, в котором не ждёт никто. `safe_next_actions` MUST
по-прежнему содержать `session.task`. Ответ с `awaiting_host` MUST нести в
`next_command` команду, которой хост берёт задание (`session task --run RUN
--all`), а инструкция раннера (`prifly-run`) MUST говорить, что с ним делать.
Текстовая форма `run next` остаётся JSON: её читают хосты. Runs, чей ответ
закреплён прежней версией, MUST отвечать по ней.

#### Scenario: Задание выдано сразу после старта
- **WHEN** `project start` создал Run, и первая стадия ассистируемая
- **THEN** `run next RUN --json` отвечает `action: awaiting_host` с
  `next_command: session task --run RUN --all`

#### Scenario: Ждать нечего
- **WHEN** Run не держит ни готовой стадии, ни выданного задания
- **THEN** `run next` отвечает `idle`, как прежде

### Requirement: run list показывает Runs authority

`run list` MUST читать Runs выбранной authority без изменения состояния и
выводить их новыми первыми: id, workflow, статус, исход, время создания и
признак, ждёт ли Run хоста. Формат `--json` MUST быть версионирован
(`run-list/1`). Команда MUST быть описана в справке. Отказ на неизвестную
операцию `run` MUST называть `run list`.

#### Scenario: Оператор ищет id своего Run
- **WHEN** оператор вызывает `run list` в authority с несколькими Runs
- **THEN** ответ называет каждый Run с его workflow и статусом, новый первым

### Requirement: Путь к событиям и обратимость вывода пакета названы

Справка `run events` MUST говорить, что в `--json` события лежат в
`.view.events`, а продолжение задаётся `--after` со значением `next_after`.
Текстовый `run events` MUST печатать сами события. Отказ `dependency_limit` и
справка `package remove` MUST говорить, что вывод редакции отменяется командой
`package restore`, и приводить её форму.

#### Scenario: Модель читает события
- **WHEN** модель читает справку `run events`
- **THEN** она находит путь `.view.events` и способ продолжить чтение

#### Scenario: Хост считает вывод пакета необратимым
- **WHEN** хост видит отказ `dependency_limit`
- **THEN** текст отказа говорит, что `package remove` отменяется
  `package restore --id ID --version X.Y.Z --reason TEXT`

### Requirement: Host отчитывается о вопросах шага по записанной форме
Задача assisted-сессии MUST сообщать host, что отчёт обязан нести список вопросов, а сгенерированные инструкции host runner MUST называть форму записи, допустимые основания ответа и то, что пустой список — это заявление. Отказ отчёта без списка или с неверной записью MUST называть поле и допустимые значения, чтобы host исправил отчёт без чтения схем движка. `run status` SHALL печатать вопросы каждого Attempt с ответом и основанием.

#### Scenario: Host со старыми инструкциями
- **WHEN** host, не знающий о списке, присылает отчёт без него в Run, который его требует
- **THEN** отказ называет поле `answered_questions`, форму записи и команду обновления инструкций runner

#### Scenario: Оператор читает Run из терминала
- **WHEN** оператор вызывает `run status` для Run с записанными вопросами
- **THEN** он видит по строке на вопрос: шаг, вопрос, ответ и основание

### Requirement: Проект объявляет исключительные ресурсы и их стадии
Расширение workflow SHALL объявлять именованные ресурсы с лимитом и стадиями этого workflow, которые их занимают; локальная конфигурация машины MAY переопределять лимит. Запуск и его обзор MUST отказывать по имени, если расширение называет стадию, которой нет в workflow, до создания Run. `capacity show` SHALL показывать держателей каждого ресурса.

#### Scenario: Расширение называет несуществующую стадию
- **WHEN** `stage_resources` называет стадию, которой нет в workflow
- **THEN** `project start` и `--prepare` отказывают `project_resource_stage_unknown`, называя ресурс, стадию и workflow, и Run не создаётся

### Requirement: Отказ запуска проекта называет исходную причину
Если `project start` создал Run, но не смог его вести, отказ SHALL нести код исходной причины и id созданного Run, а не общий код незавершённого запуска.

#### Scenario: Драйвер занят
- **WHEN** `project start` создал Run, а вести его мешает другой драйвер
- **THEN** отказ несёт `driver_already_active`, id созданного Run и команду, которой его довести

### Requirement: План цепочки возобновления объясняет перенос и безопасный следующий шаг

`project continue --prepare` того же workflow и `project recover --prepare` SHALL показывать весь доказанно переносимый префикс, различая исходное исполнение и перенос через непосредственный source Run, точку возобновления и текущий передаваемый claim. Оператор MUST иметь возможность проверить происхождение унаследованных результатов по публичному плану и чтению связанных Runs без извлечения входных ArtifactRefs вручную. Prepare MUST NOT создавать Run, импортировать package, менять claim или dirty tree и запускать программы.

`resume_prefix_empty` SHALL означать только отсутствие собственного и доказанного унаследованного префикса до выбранной точки. Отказы на недоступное или несовместимое evidence MUST называть это основание, а не советовать новый полный Run как эквивалент сохранённой цепочки. Runner guidance SHALL объяснять, что `run next` перечисляет объявленные возможности installed workflow, а окончательная применимость конкретного launch проверяется prepare. При отказе prepare агент MUST сохранять текущего владельца и дерево; обход через raw fork, ручную правку authority или самовольный сброс работы не является следующим шагом.

Повторный start SHALL использовать проверенные repository, authority, launch, host и review digest prepare; input/answers source не подменяются. Он MUST выдавать новый Run с публичной связью на source и без фиктивных Attempts для перенесённых стадий. Изменившийся review MUST требовать нового prepare, а не снятия guards.

Нормативный source set — `openspec/specs/cli-protocol/spec.md`. CLI-флаги и сохранённые JSON-поля не переименовываются; изменение формы публичного ответа, если необходимо, SHALL иметь новую edition рядом с прежней.

#### Scenario: Агент продолжает B после решения владельца
- **WHEN** B завершился объявленным resumable outcome и хранит унаследованный префикс и claim
- **THEN** prepare показывает перенос ранних стадий, current source B и его claim; после проверки оператор запускает новый Run с этим digest и продолжает по выданным Attempts

#### Scenario: Индекс объявлений не равен admission
- **WHEN** `run next` называет workflow в continuations, но конкретный launch или его package несовместим с source
- **THEN** prepare сообщает конкретный отказ, а guidance направляет агента сохранить состояние и сообщить причину без перебора обходных запусков

#### Scenario: Настоящий пустой префикс при выборе entry
- **WHEN** оператор выбрал начальную стадию и до неё нет ни собственного, ни унаследованного результата
- **THEN** CLI возвращает `resume_prefix_empty`, объясняет отсутствие переноса и не освобождает уже существующий claim

#### Scenario: Prepare повторяется без изменений
- **WHEN** агент повторяет prepare при неизменных source, target и authority evidence
- **THEN** перенос и review digest детерминированы, а Run versions, claims, Registry и рабочее дерево остаются неизменными

### Requirement: Предстартовый выбор использует применимый нативный инструмент host

При необходимом конечном выборе launch, Workspace, package profile, policy
или объявленного finite preflight answer runner MUST вызвать предоставленный
host native question tool, если его текущий контракт допускает этот вопрос;
правило SHALL действовать независимо от наличия рекомендации. Указания только
для `waiting_decision` MUST NOT заменять это правило до Run.

Codex runner SHALL использовать доступный применимый `request_user_input`
или `request_user_input_async`, а Claude Code runner — `AskUserQuestion`.
Host MUST соблюдать ограничения режима, назначения и формы фактически
предоставленного tool; наличие имени в skill MUST NOT считаться доступностью.
Runner MUST NOT переключать режим, менять настройки host или создавать
собственный UI ради обхода недоступного инструмента. При отсутствии применимого
tool host SHALL задать один явный текстовый вопрос и ждать ответа.

Варианты SHALL содержать реальные selectable значения с понятными labels и
последствиями; launch SHALL быть однозначно связан с exact ID. При наличии
обоснованного совета рекомендуемый вариант SHALL быть явно обозначен; без
совета runner MUST NOT изобретать рекомендацию ради заполнения формы. Если
варианты не помещаются в tool, host SHALL предоставить детерминированные
страницы с навигацией и возможностью отказаться, сохранив все реальные выборы.
Свободный ввод task text, file path, RunBrief или произвольного JSON MUST NOT
превращаться в придуманный конечный список.

При асинхронном question tool host MUST ждать фактического ответа до любого
зависимого действия. Receipt показа вопроса, preselected option, dismiss,
timeout и молчание MUST NOT считаться выбором. Исполнительная авторизация
SHALL следовать своим действующим правилам и ограничениям инструмента;
инструмент предпочтений MUST NOT использоваться для approval, если его
контракт это запрещает.

#### Scenario: Codex показывает варианты до Run

- **WHEN** Codex host предоставляет применимый question tool, а пользователь ещё не выбрал launch
- **THEN** runner вызывает этот tool с реальными вариантами вместо одного только текстового списка и ждёт отправленный ответ без создания package, claim или Run

#### Scenario: Доступен только асинхронный инструмент Codex

- **WHEN** blocking `request_user_input` недоступен в текущем режиме, а `request_user_input_async` предоставлен и допускает выбор launch
- **THEN** runner вызывает асинхронный инструмент и не выполняет зависимые questionnaire/start действия до фактического ответа

#### Scenario: Claude показывает конечный выбор

- **WHEN** Claude Code предоставляет `AskUserQuestion` и предстартовый конечный выбор ещё не сделан
- **THEN** runner вызывает `AskUserQuestion`, показывает реальные варианты и ждёт ответ до mutation

#### Scenario: Совет отключён, интерфейс выбора доступен

- **WHEN** рекомендация пропущена или отключена через `PROJECT.md`, но finite выбор требуется и native tool применим
- **THEN** runner показывает native выбор без придуманной рекомендации и принимает явный ответ

#### Scenario: Вариантов больше лимита tool

- **WHEN** число объявленных launch превышает текущий лимит question tool
- **THEN** native страницы позволяют достичь каждого launch, вернуться и отказаться без скрытого default или mutation до выбора

#### Scenario: Инструмент недоступен или не допускает вопрос

- **WHEN** host не предоставил native tool либо его контракт запрещает этот вопрос в текущем режиме
- **THEN** runner задаёт явный текстовый вопрос и ждёт ответ без фиктивного tool call или обхода host ограничений

#### Scenario: Вопрос показан без ответа

- **WHEN** асинхронный tool подтвердил показ либо пользователь закрыл вопрос без выбора
- **THEN** runner сохраняет ожидание и не использует предвыбранный вариант как ответ

### Requirement: Host runner условно советует объявленный launch перед выбором

Если точный launch ID не задан, общий `prifly-run` SHALL получить текущие
объявленные launch через `project workflows --repository ... --json` и показать
все варианты для явного выбора. Runner MUST отделять применимость совета от
права на запуск. Совет SHALL выполнять текущая host-сессия, только когда
выбор между несколькими launch зависит от описанной пользователем задачи,
проектные инструкции допускают такой совет и пользователь его не пропустил.

Основаниями SHALL служить исходная задача из текущего разговора, `title` /
`description` возвращённых launch и применимые критерии соседнего `PROJECT.md`.
`PROJECT.md` SHALL позволять уточнить применимость либо отключить совет для
проекта или отдельных launch; это MUST NOT скрывать launch из списка выбора
или разрешать запуск без явного ответа. Новые schema fields и обязательные
workflow settings MUST NOT требоваться.

При достаточных основаниях runner SHALL дать одну краткую рекомендацию с
exact ID, названием и причиной, назвав существенную неопределённость. При
неразличимых или недостаточных описаниях runner MUST сообщить, что обоснованный
совет невозможен, сохранив ручной выбор. Он MUST NOT придумывать возможности,
цену или время, подменять отсутствующие описания именами, вводить числовой
confidence, выбирать модель, создавать triage Run либо требовать полного
исследования репозитория ради совета. Записи удалённого каталога MUST NOT
становиться кандидатами на запуск без объявления в проекте.

При отсутствии описания задачи runner SHALL предложить описать её для совета
либо сразу выбрать launch. Этот ввод MUST NOT становиться обязательным для
workflow, который задачи не требует. Единственный launch SHALL показываться
без отдельного совета; пустой список SHALL объясняться без автоматической
установки workflow. До явного выбора runner MUST NOT создавать package, claim
или Run, в том числе когда вариант только один.

Точный объявленный ID SHALL вести к его анкете без повторного подбора.
Существенное известное несоответствие назначения runner SHALL кратко назвать
и уточнить намерение, если оно неоднозначно; уже явный выбор MUST уважаться.
Неизвестный ID MUST NOT заменяться похожим. Отказ от совета и выбор альтернативы
MUST приниматься без навязывания рекомендованного launch.

#### Scenario: Обоснованный совет при нескольких launch

- **WHEN** пользователь описал задачу без ID, а назначения нескольких объявленных launch позволяют обоснованно рекомендовать один
- **THEN** host показывает рекомендацию с ID, названием и причиной вместе со всеми вариантами и ждёт явного выбора без `project start`, package, claim или Run

#### Scenario: Пользователь выбирает альтернативу

- **WHEN** после рекомендации пользователь явно выбрал другой объявленный launch
- **THEN** runner переходит к анкете выбранного пользователем launch без повторного навязывания совета

#### Scenario: Точный ID уже известен

- **WHEN** пользователь назвал точный объявленный launch ID
- **THEN** runner использует этот ID без повторного выбора и отдельного подбора

#### Scenario: Неизвестный ID

- **WHEN** указанный ID отсутствует среди объявленных launch
- **THEN** runner сообщает об отсутствии ID, показывает доступные варианты и ждёт исправленного выбора без автоматической замены

#### Scenario: Задача не описана

- **WHEN** точный ID и описание задачи отсутствуют
- **THEN** host предлагает описание для совета либо непосредственный выбор из списка, не требуя фиктивного brief

#### Scenario: Пользователь пропускает совет

- **WHEN** пользователь отказался от подбора по задаче
- **THEN** runner показывает варианты и принимает явный выбор без обязательного объяснения задачи

#### Scenario: Описания не позволяют различить назначения

- **WHEN** descriptions формально присутствуют, но не дают основания для выбора по задаче
- **THEN** host сообщает о недостатке сведений и оставляет все launch доступными для ручного выбора без уверенного выдуманного совета

#### Scenario: Единственный launch

- **WHEN** ID не задан и проект объявляет ровно один launch
- **THEN** host показывает его и ждёт явного выбора, не проводя отдельный подбор и не начиная Run автоматически

#### Scenario: Пустой список

- **WHEN** проект не объявляет ни одного launch
- **THEN** host сообщает об отсутствии вариантов без запуска и без автоматического поиска или установки сценария

#### Scenario: Проект отключил совет для специального launch

- **WHEN** `PROJECT.md` исключает launch из рекомендаций по задаче либо отключает совет целиком
- **THEN** host соблюдает ограничение совета, сохраняет все объявленные launch в списке и допускает явный выбор исключённого launch

#### Scenario: Названия workflow не определяют совет

- **WHEN** generic project использует произвольные ID, а descriptions задают назначения, отличные от подсказок в именах
- **THEN** host основывает совет на объявленном назначении и проектных критериях без правил AI Factory или угадывания по ID

### Requirement: После совета runner сохраняет входы и проверенный запуск

После явного выбора host SHALL продолжать существующую процедуру questionnaire,
сбора объявленных typed inputs, read-only prepare и reviewed start с exact
подготовленными аргументами и digest. Рекомендация и выбор launch MUST NOT
заменять подтверждение подготовленного запуска или executable effects.

Если выбранный workflow объявляет input для предмета работы, host SHALL
перенести в него исходную задачу, сохранив её текст и переданные пользователем
ссылки; объяснение рекомендации MUST NOT заменять задачу пересказом. Упаковка
SHALL соответствовать schema и format выбранного input. Runner MUST NOT
предполагать универсальные имя `task`, JSON-форму или обязательность brief;
неясное соответствие портам SHALL уточняться до запуска, остальные missing
required values SHALL собираться обычной анкетой. Workflow без такого input
MUST NOT получать фиктивную задачу или новый порт.

Совет до Run MUST NOT создавать DecisionDefinition, DecisionAnswer,
DecisionRecord, Approval, Grant или новую execution permission. Правила
рекомендаций и automatic choice объявленного DecisionCatalog SHALL сохраняться
без изменений.

#### Scenario: Разные workflows имеют разные inputs

- **WHEN** пользователь после совета выбрал workflow с другим именем или форматом входа задачи
- **THEN** host сохраняет исходный текст и ссылки в объявленном формате выбранного input, собирает остальные значения и не переносит обоснование совета вместо задачи

#### Scenario: Workflow не принимает задачу

- **WHEN** выбранный launch использует файловые inputs либо не имеет входа для описания задачи
- **THEN** host запрашивает только объявленные значения без фиктивного `task`, RunBrief или новой schema

#### Scenario: Соответствие задачи порту неизвестно

- **WHEN** входы выбранного workflow не позволяют однозначно установить, куда передать исходную задачу
- **THEN** host уточняет binding без выдуманного порта или утраты исходного текста и не начинает запуск до разрешения вопроса

#### Scenario: Выбор не подтверждает выполнение

- **WHEN** пользователь выбрал рекомендованный launch, но ещё не подтвердил prepared summary и требуемые executable effects
- **THEN** runner сохраняет существующие ограничения reviewed start и не считает совет либо выбор разрешением на выполнение

### Requirement: Обновление runner доставляет условный совет совместимо

`project runners update` SHALL доставлять инструкции условного совета
поддерживаемым hosts `codex-cli`, `codex-app` и `claude-code`, распознавая exact
generated runner предыдущих выпусков и сохраняя соседний `PROJECT.md`
байт-в-байт. Кастомизированный `SKILL.md` MUST сохранять существующий refusal;
новое поведение MUST NOT доставляться неявной перезаписью при init.
Прежние frozen runner bytes и исторические evidence MUST сохраняться.

#### Scenario: Upgrade рядом с проектными критериями

- **WHEN** exact generated runner прежнего выпуска обновляется рядом с `PROJECT.md`, содержащим ограничения совета
- **THEN** новый skill поддерживает общий совет и читает прежний overlay, а bytes `PROJECT.md` не меняются

#### Scenario: Кастомизированный runner

- **WHEN** `SKILL.md` содержит неизвестную локальную правку
- **THEN** update возвращает существующий конфликт без перезаписи правки или частичного обновления других runner

### Requirement: Queue CLI отделяет захват от разрешения исполнения

CLI SHALL предоставлять provider-neutral `project queue add/list/show/update/claim-next/release/complete/reconcile`, typed versioned JSON outputs и стабильные refusal codes. Queue mutation и queue-bound project start/continue/recover SHALL требовать явный caller `--command-id`, а owner-sensitive операции — task revision и claim generation. Read команды и prepare MUST NOT создавать queue либо Run. Task selection SHALL передаваться project questionnaire/prepare/start явными queue identity/owner/revision/generation, а source-bound продолжение MUST сохранять recorded Task identity.

Generated runner SHALL уметь принять claimed QueueTask, сохранить original request и показать доступные project launches. Он MUST получить фактический выбор launch и необходимые typed inputs, провести existing questionnaire/prepare/review и ждать требуемого ответа до start. Claim-next MUST NOT выбирать launch, разрешать executable effects, спавнить host или продолжать draining. Вторая host-сессия и повтор команды SHALL сначала читать durable intent/association; неизвестный dispatch SHALL переводить UX к reconcile, не к ещё одному start.

#### Scenario: Host взял next и ждёт выбора
- **WHEN** generated runner получил claimed task, но пользователь ещё не выбрал launch или не дал обязательный ответ
- **THEN** request и claim сохранены, а programs, workspace claim и Run ещё не созданы

#### Scenario: Ответ start потерян
- **WHEN** runner повторно получает ту же задачу после admission и потери ответа
- **THEN** он показывает записанный Run/association либо reconcile состояние, а не запускает задачу повторно

### Requirement: Ошибка запуска показывает незавершённый cleanup без потери исходной причины
При подтверждённой ошибке до RunStart Project launch SHALL пытаться освободить
только claim, созданный именно этой попыткой. Existing, handed-over или
принадлежащий admitted Run claim MUST NOT освобождаться как rollback нового
запуска. Неизвестный исход RunStart SHALL сначала требовать reconciliation
по command/receipt и записанному Run; ошибка ответа MUST NOT доказывать, что
Run не создан.

CLI SHALL сохранять code и объяснение исходного отказа. Если cleanup не
завершён, сообщение SHALL явно называть оставшийся claim ID, generation,
наблюдаемый status, причину cleanup failure и применимые inspect/retry действия.
Unknown cleanup state MUST оставаться unknown. Команда MUST NOT сообщать
«ничего не создано» или советовать слепой новый start при оставшемся claim.
Один Problem-конверт MUST сохранять исходную schema и не маскировать ошибку
новыми полями старого closed contract. Успешный rollback SHALL отличаться от
незавершённого.

#### Scenario: Start отказал, cleanup выполнился
- **WHEN** confirmed pre-Run failure следует после создания своего свободного claim, в том числе на authority >64 МиБ
- **THEN** новый Run отсутствует, claim штатно released, а CLI возвращает исходный отказ и достоверный cleanup result

#### Scenario: Release не завершился
- **WHEN** rollback не смог доказать ownership, встретил changed generation либо filesystem failure
- **THEN** original refusal code сохраняется, сообщение называет конкретный оставшийся claim/cleanup причину, новый start не предлагается как обход

#### Scenario: Ответ RunStart потерян
- **WHEN** ошибка транспорта/хранилища не доказывает отсутствие committed Run
- **THEN** launch сверяет recorded command/Run либо сохраняет неопределённость; admitted claim не удаляется

### Requirement: Scan refusal и doctor различают ownership scan и logical-page budget
Ошибка aggregate bounded scan SHALL доходить до CLI через typed named отказ
`authority_scan_budget_exhausted` с observed count/bytes, ограничением и видом
лимита. Она MUST NOT превращаться в `invalid_input` и MUST NOT считаться
исчерпанием SQLite logical-page budget. Наблюдаемая нижняя граница MUST NOT
выдаваться за точный total population, если чтение оборвано.

Read-only `doctor --claim ID` SHALL диагностировать claim без RunID: exact
claim identity/generation/status, проверенный ReadCut, полноту ownership
проверки, observed scan metrics, blocker либо причину неполноты и отдельно
allocated logical-page usage / effective limit. При непрочитанной/несовместимой
части SHALL сохраняться unknown, без raw snapshots, task bodies или secrets.
Diagnostic observation MUST NOT быть разрешением на release.

Новый view SHALL публиковаться как отдельный `claim-ownership-diagnostics/1`
contract; plain doctor и прежний Problem MUST сохранять свои forms. Если
полезная диагностика возможна при отказе полной проверки, view SHALL явно
указывать её неполноту, а не фабриковать успешную проверку.

#### Scenario: Bounded read достиг лимита
- **WHEN** bounded read API превышает bytes/count allowance
- **THEN** Problem содержит named scan reason и наблюдённые threshold values; next actions ведут к применимой диагностике, а не к Run status несуществующего Run

#### Scenario: Orphan claim исследуется без записи
- **WHEN** оператор вызывает `doctor --claim ID` для unbound claim большой authority
- **THEN** он получает полное либо явно incomplete ownership наблюдение отдельно от storage budget, без изменения claim, Run history, files или registry

### Requirement: CLI показывает фактический storage allowance и его границу
Read-only `storage budget show` и `doctor --storage` SHALL выдавать
`storage-budget-status/1` с authority identity/epoch, allocated bytes, page
size/count/free pages, effective finite limit, source/generation setting и
применимыми действиями. Неизвестное измерение MUST оставаться unknown с
причиной. Статус SHALL явно отделять logical pages от physical disk/WAL,
aggregate scan bytes и per-snapshot limit; чтение MUST NOT выполнять checkpoint,
VACUUM, GC, изменение configuration либо storage migration.

`storage_budget_exhausted` SHALL сохранять stable code/exit class и сообщать
доступные observed usage/limit и concrete operator path к show/review/set,
не объявляя безусловный retry существующим правом. Plain doctor и прежний
Problem SHALL сохранять published shape; дополнительные structured данные
SHALL иметь отдельную version boundary.

#### Scenario: Оператор выясняет причину остановки
- **WHEN** drive вернул `storage_budget_exhausted`, а оператор вызывает storage status
- **THEN** он видит observed allocation/effective ceiling и явный recovery путь без изменения authority или запуска Run

### Requirement: Storage limit меняется только reviewed owner командой
`storage budget set --bytes N --reason TEXT --prepare` SHALL быть read-only и
возвращать `storage-budget-review/1`: exact authority/epoch, current
generation/limit, requested target/reason, необходимые storage compatibility
изменения, observation usage и review digest. Digest SHALL защищать решение
владельца, а не timestamp изменяемой telemetry.

Mutation SHALL требовать те же target/reason, explicit
`--expected-review-digest` и command identity. До commit MUST проверяться
current authority/configuration и применимость target к актуальному
allocation. Stale generation/authority/review MUST давать named refusal;
состояние quota или setting не должно меняться по старому review. Если запись
из-за physical storage failure не подтверждена, CLI MUST сохранять честный
unknown/failure и путь reconciliation по command, а не советовать слепой set.
Команда SHALL возвращать `storage-budget-result/1` с recorded
before/after/generation и command evidence. Setting MUST NOT подразумевать
execution consent или автоматически запускать drive.

#### Scenario: Старое согласование неприменимо
- **WHEN** authority/epoch/configuration generation либо protected request изменены после prepare
- **THEN** set отказывается до budget mutation и требует новый review

#### Scenario: Usage вырос, а решение осталось прежним
- **WHEN** после prepare обычные writes изменили allocation без смены config
- **THEN** set заново проверяет актуальную allocation; он либо коммитит тот же явно выбранный достаточный target, либо отказывает без auto-grow

#### Scenario: Ответ принятой команды потерян
- **WHEN** оператор повторяет тот же exact command/payload после потери ответа
- **THEN** возвращается recorded result без повторного повышения generation или ещё одной mutation

## Purpose

Определяет узкий, проверяемый профиль `foundation-sequence/1`, его границы,
сохранность фактов и условия будущей квалификации без ложного заявления о
полной готовности Pri-Fly.

## ADDED Requirements

### Requirement: Foundation profile имеет явные границы
Pri-Fly SHALL определять `foundation-sequence/1` как профиль с
последовательным локальным исполнением, управляемой остановкой, durable
историей, базовой телеметрией и объявленными state/event hooks. Профиль MUST
отделять реализуемую первую область от обязательных будущих операторов,
managed execution, package trust, планирования ресурсов и полного
эксплуатационного профиля. Он MUST не объявлять Git, AI Factory, конкретного
поставщика ИИ, marketplace, SaaS, HA-кластер или GUI обязательной частью
Pri-Fly.

#### Scenario: Пользователь видит ограничение профиля
- **WHEN** пользователь выбирает или читает профиль foundation
- **THEN** он получает поддерживаемые возможности и явно названные
  неподдерживаемые границы без заявления о полном целевом объёме

### Requirement: Workflow профиля остаётся подмножеством общего контракта
Foundation profile SHALL принимать только `step` и `finish` из общего
WorkflowRevision contract. Он MUST принимать конечную успешную цепочку,
допускать только terminal non-pass handlers и проверять достижимость, targets,
limits, output availability и типы bindings до dispatch. `choice`, `call`,
`repeat`, `parallel`, `map`, `wait`, compensation, projections, iteration
contexts, неизвестные fields и неподдержанные optional возможности MUST быть
отклонены с диагностикой расположения до первого запуска.

#### Scenario: Неподдержанный оператор не исполняется как fallback
- **WHEN** workflow содержит известный, но неподдержанный оператор
- **THEN** Pri-Fly возвращает `unsupported` с JSON Pointer до создания Attempt

#### Scenario: Непроходимый output не связывается
- **WHEN** terminal stage читает output stage, который не выполнен на данном
  пути
- **THEN** проверка definition отклоняет workflow до dispatch

### Requirement: YAML и JSON описывают одну модель
Foundation profile SHALL использовать ограниченный YAML или JSON как
представления одного versioned workflow model. Parser MUST ограничивать
размер и глубину, запрещать duplicate keys, неизвестные DTO fields, tags,
aliases, merge и environment substitution; YAML frontend MUST иметь доказанное
соответствие той же JSON model. Pri-Fly MUST не вводить упрощённый второй DSL
для последовательности команд.

#### Scenario: YAML с alias не расширяет definition
- **WHEN** authoring-файл использует YAML alias или merge
- **THEN** parser отклоняет его до построения расширенного дерева

### Requirement: Definition и конкретное исполнение имеют разную identity
Pri-Fly SHALL сохранять immutable WorkflowRevision и StepDefinition отдельно
от Run, WorkflowInvocation, StageActivation, StepInstance и Attempt. Повторное
использование одной StepDefinition MUST создавать разные identities и outputs
для разных stage activations, а новый workflow revision MUST создавать новый
Run. Активный Run MUST не подхватывать изменённые файлы, порядок шагов или
prompt после start.

#### Scenario: Одна definition включается дважды
- **WHEN** два именованных stage ссылаются на одну StepDefinition
- **THEN** их activation, instance, attempt и outputs остаются независимыми

### Requirement: Start закрепляет полный вход исполнения
Перед admission Pri-Fly SHALL закреплять definition, dependencies, input refs,
policy и поддерживаемый execution profile. Изменение исходных файлов после
start MUST либо не влиять на sealed snapshot, либо приводить к явному отказу;
оно MUST не менять смысл уже начатого Run.

#### Scenario: Файл workflow меняется после start
- **WHEN** пользователь изменяет workflow или его dependency после start
- **THEN** исполняется закреплённый снимок либо Run останавливается с явной
  диагностикой drift

### Requirement: Результат шага проверяется до маршрутизации
Pri-Fly SHALL принимать StepResult только после проверки его identity, digest,
schema и обязательных outputs. Неполный или поддельный успех MUST не закрывать
StepInstance. Предметный verdict MUST быть отделён от технического failure:
необработанный verdict не превращается в `pass`, а известный технический
failure без результата не выдаётся за предметный отказ. Профиль MUST не
выполнять automatic retry.

#### Scenario: Некорректный success не закрывает шаг
- **WHEN** runner возвращает success с неверным attempt identity или missing
  output
- **THEN** Pri-Fly не публикует terminal success для StepInstance

### Requirement: Admission остаётся последовательным и объяснимым
Foundation profile SHALL использовать один foreground driver и не более одной
допущенной незавершённой Attempt на authority. Каждая команда MUST иметь
durable command identity, payload binding и CAS/epoch protection; повтор того
же command MUST возвращать прежний receipt, а другой payload с тем же identity
MUST конфликтовать без повторной работы. Неизвестная ранее допущенная работа
MUST удерживать slot и блокировать обычный новый admission.

#### Scenario: Конфликтующая повторная команда не запускает работу снова
- **WHEN** command identity повторяется с другим payload
- **THEN** Pri-Fly возвращает conflict без нового dispatch или частичного state

### Requirement: Stop является durable границей
Pause, cancel, explicit release stop и resume SHALL быть отдельными durable
commands. Pause MUST запрещать новые ordinary admissions; cancel MUST
инициировать остановку принадлежащего runner процесса по заявленной OS policy;
resume MUST не снимать stops и не перезапускать interrupted Attempt. Новый stop
между release и resume MUST блокировать продолжение. Terminal Run MUST не
возникать при active, pending или unknown obligations.

#### Scenario: Resume не снимает stop
- **WHEN** владелец вызывает resume без отдельного release stop
- **THEN** Pri-Fly не допускает следующую работу

### Requirement: Сбой не превращается в повторное действие
Pri-Fly SHALL durably записывать границы admission, dispatch, result и stop,
чтобы recovery отличал известный факт от неизвестного. Crash вокруг spawn,
после result commit, timeout с живым descendant, поздний или повторный result
MUST не приводить к blind retry, двойному dispatch, перезаписи принятого
результата или ложному terminal outcome. Recovery MUST открывать неизвестное
состояние только для безопасного выяснения, а не для обычного продолжения.

#### Scenario: Crash после dispatch оставляет безопасную неопределённость
- **WHEN** процесс мог быть запущен, но его факт не подтверждён после crash
- **THEN** Pri-Fly блокирует обычное продолжение вместо автоматического
  повторного запуска

### Requirement: Локальный cooperative profile не выдаётся за sandbox
Foundation profile SHALL называть доверенную границу `core-local` и
cooperative. Worker MUST не получать authority database handle или
административный API как обычный input, но профиль MUST не заявлять
managed-isolated execution, независимое human approval, per-tool enforcement,
полный учёт внешних effects или жёсткий денежный cap. `external_write` и
`destructive` MUST требовать отдельной квалификации; непроверяемая потребность
в hard cap MUST быть отвергнута как неподдержанная.

#### Scenario: Неквалифицированное опасное действие не допускается
- **WHEN** workflow требует destructive effect вне заявленного local scope
- **THEN** профиль отклоняет его вместо того, чтобы объявить локальный process
  контролируемым adapter

### Requirement: Hooks и измерения не меняют маршрут без явного контракта
Pri-Fly SHALL сохранять declared state/event publications и telemetry purpose
mappings как данные наблюдения. Публикация MUST не запускать скрытый следующий
stage, не менять terminal result и не подменять artifact/close publication.
Read views и telemetry MUST различать zero от отсутствующего measurement и
указывать source, unit, coverage и as-of; оценка расходов MUST не отображаться
как точный usage или hard cap без наблюдаемого meter.

#### Scenario: Отсутствующее измерение не выглядит нулём
- **WHEN** OS или provider measurement недоступен
- **THEN** query возвращает отсутствие с причиной, а не число `0`

### Requirement: Совместимость не меняет сохранённые факты
Pri-Fly SHALL различать protocol/schema version, definition revision,
execution semantics profile и storage/event version. Новый binary MUST до
dispatch проверять способность читать сохранённые факты и исполнять pinned
profile. Исторические events MUST не переписываться; deterministic reader или
upcast требует проверки. Downgrade, hot edit активного Run и запись в
неизвестное состояние MUST не обещаться автоматически.

#### Scenario: Новый binary не понимает pinned profile
- **WHEN** binary не поддерживает profile или storage события продолжаемого Run
- **THEN** он отказывает до dispatch и сохраняет Run для чтения/recovery

### Requirement: Расширение профиля сохраняет основания модели
Pri-Fly SHALL различать добавление предметного действия и нового правила
исполнения. Условие, child invocation, repeat, parallel, map, wait, managed
actions, retry и compensation MUST получать доверенную core-semantics,
durable state, admission/recovery и отдельную квалификацию; они MUST не
моделироваться shell-циклом, reset завершённого шага, blanket plugin или
увеличением parallelism. Расширение MUST сохранять identity, provenance, CAS,
command dedup, result identity, общий budget и stop semantics.

#### Scenario: Повтор создаёт новую работу
- **WHEN** будущий repeat запускает следующую итерацию
- **THEN** он создаёт новую invocation и новые activation/instance/attempt
  identities вместо очистки статуса прежнего step

### Requirement: Исторический пример имеет ограниченный доказательный scope
Комплект foundation example SHALL оставаться language-neutral regression fixture
с exact refs и digest, а его checker MUST проверять только явно объявленные
schema, refs, paths и negative cases. Положительный результат checker MUST не
означать implementation либо qualification YAML parser, hooks, telemetry,
privacy, OS process policy, recovery или общего runtime.

#### Scenario: Проверка примера успешно завершается
- **WHEN** checker подтверждает fixture и заявленные paths
- **THEN** report сохраняет ограниченный scope и не повышает статус runtime до
  implemented или qualified

### Requirement: Foundation qualification содержит полный заявленный каталог
Профиль SHALL хранить 24 отдельных сценария будущей foundation-квалификации со
статусом `specified_not_executed`. Каждый сценарий MUST оставаться проверяемой
обязанностью, а не evidence выполненной проверки; этот каталог MUST не
подменять полный продуктовый acceptance corpus.

#### Scenario: Независимые включения definition
- **WHEN** выполняется foundation qualification первого сценария
- **THEN** она проверяет независимые activation, instance, attempt и outputs

#### Scenario: Последовательность ожидает завершения процесса
- **WHEN** StepResult получен до завершения процесса
- **THEN** следующий dispatch ожидает settlement процесса и принятие результата

#### Scenario: Предметный отказ идёт только в разрешённый finish
- **WHEN** step выдаёт verdict fail
- **THEN** результат завершает только разрешённый rejected finish

#### Scenario: Неподдержанная возможность отвергается заранее
- **WHEN** definition использует future feature или unknown field
- **THEN** её отклоняют до spawn

#### Scenario: Невалидный граф или binding отвергается заранее
- **WHEN** definition содержит cycle, unreachable stage, missing target или
  output с непроходимого пути
- **THEN** validation отклоняет definition

#### Scenario: Файлы не дрейфуют после start
- **WHEN** source files изменены после start
- **THEN** snapshot либо явный refusal сохраняет исходный смысл исполнения

#### Scenario: Повтор команды сохраняет единственную работу
- **WHEN** одна command identity приходит повторно
- **THEN** она даёт прежний receipt, а несовместимый payload конфликтует

#### Scenario: Конкурирующая мутация не создаёт частичный state
- **WHEN** stale mutation соревнуется с актуальной
- **THEN** stale CAS отклоняется без частичных событий, reservation или refs

#### Scenario: Stale UI не обходит stop
- **WHEN** stop приходит от stale UI
- **THEN** restriction durable и новые admissions запрещены после commit

#### Scenario: Граница dispatch и stop видна в журнале
- **WHEN** stop соревнуется с dispatch
- **THEN** журнал объясняет, была ли работа уже допущена

#### Scenario: Release и resume не снимают новый stop
- **WHEN** между release и resume появляется новый stop
- **THEN** continuation блокируется

#### Scenario: Crash вокруг spawn не удваивает exec
- **WHEN** crash происходит около spawn boundary
- **THEN** обычное recovery не запускает process повторно

#### Scenario: Crash после commit не запускает шаг снова
- **WHEN** crash происходит после result commit
- **THEN** повтор ответа или recovery не повторяет step

#### Scenario: Неполный success не становится завершением
- **WHEN** result имеет неверные IDs, digest, schema или missing output
- **THEN** StepInstance не закрывается success

#### Scenario: Поздний result не переписывает принятый
- **WHEN** result приходит поздно или повторно
- **THEN** он не принимается за другую Attempt и не изменяет принятый result

#### Scenario: Изменение Run version не отменяет ownership Attempt
- **WHEN** Run version меняется во время шага
- **THEN** result проверяется по owner Attempt и актуальному CAS

#### Scenario: Timeout не изображает terminal cancel
- **WHEN** после timeout жив descendant
- **THEN** stop intent не объявляется terminal cancellation и workspace
  сохраняется

#### Scenario: Повреждённый blob не даёт успеха
- **WHEN** required blob отсутствует или повреждён
- **THEN** нет dispatch или terminal success с непроверенным artifact

#### Scenario: Replay не повторяет процессы
- **WHEN** state восстанавливается из журнала
- **THEN** projection совпадает с committed facts без нового process dispatch

#### Scenario: Exhaustion блокирует новый admission
- **WHEN** исчерпан budget или storage
- **THEN** следующий admission не создаётся и error не оставляет частичный
  success state

#### Scenario: Неизвестный profile отвергается до dispatch
- **WHEN** Run требует неподдержанный profile или version
- **THEN** Pri-Fly отказывает без интерпретации неизвестных events

#### Scenario: Cancel проверяется на заявленной OS
- **WHEN** выполняется cancel на поддержанной OS
- **THEN** qualification проверяет ownership process, его остановку или
  unknown outcome и отсутствие скрытого cleanup

#### Scenario: Terminal Run не имеет обязательств
- **WHEN** Run становится completed, failed или cancelled
- **THEN** в нём нет active, pending или unknown obligations

#### Scenario: Foundation workflow независим от исходного проекта
- **WHEN** выполняется foundation qualification независимости
- **THEN** цепочка проходит без Git, AI Factory, SMSPlace и AI credentials

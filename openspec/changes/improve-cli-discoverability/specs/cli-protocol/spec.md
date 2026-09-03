Authoritative source set: `openspec/specs/cli-protocol/spec.md` (перенесено).
Compatibility path: опубликованные контракты не получают новых обязательных
полей и новых значений enum; `safe_next_actions` уже свободный список строк, а
`description` — аннотация, не меняющая validation.

## MODIFIED Requirements

### Requirement: Problem и exit code сохраняют safe meaning

Problem MUST include stable code, message, correlation, violations and safe
next action without secrets or foreign-object detail. `retryable` describes
command/check retry only. CLI exit zero means read or command commit, not Run
success; typed result carries workflow state. Runtime refusal, поднятый со
stable code, MUST доходить до клиента под этим code независимо от того,
сопровождён ли он message; engine-authored detail такого отказа (port, path,
version) MUST сообщаться в `violations`. Только текст без stable code MUST
схлопываться в `invalid_input`, и такой текст MUST NOT попадать в ответ.

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

### Requirement: CLI exposes scoped commands without hidden mutation

CLI MUST provide typed installation, authoring, preview/start, observation,
control, correction, decision and export commands through safe JSON/file input.
Read-only `next`, explain and events do not dispatch; init/install/remove and
project-wide control use their own scopes, not Run CAS.

Форма вызова MUST быть читаема из самого инструмента и MUST NOT требовать
authority: запрос справки у любой команды, запрос версии и перечисление
доступных публичных контрактов MUST отвечать своей информацией, а не отказом
про ненайденный объект. Запрос справки по теме MUST возвращать только эту тему.
Имя публичного контракта MUST приниматься и в форме declared reference, под
которой он назван в handed задании. Установленный путь к binary MUST меняться
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

#### Scenario: Отказ называет неприменимую команду

- **WHEN** проверка графа запрошена для пути вне authority
- **THEN** отказ называет команду, которой проверяется авторская папка без
  создания Run

## ADDED Requirements

### Requirement: Handoff описывает, что требуется от host

Sealed handoff MUST быть самодостаточным описанием ожидаемого от host: каждая
закреплённая запись контекста MUST быть идентифицируема из самого bundle, без
опоры на порядок перечисления, а выходные слоты MUST быть разделены на те,
которые заполняет host, и те, которые движок закрывает сам объявленным
захватом. Host MUST NOT восстанавливать эту раскладку из содержимого файлов или
из прошлых прогонов.

#### Scenario: Bundle содержит несколько закреплённых записей контекста

- **WHEN** шагу закреплены skill и его bridge
- **THEN** host определяет, что есть что, по самому bundle, а не по порядку
  ссылок

#### Scenario: Часть выходов закрывается захватом

- **WHEN** шаг объявляет и обычный выход, и выход с привязкой workspace tree
- **THEN** handoff называет, какой слот host заполняет сам, а какой движок
  закрывает захватом

### Requirement: Read-only виды не занижают то, что движок держит

Read-only вид MUST называть то, что считает, и MUST NOT показывать нулевое
значение для состояния, которое движок держит. Сводка Run MUST различать
выходы Run и запечатанные выходы его шагов. Ожидание host MUST быть видно из
read-only вида как безопасное следующее действие. Команда обновления MUST
называть адрес, по которому проверяла release.

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

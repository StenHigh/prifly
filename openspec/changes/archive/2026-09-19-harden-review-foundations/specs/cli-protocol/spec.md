## MODIFIED Requirements

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

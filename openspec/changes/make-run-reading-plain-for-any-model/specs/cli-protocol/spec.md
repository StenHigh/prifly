## ADDED Requirements

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

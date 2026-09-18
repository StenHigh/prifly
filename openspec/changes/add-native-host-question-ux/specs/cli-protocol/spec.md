## MODIFIED Requirements

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

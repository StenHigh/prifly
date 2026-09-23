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

#### Scenario: Claude Code запускает общий проект
- **WHEN** developer вызывает установленный `.claude/skills/prifly-run`
- **THEN** он передаёт `claude-code` и не читает Codex root

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

## ADDED Requirements

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

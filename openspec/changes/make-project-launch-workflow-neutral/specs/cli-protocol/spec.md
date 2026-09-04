## MODIFIED Requirements

### Requirement: Project entry points select their host mechanically
`project init` SHALL создавать нейтральный profile `/3` в обычной папке без
обязательного Git и без AI skills. Host entry points SHALL добавляться только
явно выбранным поддержанным hosts; каждый передаёт свой identity, не угадывает
его по directory. Compile `/3` MUST требовать host лишь при чтении host-bound
source; `/2` сохраняет explicit host. Fresh init MUST отвергать unsafe root или конфликт runner без
перезаписи. Для valid existing profile после clone/copy init MUST создавать
только отсутствующую local configuration, сохраняя shared YAML и exact runners.
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

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

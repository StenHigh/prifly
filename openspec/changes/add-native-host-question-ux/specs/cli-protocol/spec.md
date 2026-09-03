## MODIFIED Requirements

### Requirement: Project entry points select their host mechanically
`prifly project init` SHALL записывать fixed repository-relative skills roots
для `codex-cli`, `codex-app` и `claude-code` и создавать один `prifly-run`
entry point внутри каждой соответствующей host directory. Каждый entry point
SHALL вызывать Project compilation со своим host identity. Public compile
command MUST требовать host identity и MUST NOT выводить его из существующих
directory. Fresh init MUST отвергать unsafe root или existing runner, не
перезаписывая runner или profile. Для valid tracked profile после clone init
MUST проверить exact runners и создать только отсутствующий ignored `local.yaml`;
он не переписывает shared profile или runner. Эти entry points поддерживают
Codex CLI, Codex app и Claude Code, не делая ни один из них Core dependency.

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
- **WHEN** developer вызывает `prifly-run` из `.claude/skills`
- **THEN** он компилирует с host `claude-code` и никогда не читает Codex skills
  root

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
- **WHEN** любой из трёх путей runner уже существует
- **THEN** fresh init возвращает safe diagnostic и не создаёт profile или другой runner

#### Scenario: Clone получает только local authority configuration
- **WHEN** repository уже содержит valid tracked Project profile и exact runners,
  но не содержит ignored `.prifly/local.yaml`
- **THEN** init создаёт только эту local configuration и не меняет shared YAML
  или host runners

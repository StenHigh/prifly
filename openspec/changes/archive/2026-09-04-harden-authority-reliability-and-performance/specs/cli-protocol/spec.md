Authoritative source set: `openspec/specs/cli-protocol/spec.md` (перенесено).
Compatibility path: существующие команды, exit codes и Problem contract не
меняются; добавляется одна typed команда.

## ADDED Requirements

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

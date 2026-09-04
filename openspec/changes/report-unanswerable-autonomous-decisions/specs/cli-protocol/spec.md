Authoritative source set: `openspec/specs/cli-protocol/spec.md`. Прежние поля
структурированного результата launch сохраняются.

## MODIFIED Requirements

### Requirement: Project launch принимает typed per-Run decision selection
`project start` MUST accept an explicit package-profile selection, typed
answers for declared preflight decision IDs, and, under a separate flag so the
phase is readable in the command itself, typed answers for declared runtime
decision IDs. Interactive host launch MUST
ask for an omitted required selection before compilation; non-interactive
launch MUST return a stable missing-decision diagnostic unless a sealed default
rule fills it. Structured launch result MUST name selected profile, decision
catalog digest and decision ledger reference. Под autonomous policy тот же
результат MUST дополнительно нести перечень применимых runtime-решений, которые
эта политика взять не сможет, с ID и причиной каждого; перечень MUST
присутствовать и пустым, чтобы его отсутствие не читалось как «нечего
сообщать».

#### Scenario: CLI запускает Ultra без изменения проекта
- **WHEN** developer supplies declared `ultra` package-profile to `project start`
- **THEN** CLI creates an Ultra Run and leaves all tracked workflow files
  unchanged

#### Scenario: Non-interactive launch не имеет обязательного выбора
- **WHEN** required preflight decision has neither explicit answer nor allowed
  default
- **THEN** CLI returns a stable diagnostic before compilation and Run creation

#### Scenario: Анкета устарела до запуска
- **WHEN** host передаёт catalog digest из questionnaire, а tracked catalog
  изменился до `project start`
- **THEN** CLI returns `project_start_stale_decision_catalog` before package,
  Workspace claim or Run creation

#### Scenario: Autonomous launch называет решения, которых политика не возьмёт
- **WHEN** владелец запускает Run под autonomous policy, а catalog объявляет
  применимое runtime-решение без разрешённого automatic selection
- **THEN** typed результат launch содержит его ID и причину, а Run создаётся

#### Scenario: Владелец предответил runtime-решение
- **WHEN** он передаёт `project start` typed ответ на объявленный runtime
  decision ID
- **THEN** CLI проверяет значение до создания Run, запечатывает его в decision
  sheet и не называет это решение среди тех, которых политика не возьмёт

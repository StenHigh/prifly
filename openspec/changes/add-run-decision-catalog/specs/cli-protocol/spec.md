## ADDED Requirements

### Requirement: Project launch принимает typed per-Run decision selection
`project start` MUST accept an explicit package-profile selection and typed
answers only for declared preflight decision IDs. Interactive host launch MUST
ask for an omitted required selection before compilation; non-interactive
launch MUST return a stable missing-decision diagnostic unless a sealed default
rule fills it. Structured launch result MUST name selected profile, decision
catalog digest and decision ledger reference.

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

### Requirement: CLI передаёт lifecycle runtime-решения versionedly
Public CLI MUST expose typed request, read and answer operations for a pending
decision. Compatible executor's request command MUST require its current
Attempt, envelope, declared decision ID and Run generation; CLI derives the
definition digest only from the sealed catalog, never from executor prose.
Answer command MUST require Run ID, decision ID, request/version identity and
typed value; it MUST report accepted, stale, conflict, schema-invalid or
not-pending result without hidden dispatch. Read output MUST be safe to render
by every supported host and contain no secret answer bytes outside authorized
scope.

#### Scenario: Пользователь отвечает из другого host
- **WHEN** second authorized host reads a pending decision and submits its
  current typed answer
- **THEN** CLI accepts it once and the first host can observe the same ledger
  transition after reconnect

#### Scenario: Совместимый adapter отправляет declared request
- **WHEN** adapter вызывает `run decision RUN_ID request` с identity текущего
  SessionTask и declared runtime ID
- **THEN** CLI передаёт ровно sealed definition в Universal Decision Bridge и
  не принимает digest либо вопрос, придуманный adapter-ом

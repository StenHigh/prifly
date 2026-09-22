## ADDED Requirements

### Requirement: Provenance continuation не подменяет lifecycle source Run
Run, созданный для continuation, MUST быть специализированным fork с
собственной Run identity, package lock, claims, admissions и lifecycle. Он
MUST сохранять exact source Run ID и ArtifactRevision refs в существующем fork
provenance, но MUST NOT наследовать source Run status,
активные права, grants, approvals, Attempts или позицию в graph. Accepted
предметный outcome source Run MUST не становиться технической ошибкой и не
давать права на `reopen`.
Source task, handoff и plan MUST быть проверены по exact refs и записаны в
typed inputs нового Run с provenance исходных ревизий. Claim текущего
workspace MUST быть привязан к continuation Run до первого read-only gate;
неудавшееся создание Run MUST не оставлять claim привязанным к нему.

#### Scenario: Source Run имеет accepted partial outcome

- **WHEN** owner создаёт continuation от completed partial source Run
- **THEN** source остаётся completed partial, а continuation получает новую
  identity и проходит собственные admission boundaries

#### Scenario: Старый Run не содержит нужный artifact

- **WHEN** source Run не имеет exact sealed task, handoff или plan, нужного
  continuation route
- **THEN** continuation отказывает с диагностикой отсутствующего provenance и
  не заменяет его похожим artifact из другого Run

#### Scenario: Raw fork не получает internal evidence

- **WHEN** оператор вызывает обычный `run fork` и указывает internal artifact
  source Run
- **THEN** runtime сохраняет существующее ограничение на root outputs и
  отказывает; только Project continuation может передать declared internal
  artifact в typed continuation input

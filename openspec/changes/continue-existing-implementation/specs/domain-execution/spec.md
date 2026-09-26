## ADDED Requirements

### Requirement: Provenance continuation не подменяет lifecycle source Run
Run, созданный для continuation, MUST быть специализированным fork с
собственной Run identity, package lock, admissions и lifecycle. Он MUST
сохранять exact source Run ID и ArtifactRevision refs в существующем fork
provenance, но MUST NOT наследовать source Run status, активные права, grants,
approvals, Attempts или позицию в graph. Claim рабочего дерева завершённого
source Run SHALL передаваться continuation целиком, если оператор не выбрал
новый claim от указанного commit. Accepted предметный outcome source Run MUST
не становиться технической ошибкой и не давать права на `reopen`. Перенесённые
входы MUST быть проверены по exact refs и записаны в typed inputs нового Run с
provenance исходных ревизий; неудавшееся создание Run MUST не оставлять claim
привязанным к нему.

#### Scenario: Source Run имеет accepted partial outcome
- **WHEN** owner создаёт continuation от completed partial source Run
- **THEN** source остаётся completed partial, а continuation получает новую
  identity и проходит собственные admission boundaries

#### Scenario: Старый Run не содержит объявленный artifact
- **WHEN** source Run не имеет exact sealed artifact в месте, которое объявил
  workflow продолжения
- **THEN** continuation отказывает с диагностикой отсутствующего источника и
  не заменяет его похожим artifact из другого Run

#### Scenario: Raw fork не получает internal evidence
- **WHEN** оператор вызывает обычный `run fork` и указывает internal artifact
  source Run
- **THEN** runtime сохраняет существующее ограничение на root outputs и
  отказывает; только объявленное продолжение передаёт internal artifact в
  typed continuation input

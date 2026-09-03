## ADDED Requirements

### Requirement: Каталог решений Run остаётся отдельной active работой
Единый current backlog MUST хранить `add-run-decision-catalog` как active
high-priority change до закрытия его OpenSpec tasks. Запись MUST называть
prerequisite: versioned Project launch, sealed package profile and durable
Run-state; следующий шаг: реализовать typed catalog, preflight selection,
wait/recovery и host/CLI evidence. Она MUST отделять этот scope от
`assisted-model-profile-protocol`, upstream AI Factory compatibility и live
pilot qualification.

#### Scenario: Команда выбирает следующую работу
- **WHEN** владелец читает current delivery backlog
- **THEN** он видит, что per-Run Fast/Full/Ultra и безопасный autonomous
decision policy требуют этой отдельной change, а не правки `extend.yaml`

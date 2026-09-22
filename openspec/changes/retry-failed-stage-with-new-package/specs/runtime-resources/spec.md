## ADDED Requirements

### Requirement: Новый пакет не подменяет lock технически failed Run

Recovery на исправленном пакете MUST создавать отдельный связанный Run с новым exact dependency closure и собственными state/event identities. Исходный Run продолжает читаться со своим закреплённым interpreter и техническим failure. Перенесённое evidence MUST ссылаться на source Run/version и exact ArtifactRefs; новая оценка candidate MUST быть отдельной от исторической оценки.

#### Scenario: Чтение source после восстановления
- **WHEN** новый Run принял ранее отвергнутый candidate по исправленной схеме
- **THEN** source Run по-прежнему сообщает прежний `invalid_output`, а новый Run показывает новую приёмку и её происхождение

### Requirement: Recovery не наследует открытые обязательства

Новый Run MUST NOT наследовать активную Attempt, ExecutionAdmission, ActionAdmission, Stop, Grant, Approval или WorktreeClaim source Run. При unresolved obligation либо невозможности доказать остановку исполнителя создание нового Run MUST отказывать до любых эффектов.

#### Scenario: Неурегулированный процесс
- **WHEN** судьба прежнего процесса или действия остаётся uncertain
- **THEN** recovery не допускает повтор работы и требует обычного выяснения исхода

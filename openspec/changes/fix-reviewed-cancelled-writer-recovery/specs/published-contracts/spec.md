## ADDED Requirements

### Requirement: Resolved cancellation использует существующие immutable facts

Reviewed recovery после отмены writer SHALL читать existing cancellation Stops, flags, `attempt.resolved` journal и resolution diagnostics без расширения historical closed shapes. Recovery plan/provenance SHALL использовать existing execute action; resolution MUST NOT публиковаться как accepted StepResult или revalidated candidate. Historical schema bundles и release evidence MUST оставаться неизменными.

Current source set — `openspec/specs/published-contracts/spec.md`; ownership сохраняется. Исправление eligibility использует совместимые existing plan/provenance contracts, не вводя новую WorkflowRevision или authoring field.

#### Scenario: Старый source прочитан новым planner
- **WHEN** source записан прежним поддерживаемым binary с отменой и owner resolution
- **THEN** planner проверяет сохранённые immutable facts и может подготовить execute recovery без изменения source или его contract version

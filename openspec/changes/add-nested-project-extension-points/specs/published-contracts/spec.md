## ADDED Requirements

### Requirement: Nested extension points use explicit authoring revisions
Editor contracts SHALL публиковать отдельные closed forms `prifly-extension-points/1` и `prifly-extension/4` и capability `project_extension_points`. Legacy extension-v1/v2, WorkflowRevision и saved Run contracts MUST сохранять прежние bytes и semantics. Новая возможность SHALL иметь author index, authoring reference и runnable generic example с минимальными native revisions.

#### Scenario: Old package is compiled again
- **WHEN** folder не содержит point sidecar и extend использует прежний syntax
- **THEN** compiled definitions и build identity сохраняют прежнюю форму inputs

#### Scenario: New example is qualified locally
- **WHEN** bounded native fixture выполняется на candidate binary
- **THEN** evidence отличает source qualification от pending released-binary и consumer acceptance

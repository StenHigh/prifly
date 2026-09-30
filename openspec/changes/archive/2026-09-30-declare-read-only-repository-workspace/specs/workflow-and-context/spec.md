## ADDED Requirements

### Requirement: Ассистируемый шаг явно запрашивает Git Workspace только для чтения

Ассистируемый step MAY объявить `repository_workspace: read_only` независимо от `effects.class`. Эта декларация MUST требовать выбранный Git Workspace для Run и MUST опускаться из `prifly-step/2` в отдельный StepDefinition v13 без изменения старых definitions. Она MUST NOT разрешать изменение дерева. Программный step, старый authoring marker и более ранние StepDefinition MUST отказывать новому полю. Шаг без такой декларации MUST NOT получать claim только потому, что объявил внешний эффект.

#### Scenario: Внешний эффект и чтение репозитория независимы
- **WHEN** ассистируемый step объявляет `external_write` и `repository_workspace: read_only`
- **THEN** ему нужен Git Workspace для чтения, а разрешённая внешняя операция остаётся ограниченной отдельной границей

#### Scenario: Внешняя задача без Git
- **WHEN** ассистируемый step объявляет внешнюю запись без `repository_workspace`
- **THEN** Project launch не требует Git Workspace из-за одного только внешнего эффекта

#### Scenario: Исторический контракт не расширен
- **WHEN** новое поле подано под старым marker или StepDefinition до v13
- **THEN** compile отказывает до Run, не дописывая поле в старую форму

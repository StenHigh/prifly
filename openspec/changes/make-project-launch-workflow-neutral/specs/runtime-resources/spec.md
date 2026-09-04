## ADDED Requirements

### Requirement: Execution bindings запуска отделены от обычных inputs
Versioned запуск SHALL принимать разрешённые владельцем execution bindings
для exact StepDefinition и CheckDefinition refs выбранного closure отдельно
от typed workflow inputs. Preview, validation и Start MUST использовать один
проверенный набор bindings; executable bytes, argv, supporting files и context
configuration MUST закрепляться прежним механизмом Run lock до dispatch.
Bindings MUST NOT временно или постоянно менять общую конфигурацию authority,
выбирать чужие definitions либо давать новые effects/permissions. Старый путь
без явных bindings MUST сохранять опубликованное разрешение через конфигурацию.
Файлы package MUST читаться confined и закрепляться; никакое package file
MUST NOT исполняться во время obtain, validation или compilation.

#### Scenario: Два запуска используют разные программы
- **WHEN** два packages или две версии step запускаются в одной authority с
  разными разрешёнными execution bindings
- **THEN** каждый Run использует собственные pinned executors после restart,
  а общая authority configuration не изменяется

#### Scenario: Повтор команды меняет executable binding
- **WHEN** прежний command ID повторён с другим binding
- **THEN** система сообщает конфликт команды, не создаёт второй Run и не
  изменяет executor первого

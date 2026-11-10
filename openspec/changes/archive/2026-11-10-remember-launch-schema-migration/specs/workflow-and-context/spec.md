Текущий source set — `openspec/specs/workflow-and-context/spec.md`. Ownership не меняется.
До archive действует эта delta. Сохранённые Runs и прежние bundles сохраняют смысл.

## MODIFIED Requirements

### Requirement: Uninstall и revocation сохраняют историю
Uninstall MUST закрывать package для новых selection, но не удалять bytes,
pins, receipts, effects или evidence живого/сохраняемого Run. Security
revocation MUST блокировать новые admissions и trusted reuse, сохраняя
наблюдения прошлых effects и честную unavailable history.

Явная миграция проверенных общих schemas SHALL допускать ограниченное исключение
для нового Run. Исключение MUST связывать точный consumer package, schema refs,
выбранных доверенных поставщиков и покрытые записи отзыва. Оно MUST NOT
восстанавливать revoked package, менять старые Runs или допускать executable content.
Без отдельного решения и выбора этого решения при Start прежний запрет MUST сохраняться.
Явный разовый flag либо действующий сохранённый выбор владельца для точной сборки SHALL задавать такой выбор.

#### Scenario: Package отозван во время Run
- **WHEN** следующий admission требует отозванный package
- **THEN** он блокируется, а уже наблюдаемый external effect не исчезает

#### Scenario: Новый consumer выбирает одобренную миграцию schemas
- **WHEN** новый Run явно выбирает действующее решение для своих точных schemas и consumer package
- **THEN** движок закрепляет выбранных поставщиков и происхождение допуска, сохраняя исходный revoked package и pins старых Runs

### Requirement: Допуск схем нового Run имеет собственное происхождение
Новый Run с выбранной миграцией SHALL закреплять решение, точный consumer,
schema refs и конкретных поставщиков. Движок MUST проверять эти bindings при
Start и каждом последующем admission, включая вложенную работу.
Решение MUST NOT менять supplier произвольно или распространяться на другой consumer revision.
Старый Run без такого binding MUST сохранять прежний запрет даже при одинаковом schema Ref.
Решение MUST NOT автоматически делать результаты старого revoked Run пригодными для trusted reuse.

#### Scenario: Новый Run продолжает работу после Start
- **WHEN** новый consumer с действующим binding завершает первый шаг и запрашивает следующий
- **THEN** admission использует закреплённое решение и допускает работу при сохранении остальных условий

#### Scenario: Старый Run содержит тот же schema Ref
- **WHEN** старый Run запрашивает admission после применения миграции для нового consumer
- **THEN** прежний revoked pin продолжает блокировать работу; его history и claims не переписываются

#### Scenario: Start не выбирает решение
- **WHEN** consumer запускается без разового flag и без сохранённого выбора владельца после успешного apply
- **THEN** прежняя revocation policy продолжает действовать

## ADDED Requirements

### Requirement: Сохранённый выбор относится к точному project launch
Владелец SHALL сохранять выбор существующего SchemaMigrationDecision для одного проекта и launch в одной authority.
Выбор MUST закреплять build key, exact consumer package Ref и exact workflow Ref.
Отсутствие записи MUST сохранять прежний admission.
Установка, update, compile, migration apply, prepare и start MUST NOT самостоятельно создавать либо менять выбор.

#### Scenario: Одна сборка запускается повторно
- **WHEN** владелец сохранил действующее решение и повторяет prepare и start того же launch без разового flag
- **THEN** обе команды выбирают это решение; каждый новый Run закрепляет собственный SchemaMigrationBinding

#### Scenario: Другой launch использует тот же package
- **WHEN** другой launch запускает даже тот же workflow из той же сборки
- **THEN** движок не использует выбор первого launch

#### Scenario: Сборка изменилась
- **WHEN** изменились build key, consumer Ref либо workflow Ref сохранённого launch
- **THEN** движок отказывает до создания Run с причиной и маршрутом проверки; другое разрешение автоматически не выбирается

#### Scenario: Новая authority не содержит выбора
- **WHEN** те же package bytes и project files открывают в другой authority
- **THEN** прежняя authority не передаёт туда выбор или право допуска

#### Scenario: Пакет содержит executable content
- **WHEN** сохранённый выбор сопровождает запуск, требующий отозванный executable ref
- **THEN** прежний запрет code сохраняется; выбор разрешает только schemas существующего решения

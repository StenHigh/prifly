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
Без отдельного решения и явного выбора этого решения при Start прежний запрет MUST сохраняться.

#### Scenario: Package отозван во время Run
- **WHEN** следующий admission требует отозванный package
- **THEN** он блокируется, а уже наблюдаемый external effect не исчезает

#### Scenario: Новый consumer выбирает одобренную миграцию schemas
- **WHEN** новый Run явно выбирает действующее решение для своих точных schemas и consumer package
- **THEN** движок закрепляет выбранных поставщиков и происхождение допуска, сохраняя исходный revoked package и pins старых Runs

## ADDED Requirements

### Requirement: План миграции показывает полный schema closure
Read-only prepare SHALL проверять установленный точный consumer package и весь
его dependency closure. План MUST показывать все затронутые schema refs, все
отозванные packages, их status/reason и доступное происхождение lifecycle.
План MUST различать первоначальный trust reason и последующий status reason.
Для каждой схемы план MUST показывать выбранного точного поставщика либо отсутствие выбора.
Поставщик MUST находиться в объявленном closure consumer, иметь проверенные bytes и разрешаемые доверенные зависимости.
План MUST перечислять затронутые текущие и сохранённые Runs и объяснять сохранение их прежних ограничений.
Неполный или превышающий опубликованный предел scan MUST NOT выдавать применимый план.

#### Scenario: Старый пакет содержит несколько общих schemas
- **WHEN** consumer требует несколько schemas, которые объявлял revoked package
- **THEN** план перечисляет все такие refs и состояние покрытия каждого, включая refs после первого отказа Start

#### Scenario: Один schema Ref объявляли два revoked пакета
- **WHEN** два сохранённых revoked пакета объявляли одну требуемую схему
- **THEN** план показывает обе записи отзыва; применимое решение покрывает обе либо prepare сообщает неполное покрытие

#### Scenario: Владелец ещё не выбрал поставщиков
- **WHEN** prepare получает consumer без полного списка выбранных поставщиков схем
- **THEN** он возвращает неприменимый диагностический план с кандидатами и причинами; trust, Runs и claims не меняются

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
- **WHEN** consumer запускается без явного выбора решения после успешного apply
- **THEN** прежняя revocation policy продолжает действовать

### Requirement: Миграция не расширяет исполняемый scope
Разрешение SHALL относиться только к проверенным компонентам вида schema.
Workflow, step, adapter, context, executable bytes и непроверенные refs MUST
сохранять прежние trust checks. Отличающаяся version или digest схемы MUST
требовать нового проверенного плана. Обычный import, alias и повторная компиляция MUST NOT создавать решение миграции.

#### Scenario: Старый package также содержит code
- **WHEN** новый consumer требует executable ref из revoked package наряду с одобренной общей схемой
- **THEN** Start отказывает до создания Run; разрешение схемы не допускает code

#### Scenario: Неизвестный schema Ref добавлен после prepare
- **WHEN** выбранный consumer или его closure требует schema Ref вне одобренного набора
- **THEN** движок не распространяет исключение на этот Ref и возвращает именованный отказ

Источник — `openspec/specs/workflow-and-context/spec.md`; ownership не меняется.
Прежние package contracts и Runs сохраняют смысл. Новые bindings используют
отдельную opt-in version boundary из delta `published-contracts`.

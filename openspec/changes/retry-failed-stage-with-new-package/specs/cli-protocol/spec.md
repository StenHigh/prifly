## ADDED Requirements

### Requirement: Project CLI проверяемо готовит восстановление failed Run

CLI SHALL предоставлять read-only `project recover --prepare` с exact source Run/version, target compiled package, деревом исходного Run, которое будет передано, последним принятым checkpoint исходного Run, если workflow его объявил, планом reuse/candidate/re-execution, отказами и review digest. Mutating `project recover` MUST принимать тот же digest и повторно проверять source, package, права и передаваемое дерево до Run creation; executable effects требуют прежнего явного разрешения. CLI MUST NOT выбирать дерево по содержимому артефактов или по имени стадии. Machine-readable итог MUST отличать новый Run от исходного и называть точку продолжения.

#### Scenario: Оператор видит стоимость до запуска
- **WHEN** владелец готовит восстановление Run, технически упавшего на поздней стадии после двух принятых
- **THEN** CLI заранее показывает, что две принятые стадии будут reused, а candidate упавшей стадии будет revalidated или её потребуется выполнить снова, с причиной каждого решения

#### Scenario: Stale prepare
- **WHEN** package, source Run или передаваемое дерево (его claim или generation) изменились после `--prepare`
- **THEN** start отказывает по stale digest до создания Run

### Requirement: Чтение Run различает новое исполнение и reuse

Versioned run view and events MUST показывать source provenance, exact ссылки reused evidence и новую оценку candidate отдельно от реально исполненных стадий. Старые версии DTO и сохранённые Runs MUST сохранять прежние поля и смысл; новый reader MUST не утверждать, что перенесённая стадия была повторно выполнена.

#### Scenario: Просмотр восстановленного Run
- **WHEN** клиент читает новый Run после принятия перенесённой стадии
- **THEN** он видит `reused from <source Run/Attempt>` и новый статус маршрута, а не новую Attempt этой стадии

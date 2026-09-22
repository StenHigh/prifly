## ADDED Requirements

### Requirement: Project CLI проверяемо готовит восстановление failed Run

CLI SHALL предоставлять read-only `project recover --prepare` с exact source Run/version, target compiled package, Git subject, планом reuse/candidate/re-execution, отказами и review digest. Mutating `project recover` MUST принимать тот же digest и повторно проверять source, package, права и Git до claim и Run creation; executable effects требуют прежнего явного разрешения. Machine-readable итог MUST отличать новый Run от исходного и называть точку продолжения.

#### Scenario: Оператор видит стоимость до запуска
- **WHEN** владелец готовит восстановление Run, упавшего на tests
- **THEN** CLI заранее показывает, что `verify` и `review` будут reused, а tests candidate будет revalidated или tests потребуется выполнить снова, с причиной каждого решения

#### Scenario: Stale prepare
- **WHEN** Git tree, package или source Run изменился после `--prepare`
- **THEN** start отказывает по stale digest до создания Run

### Requirement: Чтение Run различает новое исполнение и reuse

Versioned run view and events MUST показывать source provenance, exact ссылки reused evidence и новую оценку candidate отдельно от реально исполненных стадий. Старые версии DTO и сохранённые Runs MUST сохранять прежние поля и смысл; новый reader MUST не утверждать, что перенесённая стадия была повторно выполнена.

#### Scenario: Просмотр восстановленного Run
- **WHEN** клиент читает новый Run после принятия перенесённого review
- **THEN** он видит `reused from <source Run/Attempt>` и новый статус маршрута, а не новую review Attempt

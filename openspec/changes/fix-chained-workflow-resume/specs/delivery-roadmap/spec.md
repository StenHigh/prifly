## ADDED Requirements

### Requirement: Исправление цепочки возобновлений имеет отдельную границу приёмки

Delivery backlog SHALL учитывать `fix-chained-workflow-resume` как приоритетное исправление блокирующего дефекта существующего пути. Prerequisite — доступные sealed source Runs и доказательства префикса, существующие recovery/resumable contracts и текущий claim. Следующий шаг — воспроизведение цепочки без ИИ, исправление общего planner и выпуск проверенного бинарника. Прогресс SHALL храниться в tasks этого change; закрытие MUST различать инженерную регрессию, поставку и фактическое продолжение пилота SMSPlace #163.

Проверка документа или зелёный fixture MUST NOT объявлять SMSPlace завершённым, закрывать независимый `retry-failed-stage-with-new-package` либо формальную приёмку P1/P2. Живой pilot SHALL использовать read-only prepare от текущего владельца дерева и отдельное проверенное разрешение start; evidence MUST называть installed build, Run IDs, transferred claim и фактический outcome.

Current source set остаётся `openspec/specs/delivery-roadmap/spec.md` с delta этого change до sync; historical release records не изменяются.

#### Scenario: Инженерный фикс готов, пилот ещё не продолжен
- **WHEN** регрессионная цепочка прошла, но установленный бинарник или живой Run ещё не проверены
- **THEN** delivery record сохраняет незавершённую поставку/приёмку и не объявляет бизнес-задачу выполненной

При sync в текущую таблицу «Очередь дальнейшей поставки» SHALL добавляться
одна строка: «Цепочка восстановления/возобновления» →
`fix-chained-workflow-resume/tasks.md`; prerequisite — immutable source ancestry,
доказательства принятого пути и текущий source claim; следующий шаг после
инженерных gates — owner merge, qualify/release и read-only prepare SMSPlace B.
Счётчик берётся из tasks на момент sync. Строка «Продолжение и recovery» для
`retry-failed-stage-with-new-package` сохраняется: этот change не является
приёмкой его оставшихся задач и не изменяет historical evidence.

Authoritative source set: `openspec/specs/domain-execution/spec.md`. Поведение
проверки не меняется; называется то, по каким часам она выполняется.

## MODIFIED Requirements

### Requirement: Время приходит от доверенного источника
Reducer MUST принимать recorded event time, а не читать wall clock. Trusted
authority or clock adapter MUST назначать admission/deadline time; worker
timestamp MUST не продлевать право. Public timestamps MUST быть UTC с `Z`, а
source offset/timezone MAY сохраняться отдельно.

Live timeout MUST использовать monotonic duration в пределах одного boot,
после restart MUST иметь declared conversion. Clock rollback, skew или
неопределённое current time MUST блокировать чувствительный admission до
восстановления.

Проверка срока, пересекающая границу процесса, MUST выполняться по локальному
настенному времени authority: монотонное показание принадлежит сессии часов
своего процесса и за его пределами не сравнимо. Норма MUST называть это прямо.
Пометка неквалифицированного настенного времени MUST описывать непригодность
величины для сравнения между машинами, а не запрет использовать её внутри одной
authority; защита от обратного хода часов MUST сохраняться и отказывать
явно.

#### Scenario: Host clock откатывается
- **WHEN** approval или deadline проверяется после clock rollback
- **THEN** старое client time не продлевает его и Core возвращает безопасный
  отказ при невозможности доверенного сравнения

#### Scenario: Отчёт принимается другим процессом
- **WHEN** отчёт приходит в процесс, чья сессия часов отличается от выдавшей
  задание
- **THEN** срок проверяется по настенному времени authority, и норма называет
  это ограничением сравнения, а не скрытым доверием к непроверяемой величине

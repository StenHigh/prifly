Authoritative source set:
`openspec/specs/observability-publication-reactions/spec.md` (перенесено).
Compatibility path: shape read view не меняется — поле `summary` в нём уже
объявлено; меняется только то, чем оно заполнено.

## MODIFIED Requirements

### Requirement: Read view имеет точный subject и согласованный cut
Read surface SHALL различать workflow definition и исполняемые Run,
invocation, activation, step и attempt. Один authority snapshot MUST читать
согласованный cut; внешний источник MUST показывать собственные version и
freshness без обещания глобальной атомарности.

Read view MUST нести то, что worker сообщил о собственной работе: accepted
summary attempt и report сохранившегося check MUST доходить до владельца
authority целиком. Пустое значение MUST означать, что worker ничего не
написал, и MUST NOT использоваться для скрытого содержимого. Executable
arguments, environment, raw definitions, envelope, candidate bytes и
credentials MUST NOT попадать в read view. Поскольку reported text не
проверяется authority, read view — чтение владельца, а не документ для
пересылки: telemetry и export сохраняют собственные границы и MUST NOT
включать reported text.

#### Scenario: Cross-run cut и независимая authority
- **Этап:** `P2-12`; **Статус:** `specified_not_executed`; **Вид проверки:** `runtime_integration`.
- **WHEN** Выполнить query и guard decision.
- **THEN** Первые читаются согласованно; внешний имеет отдельную версию/freshness, права проверены; global atomic snapshot не заявлен.
- **Контекст:** Два subjects одной authority и третий внешний.

#### Scenario: Владелец читает summary принятого результата
- **WHEN** worker сообщил summary и его результат принят
- **THEN** read view несёт этот текст целиком, а пустое значение остаётся
  признаком того, что worker ничего не написал

#### Scenario: Telemetry не повторяет reported text
- **WHEN** тот же Run попадает в telemetry report
- **THEN** reported summary и limitations в него не входят

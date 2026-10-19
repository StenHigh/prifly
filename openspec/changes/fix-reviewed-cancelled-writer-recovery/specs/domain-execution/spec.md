## ADDED Requirements

### Requirement: Reviewed recovery отменённого writer требует доказанной резолюции

Authority SHALL разрешать новый Run через existing recovery после cancellation и inspected owner resolution writer, если source failed без outcome, settled, не держит активной работы или неизвестных effects и имеет единственную доказанную failure chain. Сохранённые cancellation flags и cancel Stops SHALL оставаться фактами source; они MUST NOT переноситься в новый Run либо очищаться в source. Иные активные Stops MUST сохранять `recover_source_unsettled`.

Owner resolution SHALL подтверждаться связанными journal и diagnostic facts конкретной Attempt после относящегося к ней запроса отмены. Один `resolved_*` diagnostic без journal MUST NOT открывать повтор. Проверка SHALL охватывать вложенные call/repeat; повтор начинается с корневой стадии целиком. `applied` и `not_applied` MUST NOT становиться успешным StepResult. Frontier и все последующие quality stages SHALL исполняться заново; effective prefix mismatch SHALL сохранять именованный отказ. Повторная отмена и цепочка recovery SHALL проверяться на тех же основаниях.

Current source set — `openspec/specs/domain-execution/spec.md`; ownership не меняется. Существующие сохранённые facts читаются без изменения их схем или смысла.

#### Scenario: Consumer отменён до actor write
- **WHEN** owner остановил writer, осмотрел effects, выполнил `resolve not_applied` и source имеет доказанный accepted prefix
- **THEN** reviewed recovery повторяет writer и quality в исходном claim, сохраняя original inputs/base и decisions

#### Scenario: Producer успел оставить partial bytes
- **WHEN** owner выполнил `resolve applied` после осмотра незавершённого PLAN
- **THEN** resolution допускает только проверку recovery; partial bytes не переносятся как принятый output и не дают успешного writer result

#### Scenario: Неизвестность или посторонний stop остались
- **WHEN** effects не разрешены либо source держит активный pause/hold
- **THEN** prepare отказывает `recover_source_unsettled` без создания Run и изменения bytes

#### Scenario: Вложенный writer и повторная отмена
- **WHEN** resolved writer находится в call/repeat либо уже созданный recovery Run отменён снова
- **THEN** planner заново проверяет единственную failure chain и резолюцию текущего source, сохраняя только доказанный prefix

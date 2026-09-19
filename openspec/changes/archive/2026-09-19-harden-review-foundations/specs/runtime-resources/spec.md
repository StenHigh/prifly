## MODIFIED Requirements

### Requirement: Budgets и backpressure учитывают весь admission

До admission MUST резервироваться limits slots, attempts, capacity, execution,
context/output/storage и enforceable cost. Reserve, transfer, settlement и
release идемпотентны и имеют exact provenance; child invocation не создаёт
новый budget. Unknown effect/usage сохраняет reserve до known outcome.
Measured, provider-reported, reserved и estimated значения различаются, а
missing value — null с причиной. Queue или disk backpressure останавливает
ordinary admission, оставляя bounded recovery reserve.

Необязательная диагностика, записываемая внутри команды (её собственные
sample-записи), подчиняется той же allowance: если пакет diagnostics не
помещается, команда MUST коммититься, а её пакет MUST быть откачен целиком
и не оставлять частичных записей. Отказ пакета телеметрии MUST NOT
превращаться в отказ или подмену исхода самой команды.

#### Scenario: Provider timeout не сообщает стоимость

- **WHEN** external call timed out без usage receipt
- **THEN** cost reserve не освобождается и soft estimate не выдаётся за hard cap

#### Scenario: Диагностика команды не помещается в allowance

- **WHEN** допустимая команда завершилась, её диагностический пакет пересёк
  soft limit хранилища
- **THEN** команда коммитится со своим исходом, ни одна запись её пакета не
  сохраняется, и отказ телеметрии назван в её own telemetry, а не скрыт

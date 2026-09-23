## ADDED Requirements

### Requirement: Ожидание решения durable и восстанавливаемо
Runtime MUST persist decision request, allowed response contract, originating
Attempt/Step identity, decision ledger transition и wait reason atomically.
Pending decision MUST prevent its step from speculative retry or successor
selection. Recovery, reconnect and host replacement MUST rehydrate the same
pending request; they MUST NOT redispatch a prior external effect merely to
ask the question again.

#### Scenario: Host завершился во время вопроса
- **WHEN** authority restarts with a pending decision
- **THEN** observation возвращает тот же stable decision ID and request digest,
  а Run остаётся waiting до typed answer, stop или declared refusal

#### Scenario: Два клиента отвечают одновременно
- **WHEN** две valid answer commands используют одну expected Run generation
- **THEN** atomically принимается только одна, а вторая получает conflict без
  второй передачи значения worker-у


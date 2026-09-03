# Action admission атомарно consumes approval с Run ledger

## Контекст

Exact action intent can be durable, but approval cannot be a separate
preliminary operation because revoke or concurrent command could split it from
the admitted action.

## Решение

Admission rechecks active Attempt, current handoff, controls, retained
descriptor, selected approval and Run CAS, then commits admission and approval
consumption in one authority transaction. Receipt-only retries cannot run the
mutation callback.

## Последствия

There is no half-admission: rejection records an appropriate outcome while
infrastructure error creates no committed claim. Admission is a durable
decision, not adapter dispatch or evidence of external effect.

## Пересмотр

Grant, delivery, remote executor or new approval class requires a compatible
versioned admission contract.

## Не входит

Нет credential, ActionDelivery, receipt, target dispatch, effect status, retry
или reconciliation.

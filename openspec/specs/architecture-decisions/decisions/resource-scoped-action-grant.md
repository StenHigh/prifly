# Resource-scoped action grant consumes atomically

## Контекст

Some declared action may use a grant, but it must never widen from a named
resource to arbitrary target or bypass a separately required approval.

## Решение

Action admission accepts one grant with explicit action capability and exact
resource identities. Target must literally match its allowed resource set;
grant, required approval and admission are consumed in one authority commit.

## Последствия

Failed scope check changes neither Run nor grant accounting. Grant complements
rather than replaces approval, and earlier contracts retain their narrower
proposal-only or approval-only behaviour.

## Пересмотр

Multiple grants, broader capability or new resource identity requires a
versioned composition and policy decision.

## Не входит

Нет external delivery, credential, adapter dispatch, receipt, retry,
reconciliation или remote execution.

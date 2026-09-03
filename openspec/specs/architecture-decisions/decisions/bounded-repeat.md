# Ограниченный repeat и состояние итераций

## Контекст

Iteration должна улучшать или проверять current state without becoming an
unbounded retry loop that loses previous effects or human decisions.

## Решение

Repeat declares finite iteration cap, body workflow, exit decision and explicit
initial/next bindings. Every accepted iteration and control transition is
durable; delivery retry and whole-step retry preserve their own identities.

## Последствия

The next iteration reads the declared output of the previous one, counters do
not reset after restart, and exhaustion produces a bounded human-facing
decision rather than perpetual automation.

## Пересмотр

New repeat outcome, body scope or replay mechanism requires a versioned
contract and proof that it preserves iteration/accounting semantics.

## Не входит

Нет implicit retry-until-success, unbounded recursion или automatic acceptance
of a model proposal.

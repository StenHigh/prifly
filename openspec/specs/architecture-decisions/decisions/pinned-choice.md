# Выбор ветви по закреплённым данным

## Контекст

Conditional workflow должен принимать воспроизводимое решение, не зависящее от
model prose, live reads или неявно меняющегося порядка данных.

## Решение

Choice evaluates a bounded typed predicate over sealed inputs with declared
missing, null, type-error and unknown semantics. Branch ordering, exclusive
ambiguity, default path and proof of predicate inputs are part of the pinned
workflow definition.

## Последствия

Replay explains the accepted fact and selected route. Missing or unknown data
follows the declared path; it does not silently select a convenient branch.
New predicate features require a versioned grammar and validation.

## Пересмотр

Any alternative evaluator must preserve bounded execution, typed values,
deterministic traceability and current authority checks.

## Не входит

Нет eval, shell condition, model confidence routing или live external lookup
inside the reducer.

# Ранний artifact становится publication после authority commit

## Контекст

Consumer may need sealed producer output before terminal result, but workspace
path or a file-ready message does not prove immutable bytes or readable state.

## Решение

An artifact hook declares type, cardinality, policy and bounds. Authority reads
the confined candidate, seals its own copy, rechecks current generation and
controls, then atomically records durable publication and receipt. Logical item
identity is stable per producer step, hook and key.

## Последствия

Subscriber/readiness surfaces trust only committed publication. Blob metadata
may exist before a rejected transaction, but it is an orphan rather than a
ready item. Final StepResult and stage output remain separate contracts.

## Пересмотр

New artifact kind, check policy, retention or consumer semantics needs a
versioned hook contract and admission evidence.

## Не входит

Нет automatic subscriber wakeup, generic stream, implicit content check,
retention repair или producer completion.

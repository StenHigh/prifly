# Per-item checks удерживают sealed candidate до publication

## Контекст

Early artifact bytes may have valid shape but require declared checks before a
subscriber can trust them as published input.

## Решение

Authority seals candidate and records a pending publication with exact check
identities. Required checks run as bounded admissions over that sealed subject.
Only all-pass result rechecks current controls, attaches evidence and commits
publication with any declared assignments.

## Последствия

Fail or inconclusive result leaves a diagnostic and no publication/delivery; it
does not rewrite producer verdict or repair data. Retry never rereads mutable
workspace bytes while the pending candidate remains identified.

## Пересмотр

New check class, waiver policy, artifact type or scheduler guarantee requires a
versioned boundary and qualification of actual concurrency.

## Не входит

Нет general artifact-check framework, automatic repair/retry, blob policy,
generic subscription, backpressure or physical-overlap claim.

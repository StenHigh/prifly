# Закрытие keyed-many hook требует exact manifest

## Контекст

A finite subscription needs a provable end; silence, timeout and terminal
producer state cannot be treated as successful end-of-stream.

## Решение

Close accepts an ordered manifest whose membership exactly matches all accepted
logical items of one producer hook. Authority seals the manifest, rechecks
generation, controls and prior closure, then commits one immutable closure.

## Последствия

An item either belongs to the manifest or loses the race to close. Closure does
not finish producer or advance workflow frontier by itself; future retention
must pin manifest members rather than infer them from generic provenance.

## Пересмотр

Subscription cursor, assignment, consumer activation or a different close
policy requires its own durable identity and delivery contract.

## Не входит

Нет count-only close, silent truncation, generic stream support, automatic
consumer wakeup or implied garbage collection policy.

# Terminal producer failure прерывает subscriber отдельно от timeout

## Контекст

Subscriber should not wait until deadline when authority already knows its
declared producer failed, but failure, timeout, cancellation and unknown state
must keep distinct meanings.

## Решение

Publication source may declare terminal-failure interruption. Authority writes
a durable interrupted wait or assignment with a producer-failure reason, then
the consumer follows its declared failure/recovery route.

## Последствия

Deadline retains its own expiration semantics, producer recovery before terminal
failure does not interrupt subscriber, and fan-out waits for each sibling's
declared route instead of forcing parent completion.

## Пересмотр

New failure kind, consumer retry or final-dependent policy needs a versioned
delivery contract.

## Не входит

Нет compensation, treating cancel/unknown as failure, generic external source,
blob delivery, backpressure or retention policy.

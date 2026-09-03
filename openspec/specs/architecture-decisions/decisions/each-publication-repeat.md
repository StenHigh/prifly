# Each-publication реализуется bounded repeat lowering

## Контекст

Processing every early item requires independent subscriber progress while
preserving finite work, item provenance and deterministic authority accounting.

## Решение

A declared keyed-many source lowers to direct `repeat` body `wait → choice →
call`. Authority-owned subscription handle, cursor and assignment carry tagged
Item, Closed or Interrupted delivery; next iteration can start only after the
previous assignment settles.

## Последствия

Each subscriber owns its cursor and durable ledger. Retained item is not lost,
closure creates a final non-call iteration, and timeout is interruption rather
than EOF. Each assignment updates observable workflow frontier.

## Пересмотр

Nested stream composition, reusable calls, different item type, retry or
backpressure needs new proof of bounded identity and settlement semantics.

## Не входит

Нет generic subscriptions, exactly-once external consumer effect, blob delivery,
automatic retry, spool reservation or retention policy.

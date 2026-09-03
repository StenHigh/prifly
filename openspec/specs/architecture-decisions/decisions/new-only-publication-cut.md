# New-only publication хранит authority cut

## Контекст

A consumer may request only publications that become visible after its own
registration; an empty cursor cannot prove such a boundary.

## Решение

Authority records an exact current publication sequence when it creates the
wait or subscription. Delivery excludes publication and closure records at or
before that durable cut; later records use the normal atomic assignment path.

## Последствия

Retained and new-only semantics have distinct versioned state shapes. The cut
does not reserve a future producer effect and silence or producer failure still
uses the declared finite deadline.

## Пересмотр

External source, blob delivery, different retention model or stronger wakeup
guarantee requires a new subscription contract.

## Не входит

Нет implicit future reservation, generic external subscription, automatic retry,
backpressure/spool policy или retention guarantee.

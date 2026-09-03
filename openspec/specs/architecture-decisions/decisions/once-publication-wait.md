# One publication subscription использует durable wait

## Контекст

A declared sibling consumer may use exactly one sealed item before producer
completion, but the first slice must not masquerade as an unbounded queue.

## Решение

One publication source binds a direct producer hook to a direct sibling wait.
Existing durable wait registration, inbox and event reference provide reservation
and exactly one assignment for either retained item or subsequent publication.

## Последствия

Publication and active delivery share one authority transaction. Producer
failure waits until declared deadline; external event delivery cannot forge a
core-owned publication source. The mechanism remains finite and typed.

## Пересмотр

Multiple items, cursor, close, nested composition or another source type needs
a separately declared subscription handle and bounded lowering.

## Не входит

Нет stream queue, blob delivery, generic fan-out, physical overlap claim for a
foreground process driver или automatic recovery retry.

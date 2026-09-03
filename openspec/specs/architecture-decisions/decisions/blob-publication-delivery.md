# Blob publication использует отдельный immutable source type

## Контекст

Sealed blob may be published early, but a JSON-only source contract cannot
prove its format and media type to the consumer.

## Решение

A versioned publication source declares exact format and one blob media-type
label. Compiler compares it with the producer hook; once and stream delivery
pass the sealed blob through ordinary typed ports.

## Последствия

Consumer gets exact bytes before producer terminal settlement while source
identity, assignment ledger and stream cursor remain unchanged. Blob descriptor
validation stays a producer-side sealing concern.

## Пересмотр

Multiple media choices, external source or new blob policy requires a versioned
source contract and delivery qualification.

## Не входит

Нет generic subscription, retry/final dependency, spool, retention or
compensation guarantee.

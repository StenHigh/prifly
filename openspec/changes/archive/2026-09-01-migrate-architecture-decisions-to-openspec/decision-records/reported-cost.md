# Reported cost не является расчётом ядра

## Контекст

Host может наблюдать расход, но core не владеет provider rate cards и не может
честно вычислить price from tokens or provider metadata.

## Решение

Cost is accepted as a bounded, exact decimal observation from an authenticated
source and attached to an Attempt. Currency and source remain distinct;
unobserved, reported zero and multiple reports have separate meanings.

## Последствия

Core records who reported the value and preserves it even if later output is
rejected. It does not normalize, convert, aggregate, reserve money or treat a
claimed source as qualified provider identity.

## Пересмотр

Provider usage or monetary enforcement requires a separately versioned
contract, source qualification and evidence model.

## Не входит

Нет rate card, billing integration, token-to-price calculation, budget reserve
или hard monetary cap.

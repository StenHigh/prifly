# Action intent становится durable proposal до admission

## Контекст

Admitting a Step Attempt does not authorize an unknown future tool operation;
the exact proposed operation must survive restart before approval or delivery.

## Решение

Authority records immutable action intent with canonical digest, active Attempt,
authenticated actor, retained tool descriptor and current controls. Proposal is
idempotent for exact identity/payload and does not call an adapter, consume
approval/grant, expose credential or change result.

## Последствия

The host can propose only supported bounded operation classes, and later
admission compares exact retained bytes rather than a mutable registry entry.
Proposal remains observability of intent, not proof or permission for effect.

## Пересмотр

New tool class, remote source or proposal field requires a versioned command
contract and current-authority checks.

## Не входит

Нет action delivery, receipt, credential, external dispatch, effect status,
retry or reconciliation.

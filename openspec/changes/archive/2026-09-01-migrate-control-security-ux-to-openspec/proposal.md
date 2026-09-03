## Why

Контракт управления, безопасности и операторского интерфейса остаётся в двух
legacy-документах. Пока они не перенесены, OpenSpec не может быть единым местом
для изменения этой capability.

## What Changes

- Создать `control-security-ux` с нормами управления, security boundaries,
  evidence и human-facing control surface.
- Сохранить traceability прежних control, security и UX rules вместе с их
  acceptance links только в archived crosswalk.
- Включить в coverage current F1 operating boundaries из `docs/operations.md`.

## Capabilities

### New Capabilities

- `control-security-ux`: проверяемое управление, security boundaries, evidence
  и operator UX Pri-Fly.

### Modified Capabilities

- Нет.

## Impact

Меняется только ownership документации. Go runtime, CLI/wire contracts,
schemas, evidence, manifests и historical operating record не меняют поведения.

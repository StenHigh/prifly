## Why

Runtime, state, resources and retention остаются в legacy-главе, поэтому
OpenSpec ещё не является единым нормативным источником исполнения Pri-Fly.

## What Changes

- Создать `runtime-resources` с правилами state, journal, admission, recovery,
  claims, storage, telemetry и failure boundaries.
- Сохранить legacy traceability только в archived coverage crosswalk и затем
  переключить соответствующую строку source map.

## Capabilities

### New Capabilities

- `runtime-resources`: проверяемое runtime-исполнение, ресурсы, storage и
  восстановление Pri-Fly.

### Modified Capabilities

- Нет.

## Impact

Меняется только ownership документации; Go runtime, CLI, schemas, evidence и
historical manifests не меняют поведения.

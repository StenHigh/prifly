## Why

Public protocol and CLI contract remain in one legacy chapter, so OpenSpec is
not yet the sole place to change user-facing command, DTO and compatibility
rules.

## What Changes

- Create `cli-protocol` with the versioned command protocol, public DTO,
  validation, executor, error and compatibility boundaries.
- Retain each former API record and its acceptance links only in an archived
  migration crosswalk.

## Capabilities

### New Capabilities

- `cli-protocol`: versioned public protocol and CLI behavior of Pri-Fly.

### Modified Capabilities

- Нет.

## Impact

Меняется только ownership документации. Go runtime, existing CLI behavior,
versioned JSON Schema, stored Runs, evidence and manifests do not change.

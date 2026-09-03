## Context

Legacy runtime chapter contains RUN and DATA boundaries for execution, state,
resources, storage and failure recovery. It remains authoritative until cutover.

## Goals / Non-Goals

**Goals:** Move every runtime boundary to `runtime-resources` and retain its
legacy coverage/acceptance links in the archive.

**Non-Goals:** Do not change runtime code, schemas, evidence or manifests.

## Decisions

Runtime state, resource authority and storage/recovery remain one capability.
Apply expands the chapter's 35 numbered records into an individual archived
matrix, validates each acceptance link, then changes only its source-map row.

### Source inventory

The archived crosswalk MUST cover `RUN-001` through `RUN-026` and `DATA-001`
through `DATA-009`, with their exact acceptance links from
`docs/roadmap/requirements-map.csv`. These identifiers are migration evidence,
not names in the permanent spec.

### Legacy coverage crosswalk

| Legacy record | Acceptance cases | Replacement requirement | Review |
|---|---|---|---|
| `RUN-001` | `AC-001`, `AC-002` | «Ядро управляет выполнением детерминированным кодом» | verified |
| `RUN-002` | `AC-007`, `AC-008` | «Assisted и managed используют единый протокол authority» | verified |
| `RUN-003` | `AC-012`, `AC-119`, `AC-126` | «Reference profile использует одну локальную authority» | verified |
| `DATA-001` | `AC-019`, `AC-020` | «Run закрепляет полный исполняемый контракт» | verified |
| `DATA-002` | `AC-013`, `AC-015`, `AC-052`, `AC-053` | «External snapshot и signal имеют точное происхождение» | verified |
| `DATA-003` | `AC-014`, `AC-034`, `AC-046`, `AC-063`, `AC-069` | «Входы StepInstance согласованы и sealed» | verified |
| `DATA-004` | `AC-072`, `AC-090`, `AC-126` | «Journal хранит единую проверяемую историю» | verified |
| `DATA-005` | `AC-072`, `AC-073`, `AC-074`, `AC-101` | «Управляющая команда коммитится атомарно и идемпотентно» | verified |
| `DATA-006` | `AC-075`, `AC-076`, `AC-123`, `AC-126` | «Durable storage принимает только sealed bytes» | verified |
| `RUN-004` | `AC-083`, `AC-088` | «Run завершает только известный aggregate outcome» | verified |
| `RUN-005` | `AC-049`, `AC-089` | «StepInstance и Attempt имеют разные lifecycle» | verified |
| `RUN-006` | `AC-042`, `AC-043`, `AC-045`, `AC-046`, `AC-048` | «Scheduler разворачивает только конечный и честный граф» | verified |
| `RUN-007` | `AC-071`, `AC-079`, `AC-097` | «Turn разделяет routing, admission и effect» | verified |
| `RUN-008` | `AC-057`, `AC-077`, `AC-078` | «ExecutionEnvelope ограничивает результат конкретной попытки» | verified |
| `RUN-009` | `AC-058`, `AC-060`, `AC-107` | «Контекст и evidence честно описывают свои границы» | verified |
| `RUN-010` | `AC-082`, `AC-083`, `AC-085` | «External effect имеет отдельный durable ledger» | verified |
| `RUN-011` | `AC-030`, `AC-084`, `AC-110` | «Capability adapter доказывается для точной operation» | verified |
| `RUN-012` | `AC-080`, `AC-081`, `AC-084` | «Retry сохраняет identity и unknown effect» | verified |
| `RUN-013` | `AC-086`, `AC-087` | «Partial failure и компенсация остаются видимыми» | verified |
| `RUN-014` | `AC-090`, `AC-091` | «Claims используют физическую identity и generation» | verified |
| `RUN-015` | `AC-091` | «Lease не доказывает прекращение старого владельца» | verified |
| `RUN-016` | `AC-061`, `AC-063`, `AC-092`, `AC-109` | «Isolation profile объявляет реальные границы исполнения» | verified |
| `RUN-017` | `AC-043`, `AC-092` | «Cleanup проверяет ownership и exact generation» | verified |
| `RUN-018` | `AC-098`, `AC-101`, `AC-103` | «Stop и cancel сохраняют обязательства» | verified |
| `RUN-019` | `AC-115` | «Recovery классифицирует каждое открытое обязательство» | verified |
| `RUN-020` | `AC-081`, `AC-089`, `AC-100` | «Resume и fork не переписывают исходный Run» | verified |
| `DATA-007` | `AC-116`, `AC-130` | «Checkpoint и reader version сохраняют history» | verified |
| `RUN-021` | `AC-117`, `AC-118`, `AC-119` | «Restore начинается без права на новые effects» | verified |
| `RUN-022` | `AC-051`, `AC-120`, `AC-121` | «Время поступает как сохранённое наблюдение» | verified |
| `RUN-023` | `AC-065`, `AC-122`, `AC-123` | «Budgets и backpressure учитывают весь admission» | verified |
| `DATA-008` | `AC-113`, `AC-118`, `AC-129` | «Storage соблюдает privacy, retention и erasure boundary» | verified |
| `DATA-009` | `AC-127`, `AC-133` | «Telemetry и report выводятся из journal» | verified |
| `RUN-024` | `AC-048`, `AC-124` | «Local-1 profile объявляет qualification envelope» | verified |
| `RUN-025` | `AC-117`, `AC-125` | «Failure model честно ограничивает RPO и RTO» | verified |
| `RUN-026` | `AC-125`, `AC-143` | «Готовность подтверждается воспроизводимой qualification» | verified |

## Risks / Trade-offs

- [A recovery edge is lost] → compare every numbered record before cutover.
- [Documentation validation is reported as product evidence] → preserve gates.

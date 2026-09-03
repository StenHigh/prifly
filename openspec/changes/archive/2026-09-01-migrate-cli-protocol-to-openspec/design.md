## Context

`docs/spec/06-cli-protocol.md` contains 26 named rules for public commands,
DTO, wire framing, workflow language and compatibility. It remains the sole
current source until this migration switches its source-map row.

## Goals / Non-Goals

**Goals:**

- Move every named public-protocol boundary to a descriptive requirement and
  scenario.
- Retain exact legacy acceptance links only in the archived crosswalk.
- Preserve published JSON Schema and stored-run compatibility unchanged.

**Non-Goals:**

- Do not alter CLI code, JSON Schema, Go definitions, saved JSON fields,
  evidence, manifests or product qualification.

## Decisions

### Один rule — один читабельный requirement

The permanent capability keeps one named requirement for each former public
protocol boundary. Combining DTO, admission and retry rules would make public
compatibility review untraceable.

### Legacy IDs stay only in migration evidence

The archived crosswalk will map each former API record and exact acceptance link
to its replacement heading. The permanent OpenSpec spec intentionally has no
legacy IDs, following the owner's decision that those IDs are not the future
authoring interface.

### Versioned schemas remain product artifacts

OpenSpec describes behavior and compatibility, but does not copy versioned JSON
Schema or Go types. The migration preserves their current paths and checks them
as protected artifacts.

### Cutover is one reversible source-map change

Apply reviews all records and links, validates the replacement, then changes
only the CLI row in the source map. Legacy bytes remain unchanged until final
cleanup.

### Legacy coverage crosswalk

| Legacy source | Legacy record | Acceptance cases | Replacement requirement | Review |
|---|---|---|---|---|
| `docs/spec/06-cli-protocol.md` | `API-001` | `AC-131` | «Все клиенты используют один command protocol» | verified |
| `docs/spec/06-cli-protocol.md` | `API-002` | `AC-040`, `AC-056`, `AC-065` | «Wire framing имеет strict version и limits» | verified |
| `docs/spec/06-cli-protocol.md` | `API-003` | `AC-041`, `AC-056` | «Shape validation не является admission» | verified |
| `docs/spec/06-cli-protocol.md` | `API-004` | `AC-017` | «Canonical identity сохраняет exact bytes» | verified |
| `docs/spec/06-cli-protocol.md` | `API-005` | `AC-014` | «Public DTO имеет named authority происхождения» | verified |
| `docs/spec/06-cli-protocol.md` | `API-006` | `AC-032` | «StepDefinition описывает contract без self-qualification» | verified |
| `docs/spec/06-cli-protocol.md` | `API-007` | `AC-033`, `AC-036` | «Ports явно описывают required data» | verified |
| `docs/spec/06-cli-protocol.md` | `API-008` | `AC-034`, `AC-055` | «InputBinding выбирает declared provenance» | verified |
| `docs/spec/06-cli-protocol.md` | `API-009` | `AC-035`, `AC-041`, `AC-050` | «Workflow graph имеет finite typed transitions» | verified |
| `docs/spec/06-cli-protocol.md` | `API-010` | `AC-037`, `AC-038`, `AC-039` | «Choice использует declared three-valued semantics» | verified |
| `docs/spec/06-cli-protocol.md` | `API-011` | `AC-042`, `AC-043`, `AC-044` | «Parallel aggregate declares quorum и remainder» | verified |
| `docs/spec/06-cli-protocol.md` | `API-012` | `AC-045`, `AC-046`, `AC-047` | «Map seals collection до children» | verified |
| `docs/spec/06-cli-protocol.md` | `API-013` | `AC-048`, `AC-049` | «Repeat сохраняет persistent bounds и decision state» | verified |
| `docs/spec/06-cli-protocol.md` | `API-014` | `AC-015`, `AC-051`, `AC-052`, `AC-053`, `AC-054` | «Wait и schedule имеют durable correlation» | verified |
| `docs/spec/06-cli-protocol.md` | `API-015` | `AC-086` | «Compensation сохраняет original effect history» | verified |
| `docs/spec/06-cli-protocol.md` | `API-016` | `AC-079`, `AC-080` | «Admission и retry имеют разные identities» | verified |
| `docs/spec/06-cli-protocol.md` | `API-017` | `AC-082`, `AC-084`, `AC-085` | «Delivery status не равен effect status» | verified |
| `docs/spec/06-cli-protocol.md` | `API-018` | `AC-073`, `AC-074`, `AC-100`, `AC-102` | «CAS, dedup и stop имеют отдельные semantics» | verified |
| `docs/spec/06-cli-protocol.md` | `API-019` | `AC-098`, `AC-099`, `AC-104` | «Approval учитывает current authority при consume и dispatch» | verified |
| `docs/spec/06-cli-protocol.md` | `API-020` | `AC-077`, `AC-078` | «Result intake проверяет exact Attempt и sealed output» | verified |
| `docs/spec/06-cli-protocol.md` | `API-021` | `AC-021`, `AC-026` | «CLI exposes scoped commands without hidden mutation» | verified |
| `docs/spec/06-cli-protocol.md` | `API-022` | `AC-007` | «Executor interface has no general state.write» | verified |
| `docs/spec/06-cli-protocol.md` | `API-023` | `AC-112`, `AC-131`, `AC-135` | «Problem и exit code сохраняют safe meaning» | verified |
| `docs/spec/06-cli-protocol.md` | `API-024` | `AC-005`, `AC-132`, `AC-138` | «Preview и validation не создают effect» | verified |
| `docs/spec/06-cli-protocol.md` | `API-025` | `AC-016`, `AC-130` | «Extension changes semantics only versionedly» | verified |
| `docs/spec/06-cli-protocol.md` | `API-026` | `AC-140`, `AC-141` | «Protocol delivery distinguishes schema evidence from qualification» | verified |

## Risks / Trade-offs

- [A protocol compatibility boundary is omitted] → crosswalk every named rule
  and compare acceptance links before cutover.
- [Markdown becomes a second schema] → keep exact wire shapes in published
  JSON Schema, not a duplicate prose table.
- [Document validation is mistaken for runtime proof] → retain gates and
  historical qualification evidence unchanged.

## Migration Plan

1. Verify each named source record and acceptance link against the replacement
   spec; record an individual archived crosswalk.
2. Sync `cli-protocol`, run strict spec validation and switch one source-map
   row without modifying legacy source bytes.
3. Check code/schema/evidence/manifests, archive the change and run strict
   archived validation.

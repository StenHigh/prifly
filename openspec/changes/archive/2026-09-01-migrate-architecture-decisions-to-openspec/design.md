## Context

The current architecture source set has 17 named boundaries in one chapter and
22 decision records in `docs/decisions/`. Two records use the same legacy
ordinal, so filenames and descriptive names, not numbers, must identify the
future records. The source set remains normative until one map-row cutover.

## Goals / Non-Goals

**Goals:**

- Move all architecture boundaries to descriptive OpenSpec requirements.
- Keep decision context, decision, consequences and reconsideration trigger
  readable as part of the capability, without old ordinal IDs.
- Preserve exact legacy links to acceptance and other source sets in archive.

**Non-Goals:**

- Do not revise a technical decision, implement an ADR, alter Go code, module
  dependencies, schemas, qualification or historical release evidence.

## Decisions

### Requirement spec and decision records have distinct roles

`spec.md` will be the normative contract with 17 requirements. Companion
decision records will live under
`openspec/specs/architecture-decisions/decisions/` with descriptive filenames.
They retain rationale and trade-offs without being mistaken for another set of
requirements. The main spec will link the decision-record index.

### Old numbers are archive-only crosswalk keys

The permanent filenames and headings omit legacy ARCH/ADR ordinal names. An
archived table maps each of 17 architecture rules and all 22 old decision files
to a replacement requirement or decision record. This resolves the legacy
duplicate ordinal without preserving it as future authoring API.

### Decision records are migrated semantically, not byte-copied

Each record will retain its current context, decision, consequences, delivery
status and explicit non-goals. Relative legacy links will become links to the
corresponding OpenSpec capability when available; unavailable owners remain
named as transition dependencies in the archive rather than fabricated links.

### Cutover is reversible

After coverage review, the permanent spec and decision records are validated,
then only the architecture row in the source map switches. Legacy bytes remain
unchanged until final cleanup.

### Legacy coverage crosswalk

| Legacy source | Legacy record | Acceptance cases | Replacement | Review |
|---|---|---|---|---|
| `docs/spec/08-architecture-decisions.md` | `ARCH-001` | `AC-001`, `AC-011` | «Core ограничен protocol исполнения» | verified |
| `docs/spec/08-architecture-decisions.md` | `ARCH-002` | `AC-012`, `AC-147` | «Модули имеют единые границы ответственности» | verified |
| `docs/spec/08-architecture-decisions.md` | `ARCH-003` | `AC-071`, `AC-102` | «Все способы управления используют один contract» | verified |
| `docs/spec/08-architecture-decisions.md` | `ARCH-004` | `AC-075`, `AC-076`, `AC-115` | «Authority и artifact store имеют разные atomicity boundaries» | verified |
| `docs/spec/08-architecture-decisions.md` | `ARCH-005` | `AC-079`, `AC-081` | «Step execution не является blanket permission» | verified |
| `docs/spec/08-architecture-decisions.md` | `ARCH-006` | `AC-008`, `AC-016`, `AC-119`, `AC-143` | «Сильные свойства включаются qualification profile» | verified |
| `docs/spec/08-architecture-decisions.md` | `ARCH-007` | `AC-012`, `AC-147` | «Architectural decisions имеют versioned основания пересмотра» | verified |
| `docs/spec/08-architecture-decisions.md` | `ARCH-008` | `AC-010`, `AC-028`, `AC-147` | «Extension не требует бесконечного SDK» | verified |
| `docs/spec/08-architecture-decisions.md` | `ARCH-009` | `AC-030`, `AC-143`, `AC-145` | «Capability readiness подтверждается evidence» | verified |
| `docs/spec/08-architecture-decisions.md` | `ARCH-010` | `AC-141`, `AC-142` | «Definition of done относится к конкретной поставке» | verified |
| `docs/spec/08-architecture-decisions.md` | `ARCH-011` | `AC-006`, `AC-145` | «Owner decisions имеют safe defaults» | verified |
| `docs/spec/08-architecture-decisions.md` | `ARCH-012` | `AC-025`, `AC-128`, `AC-146` | «Security update является delivery responsibility» | verified |
| `docs/spec/08-architecture-decisions.md` | `ARCH-013` | `AC-117`, `AC-118`, `AC-130` | «Storage migration, restore и export не возобновляют effects» | verified |
| `docs/spec/08-architecture-decisions.md` | `ARCH-014` | `AC-122`, `AC-124` | «Cost и capacity управляются измерениями» | verified |
| `docs/spec/08-architecture-decisions.md` | `ARCH-015` | `AC-022`, `AC-146` | «License provenance и runtime trust проверяются отдельно» | verified |
| `docs/spec/08-architecture-decisions.md` | `ARCH-016` | `AC-029`, `AC-148` | «Pilot сохраняет principles, а не fixed lifecycle» | verified |
| `docs/spec/08-architecture-decisions.md` | `ARCH-017` | `AC-144` | «Handover связывает результат с поставкой и ограничениями» | verified |
| `docs/decisions/0001-foundation.md` | Foundation decision | — | `decisions/foundation-sequence.md` | verified |
| `docs/decisions/0002-core-workflow.md` | Core workflow decision | — | `decisions/core-workflow-compatibility.md` | verified |
| `docs/decisions/0003-choice.md` | Choice decision | — | `decisions/pinned-choice.md` | verified |
| `docs/decisions/0004-call.md` | Nested workflow decision | — | `decisions/nested-workflows.md` | verified |
| `docs/decisions/0005-repeat.md` | Repeat decision | — | `decisions/bounded-repeat.md` | verified |
| `docs/decisions/0006-context-checks.md` | Context and checks decision | — | `decisions/context-sources-and-checks.md` | verified |
| `docs/decisions/0007-authority-controls.md` | Authority controls decision | — | `decisions/authority-controls.md` | verified |
| `docs/decisions/0008-aif-pilot-critical-path.md` | Assisted pilot decision | — | `decisions/assisted-aif-pilot.md` | verified |
| `docs/decisions/0009-reported-cost.md` | Reported cost decision | — | `decisions/reported-cost.md` | verified |
| `docs/decisions/0010-early-artifact-publication.md` | Early publication decision | — | `decisions/durable-early-publication.md` | verified |
| `docs/decisions/0011-once-publication-subscription.md` | Once subscription decision | — | `decisions/once-publication-wait.md` | verified |
| `docs/decisions/0012-artifact-close-manifest.md` | Closure manifest decision | — | `decisions/exact-closure-manifest.md` | verified |
| `docs/decisions/0013-each-publication-subscription.md` | Each-publication decision | — | `decisions/each-publication-repeat.md` | verified |
| `docs/decisions/0014-checked-artifact-publication.md` | Checked publication decision | — | `decisions/checked-artifact-publication.md` | verified |
| `docs/decisions/0015-new-only-publication-source.md` | New-only source decision | — | `decisions/new-only-publication-cut.md` | verified |
| `docs/decisions/0016-action-intent-proposal.md` | Durable action intent decision | — | `decisions/durable-action-intent.md` | verified |
| `docs/decisions/0016-terminal-producer-failure-interruption.md` | Producer interruption decision | — | `decisions/producer-failure-interruption.md` | verified |
| `docs/decisions/0017-blob-publication-delivery.md` | Blob delivery decision | — | `decisions/blob-publication-delivery.md` | verified |
| `docs/decisions/0018-action-admission.md` | Action admission decision | — | `decisions/atomic-action-admission.md` | verified |
| `docs/decisions/0019-action-grant-admission.md` | Resource grant decision | — | `decisions/resource-scoped-action-grant.md` | verified |
| `docs/decisions/0020-task-intake-source-adapters.md` | Task intake decision | — | `decisions/provider-neutral-task-intake.md` | verified |
| `docs/decisions/0021-versioned-project-profile-and-local-authority.md` | Project profile decision | — | `decisions/versioned-project-profile.md` | verified |

## Risks / Trade-offs

- [Decision rationale is lost by reducing ADR numbers] → individually count and
  crosswalk all 22 source records.
- [A decision record is mistaken for delivery evidence] → retain its stated
  status and separate it from qualification evidence.
- [Broken relative link after relocation] → validate every internal OpenSpec
  link and record the old path only in archive.

## Migration Plan

1. Verify 17 architectural requirements and migrate 22 decision records with
   descriptive paths and an index.
2. Build archive crosswalks for rules, decision files and acceptance links;
   verify no legacy IDs remain in permanent architecture docs.
3. Sync the capability, switch one source-map row, preserve historical files,
   archive the change and run strict validation.

## Context

См. [proposal.md](proposal.md). Source set включает contract guide и baseline
fixtures в `docs/spec/contracts/`, distributed schemas в `schemas/` и exact
runtime representations в `internal/flow/` and `internal/runtime/`. Existing
OpenSpec capabilities уже владеют product semantics; этот change владеет
способом публиковать и проверять их machine contracts.

## Goals / Non-Goals

**Goals:**

- Сделать `published-contracts` current source для правил publication,
  compatibility и proof boundary.
- Дать contributor понятный маршрут: semantic capability → exact schema/type
  artifact → generated/fixture verification.
- Сохранить legacy baseline facts in archive and expose any count drift before
  cutover.

**Non-Goals:**

- Не переписывать schemas, generated Go contracts, fixtures, runtime behavior,
  accepted Runs, evidence или historical manifests.
- Не переносить every JSON field into Markdown, не добавлять SDK/API и не
  квалифицировать unexecuted baseline operations.

## Decisions

### OpenSpec describes the contract discipline, not generated JSON

Permanent spec names the observable publication/compatibility rules. JSON
Schema and Go type sets remain canonical bytes because duplicating their
hundreds of fields in Markdown creates a second diverging interface. Existing
semantic capabilities remain the place to edit meaning; contract change must
update both that capability and the corresponding machine artifact.

### Inventory first, then one explicit discrepancy decision

Before cutover, inventory each legacy guide/fixture, distributed schema and
runtime generation/embedding path. The guide currently states one baseline
component count while direct inspection produces a different count. Archive
will preserve the original statement and source path; migration will establish
the actual definition of the count and add a small reproducible check before
changing any current claim. It will not restamp bytes or recast historic
qualification.

### Archive preserves traceability; permanent spec avoids legacy IDs

Crosswalk records baseline components, command/fixture/security inventory,
their declared statuses and source paths. Permanent headings use readable
subjects only. This follows the migration policy that old requirement and
acceptance identifiers remain historical traceability, not a new authoring
interface.

### Cutover changes one ownership row

After strict validations and protected diff, switch only the
`published-contracts` row in `SOURCE-OF-TRUTH.md`. Legacy docs remain
byte-identical through final cleanup; schemas and Go artifacts remain in the
release tree after cleanup because they are product artifacts, not legacy docs.

## Risks / Trade-offs

- [Markdown re-describes a schema] → Permanent spec contains rules and exact
  artifact paths, never an independent field inventory.
- [Count drift is hidden by migration] → Record both old claim and measured
  inventory; resolve before source-map cutover.
- [Fixture success is mistaken for implementation] → Preserve declared
  structural/execution status in crosswalk and state the distinction in spec.
- [A schema change is missed by documentation checks] → Reuse existing schema
  generation/freshness tests and add only the smallest inventory assertion if
  one is absent.

## Migration Plan

1. Inventory legacy guide, fixture registries, schemas and code paths; resolve
   the component-count discrepancy with history and a reproducible check.
2. Build permanent candidate and archive crosswalk without editing source
   artifacts; validate status/compatibility meanings.
3. Sync permanent spec, switch one ownership row, run strict OpenSpec and
   protected artifact checks, then archive the change.

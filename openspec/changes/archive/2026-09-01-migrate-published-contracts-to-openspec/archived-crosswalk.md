# Архивная crosswalk: опубликованные контракты

Эта карта сохраняет legacy contract inventory, его статусы и точные source
paths. Постоянная capability задаёт нынешние правила публикации и
совместимости; она не дублирует поля JSON Schema или Go types и не объявляет
fixture check product qualification.

## Legacy contract guide and fixtures

| Legacy material | Exact source | Inventory / declared status | Permanent subject |
|---|---|---|---|
| Contract guide | `docs/spec/contracts/README.md` | Explains baseline, command catalogue, fixtures and future-schema boundary | Semantic rule and machine shape have separate sources |
| Protocol baseline | `docs/spec/contracts/protocol.schema.json` | Closed Draft 2020-12 document; root rejects arbitrary documents | Published contract preserves compatibility boundary |
| Command catalogue | `docs/spec/contracts/commands.json` | 43 entries, all `proposed_not_implemented` | Fixtures and schema checks are not qualification evidence |
| Fixture registry | `docs/spec/contracts/workflow-examples.json` | 20 components, 13 resources, 3 external dependencies, 13 invalid cases; eight root workflows and 36 shape cases; all components are `not_qualified_fixture` | Fixtures and schema checks are not qualification evidence |
| Security shapes | `docs/spec/contracts/security-cases.json` | 23 shape cases; six positive and 17 negative runtime expectations, none a runtime qualification | Fixtures and schema checks are not qualification evidence |
| Workflow extract 01 | `docs/spec/contracts/examples/01-normalise-csv.workflow.json` | Root fixture, digest-backed registry member | Legacy material stays historical until final cleanup |
| Workflow extract 02 | `docs/spec/contracts/examples/02-research-brief.workflow.json` | Root fixture, digest-backed registry member | Legacy material stays historical until final cleanup |
| Workflow extract 03 | `docs/spec/contracts/examples/03-route-request.workflow.json` | Root fixture, digest-backed registry member | Legacy material stays historical until final cleanup |
| Workflow extract 04 | `docs/spec/contracts/examples/04-parallel-checks.workflow.json` | Root fixture, digest-backed registry member | Legacy material stays historical until final cleanup |
| Workflow extract 05 | `docs/spec/contracts/examples/05-map-documents.workflow.json` | Root fixture, digest-backed registry member | Legacy material stays historical until final cleanup |
| Workflow extract 06 | `docs/spec/contracts/examples/06-revise-until-ready.workflow.json` | Root fixture, digest-backed registry member | Legacy material stays historical until final cleanup |
| Workflow extract 07 | `docs/spec/contracts/examples/07-wait-for-decision.workflow.json` | Root fixture, digest-backed registry member | Legacy material stays historical until final cleanup |
| Workflow extract 08 | `docs/spec/contracts/examples/08-checked-workspace-file.workflow.json` | Root fixture, digest-backed registry member | Legacy material stays historical until final cleanup |

Fresh isolated execution of the legacy fixture verifier passed 519 of 519
structural checks. It reports 108 protocol definitions, 43 proposed commands,
36 workflow shape cases, eight root workflow examples, 36 payload digests and
23 security shapes. Its scope is document fixtures only: it does not execute a
Pri-Fly product runtime, provider, sandbox, database recovery or workflow.

## Baseline count reconciliation

| Record | Finding |
|---|---|
| Original baseline at `44e4b7b` | Protocol contained 106 `$defs`; the guide also said 106. |
| Change `c089372` (2026-08-31) | Added the two closed definitions `TaskInput` and `TaskSource`, taking the protocol to 108. |
| Legacy guide after that change | Was not edited; it still says 106. |
| Current canonical count | 108, because the schema itself and its verifier enumerate every `$defs` entry. |
| Historic result | The legacy 106 wording remains byte-identical until final cleanup. It is not restamped and no historic evidence is reclassified. |

The reproducible count check is:

```sh
test "$(jq '."$defs" | length' docs/spec/contracts/protocol.schema.json)" -eq 108
```

The existing fixture verifier independently records the same dynamic count in
`counts.schema_definitions`; no second generator or Markdown field list is
introduced for this migration.

## Distributed product artifacts

`scripts/check-schema.py` has 35 generation profiles and validates 36
distributed JSON files. Twenty-four generated public bundles have matching
runtime copies; the baseline protocol is separately embedded by
`internal/flow/schema.go`. `make schemas-check` and targeted Go contract tests
verified these relations during migration.

| Area | Exact artifacts |
|---|---|
| Foundation | `schemas/foundation/public.schema.json`; `schemas/foundation/step-definition-v2.schema.json` |
| Core public bundles | `schemas/core/action-admission.schema.json`; `action-delivery.schema.json`; `action-grant-admission.schema.json`; `action-intent.schema.json`; `artifact-closure.schema.json`; `artifact-publication.schema.json`; `choice-decision.schema.json`; `contexts.schema.json`; `fork.schema.json`; `guards.schema.json`; `invocations.schema.json`; `map.schema.json`; `parallel.schema.json`; `public.schema.json`; `publication-checks.schema.json`; `publication-failure.schema.json`; `publication-new-only.schema.json`; `publication-subscription.schema.json`; `repeats.schema.json`; `reported-cost.schema.json`; `sessions.schema.json`; `wait.schema.json`; `waivers.schema.json` |
| Core authoring and source contracts | `schemas/core/step-definition-v3.schema.json`; `step-definition-v4.schema.json`; `workflow-revision-v3.schema.json`; `publication-source-v1.schema.json`; `publication-source-v2.schema.json`; `publication-source-v3.schema.json`; `publication-source-v4.schema.json`; `publication-source-v5.schema.json`; `publication-source-v6.schema.json`; `publication-source-v7.schema.json`; `publication-source-v8.schema.json` |
| Embedded baseline | `internal/flow/protocol.schema.json`, byte-identical to `docs/spec/contracts/protocol.schema.json` at migration time |
| Generated runtime copies | Corresponding `internal/runtime/*.schema.json` files; generator and runtime tests are their canonical relationship check |

These paths are product artifacts, not legacy Markdown. They remain in the
release tree after final documentation cleanup; semantic rules stay in their
respective OpenSpec capabilities and publication discipline in
`published-contracts`.

## Migration protections

- `docs/spec/contracts/**`, `schemas/**`, `internal/flow/**`,
  `internal/runtime/**`, `docs/evidence/**`, root `file-manifest.json` and
  `docs/spec/file-manifest.json` remain byte-identical through this migration.
- Generated-schema agreement proves contract bytes and types, not a qualified
  executor, permission, external effect or release profile.
- Old record labels and historical wording stay in this archive and Git only;
  permanent OpenSpec uses readable current requirements.

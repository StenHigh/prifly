## Context

См. [proposal.md](proposal.md) и delta specs. Raw blob ArtifactRevision уже
хранит точные bytes, а current assisted SessionTask уже передаёт claimed
RepositoryWorkspace. Не хватает только declared связи между набором таких
артефактов и нативным планом в рабочей копии: context materialization кладёт
входы в authority scratch `context/sources/`, а AI Factory читает Fast, Full
или Ultra plan по путям в `.ai-factory/`.

## Goals / Non-Goals

**Goals:**

- Передавать один native AI Factory plan между skills без JSON-пересказа и без
  зависимости Core от AI Factory.
- Сохранить Fast single file, Full named file и Ultra bundle (`index.md` плюс
  phase files) как exact files и структуру.
- Сделать это generic contract-ом StepDefinition, пригодным для другого
  document/tree transform, с exact bytes, provenance, WorktreeClaim
  confinement и compatibility прошлых Runs.

**Non-Goals:**

- Не давать host произвольный filesystem export, glob, recursive sync или
  возможность выбирать путь вне declared capture policy.
- Не давать binding-у новые права на repository и не импортировать AI Factory
  configuration либо terminology в Core.
- Не менять прежние workflow revisions, session handoffs или historical
  evidence.

## Decisions

### Один sealed WorkspaceTreeManifest для файла и bundle

Новая StepDefinition v5 получает optional `workspace_trees`: bounded list
элементов с одним compatible manifest input port, одним manifest output port и
declared capture policy. `WorkspaceTreeManifest` сам является sealed JSON
ArtifactRevision; он содержит workspace-relative root, entrypoint и конечный
список `{relative_path, ArtifactRef}`. Content каждого файла остаётся в уже
существующем raw ArtifactRevision — JSON не содержит копии плана.

Fast и Full — manifest с одной entry. Ultra — manifest с `index.md` и прямыми
phase Markdown entries. Поэтому именно native files, а не Pri-Fly summary,
передаются дальше. Атомарность означает, что runtime принимает новый manifest
только после successful sealing всех entries.

Capture policy имеет только три provider-neutral формы: exact file, one direct
child file of declared parent и one direct-child directory with direct regular
files. Для input/output tree captured root обязан совпадать с input manifest;
для output-only plan creation host сообщает typed location в пределах declared
policy, но не ArtifactRef. Эти формы покрывают Fast, Full и Ultra, не создавая
общий virtual filesystem или unbounded directory exporter.

Альтернатива — one-file API и отдельный Ultra archive позже. Она снова создала
бы ложную границу между canonical AIF modes. Альтернатива — host-supplied list
of paths/ref values. Она нарушает owner-only artifact access и provenance.

### Materialize/capture принадлежат runtime, а не host

До handoff runtime validates binding, WorktreeClaim и capture policy. Для
input manifest он materialize-ит exact regular files по recorded paths; любой
existing byte drift, symlink, traversal или non-regular parent даёт refusal без
перезаписи. Для output-only capture runtime records the declared target state
before handoff and accepts only a policy-conforming new file/tree; it does not
silently replace an unrelated existing document.

После terminal result runtime, not host, enumerates only files allowed by the
capture policy, validates root/entrypoint, rechecks confinement, seals every
regular file, creates `WorkspaceTreeManifest` and supplies its ArtifactRef to
the declared output port before normal result validation. Missing, extra,
symlinked or changed-during-capture entries refuse the result; a partial bundle
never becomes accepted state.

This deliberately has no second question flow: in checkout mode an owner who
has a conflicting plan chooses whether to keep it, remove it, or start a new
Run.

### Новая revision только для tree-aware state и session contract

Срез добавляет StepDefinition v5, `assisted-session/4` и следующий Core
state/read/schema generation. New Run выбирает их только когда compiled closure
contains `workspace_trees`; reader продолжает принимать v1–v4 definitions и
assisted-session/1–3 exactly as before. Generated protocol schemas, capability
inventory, compatibility guards и glossary bindings обновляются вместе;
retained bundles не переписываются.

SessionTask v4 carries the typed tree binding and its capture policy.
SessionSubmission v4 may report only a typed selected capture location for an
output-only tree. It never supplies a manifest ref, digest or arbitrary path;
runtime validates the location and seals the contents. This preserves
owner-only access to artifact storage.

### `aif-classic` maps default AIF locations through the generic contract

`aif-classic` replaces `schemas/plan.yaml` with a plan-manifest port. Its
package-level profile selects native `fast`, `full` or `ultra` form and maps
only the documented default paths to the generic three capture policies:
`.ai-factory/PLAN.md`, a direct `.md` child of `.ai-factory/plans`, or a direct
bundle directory there. `aif-plan` creates the first manifest; every accepted
`aif-improve` manifest becomes the next repeat input; `aif-implement` receives
that exact final manifest and emits its checkbox-updated final manifest.

The optional package checks its profile against the selected AI Factory plan
mode and path configuration at the package/host boundary. A non-default or
unsupported layout gives an explicit compatibility refusal; Core remains
provider-neutral and never parses `.ai-factory/config.yaml`.

## Risks / Trade-offs

- [Host changes a plan after capture] → accepted manifest is the exact
  capture-time snapshot; later filesystem mutation is not attributed to the
  settled Attempt.
- [Checkout contains manually edited plan] → pre-handoff drift refuses instead
  of overwriting; this is intentionally visible work for the owner.
- [AIF config uses a different layout] → optional package refuses clearly;
  guessed path discovery is not introduced into Core.
- [Ultra has broken links/orphan phase files] → package validation refuses the
  bundle rather than passing an incomplete plan to improve or implement.
- [Version growth touches generated contracts] → retain old schemas and add
  positive/negative compatibility tests before advertising the new capability.

## Migration Plan

1. Add versioned flow/runtime/session schema support with old-reader guards;
   compile and validate an unrelated legacy workflow unchanged.
2. Add tree materialize/capture plus tests for Fast, Full and Ultra happy
   paths, drift, traversal/symlink, missing entry and atomic failure.
3. Convert `aif-classic`, remove its JSON plan schema, update its docs and
   prove that a later improve and implement receive the prior captured native
   plan manifest.
4. Update terms, reference YAML, delivery backlog and public capability
   inventory. Do not mark the live pilot qualified merely because the contract
   exists.

Rollback is a new package/revision selection: old sealed Runs continue with
their pinned pre-v5 contracts; the new optional example is not selected.

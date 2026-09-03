## 1. Release layout and traceability

- [x] 1.1 Update the source-of-truth map with every final OpenSpec capability
  path and verify that no product source set is unowned.
- [x] 1.2 Define the archived legacy-coverage crosswalk used by every
  capability change and verify it against one representative chapter and its
  acceptance scenarios.

## 2. Capability migrations

- [x] 2.1 Create and archive focused OpenSpec changes for `product-model` and
  `domain-execution`; verify each archived legacy-coverage crosswalk and
  `openspec validate --specs --strict`.
- [x] 2.2 Create and archive focused OpenSpec changes for
  `workflow-and-context`, `runtime-resources` and `control-security-ux`; verify
  versioned-contract links and that the old source set remains unchanged until
  each cutover.
- [x] 2.3 Create and archive focused OpenSpec changes for `cli-protocol`,
  `quality-and-acceptance`, `architecture-decisions` and
  `foundation-profile`; verify requirement/case meaning and applicable Go or
  schema checks.
- [x] 2.4 Create and archive focused OpenSpec changes for
  `delivery-roadmap` and `published-contracts`; verify phases, status language
  and JSON Schema references remain explicit without custom CSV/JSON document
  maps.

## 3. Release cleanup

- [x] 3.1 Replace `README.md` with a polished GitHub product home that contains
  only product introduction, quick start and OpenSpec links; verify it has no
  duplicated normative requirements.
- [x] 3.2 Perform the final cleanup change after every map row is migrated:
  remove `SPECIFICATION.md`, legacy `docs/`, `tools/docs/`, historical evidence
  and manifests; verify `openspec validate --all --strict`, no legacy links and
  no runtime or wire-contract diff.

## 4. Release verification

- [x] 4.1 Run the qualified Go and JSON Schema checks, OpenSpec validation and
  `git diff --check`; record their exact results without treating document
  validation as product-release evidence.
  - `make test`, `make race`, `make vet fmt-check schemas-check` and `make e2e`
    completed with exit code 0; the schema check matched 35 public profiles.
    E2E covered 6 examples, CLI scenarios, 169 core commands and 10 context
    cases.
  - `openspec validate --specs --strict` passed 14 permanent specs;
    `openspec validate --all --strict --archived` passed 14 archived changes;
    `git diff --check` produced no output.
- [x] 4.2 Verify the release tree has no legacy documentation source, evidence
  or custom document tooling and that Git history retains the final cleanup
  change; review the final diff before the first release tag.
  - The tracked tree contains none of `SPECIFICATION.md`, `docs/`,
    `file-manifest.json` or `tools/docs/`; every source-map capability is
    marked `Перенесено`. Runtime and schema paths did not change; only two Go
    tests now read relocated fixtures.
  - Commit `58ff5bc` records the final cleanup, while the removed sources
    remain recoverable from its parent and earlier Git history.

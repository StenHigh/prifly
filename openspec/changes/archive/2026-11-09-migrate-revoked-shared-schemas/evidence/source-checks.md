# Source implementation checks

This evidence covers source implementation for issue #21. It does not qualify a
released binary or attest repair of a private authority. Task 6.2 remains open.

## Environment and scope

- Host: codex-app, explicitly authorized local worktree edits, checks and commits.
- Checks used sandbox escalation because the default sandbox could not start its
  loopback interface. `CHECK_CONTEXT=issue-21-schema-migration` identifies this
  escalated local check environment; it grants no additional effects.
- Go: 1.27.3 Linux amd64, the exact project-cached toolchain.
- Source base: `0245eb79260aab5f68767079ff239736642c75e6`.
- First tested implementation checkpoint: `360820e1edeee67fb083ebba0c07fc186969e3f9`.
- The subsequent correction removes only an extra trailing newline from the new
  schema generator output and updates this evidence and the task ledger.

## Results

- Focused runtime, local receipt-guard and public CLI tests passed. The native
  matrix covers multiple schemas, subsequent and nested work, restart, old Runs,
  stale plans, concurrent revoke, retained decisions and exact command replay.
- `make test-changed` passed. `make schemas-check` matched all 93 bundles.
- `make ci-check-once CHECK_CONTEXT=issue-21-schema-migration CHECK_FOR_COMMIT=1`
  passed for the checkpoint's exact staged bytes. It included all Go tests,
  native and CGO-disabled Linux/Darwin vet, formatting, refusal guard,
  Linux/Darwin staticcheck, vulnerability scanning, schema generation checks,
  release-tooling contract tests and module verification.
- `openspec validate --all --strict --no-interactive` passed: 41 items, 0 failures.
- The final correction receives the same complete local gate before its commit.
- Historical tracked schema bundles, archived changes and historical release
  evidence retain their base bytes. Only the two new schema bundles are added.

The host-local detailed logs are `/tmp/prifly-issue21-ci-check.log`,
`/tmp/prifly-issue21-test-changed-passed.log` and
`/tmp/prifly-issue21-final-ci-check.log`. They are local diagnostic paths, not
published release evidence or Pri-Fly ArtifactRefs.

## Remaining acceptance

After a release candidate passes fixed-SHA release gates, task 6.2 requires public
CLI acceptance with that released binary. Source tests do not close it. Private
acceptance requires separate owner participation; see `owner-handoff.md`.

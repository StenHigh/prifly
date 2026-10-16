# Issue #8: implementation evidence and operational handoff

Scope: `fix-unbound-claim-ownership-scan`; no #7/#9 implementation, release,
installation or recovery of the external authority from the issue.
Base commit: `25583d094163a7bc47ee8cdb7732a923553af663`. The Run implementation
artifact records the resulting commit; the local handoff manifest records the
exact binary and source commit after implementation.

## Evidence

- A valid synthetic authority has 66 saved Runs and approximately 72.5 MB
  restored snapshot bytes. The installed 0.13.76 binary refuses its free
  unbound claim with the old generic `invalid_input`. The fixed binary scans
  the population, releases the claim, and refuses a stale generation.
- The CLI then binds a new workspace Run and refuses release ownership while
  it is active. Every pre-existing snapshot retains its SHA-256.
- A real subsequent Project start hits `project_start_package_identity_conflict`
  after taking a claim: stdout is empty; its single Problem/1 retains the
  refusal code and confirms `released claim … generation 1`.
- Packed/unpacked restored totals below/equal/above 64 MiB, the separate 1001
  record boundary, late holders after both bounds, unsupported state,
  cancellation, corruption/missing/oversized pins and consistent read cuts are
  covered by `store_scan_test.go` and `claim_scan_test.go`.
- Both write paths reject stale cuts before their reducers, including an update
  of an existing Run. Duplicate receipts precede the guard; adding a cut does
  not change command digests. Existing interrupted release tests remain green.
- Failed-start cleanup always reconciles the deterministic Run and receipt.
  Creation ownership comes from the atomic claim receipt; a concurrent
  idempotent caller is never treated as the creator from an earlier list.
  Partial preparation failures return their candidate claim for inspection.
  Rollback additionally requires an unbound claim and no legacy holder,
  including a finished one; ordinary operator release keeps its terminal-Run
  predicate. Tests preserve an admitted terminal Run, unknown outcomes,
  existing/handed claims, and the original Problem while exposing cleanup and
  package cleanup failures.
- `doctor --claim ID` emits the separate closed
  `claim-ownership-diagnostics/1` schema, also selected as public contract
  `ClaimOwnershipDiagnostics`. A legacy holder is `holder_run_id`; claim
  `run_id` remains empty when unbound. SQLite page usage is separate. A changed claims ledger during diagnosis
  yields `unknown` / `claims_changed` and `claim_ownership_conflict`; a
  deterministic interleaving test checks the final version guard.

The final local binary is `/tmp/prifly-issue8-fixed` (Linux amd64, Go 1.27.2,
version `0.2.0-dev.5`, an unreleased development build), SHA-256
`1eca8c15c34276554c6a951deb7e5424e6e707d7d00913b7edc6413fc0de2046`.
For the separate doctor process the measured 66-Run scan was 1.703788981s,
72,472,658 restored bytes, peak RSS 43,432 KiB (~42.4 MiB). RSS includes
startup, runtime and package verification; it is not the restored-byte total
or an authority quota. GNU time measures the executed child separately from
the parent Go test process (including its pre-exec fork high-water mark). The implementation retains one bounded snapshot at a
time (16 MiB maximum); the process figure is an observation, not a global
memory guarantee. Synthetic setup and workspace Run creation use the same CLI
code in-process; claim create/doctor/release use the independent executable.

Commands and local logs:

```sh
make check-fast GO=/home/dev/.local/lib/go1.27.2/bin/go
make vet schemas-check refusal-check GO=/home/dev/.local/lib/go1.27.2/bin/go
openspec validate fix-unbound-claim-ownership-scan --strict --no-interactive
git diff --check
# Staged final snapshot, actual permission/toolchain profile:
make ci-check-once GO=/home/dev/.local/lib/go1.27.2/bin/go \
  CHECK_CONTEXT=codex-app-escalated-bwrap-unavailable CHECK_FOR_COMMIT=1
```

Logs: `/tmp/prifly-issue8-check-fast.log`,
`/tmp/prifly-issue8-static-checks.log`,
`/tmp/prifly-issue8-binary-journey.log`,
`/tmp/prifly-issue8-final-ownership.log`,
`/tmp/prifly-issue8-final-gate.log`.
Protected baseline comparison: 1369 existing schemas, archived changes,
release evidence and canonical workflow/skill files remain byte-identical.
Manifest `/tmp/prifly-issue8-protected-hashes.json`, SHA-256
`1b4bc7ac775a66c2740dbfe926611ece979abab40515156e7b0002efb7c7eec2`.
New diagnostics schema files are additions; old bundles are unchanged.

## Operational handoff (owner action after separate authorization)

Verify the binary SHA-256 and source commit in
`/tmp/prifly-issue8-handoff.json`. First use only read-only commands against the
original authority:

```sh
/tmp/prifly-issue8-fixed --project AUTHORITY doctor --claim CLAIM --json
/tmp/prifly-issue8-fixed --project AUTHORITY claim list --json
```

An observation does not authorize release. The owner must verify the exact
claim and generation, the absence of an active/uncertain holder and any
remaining filesystem work. With a separate operational instruction, use the
ordinary command with the observed generation:

```sh
/tmp/prifly-issue8-fixed --project AUTHORITY claim release --id CLAIM --generation N --json
```

Then repeat read-only diagnosis and launch preparation; neither this local
PASS nor claim release resolves #7's storage budget or authorizes a new start.
No SQL edits, forced deletion or manual worktree cleanup are prescribed.
No original external orphan was inspected or modified by these fixtures.
This is not a release: no tag/push/merge was performed, and a published release
still needs exact-SHA verify/qualify gates from AGENTS.md.

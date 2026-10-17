# Local development evidence

Development implementation of issue #11 through ordinary OpenSpec apply, without
prifly-run. No release, publication, production service/proxy change or owner
production acceptance. E6 and issue #11 remain open pending A9/A10.

Baseline: Core `86676dd6d1560216aed2ed233722c3febe9e465a`; consumer
`3499115956903abd264fbde2be2f432d07bc2c43`. This is an uncommitted development
snapshot, not an exact released SHA. Binary SHA-256:
`b4a1398868362ac4ffbfc9a67d788ccaf5e8d3b8ee48f726715c100483342672`.
The binary was built with `make build` (trimpath, buildvcs=false).

Profile: Linux 6.8.0-146-generic x86_64, AMD EPYC (KVM, 4 vCPU), 7941 MiB RAM;
Go 1.27.2 linux/amd64, cgo enabled, Node 24.21, Chromium 148.0.7778.96.
No production data or credentials were used. Local fixtures and server logs live
under ignored `.cache/light-acceptance/`; retained measurements/screenshots are
in this evidence directory. Replay commands are in `test/README.md`.

## Semantic coverage

- A1/A8: separate embedded Light DOM/assets and listener; default full frontend
  bytes, API and probe remain compatible. GET/HEAD allowlist, loopback/Host/access
  guards, prefix routing, refusal Problems, no maintenance/write routes.
- A2: native queue integration (`TestCLIProjectQueuePartialCancelContinueKeepsTaskAndRunHistory`)
  exports partial→cancelled continuation→continuation. Three Runs remain one
  task with one current frontier. Unit/integration tests cover queued-before-Run,
  duplicate titles, independent standalone/forks, mixed frontier and canonical
  binding when queue indexing lags (`TestTaskQueueLostIndexStillNamesTheAdmittedRun`).
- A3: unchanged shipped aif-feature copied into a throwaway `/3` project,
  compiled/imported/admitted without Drive or LLM; exact mapping resolved from
  sealed refs and compiler b1 provenance. Mapped phase is plan. Generic native
  fixture without mapping keeps explicit unmapped work. Monitor reason projection
  covers owner/decision/admission/resource and terminal states.
- A4/A5: pinned real W1/W2/W5 consumer schemas/graphs and optional mappings,
  including parallel backend/UI/UX, kept as data outside runtime. Real native
  two-step session-protocol fixture accepts a usable passed report and blocked
  gate: successful Run remains attention/unavailable. Actual browser opens
  report/gate provenance and exact gate bytes. Runtime tests additionally cover
  passed/defect/warnings/optional skip/disabled/N/A, unknown predicates, missing
  and invalid reports, request/candidate/round mismatch, repair and historical
  rows, shared aggregate gates, owner blockers and exact bundled raw digest.
  These are deterministic fixtures, not new executions of consumer reviewers.
- A6: actual Chromium desktop 1440 and mobile 390 replay covers separate screens,
  keyboard, filters/search beyond 200, loading/reset/empty, source loss with
  healthy authorities retained, late query responses, exact full source/Run
  navigation, raw details and dark theme. Hidden scheduling is tested by injecting
  the browser visibility signal; this is not an OS background-tab observation.
  Fixed-generation pagination and expiry/refusal semantics are HTTP tests.
- A7: 1000 genuine native Runs, 1001 native queue tasks, separate continuation,
  typed-review, shipped-feature and long-history authorities. Queue bulk fixture
  uses the production native validator/authority transaction, while ordinary
  QueueAdd/claim/continuation are tested separately. Long history: 1404 events,
  one snapshot decode and one brief read. SQLite coherent-cut concurrency test
  prevents mixing a Run snapshot with resource occupancy from another cut.
  Readers close transactions/connections before publishing or returning HTTP;
  no retained connection, persistent verification marker, queue initialization,
  migration or write is introduced by Light reads. Compact cache streams one
  changed snapshot at a time; byte budgets and cursor retention are bounded.

## Results

Targeted Light/local/CLI, lost-index, glossary and author-index tests passed.
`make schemas-check` and `python3 -B scripts/test_light_projection.py` passed.
828 protected baseline files match SHA-256 with no exceptions, recorded in
`protected-verification.json`; pinned fixture Git blob/SHA checks pass.
`git diff --check` and strict OpenSpec validation passed.

Final runtime index measurement: cold 1761.578 ms (budget 5000), unchanged
19.262 ms, 1000 cold decodes, zero unchanged snapshot decodes/body reads;
one changed Run → one decode; queue-only → zero Run/one queue decode;
admission-only → zero Run decodes. See `performance.json`.

Final actual browser replay passed: 5 initial requests (budget 6), 25,834-byte
board page (128 KiB), 10,950-byte gzip assets (250 KiB), warm API p95
98.332 ms over 100 requests (250 ms), 2 polls/4.5 s and one hidden poll/16 s.
Shipped feature phase mapping passed. Both throwaway listeners were stopped by
the replay runner. See `browser-metrics.json`, `browser-requests.json` and
screenshots. Strict validation: `openspec validate add-prifly-light-task-board --strict`.

Final full gate uses `make ci-check-once CHECK_CONTEXT=linux-amd64-cgo-local-openspec-light`.
The exact check inputs, toolchain, logs and result are stored by the existing
runner under `.cache/verification/`; result: **PASS, exit 0**. It executed the full batch (reuse disabled because of
unsupported symlink input), including all Go tests, Linux/Darwin vet and staticcheck,
format/refusal/testguard, vuln/schema/release-contract/script/shard checks and
`go mod verify`. 740 runtime tests belong to exactly one of three shards.
See `ci-check.log` and `ci-result.json` for command, log digest and tested source
snapshot digest. Subsequent edits only record gate evidence and mark task 6.6;
implementation bytes remain those tested. No reusable cached success is claimed.
A9/A10, release tasks 7.1–7.4 and archive are intentionally pending.

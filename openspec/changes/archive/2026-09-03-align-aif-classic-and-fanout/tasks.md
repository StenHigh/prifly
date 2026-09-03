## 1. Host-aware authoring and launcher

- [x] 1.1 Add declared roots for Codex CLI, Codex app and Claude Code, explicit `project compile --host`, and the root-relative context source mapping; verify each host seals only its own skills and rejected traversal/unknown-host cases in focused Go tests.
- [x] 1.2 Make `project init` preflight and create the three host-specific `prifly-run` entry points; verify a Claude launcher passes `claude-code`, a Codex launcher passes its own host, and an existing runner leaves no partial profile.
- [x] 1.3 Update authoring JSON schemas and reference YAML so an editor describes the root-relative context form; verify authoring corpus covers it without network or AI Factory.

## 2. Separate AI Factory workflows

- [x] 2.1 Replace the unreleased `aif-cycle` example with sequential `aif-classic`, including warmup, bounded state-carrying improve, implement, read-only verify/security/review, explicit `/aif-fix` terminal guidance and independent feature bypasses; verify compilation and graph assertions prove no classic parallel improve/review.
- [x] 2.2 Add the narrow `aif-fanout` plan-refinement package with YAML-declared parallel profile branches, aggregation, human choice and plan application; verify its graph alone contains the parallel join and documentation does not claim live model selection.
- [x] 2.3 Update example index, package READMEs, generic `prifly-run` wording and project fixtures to use `aif-classic`/`aif-fanout`; verify one shared graph seals host-specific skills through each host entry point.

## 3. Normative documentation and future protocol boundary

- [x] 3.1 Update current OpenSpec specs, terminology and roadmap from the accepted deltas, including high-priority `assisted-model-profile-protocol`; verify `openspec validate --strict --change align-aif-classic-and-fanout` passes.
- [x] 3.2 Record that `aif-fanout` is the future model-profile acceptance fixture but current checks prove graph/profile data only; verify no README, test name or delivery claim calls it actual provider/model/effort selection.

## 4. Verification and integrity

- [x] 4.1 Run focused compiler/CLI and example tests, then the relevant repository gates; verify a second classic improve iteration binds `iteration_output` and a blocked quality gate cannot reach commit.
- [x] 4.2 Inspect `git diff --check`, `git status --short` and historical evidence paths before commit; verify no historical evidence, manifest or user-owned `CONTINUATION.md` is rewritten or staged.

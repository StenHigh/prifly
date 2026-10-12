# Pri-Fly repository: rules for the runner

Read after SKILL.md; where the two conflict, this file wins. It cannot widen
what the engine measures (declared effects, claimed workspace, output slots).

- The task input is typed `aif:schema/task` (`title`, `description`, optional
  `reference`); write it to a file and pass `--input task=FILE`.
- Standing answers live in `.prifly/workflows/aif-classic/extend.yaml`
  (`profile: full`, `gate_checks`, `gate_warnings`, `plan_tests`, attended
  policy); pass a `--preflight-answer` or `--package-profile` only to override
  one of them for this Run, and say so in the report.
- `gate_checks` is the whole verify composition. `ci-check-once` runs the full
  local gate or validates its evidence; set CHECK_CONTEXT to the actual host
  permission profile (for example codex-app-authorized or claude-terminal).
  Different sandbox/escalated profiles need different labels; unknown means
  execute-only. A label grants no permissions. Keep it stable for repeat calls.
- Full race/e2e belong to release qualify, not the ordinary Run. Explicit
  targeted scenario tests from the plan still apply. Never replace semantic
  verify/review or accepted StepResult with a command cache hit.
- Before commit ensure staged bytes match tested working bytes; helper
  CHECK_FOR_COMMIT=1 checks that. CHECK_FORCE=1 explicitly reruns the local gate.
  Raw ci-check always runs. Reuse performs fresh vuln-check and mod verification.
- For release preparation pin candidate SHA and use scripts/ensure-release-checks.py;
  observe an active run, reuse a successful direct batch on the same SHA,
  explicitly retry failures with --force. Every different SHA needs its own CI
  verify/qualify. Local evidence cannot authorize tag or release.
- A normative change (spec, glossary, published contract, roadmap) starts with
  an OpenSpec change; a Run without one changes code, tests and references only.
- Commit messages are English conventional style (`feat:`, `fix:`, `docs:`),
  never amended. Pushing and tagging are the owner's own actions.
- Report in Russian, result first: done, deliberately not done, exact gate
  results.
- When the developer names an OpenSpec change instead of a task, build the
  task input from it: `title` = the change's proposal title, `description` =
  "Implement openspec/changes/<name>: <the open task ids, or all>; requirements
  in specs/ and design.md", `reference` = `openspec/changes/<name>`. Ask
  before narrowing to a subset of its tasks.
- A worktree Run commits on the claim branch `prifly/<claim>`; the next
  `project start` deletes it. The final report names that branch and says the
  owner merges it into `main` (`git merge --ff-only`) or pushes it first.
- If a quality-tail Run fails technically after accepted verify/review, use
  `project recover --prepare --source-run RUN --launch aif-classic-continuation`
  on the corrected package. Review the reused stages, failed frontier and
  digest before starting with `--expected-launch-digest DIGEST --allow-execution`.
  Follow the new Run with `run next`; never copy Artifact refs by hand or
  treat a passing process as an accepted StepResult. This does not replace
  `run reopen` or ordinary `project continue`.

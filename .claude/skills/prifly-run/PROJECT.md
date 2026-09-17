# Pri-Fly repository: rules for the runner

Read after SKILL.md; where the two conflict, this file wins. It cannot widen
what the engine measures (declared effects, claimed workspace, output slots).

- The task input is typed `aif:schema/task` (`title`, `description`, optional
  `reference`); write it to a file and pass `--input task=FILE`.
- Standing answers live in `.prifly/workflows/aif-classic/extend.yaml`
  (`profile: full`, `gate_checks`, `gate_warnings`, `plan_tests`, attended
  policy); pass a `--preflight-answer` or `--package-profile` only to override
  one of them for this Run, and say so in the report.
- `gate_checks` is the whole verify composition. `make race` is a release gate,
  not a Run gate; never add it inside a Run.
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

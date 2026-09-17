# aif-implement: project context

- When the plan cites `openspec/changes/<name>/tasks.md` items, tick each
  item (`- [ ]` → `- [x]`) in that file at the checkpoint that completes it.
  That file is the change's progress record; it is not the plan's checkboxes.
- Never mark roadmap milestones: the roadmap is
  `openspec/specs/delivery-roadmap/spec.md` and a milestone closes only by an
  acceptance change with evidence. `WARN [roadmap]` at most.
- `.ai-factory/DESCRIPTION.md` and `ARCHITECTURE.md` are maps: update them only
  when a package, entry point or gate changed. A design decision goes to
  `openspec/specs/architecture-decisions` through a change, never into them.
- Never touch `openspec/changes/archive/**`, `release-*.md`, the frozen
  bundles under `schemas/`, or `CONTEXT_STATE.md`.
- Commit messages: English, Conventional Commits (see the `aif-commit` context).

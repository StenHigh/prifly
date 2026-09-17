# aif-verify: project context

- "Nothing forgotten" includes the change: every `tasks.md` item the plan
  cites is ticked, `openspec validate --all --strict` passes, and nothing under
  `openspec/changes/archive/**` or the frozen `schemas/` bundles changed.
- Roadmap gate: the delivery-roadmap spec is direction, not a checklist.
  Missing linkage is a warning; fail only on a contradiction with a
  milestone's stated boundary.

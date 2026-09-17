# aif-plan: project context

Source priority for requirements, highest first:

1. The OpenSpec change the task names (`openspec/changes/<name>/`): its
   `specs/**` delta and `design.md` decide behaviour; its `tasks.md` is the
   task list.
2. `openspec/specs/<capability>/spec.md` — current normative truth;
   `openspec/SOURCE-OF-TRUTH.md` says which capability owns a rule.
3. `.ai-factory/*.md`, `openspec/specs/delivery-roadmap/spec.md`,
   `CONTEXT_STATE.md` — orientation and scope, never a contract.

- A task that changes a spec, the glossary, a published contract or the
  roadmap needs an OpenSpec change first. If the task names none and needs
  one: manual mode asks the user and stops; `HANDOFF_MODE=1` emits
  `ERROR [requirement-conflict]`. Never create or edit `openspec/changes/**`
  from this skill.
- When the task names a change, the plan's tasks are that change's `tasks.md`
  items in order: an item may split into steps, never disappear or be renamed.
  Each plan task cites its item id (`2.3`) and the verification command
  `tasks.md` names for it.
- `## Roadmap Linkage`: the roadmap is the delivery-roadmap spec and is
  read-only here; link the milestone the change names, else `none`.
- Never create a branch: the Pri-Fly claim or the current checkout owns it
  (`git.create_branches: false`).
- Documentation is `README.md`, `examples/` and `openspec/specs/`; never plan
  a `docs/` directory or a `/aif-docs` step.

# Pinned AI Factory decision inventory

This tree is checked against the local AI Factory skill revision used for the
example. The YAML files are the finite, preflightable part of the questionnaire
plus one declared runtime choice. They become a sealed Pri-Fly Decision Sheet.
The rest of the list is an inventory, not a claim that Core intercepts a native
chat dialog.

Pinned sources — a record of the bytes this adapter was written against, not a
guard. The host supplies whatever revision it has, and nothing in this package
compares the two: the skills are not in this repository, and CI compiles against
stubs. A hash below that no longer matches a host's skill means the bridges were
written for a different revision, which is worth knowing but is not an error
anything here can raise.


Upstream is the npm package `ai-factory` (github.com/lee-to/ai-factory); these
are the bytes of **2.19.0**. Against 2.18.1 the plan, implement, improve and
verify skills changed and `aif-warmup` appeared; commit, fix, review and the
security checklist did not move.

- `aif-warmup/SKILL.md` — SHA-256 `ad7fd1bdb62d97881b190ed596a175fdf02e9e92b38c2ad7b941a68a84ffb8b2`.
- `aif-plan/SKILL.md` v1.0.0 — SHA-256 `086d68806b9c8a27d8de51ae6389492715e9b4caa5abb277f9bc02fde047fe9d`.
- `aif-implement/SKILL.md` — SHA-256 `0269d7931c6a0c001c1fe6c99fec98b0a466e177259c302033fd4f02e5f8ae19`.
- `aif-verify/SKILL.md` — SHA-256 `fcb0e41cd63595b7e78d890696cb0f709f2a5c2fc377f6ba17f8b0677ab71ef7`.
- `aif-security-checklist/SKILL.md` — SHA-256 `3d9c4993ca6ed6cc83142ac481a672dc78dfefc2d73489496e58e8320b3e97ca`.
- `aif-commit/SKILL.md` — SHA-256 `3dbeec8295c3cc592faf67d1669295803d472944c30ee7daeb8d330b0c9c9028`.
- `aif-improve/SKILL.md` (upstream `2.x`) — SHA-256 `0e26c5d27d7a91e892474dccddacdfe63f175257671078a078694c0b0a7b412b`. Its
  `references/LIST-MODE.md`, `CHECK-MODE.md`, `EXAMPLES.md` and `VALIDATOR.md`
  are pinned as their own contexts, because a skill's own reference file is not
  carried by pinning the skill.

## aif-improve

Step 5 ends with the exact question “Apply these improvements?” and three
options, and the skill edits nothing before it is answered. That question is
declared as the `improve_apply` runtime decision, so the answer is recorded in
the Run rather than living in a chat window that no later step can see. The
`select` option still needs the upstream free-form follow-up about which
findings to take.

It stays `scope-changing` and not automatic: taking a refinement changes what
the Run was planned to build. 1.8.0 relabelled it ordinary and automatic so an
unattended Run could answer it — the first night had stalled there with nine
verified defects left unapplied — and 1.10.0 put the label back. Pri-Fly 0.9.0
made the trade unnecessary: the owner seals an answer before the Run starts
with `project start --runtime-answer improve_apply=<choice>`, and the bridge
applies it when the step asks. The night is covered by the owner's own answer
instead of by calling a scope-changing question ordinary for every Run.

## aif-plan

Preflight YAML records plan depth, tests, Full/Ultra logging, Full/Ultra
documentation, roadmap linkage, conditional milestone and free-form
constraints. A selected `plan_profile` is passed to `aif-plan` as its leading
mode (`fast`, `full` or `ultra`).

## aif-implement

Native dialogs can still concern uncommitted work, missing active plan,
commit checkpoints, project-rule capture, documentation handling, plan cleanup,
worktree merge/cleanup and the final verify-or-commit choice. Their options
depend on repository state and generated plan contents, so this upstream skill
does not currently emit a DecisionRequest for them.

## aif-commit

This route declares no commit-time decision. `commit_grouping` was declared
until 1.4.0 and never fired once: the implementation skill commits at its own
checkpoints, so the commit step always receives a clean workspace and the
upstream skill stops before its grouping question. A declared choice the
developer is never shown is worse than an honest absence, so it was removed
rather than left as evidence of a mechanism nobody exercised.

The step still exists for the case where work is left uncommitted, and its
adapter now says to skip the skill entirely when nothing is staged instead of
spending a session on a "nothing staged" warning.

Native dialogs can concern grouping an active Commit Plan, free-form adjusted
groups, confirming or editing a generated commit message, splitting unrelated
staged changes and pushing a completed commit. Push is an external effect and
remains subject to its own Pri-Fly authorization; a preference is not an
Approval or Grant.

An upstream native `AskUserQuestion` is not a Pri-Fly decision. It remains a
normal attended host interaction until the executor emits `DecisionRequest/1`.

## aif-security-checklist

Until 1.37.0 the context pinned `aif-security/SKILL.md`, a name no upstream
release has ever shipped; the skill of the canonical route is
`aif-security-checklist`, which verify's own text hands over to, and the pin
names it now. A host carrying the skill under the old name compiles only if
it also has it under the upstream one.

The step ran without an adapter until 1.11.0: its instructions pointed straight
at the pinned skill, so nothing told it about `gate_warnings`, about reporting
rather than saving, or about who decides the next stage. The skill's own
`allowed-tools` carry `Write` and `Edit` and it saves a report and an
ignored-item artifact when a person runs it, while the step declares no
workspace effect — bytes written there would be refused, not kept.

`gate_warnings: fix` and `stop` do the same thing at this gate, and the adapter
says so rather than implying a repair that cannot happen: no fix round is wired
after security, so a blocking result finishes the Run as `partial` with the
findings reported unchanged. Verify and review are the gates that loop.

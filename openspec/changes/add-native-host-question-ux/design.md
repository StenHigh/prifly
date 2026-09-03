## Context

See `proposal.md` for motivation and the delta specs for observable behavior.
`project init` now renders one generic `prifly-run` text with only the host ID
substituted. It also requires exact runner bytes on a clone, which prevents
silent replacement of reviewed instructions.

## Goals / Non-Goals

**Goals:**

- Render host-native finite developer decisions as native questions.
- Keep host identity, command protocol and the no-mutation-before-answer
  boundary unchanged.
- Allow a clone with a previously generated runner to create only its local
  configuration while its owner prepares a reviewed runner update.

**Non-Goals:**

- Add a Pri-Fly UI, a new command DTO, a workflow operator, or a runtime
  concept named "choice".
- Turn a RunBrief, file reference or arbitrary JSON into a misleading fixed
  option list.
- Claim that a native question tool is available in every Codex runtime.

## Decisions

### Two generated templates, three fixed host IDs

The renderer selects a Codex template for `codex-cli` and `codex-app`, and a
Claude template for `claude-code`; only the fixed host ID is substituted.
This preserves the three existing roots while avoiding a third near-duplicate
instruction set.

The Codex template tells the host to call `request_user_input` when it is
available. The Claude template tells it to call `AskUserQuestion`. Both state
the same protocol boundary: ask before a launch or workspace mutation, wait
for the returned answer, and use a one-question text fallback only when the
native tool is absent. The template uses the host tool's documented option
limit rather than inventing a common limit.

### Finite choices only; complete paging

The runner uses a native question for launch selection, Workspace mode and any
later developer decision with a complete finite option set already known to
the host. It marks the recommended option and describes the effect of each
option. It asks free-form values normally.

When a complete finite set is larger than the host tool allows, the runner
shows deterministic pages and retains navigation and refusal. It never omits
an option to fit one dialog and never promotes a recommendation to a default.
This is preferable to reading internal workflow source or adding schema fields
only to drive a presentation widget.

### Explicit compatibility window

New projects receive the two new templates. The exact-runner verifier accepts
the immediately preceding generated runner text as a legacy compatibility
form, but continues to reject unknown or modified runners. Thus an existing
clone remains bootstrapable without overwriting reviewed files. Its owner
updates the tracked runners explicitly and commits the diff to receive native
questions. The legacy form is test data, not a second independently authored
contract.

### Verification at the generator boundary

Unit tests compare rendered files with their expected host-specific templates,
including the fixed host ID, native tool name, fallback and paging rules. A
clone test proves that the legacy exact runner is accepted but unchanged.
Host UI rendering is exercised manually in one Codex runtime that exposes
`request_user_input` and in Claude Code; this is host evidence, not a Pri-Fly
runtime or release qualification claim.

## Risks / Trade-offs

- [A Codex runtime omits `request_user_input`] → The template waits for one
  explicit text response and never simulates a clickable UI or mutates state.
- [Host option limits change] → The templates defer to the tool schema and
  page known options instead of baking a shared numeric limit into Pri-Fly.
- [Legacy exact runners accumulate indefinitely] → Accept only the one
  predecessor during this migration; a future template migration must replace
  or deliberately retire that compatibility form in its own change.
- [Manual runner update is missed] → Fresh projects are correct; existing
  projects remain safe and their reviewed diff visibly contains the upgrade.

## Migration Plan

1. Release the generator with the new templates and one frozen legacy matcher.
2. A new project receives the new runners through `project init`.
3. Existing project owners replace each tracked `prifly-run/SKILL.md` with the
   generated host-specific text, review it and commit it; `project init` never
   performs that replacement.
4. If rollback is needed, restore the prior tracked runner commit. Authority,
   packages and Runs require no migration because they are not changed.

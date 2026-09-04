## ADDED Requirements

### Requirement: Assisted workspace-write intent сохраняет exact Workspace

New assisted workspace-write Run MUST bind every handoff and protected
workspace-write intent to its exact claimed Workspace identity and generation.
It MUST permit writes only inside that claim, including direct checkout mode;
the mode MUST NOT grant branch switching, reset, clean, deletion or access to
authority data. Existing worktree-only intent and Grant values remain valid
only for their pinned historical Runs and MUST NOT be silently reinterpreted.

#### Scenario: Checkout handoff proposes write
- **WHEN** an assisted host holds a direct checkout Workspace
- **THEN** its handoff and proposed intent name that exact claim and cannot
  address a different repository or authority path

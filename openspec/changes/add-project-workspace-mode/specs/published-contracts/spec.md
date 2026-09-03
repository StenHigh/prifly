## ADDED Requirements

### Requirement: Direct checkout state имеет отдельную compatible version boundary

Published machine contracts for direct checkout Workspace SHALL use a new
declared state/read compatibility boundary. Previous worktree-only schemas,
bundles and saved Runs MUST retain their original shape and meaning; a reader
without the new boundary MUST refuse new state rather than treating checkout as
a disposable worktree.

#### Scenario: Старый reader встречает новый Workspace mode
- **WHEN** binary without the declared compatible state reader opens a Run with
  direct checkout Workspace data
- **THEN** it refuses that authority state without changing or cleaning the
  repository

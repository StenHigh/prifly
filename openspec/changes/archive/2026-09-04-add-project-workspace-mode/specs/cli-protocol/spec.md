## ADDED Requirements

### Requirement: CLI запускает declared Project workflow с explicit workspace mode

CLI MUST provide one typed `project start` command for a declared Project
launch. It MUST require repository, launch ID, host and RunBrief/input sources;
it MUST accept only `worktree` and `checkout` as workspace mode. When invoked
without an interactive host, omitted mode MUST default to `worktree`. Invalid
launch, host, input, repository identity or workspace mode MUST return a stable
diagnostic without partial package registration, claim or Run. The response
MUST name Run and selected Workspace identities.

#### Scenario: CLI starts default isolated workspace
- **WHEN** user starts a valid Project launch without workspace flag
- **THEN** response reports an isolated worktree Workspace and its Run identity

#### Scenario: CLI rejects an unknown workspace mode
- **WHEN** user passes a workspace mode other than `worktree` or `checkout`
- **THEN** CLI returns `invalid_usage` and creates no package, claim or Run

## ADDED Requirements

### Requirement: Project entry points select their host mechanically
`prifly project init` SHALL record the fixed repository-relative skills roots
for `codex-cli`, `codex-app` and `claude-code` and write one `prifly-run`
entry point below each corresponding host directory. Each entry point SHALL
invoke Project compilation with its own host identity. The public compile
command MUST accept that host identity and MUST NOT infer it from which
directories happen to exist. Init MUST reject an unsafe root or any existing
runner without overwriting a runner or profile. These entry points support
Codex CLI, Codex app and Claude Code without making any of them a Core
dependency.

#### Scenario: Claude Code starts a shared project
- **WHEN** developer invokes `prifly-run` from `.claude/skills`
- **THEN** it compiles using host `claude-code` and never reads the Codex
  skills root

#### Scenario: Existing host runner prevents initialization
- **WHEN** any of the three host runner paths already exists
- **THEN** init returns a safe diagnostic and creates neither profile nor
  another runner

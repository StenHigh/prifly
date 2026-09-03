## Context

See `proposal.md`. The current `aif-cycle` mixes a two-branch review fanout
with the normal AI Factory route, and context source handling hard-codes the
Codex CLI directory. The assisted-session adapter accepts a host session but
has no field or admission evidence for provider/model/reasoning selection.

## Goals / Non-Goals

**Goals:**

- Make the normal AI Factory route readable as one sequential `aif-classic`
  package while retaining the existing bounded, state-carrying improve loop.
- Keep model-profile fanout separate in a small `aif-fanout` package that is
  useful as a future acceptance fixture.
- Let one YAML package run with Codex CLI, Codex app or Claude Code by resolving
  its contexts from the host identity supplied by its own entry point.

**Non-Goals:**

- No Core model/provider field, host-side process control, automatic `/aif-fix`,
  automatic task intake, new external dependency or model SDK.
- No compatibility alias for the unreleased `aif-cycle` example; its tracked
  callers and documentation move to `aif-classic` together.
- No claim that a fanout profile changes the live model before the separate
  `assisted-model-profile-protocol` change qualifies that host contract.

## Decisions

### 1. Classic is a single sequential package

Replace the sample folder with `examples/aif-classic`. Its root composes
`warmup -> plan -> improve -> implement -> verify -> security -> review ->
commit`. `explore`, `grounded`, `rules`, `qa` and `evolve` remain documented
operator choices rather than hidden mandatory stages.

Plan improvement has one assisted `aif-improve` body. That body invokes the
skill, lets the skill ask the developer which proposals to apply, and returns
the resulting plan artifact reference plus its no-change state. The repeat's
initial binding uses the plan stage and next binding uses iteration output.
The project-limited maximum stays three; reaching it uses the existing human
continuation route. This is an explicit Pri-Fly policy overlay, not a claim
that every AI Factory host repeats improve this way.

`verify`, `security` and `review` are read-only quality gate steps and use one
`aif-gate-result` schema. A blocking gate goes to a terminal result carrying
`/aif-fix`; it never calls `aif-fix`, writes workspace, retries or commits.
The existing feature mechanism supplies independent safe bypasses for improve,
verify, security and review.

### 2. Fanout is a distinct, narrow package

`examples/aif-fanout` is a plan-refinement package, not a second complete
development route. It contains the prior pattern: parallel independent
`aif-improve` profile branches, aggregation, human choice and application to
the plan. Each branch's profile is an explicit YAML literal with its intended
host/model/effort description; no code treats it as dispatch authority.

This keeps the valuable parallel graph and gives the future assisted-host
model-profile protocol one stable acceptance fixture, without duplicating the
classic workflow or promising a switch that cannot happen today.

### 3. Host entry point selects one of three profile roots

Profile v2 declares three fixed named roots: `codex-cli` -> `.codex/skills`,
`codex-app` -> `.agents/skills`, and `claude-code` -> `.claude/skills`.
`project init` preflights then writes a thin `prifly-run` skill into each root.
The source location determines the host mechanically: its wrapper calls
compilation with `--host codex-cli`, `--host codex-app` or `--host
claude-code`. Users do not choose a root, and compiler never guesses from the
set of directories on disk.

Context YAML uses one portable source mapping:

```yaml
source:
  root: host_skills
  path: aif-plan/SKILL.md
```

`root: host_skills` means only the root selected by `--host`; it does not name a
provider or infer a host. A raw source string remains available for content in
`.prifly`. There is no interpolation, environment access or host
autodetection. Compiler canonicalizes the result, confines it to the selected
root, checks a regular file and seals its bytes.

The three paths are current Project profile contract entries, not a Core host
abstraction. Adding another host later requires its own compatibility decision.

### 4. Model selection is a separate protocol change

The high-priority roadmap task `assisted-model-profile-protocol` will define
the versioned request, host support/denial, sealed requested profile and
reported actual profile required to prove real selection. It will reuse
`aif-fanout` for host integration evidence. This change intentionally stops at
compiler graph/profile-data checks.

## Risks / Trade-offs

- [AIF installation has an incomplete skills directory] -> compiler fails on
  the exact missing context before sealing; no downloader or hidden fallback.
- [A user mistakes profile data for model control] -> package README and
  roadmap state the limitation beside `aif-fanout` and its tests assert graph,
  not provider behavior.
- [Host paths escape repository] -> fixed roots and one shared canonical
  confinement check are used by init and context resolution.
- [Both Codex and Claude directories exist] -> source-specific wrapper supplies
  `--host`; compiler never chooses by filesystem presence.
- [A quality gate is excluded] -> graph has an explicit feature bypass; no
  stage deletion or reinterpretation of a sealed Run occurs.

## Migration Plan

1. Add host-aware context authoring, the three generated entry points and
   explicit `project compile --host` with focused compiler/CLI checks.
2. Replace the unreleased `aif-cycle` sample with `aif-classic`; update its
   README, example index and test fixtures in the same commit.
3. Add the independent `aif-fanout` sample and assertions that it alone
   contains parallel profiles.
4. Sync accepted OpenSpec deltas to current specs. Rollback is a normal source
   revert; no Run or public versioned DTO is changed by this work.

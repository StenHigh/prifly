## ADDED Requirements

### Requirement: Project context resolves the selected host skills root
A `prifly-project-profile/2` SHALL declare the repository-relative skills
roots for `codex-cli`, `codex-app` and `claude-code`. A context source MAY name
a regular file relative to the skills root of the explicit compilation host
through an explicit YAML source mapping. Compiler MUST reject an unknown host,
absolute path, traversal, symlink escape, absent file or a source outside
`.prifly` and that host's declared skills root before sealing. It MUST seal the
selected exact bytes; host identity MUST NOT change the workflow graph or grant
authority.

#### Scenario: Claude Code skill is sealed
- **WHEN** compiler receives `claude-code` and context names
  `aif-plan/SKILL.md` relative to the selected host skills root
- **THEN** compiler seals that exact file and accepts no file outside the
  repository boundary

#### Scenario: Context escapes its root
- **WHEN** a context source uses `..` or resolves outside `.prifly` and the
  selected host skills root
- **THEN** compiler rejects the Project package before output, authority
  mutation or Run

#### Scenario: Two hosts share one project workflow
- **WHEN** Codex CLI and Claude Code compile the same Project workflow folder
- **THEN** each seals its own declared skills bytes while the YAML graph and
  package identity remain unchanged

### Requirement: AI Factory examples separate classic and fanout workflows
Repository SHALL publish two independent optional AI Factory Project workflow
folders. `aif-classic` SHALL contain the sequential documented development
path: warmup, plan, bounded plan improvement, implement, verify, security,
review and commit. Its improvement repeat MUST bind the first iteration to the
initial plan and every next iteration to the prior iteration's corrected plan;
it MUST contain no parallel improve or review stage. A blocking quality result
MUST finish with the explicit next action `/aif-fix` and MUST NOT invoke that
skill or alter the workspace automatically.

`aif-fanout` SHALL be a separate YAML package which may join independent
declared profiles in parallel and aggregate their outputs. Before a qualified
assisted-host model-profile protocol exists, its profile fields are declared
data and instructions only; compiler, Core and example documentation MUST NOT
claim that they selected a provider, model or reasoning level.

Both folders MUST remain optional Project packages. Excluding an optional
quality feature MUST use the declared bypass in its own graph and MUST NOT
delete stages or modify a sealed Run.

#### Scenario: Classic plan improvement advances state
- **WHEN** `aif-classic` compiles with plan improvement enabled
- **THEN** its second permitted improvement iteration receives the corrected
  plan output of the first iteration, not the original workflow input

#### Scenario: Classic quality gate finds a blocker
- **WHEN** verify, security or review reports a blocking result
- **THEN** `aif-classic` returns the gate result and the next action `/aif-fix`
  without automatically fixing, repeating review or committing

#### Scenario: Fanout is selected explicitly
- **WHEN** project profile names `aif-fanout` instead of `aif-classic`
- **THEN** compiler seals the fanout graph with its declared parallel join and
  does not add that join to `aif-classic`

## ADDED Requirements

### Requirement: Project init prepares a context-capable authority

`prifly project init` MUST create its separate authority with the current Core
context configuration required to pin selected host skills and other context
resources. A Project launch MUST reject an older or incompatible authority
before package registration, Workspace claim or Run creation; it MUST NOT
silently reinterpret that authority's existing Runs. When a clone already has
a valid tracked profile and exact host runners, init MUST create only its absent
ignored local authority configuration; shared Project YAML and runners remain
unchanged.

#### Scenario: Старый Core authority выбран для Project launch
- **WHEN** declared Project launch получает authority без current context
  configuration
- **THEN** CLI возвращает stable incompatibility diagnostic без package, claim
  или Run

#### Scenario: Clone получает свою authority
- **WHEN** tracked Project profile and host runners are already present, but the
  machine-local configuration is absent
- **THEN** `project init` creates the local authority configuration without
  replacing the profile or runners

### Requirement: Project launch является единственной исполнимой точкой входа

Public Project launch MUST принимать exact ID объявленного `workflow` launch,
explicit host и typed значения только его объявленных input ports. Он MUST
compile, seal и зарегистрировать exact package before creating Run; source YAML,
host skill bytes и effective inputs MUST become pinned Run inputs. Launch MUST
not выбирать сценарий по тексту задачи, default launch или наличию файлов.
Interactive project host MUST require an explicit `worktree` or `checkout`
selection before it invokes the launch; absence of that answer is a wait, not
a fallback to a different Workspace. A non-interactive CLI invocation MAY use
the declared command default.
Запуск Project workflow не запускает model/provider и не даёт host новых
полномочий: assisted handoff остаётся отдельным existing contract.

#### Scenario: Объявленный launch запускается
- **WHEN** пользователь называет существующий launch, declared host и все
  required inputs
- **THEN** система создаёт Run только из sealed revision этого launch и
  возвращает его identity вместе с выбранным workspace

#### Scenario: Launch не объявлен
- **WHEN** пользователь называет отсутствующий или не-workflow launch
- **THEN** система отказывает до compilation, package registration, claim или
  Run creation

#### Scenario: Host не получил выбор Workspace
- **WHEN** пользователь выбрал launch в диалоге, но не назвал worktree или
  checkout
- **THEN** host задаёт этот единственный вопрос и не создаёт package, claim или
  Run до ответа

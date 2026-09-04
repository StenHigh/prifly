## ADDED Requirements

### Requirement: Project launch явно связывает Run с выбранным Workspace

Перед первым assisted workspace-write Project launch MUST выбрать один Workspace
для exact repository и зарегистрировать его authority ownership вместе с
admission boundary. Interactive host MUST получить explicit `worktree` или
`checkout` выбор; only non-interactive CLI omission resolves to `worktree`.
`checkout` передаётся явно, чтобы работа велась в текущем checkout repository.
Этот выбор относится к одному Run и не превращает profile, authority root или
repository в синонимы.

#### Scenario: Пользователь выбирает current checkout
- **WHEN** Project launch получает explicit `checkout` mode
- **THEN** новый Run получает Workspace текущего canonical repository checkout,
  а не новый Git worktree

#### Scenario: Non-interactive режим не назван
- **WHEN** direct non-interactive Project launch не получает workspace mode
- **THEN** он выбирает isolated Git worktree и возвращает его exact workspace
  identity

## ADDED Requirements

### Requirement: Workspace claim сохраняет exclusivity в обоих Git режимах

Authority MUST distinguish a disposable `worktree` Workspace from a direct
`checkout` Workspace while using the same canonical physical repository
identity, owner, generation and lease conflict rules. A `worktree` claim MAY
create and later clean up only its own confined directory. A `checkout` claim
MUST refer to the current canonical Git checkout and MUST NOT create, delete,
switch branch, reset or clean that checkout. Both modes MUST block a
conflicting Pri-Fly Workspace claim until it is explicitly settled.

#### Scenario: Checkout mode leaves Git topology unchanged
- **WHEN** authority admits a direct checkout Workspace
- **THEN** no Git worktree is added and no branch, HEAD or tracked file is
  changed by admission itself

#### Scenario: Existing Workspace is held
- **WHEN** another Run requests either mode for the same physical repository
- **THEN** authority rejects the conflicting admission without creating a
  directory or changing the checkout

### Requirement: Assisted handoff не зависит от worker socket

Runtime MUST создавать owner-only local Unix socket только непосредственно
перед запуском managed local worker, которому нужен publication transport.
Assisted handoff, который передаётся существующему host и не запускает local
process, MUST достигать wait без этой зависимости. Если managed worker требует
socket, но платформа запрещает его создать, Run MUST получить stable
`local_socket_unavailable` refusal before process dispatch; это не является
успешным handoff или скрытым fallback transport. `doctor` MUST предварительно
показывать availability этой возможности; это локальная проверка, не promise,
что permission не изменится позже.

#### Scenario: Assisted Project launch на socket-restricted platform

- **WHEN** declared launch доходит до assisted host handoff без managed worker
- **THEN** он ждёт host без создания Unix socket

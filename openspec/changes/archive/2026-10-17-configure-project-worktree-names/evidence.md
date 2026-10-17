# Проверка реализации configure-project-worktree-names

Дата: 2026-10-17. Реализация выполнена напрямую, без `prifly-run`.
Проверенный код — рабочее дерево поверх
`e91e11fb311e05b4e659c7e07c51ae215f007e28`, Linux amd64, Go 1.27.2,
CGO_ENABLED=1, gcc. Коммит и release этим change не создавались.

## Реализованный контракт

- Общий `worktree_naming` в явном profile `/4`: template, constant или declared
  JSON input binding с RFC 6901 pointer; shared profile `/2`/`/3` не мигрирует сам.
- Read-only prepare показывает основу и candidate с `reserved: false`.
  Digest закрепляет policy/values; текущая доступность в него не входит.
- Durable preparing reservation выбирает `base`, `-02`…`-9999` с учётом claims,
  directory/symlink и Git ref collisions. Effects выполняются вне transform,
  no-overwrite creation и generation/CAS защищают ownership.
- Claim ID остаётся служебным; path/branch сохранены в claim. Повтор successful
  command возвращает original result; pending preparation не расходует suffix.
  Handover сохраняет имена, включая удаление policy и queue-bound resume.
- Новые closed формы: profile `/4`, summary/start `/5`, authority claims `/4`,
  claims view `/3`. Saved Run contracts и historical bundles не расширены.
- Capability inventory, author index, полный пример и generated host runner
  показывают настройку. Exact предыдущий runner сохранён в upgrade inventory.

## Проверки

| Команда / проверка | Результат |
|---|---|
| `make test-changed` | PASS; из-за schema tooling выполнены все Go packages и `schemas-check` |
| Целевые naming/claim/pointer/continuation tests | PASS; initial failures исправлены и перепроверены |
| `TestGlossaryBindings` | PASS |
| `TestEveryDeclaredCapabilityIsInTheAuthorIndex` | PASS в полном Go gate |
| `make vet` | PASS: native и `CGO_ENABLED=0` для Linux/Darwin |
| `make fmt-check refusal-check` | PASS |
| `make schemas-check` | PASS; новый bundle воспроизводим, прежние bytes совпадают |
| `openspec validate configure-project-worktree-names --strict` | PASS |
| `openspec validate --all --strict` | PASS: 34/34 |
| `make ci-check-once CHECK_CONTEXT=linux-amd64-go1.27.2-cgo1-gcc-codex-direct` | PASS, exit 0; полный свежий batch |
| `git diff --check` и protected paths audit | PASS |

Итоговый batch включает `go test ./...`, vet и staticcheck для Linux/Darwin,
vuln-check, schemas-check, release-CI contract checks, Python script tests,
shard-check и свежий `go mod verify`. Staticcheck: 12 packages на каждой
платформе, без замечаний. Runtime shard-check: 727 tests, каждый ровно в одном
из трёх shards. Проверка модулей: `all modules verified`.

Vuln-check: 0 symbol-level и 0 imported-package findings; сканер также сообщил
об одном finding на уровне required module, без вызова уязвимых symbols из
проверяемого кода. Gate завершился успешно; зависимости этим change не менялись.

Из-за существующего внешнего symlink fingerprint работает в execute-only:
проверки выполнены полностью, reusable success receipt не создан. Полный batch
log текущего checkout:
`.cache/verification/4b6a3658eca44a868dcae556ce0551c9.log`;
вывод wrapper с `go mod verify` — `/tmp/prifly-naming-ci-check-once.log`.

## Сценарии приёмки

- `TestProjectNamingProfileIsExplicitAndClosed`,
  `TestProjectNamingValueAndPointerRefusalsBeforeMutation`: профиль, закрытая
  форма, missing/null, nested/escaped/empty pointer, unsafe values и ранние отказы.
- `TestCLIProjectNamedWorktreePrepareStartAndFreshness`: prepare без mutation,
  default/ArtifactRef resolution, suffix observation вне digest, stale policy,
  original response на повтор команды.
- `TestNamedWorktreeConcurrentCLIProcessesFromPublishedExample`: отдельные
  CLI processes с barrier, два независимых старта и одновременный duplicate;
  три claim/Run на четыре successful команды, source checkout не меняется.
- `TestNamedWorktreeConcurrentReservationsAndForeignObjects`,
  `TestNamedWorktreeExternalCollisionRetriesSameClaim`: concurrent reservations,
  directory/ref/symlink/descendant-ref collisions, сохранение claim identity и
  чужих объектов при retry/cleanup.
- `TestNamedWorktreeInterruptedPreparationPreservesIdentity`: synthetic durable
  states до/после directory/ref/tree creation и activation; повтор не создаёт
  новое имя, cleanup использует проверяемые evidence/identity.
- `TestNamedWorktreeCleanupRejectsForeignBranchAndSymlinkRoot`: отказ удаления
  подменённой ветки и запрет выхода через symlink root.
- `TestNamedWorktreeAllocationLifecycleAndDuplicate`,
  `TestNamedWorktreePersistedContractsAndExhaustionPreview`: lifecycle, reuse с
  новым generation, фактические persisted/view schemas, old-contract refusal,
  bounded exhaustion без тысяч рабочих копий.
- `TestCLINamedClaimResumeKeepsNamesAfterPolicyRemoval`: native resume/handover
  с прежними path/branch, новой generation, без policy и с queue association.
- `TestStaleAdmissionObservationLeavesNoReceiptOrRun`: transient admission
  observation rollback до reducer, затем revalidation того же command.
- Полный regression gate покрывает retained profiles/claims, checkout,
  independent worktrees, command-only launch без Git/host, queue, calls,
  continuation/recovery и совместимый runner upgrade.

## Границы

Это local compile/runtime/CLI evidence; реальные AI/tracker actors не вызывались.
Vet/staticcheck читают Darwin без заявления live host qualification. Race и
полный e2e остаются release-only воротами по `AGENTS.md`; roadmap/product gate
не закрывается проверкой документа или этим локальным batch.

Проверено отсутствие изменений 748 tracked protected schema/archive files,
реального `.prifly/project.yaml` и historical release evidence. После финального
batch менялись только этот отчёт и completion markers в `tasks.md`.

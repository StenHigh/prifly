## 1. Baseline и storage compatibility

- [ ] 1.1 После интеграции #8 сверить current storage version, quota enforcement inventory и active contracts; добавить failing quota-recovery и read-only preparation tests. Проверка: `go test ./internal/local ./internal/runtime ./cmd/prifly -run 'Test.*(Storage|Budget)' -count=1` воспроизводит отсутствие owner recovery на baseline; process-env/package-state исключения отделены от новых `t.Parallel()` tests.
- [ ] 1.2 Реализовать следующую storage migration с durable finite ceiling/bootstrap generation 0, сохранив stock 512 МиБ и прежние authority/Run/event/claim bytes. Проверка: migration/verify tests на storage 9 и новом version, atomic crash rollback; read-only show/prepare не меняют header/data; historical dev overrides не угадываются.
- [ ] 1.3 Подтвердить stale old writer fence и корректное bootstrap/open поведение options. Проверка: fixture со старым writer, открытым до upgrade, отказывает на следующей write transaction; новый Store reopening не сбрасывает persisted limit; несовместимый explicit option не становится скрытым override. Existing saturation tests переведены на finite initial setup без production bypass.

## 2. Atomic budget control и enforcement

- [ ] 2.1 Добавить authority control record и atomic metadata/control/receipt mutation с owner/reason/command provenance и generation CAS. Проверка: targeted local/runtime tests дают один winner для конкурентных settings; mismatch identity/epoch/config, invalid/range/zero/overflow/decrease/same-value/new-command и insufficient headroom отвергаются до setting commit; exact duplicate возвращает прежний result без generation advance.
- [ ] 2.2 Перевести все quota consumers на current persisted limit внутри их transaction, включая already-open store instances и StorageUsage. Проверка: multi-connection tests ordinary/coupled/receipt-only/telemetry paths видят updated ceiling; следующий overflow по-прежнему `storage_budget_exhausted`, control/settlement reserve и other policy guards сохранены.
- [ ] 2.3 Проверить что quota control выше прежнего soft limit остаётся атомарным, а запись самого setting учитывается в target allocation. Проверка: small-headroom/post-write-overflow rollback, disk-full/failed commit и lost response fixtures не показывают неподтверждённый success; metadata/audit invariant verified, никакой I/O в pure transform.

## 3. CLI и публикация contracts

- [ ] 3.1 Добавить read-only `storage budget show`, `doctor --storage` и `storage budget set ... --prepare`, с новым status/review schemas. Проверка: public CLI даёт authority/epoch, allocation/page metrics, effective limit/source/generation, target/reason/upgrade plan/digest; legacy/new header/data/claims/registry не меняются, mutually exclusive doctor views не ломают #8/plain doctor.
- [ ] 3.2 Добавить reviewed set с explicit digest/command и новым result schema; digest защищает owner request/configuration, commit rechecks actual usage. Проверка: stale generation/epoch/edited request, usage growth, successful raise/restart, duplicate и command-ID payload conflict покрыты CLI tests; set не вызывает drive, не создаёт Run/slots и не принимает pending outputs.
- [ ] 3.3 Обновить typed quota diagnostics, safe actions/help и отдельные embedded/public schemas/status-review-result/control fixtures. Проверка: `make refusal-check`, `make schemas-check` и real emitted response/record validation; previous Problem/plain-doctor/Run-state/read contracts byte-compatible; current compatibility manifest объявляет реально используемую storage version.
- [ ] 3.4 Добавить Authority storage budget в terms с новыми Go/JSON mappings, capability `storage_budget_management`, author index и `examples/authoring/storage-recovery-guide.md`, troubleshooting/entry-point ссылки. Проверка: `TestGlossaryBindings`, `TestEveryDeclaredCapabilityIsInTheAuthorIndex` и смысловое review CLI полей/minimum storage version; guide не изобретает YAML настройку, auto-grow, GC либо disk-space guarantee.

## 4. Восстановление того же Run

- [ ] 4.1 Добавить reusable public CLI fixture saturation → read-only review → explicit raise → restart → drive с accepted producer и control frontier. Проверка: тот же Run ID, producer не повторяется, accepted refs/digests/provenance, claim generation и dirty/untracked file hashes сохранены; отсутствуют manual SQL/history deletion/authority substitution/fork. Записаны binary version/commit/hash и точная последовательность команд.
- [ ] 4.2 Проверить pending acceptance/checker, уже открытый driver, concurrent writes и повторное исчерпание quota. Проверка: `go test ./internal/runtime -run 'Test.*(Storage|Budget|Acceptance)' -count=1` и public fixture подтверждают новый checker admission без producer replay, current limit без reopening и штатные отказы при новом overflow/Stops/unknown effect; cancellation/settlement остаются разрешёнными по обычным правилам.

## 5. Gates и operational handoff

- [ ] 5.1 Выполнить рабочие `make test-changed` или `make check-fast`, targeted storage/runtime/CLI checks, `make vet`, `make refusal-check`, `make schemas-check`, `openspec validate recover-authority-storage-budget --strict` и `git diff --check`. Проверка: no driver leakage, pure transforms, реально достижимые schemas; quota/scan/per-snapshot/disk/incompatible version reasons различаются.
- [ ] 5.2 Перед implementation commit выполнить один `make ci-check-once CHECK_CONTEXT=<actual-profile>` с `CHECK_FOR_COMMIT=1` при staged check. Проверка: actual snapshot/context/toolchain и fresh vuln/mod requirements соблюдены; `git diff --name-status <baseline>` и protected hashes доказывают неизменность archive/release evidence, старых public bundles, канонических workflows и saved Run meanings.
- [ ] 5.3 Подготовить exact-binary recovery handoff для владельца и, при выпуске, fixed-SHA release checks по AGENTS.md. Проверка: guide показывает explicit status/review/set и отдельный drive существующего Run, downgrade/storage migration limitations названы; реальная authority не менялась без operational запроса, local/synthetic PASS не выдан за восстановление исходного Run или qualification всех profiles.

## Acceptance traceability

| Issue #7 expectation | Tasks |
|---|---|
| Поддержанный owner recovery без потери Run/workspace/evidence | 2.1–2.3, 3.1–3.2, 4.1–4.2 |
| Actual usage/limit и concrete next actions | 3.1, 3.3–3.4 |
| Отсутствие автоматического ослабления guard | 2.1–2.3, 3.2, 4.2 |
| Сохранение accepted stages и pending boundary | 4.1–4.2 |
| Restart/concurrency/compatibility | 1.2–1.3, 2.1–2.3, 3.2 |

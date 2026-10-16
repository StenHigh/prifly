# План реализации: восстановление Authority storage budget

Branch: prifly/e9dec1244fb741be6ca344fce8af2541ef1116f3e2b2138650134e74bd2fc534
Created: 2026-10-16
Baseline: 9e162b04 (исправление #8 и его archive уже в main).

## Original Request

Влей исправление #8 в main и архивируй change fix-unbound-claim-ownership-scan. Затем начинай реализацию change для #7.

## Настройки

Testing: yes; Logging: minimal; Docs: no (отдельного checkpoint нет; operator guide обязателен по change).
Профиль full, attended. Все standing answers и plan_constraints из decision_context сохранены.
План — маршрут исполнения; единственный progress ledger — `openspec/changes/recover-authority-storage-budget/tasks.md`.

## Roadmap Linkage

Milestone: "none"
Rationale: change не назначает milestone; scope — отдельное исправление #7, не закрытие P1/P2 gate.

## Requirements Reconciliation

Authority: active change specs/** и design.md > main specs > AI Factory context/roadmap.
Все current source sets перенесены по SOURCE-OF-TRUTH; ownership не меняется.
Root AGENTS и sealed gate_checks старше старых советов base rules о частоте полных gates.

| Комбинация / решение | Источник | Проверка |
|---|---|---|
| storage 9 read-only show/prepare | design §5, published-contracts delta | header/history/files byte-identical, legacy_default, upgrade 9→10 в review |
| storage 9 writer / storage 10 reopen | design §4–5 | атомарный bootstrap generation 0, конечный limit, explicit option mismatch отказывает |
| уже открытый writer v9 / migration v10 | published-contracts delta | отдельный старый executable держит соединение; следующая write отказывает до mutation |
| show / prepare / explicit set | cli-protocol delta, design §1–3 | отдельные closed status/review/result schemas; prepare ничего не пишет |
| exact duplicate / stale generation / изменённый payload | design §3 | recorded result прежде CAS; command conflict; именованные отказы |
| ordinary usage вырос / конфигурация прежняя | design §3 | digest неизменен, current allocation rechecked в write transaction |
| target invalid / post-write allocation слишком велика / disk failure | runtime delta, design §2 | полный rollback metadata/control; честный failure/unknown |
| уже открытый Store / receipt-only / coupled / telemetry | runtime delta, design §4 | все consumers читают один persisted ceiling внутри transaction |
| accepted producer / pending checker / Stop / unknown effect | runtime delta, design §7 | тот же Run ID и claim, один producer, accepted bytes неизменны; controls сохраняются |

Представительные существующие fixtures: acceptance_storage_test.go, repeat_control_test.go, check_execution_test.go, telemetry_test.go; они переходят с process-local injection на finite initial bootstrap и supported saturation. Нельзя оставлять hidden production bypass ради теста.

## Маршрут задач

Каждая строка ссылается на исходный id и его полную приёмку в tasks.md; исходные формулировки и проверки не сужаются.

- **1.1** — Inventory store.go/store_usage.go/store_samples.go, quota callers и CLI dispatch; сначала красные owner recovery/read-only tests. Приёмка: полный пункт 1.1 в `openspec/changes/recover-authority-storage-budget/tasks.md`.
- **1.2** — store_budget.go: migration 9→10, scalar + generation-0 provenance, verification invariant, unchanged prior Run/event bytes. Приёмка: полный пункт 1.2 в `openspec/changes/recover-authority-storage-budget/tasks.md`.
- **1.3** — Bootstrap-only StoreOptions, explicit mismatch refusal; old executable fixture; обновить saturation fixtures без production bypass. Приёмка: полный пункт 1.3 в `openspec/changes/recover-authority-storage-budget/tasks.md`.
- **2.1** — AuthorityStorageBudget control + атомарный metadata/state/receipt update, identity/epoch/CAS, bounded reason, finite raise и exact retry. Приёмка: полный пункт 2.1 в `openspec/changes/recover-authority-storage-budget/tasks.md`.
- **2.2** — Все softLimitBytes consumers используют transaction-owned persisted setting; multi-connection, coupled, receipt-only и telemetry проверки. Приёмка: полный пункт 2.2 в `openspec/changes/recover-authority-storage-budget/tasks.md`.
- **2.3** — Savepoint/post-write target headroom; rollback/commit-failure/lost-response fixtures; pure transform без driver leakage. Приёмка: полный пункт 2.3 в `openspec/changes/recover-authority-storage-budget/tasks.md`.
- **3.1** — runtime/storage_budget.go и cmd/prifly/storage.go: show, doctor --storage, prepare; legacy/new read-only byte preservation. Приёмка: полный пункт 3.1 в `openspec/changes/recover-authority-storage-budget/tasks.md`.
- **3.2** — Explicit reviewed set --bytes --reason --expected-review-digest --command; current intent/CAS и actual usage recheck, без drive. Приёмка: полный пункт 3.2 в `openspec/changes/recover-authority-storage-budget/tasks.md`.
- **3.3** — Четыре новые отдельные embedded/public closed schemas; schema-gen, reachability, named quota diagnostics, help/safe actions, compatibility inventory. Приёмка: полный пункт 3.3 в `openspec/changes/recover-authority-storage-budget/tasks.md`.
- **3.4** — terms.md canonical definition/bindings, capability storage_budget_management, examples index, storage-recovery-guide и troubleshooting. Приёмка: полный пункт 3.4 в `openspec/changes/recover-authority-storage-budget/tasks.md`.
- **4.1** — Public CLI fixture accepted producer → saturation → review → set → restart → drive, dirty/untracked/refs/claim hashes и exact binary handoff. Приёмка: полный пункт 4.1 в `openspec/changes/recover-authority-storage-budget/tasks.md`.
- **4.2** — Pending checker, уже открытый driver, concurrent updates/writes, repeated overflow, Stop/unknown effects и settlement reserve. Приёмка: полный пункт 4.2 в `openspec/changes/recover-authority-storage-budget/tasks.md`.
- **5.1** — Рабочие checks, все targeted commands из tasks.md, schemas/purity/CGO refusal checks, OpenSpec strict и diff check. Приёмка: полный пункт 5.1 в `openspec/changes/recover-authority-storage-budget/tasks.md`.
- **5.2** — Один финальный staged ci-check-once с actual context; protected hashes старых bundles/archive/workflows и сохранённой семантики. Приёмка: полный пункт 5.2 в `openspec/changes/recover-authority-storage-budget/tasks.md`.
- **5.3** — Exact binary/version/hash и recovery sequence в implementation-evidence.md; реальная authority и release qualification остаются отдельными действиями. Приёмка: полный пункт 5.3 в `openspec/changes/recover-authority-storage-budget/tasks.md`.

Зависимости: baseline/compatibility → атомарный control/enforcement → CLI/contracts/docs → recovery fixtures → final gates/handoff.

## Проверки

Нормативный набор команд — tasks.md §1.1, §4.2, §5.1–5.2 и sealed gate_checks.
Baseline command уже запущен в этом claim worktree: `go test ./internal/local ./internal/runtime ./cmd/prifly -run 'Test.*(Storage|Budget)' -count=1`; лог `/tmp/prifly-issue7-baseline-tests.log`.
`openspec validate recover-authority-storage-budget --strict` и `git diff --check` выполнены успешно до записи плана.
Новые regression checks сначала краснеют на baseline; зелёный existing baseline не доказывает наличие owner recovery.
CHECK_CONTEXT=codex-app-escalated-bwrap-unavailable. Полные race/e2e — только release, не этот Run.

## План коммита

Логические checkpoints: 1.1–1.3 (storage boundary), 2.1–2.3 (enforcement), 3.1–3.4 (public surface), 4.1–5.3 (recovery/evidence).
Один итоговый compatibility commit после всех checkpoints: `feat(storage): preserve Run evidence with reviewed budget recovery`.
Причина объединения: миграция durable setting, все guards и публичный recovery образуют одну границу совместимости; промежуточный writer с обходящим setting guard не публикуется.
План сохраняется до будущего archive change; merge #7, push, tag и изменение исходной authority не входят в этот Run.

## Изменённые файлы

Фактический diff и состав файлов выводятся заново от baseline 9e162b04 до текущего tree/HEAD; moving range не закрепляется числом или скопированным списком.

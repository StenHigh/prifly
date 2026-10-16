## 1. Воспроизведение и streaming read

- [x] 1.1 Добавить valid synthetic populations с restored totals ниже/ровно/выше 64 МиБ при <1000 Run, packed и unpacked отдельно; count boundary проверить отдельно с маленькими snapshots. Проверка: baseline `go test ./internal/local ./internal/runtime -run 'Test.*(Scan|Claim)' -count=1` воспроизводит отказ свободного claim, фиксирует именно restored bytes/count, не размер DB/WAL. Новые tests параллельны, кроме process-env/package-state исключений AGENTS.md.
- [x] 1.2 Реализовать storage-neutral full snapshot streaming на одной read transaction/ReadCut с integrity checks и per-snapshot bound. Проверка: targeted local tests проходят >64 МиБ и >1000 records, не копят population; cancellation/late corruption/missing pinned bytes/unsupported state не дают complete success; mixed-cut paging не используется.
- [x] 1.3 Сохранить bounded ReadAll ceilings, заменить plain error typed scan-limit error и преобразовать в Fault/Problem `authority_scan_budget_exhausted`. Проверка: local/runtime tests отличают bytes/count threshold, observed lower bound и storage page quota; `make refusal-check` проходит, CLI не выдаёт `invalid_input`.

## 2. Полное ownership и атомарный fence

- [x] 2.1 Перевести unbound binding/release на streaming proof, сохранив legacy holder predicates и direct path bound claim. Проверка: holder после 64 МиБ/1000 records запрещает операцию; свободный claim освобождается/связывается; active/uncertain/pending/corrupt/unknown evidence не обходится; terminal legacy holder различается по binding/release правилам.
- [x] 2.2 Добавить optional expected-read-cut guard в coupled Run+claims и claims-only release-fence commit; сохранить claims-version/generation/identity checks. Проверка: deterministic race fixtures создают/меняют holder, в том числе без изменения Run count, между scan и commit; stale proof не даёт admission или deletion; unrelated telemetry даёт безопасный conflict.
- [x] 2.3 Проверить idempotency и interrupted release: duplicate проверяется до cut guard, optional field не меняет старые digests, rejection receipts не переписываются. Проверка: exact retry после собственного cut advance не повторяет mutation; новое preparation с новым command ID использует fresh proof; crash в releasing оставляет durable fence, stale generation не удаляет новый объект.

## 3. Failed-start cleanup и diagnostics

- [x] 3.1 Объединить post-claim rollback ветви Project start; перед cleanup сверять authoritative command/Run outcome, очищать только newly-created exact-generation claim. Проверка: CLI integration покрывает import/installed-path/start failures, committed-but-response-lost и unknown outcome; existing/handed claim и admitted Run не удаляются.
- [x] 3.2 Сохранить original refusal code и показать released либо remaining claim/generation/observed status и cleanup reason; не скрывать неизвестный outcome и package cleanup failure. Проверка: один Problem/1, точные применимые команды, отсутствие guessed state/новых fields; public CLI fixture доказывает rollback >64 МиБ и отдельно failed release без слепого повторного start.
- [x] 3.3 Добавить `doctor --claim ID` и closed `claim-ownership-diagnostics/1`, embedded/public schema/fixtures и help/troubleshooting. Проверка: read-only view показывает cut/completeness/scan metrics, orphan без RunID и отдельные logical-page usage/limit; complete/incomplete exit проверены; plain doctor и Problem старой формы, snapshots/secrets не выводятся.

## 4. Интеграция и приёмка

- [x] 4.1 Выполнить synthetic public CLI journey: накопленная authority → binding нового Run либо confirmed failed start → штатный release своего unbound claim. Проверка: один исходный authority, неизменные прежние Run/evidence, нет force/SQL/manual worktree removal, invalid reader/state и concurrent owner остаются refusals; записаны binary version/commit/hash и measured scan duration/memory boundary.
- [x] 4.2 Выполнить `make test-changed` или `make check-fast`, targeted local/runtime/CLI tests, `make refusal-check`, `make vet`, `make schemas-check`, `openspec validate fix-unbound-claim-ownership-scan --strict` и `git diff --check`. Проверка: pure transforms не читают files/blobs, runtime/CLI собираются без CGO; новый guide/index соответствует фактическому contract без объявления исправленным #7.
- [x] 4.3 Перед implementation commit выполнить один `make ci-check-once CHECK_CONTEXT=<actual-profile>`; staged check — с `CHECK_FOR_COMMIT=1`. Проверка: PASS/evidence относится к итоговому снимку и реальному профилю, fresh vuln/mod checks соблюдены; `git diff --name-status <baseline>` и protected hashes подтверждают неизменность archive/release evidence, прежних public bundles, canonical workflows и saved JSON semantics.
- [x] 4.4 Подготовить operational handoff: exact исправленный binary, `doctor --claim ID`, inspect claim generation и штатный release после отдельного operational запроса владельца; затем read-only оценка готовности нового start. Проверка: fixtures/release status отделены от реального orphan recovery, пользовательский claim без запроса не тронут; если нужен выпуск, применены fixed-SHA verify/qualify gates по AGENTS.md, а не обещан release по local PASS.

## Acceptance traceability

| Issue #8 acceptance | Tasks |
|---|---|
| 1. Свободный unbound release при >64 МиБ | 1.1–1.2, 2.1, 4.1 |
| 2. Полнота ownership и запрет при holder | 1.2, 2.1–2.3 |
| 3. Согласованный streaming без усечения | 1.2, 2.2 |
| 4. Named scan reason и observed limits | 1.3, 3.2–3.3 |
| 5. Start rollback / remaining claim | 3.1–3.2, 4.1 |
| 6. Packed/unpacked, count, races, corrupt/unknown | 1.1–1.3, 2.1–2.3, 3.1 |
| 7. Doctor отличает scan от storage | 3.3 |

# План реализации: безопасное ownership unbound claim (#8)

Branch: prifly/b5ff29a6abf4a4a28076b91d56bd7cbc116fd0366545af7ec1f9f396d36ceb42
Created: 2026-10-16

## Original Request

Implement openspec/changes/fix-unbound-claim-ownership-scan: all; requirements in specs/ and design.md

Original user request: Начинай реализацию change fix-unbound-claim-ownership-scan для #8.
Issue: https://github.com/StenHigh/prifly/issues/8

Implement all 13 open tasks. Preserve the boundaries and acceptance traceability in the change. Changes for #7 and #9 remain planning artifacts only; do not implement them. Do not operate on the original affected external authority or release its orphan claim. Operational handoff only. Repository development uses the declared aif-classic launch and its worktree. Use the actual permission context CHECK_CONTEXT=codex-app-escalated-bwrap-unavailable for the prescribed gate; this label records the execution profile and grants no permissions.

## Настройки

Профиль full, tests=true, logging=minimal, docs checkpoint=false. Документация остаётся обязательной частью change, но отдельный aif-docs не вызывается. Policy attended, gate_warnings=ignore. Gate composition — sealed gate_checks; CHECK_CONTEXT=codex-app-escalated-bwrap-unavailable. Git branch/worktree уже выданы engine; не создавать новые. Source baseline: 25583d094163a7bc47ee8cdb7732a923553af663.

## Roadmap Linkage

Milestone: "none"
Rationale: Change не закрывает product milestone; исходное issue не назначает formal gate.

## Requirements Reconciliation

Authority: active change specs/** + design.md > current OpenSpec source sets > .ai-factory maps/roadmap. Source map ownership не переносится. Понятия ReadCut, WorktreeClaim, Run сохраняют смысл. Неразрешённых требований нет; #7/#9 остаются отдельной работой.

| Комбинация / правило | Источник | Проверка |
|---|---|---|
| Unbound × binding/release × free/held/unknown | runtime delta, полное ownership; design §§1–2 | streaming >64 МиБ/>1000, holder в tail, unsupported/corrupt late snapshot |
| Packed/unpacked × below/equal/above aggregate cap | tasks 1.1–1.3; local per-snapshot ceiling | valid restored-byte totals; прежний bounded API всё ещё отказывает typed причиной |
| Unchanged/stale cut × Run/claims-only commit | design §2 | new holder и changed existing holder без изменения count, generation race, duplicate-before-guard |
| Newly created/existing/handed claim × confirmed no Run/created/unknown Start | CLI delta; design §4 | exact command receipt reconciliation, no unsafe release, honest remaining cleanup |
| Doctor complete/incomplete × scan/page budget | CLI delta; design §5 | emitted diagnostic schema и read-only byte comparisons, no raw body/secret |
| Старые fields/contracts и synthetic acceptance | AGENTS.md, source map, published-contracts | protected evidence/bundle hashes, plain doctor/Problem unchanged |

## Задачи

Прогресс — только openspec/changes/fix-unbound-claim-ownership-scan/tasks.md. Ниже ordered references; проверки и критерии каждого ID сохраняются полностью.

1.1 — internal/local и internal/runtime: failing valid aggregate/count fixtures, с различием stored/restored bytes.
1.2 — internal/local/store_read.go: consistent streaming API, bounded memory, EOF/cancel/integrity proof.
1.3 — local typed scan limit и runtime Problem/Fault conversion; named count/bytes diagnostics.
2.1 — internal/runtime/claim_run.go: streaming ownership predicate, direct bound path; witnesses complete только при EOF.
2.2 — internal/local/store.go, runtime/start.go/engine.go/worktrees.go: optional commit cut guard для обеих transaction forms; claims/generation/identity сохраняются.
2.3 — local/runtime idempotency, exact retry и release interrupted fence; internal guard не меняет legacy command digests.
3.1 — cmd/prifly/project_start.go: общая cleanup path с original Start receipt/Run reconciliation и exact createdClaim ownership.
3.2 — Project error presentation: original code + достоверный cleanup result, remaining identity и safe actions; package rollback failure не скрывать.
3.3 — runtime/CLI doctor --claim ID, отдельный closed diagnostic и schemas; plain doctor/Problem прежние.
4.1 — independent public CLI fixture на disposable populated authority; source history/claim/tree preservation и exact binary evidence.
4.2 — targeted tests, purity/refusal/vet/schema/OpenSpec/diff checks и author-facing diagnostics.
4.3 — один final-snapshot ci-check-once и staged-byte/evidence/protected history check перед implementation commit.
4.4 — отдельный exact-binary operational handoff; external orphan authority/claim без operational запроса не трогать.

Зависимости: 1.1→1.2→1.3→2.1→2.2→2.3; 2.1/2.2→3.1→3.2; 1.3/2.1→3.3; все→4.1→4.2→4.3→4.4. Новые tests t.Parallel(), кроме process env/shared mutation guards. Logging для всех задач minimal: typed refusal/Problem и bounded diagnostics, никаких новых raw snapshot logs/secret values.

## План коммита

Checkpoints после 1.3, 2.3, 3.3; baseline failing regression фиксируется в evidence до repair. Один итоговый implementation commit `fix(claims): prove ownership without aggregate scan limits` после accepted semantic gates и full final-snapshot check. Сохранить plan до архивирования change. Owner сам merge-ит/push-ит claim branch.

## Проверенные команды и последующая верификация

До записи этого плана на target worktree исполнены baseline `go test ./internal/local ./internal/runtime -run 'Test.*(Scan|Claim)' -count=1` (local/runtime PASS), `openspec validate fix-unbound-claim-ownership-scan --strict` (PASS) и `git diff --check` (PASS). Для прямого Go использованы existing local caches из основного checkout; тестируемые source bytes — этого claim. Логи: host scratch /tmp/prifly-issue8-baseline-tests.log. Новые failing fixtures ещё предстоит добавить в implementation.

Команды tasks.md, затрагивающие новую реализацию, выполняются по соответствующим ID; их будущий PASS здесь не заявлен. Full gate composition берётся из sealed decision_context и исполняется на итоговом снимке с actual context. Diff/проверяемая область вычисляется при проверке по baseline, без pinned moving counts. Исходный external issue environment не квалифицирован этим Run.

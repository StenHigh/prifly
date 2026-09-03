## 1. Инвентаризация и расхождение baseline

- [x] 1.1 Инвентаризировать `docs/spec/contracts/` (guide, protocol, commands, fixture registry, security cases и eight workflow extracts), их declared status и consumers; проверить count against exact source paths.
- [x] 1.2 Инвентаризировать distributed `schemas/`, generation/embedding paths и corresponding runtime tests without changing any artifact; проверить `make schemas-check` и relevant Go contract tests.
- [x] 1.3 Исследовать расхождение между documented baseline component count и actual protocol `$defs`: проверить available history and verifier semantics, записать cause/decision and add the smallest reproducible inventory check; не переписывать historical evidence, manifest или schema bytes.

## 2. Постоянный contract source и traceability

- [x] 2.1 Расширить candidate правилами semantic-versus-machine ownership, versioned compatibility, fixture/qualification boundary и generated-artifact verification; проверить отсутствие legacy requirement/acceptance IDs в candidate.
- [x] 2.2 Создать `archived-crosswalk.md` с legacy guide/fixture/schema inventory, statuses, exact artifact paths, source-to-permanent mappings и result count reconciliation; проверить complete coverage against task 1 inventory.
- [x] 2.3 Проверить candidate и crosswalk на truthful structural-versus-runtime language; выполнить `openspec validate migrate-published-contracts-to-openspec --strict`.

## 3. Переключение ownership

- [x] 3.1 Синхронизировать `published-contracts` в permanent specs; проверить `openspec validate --specs --strict` и exact machine-artifact paths without Markdown field copies.
- [x] 3.2 Переключить ровно одну строку `SOURCE-OF-TRUTH.md`; проверить, что legacy guide, fixtures, schemas, runtime types, evidence и manifests остаются byte-identical до final cleanup.

## 4. Защита и архивирование

- [x] 4.1 Выполнить protected diff для `docs/spec/contracts`, `schemas`, `internal/flow`, `internal/runtime`, `docs/evidence`, обоих manifests и `git diff --check`; доказать, что migration не изменила runtime contract bytes.
- [x] 4.2 Архивировать change после sync; выполнить `openspec validate --specs --strict` и `openspec validate --all --strict --archived`, не выдавая document migration за product qualification.

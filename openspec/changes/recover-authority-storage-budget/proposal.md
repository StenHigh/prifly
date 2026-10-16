## Why

[Issue #7](https://github.com/StenHigh/prifly/issues/7) описывает существующий Run, который после accepted fix не может продолжиться из-за `storage_budget_exhausted`: CLI не показывает effective usage/limit и не даёт поддержанного recovery пути. Историю, sealed evidence и dirty/untracked tree нужно сохранить, продолжив именно этот Run без повторения принятых стадий.

## What Changes

- Добавить read-only `storage budget show` / `doctor --storage` с allocated logical-page bytes, effective persisted limit, provenance/generation и применимыми действиями; отличать logical pages от WAL, disk space, restored scan bytes и per-snapshot cap.
- Добавить owner command `storage budget set --bytes N --reason TEXT`: сначала read-only `--prepare`, затем явное применение с reviewed digest и command identity. Первый срез разрешает только повышение конечного лимита, не auto-grow и не отключение guard.
- Хранить настройку authority durable и применять её во всех новых admissions, включая уже открытые store instances; изменение CAS/idempotent/audited, без фиктивного Run и без влияния на другие limits/rights.
- После успешного изменения обычный `run drive` продолжает существующий Run. Ничего не удаляется, claim/candidate/accepted provenance не переписываются, новый Run/fork не требуется.
- Ввести явную storage compatibility boundary для durable setting и отдельные schemas owner controls. Старые state/read/Problem/doctor forms сохраняются.
- Проверить saturation → review → raise → restart → continued execution, concurrent writers/updates, pending acceptance и сохранение control/settlement reserve; опубликовать operator guide и troubleshooting.

## Capabilities

### New Capabilities

Нет новых OpenSpec capability paths: storage owner control входит в существующие areas.

### Modified Capabilities

- `runtime-resources`: durable owner-selected logical-page ceiling и его применение каждым bounded write.
- `cli-protocol`: explicit review/set/show и диагностика budget refusal, recovery того же Run.
- `published-contracts`: новая storage version и closed budget status/review/result contracts без расширения historical forms.
- `specification-governance`: каноническая граница Authority storage budget относительно Run budgets, scan limits и физического диска.

Normative source ownership не переносится; source sets остаются по `openspec/SOURCE-OF-TRUTH.md`. Это продуктовый runtime/storage/CLI change, не только repository process.

## Impact

`internal/local/store.go` / `store_usage.go`, storage migration/guards, authority controls runtime, CLI dispatch/Problem/help и schemas/capability/docs. Текущий baseline `437e6a1c` держит default 512 МиБ в process-local StoreOptions; поддержанный внутренний range — 64 КиБ…64 ГиБ. Новый production limit остаётся конечным, default не повышается. Runtime/CLI не получают SQLite driver types и не выполняют I/O внутри transforms.

Приоритет — **после** [#8 change](../fix-unbound-claim-ownership-scan/proposal.md), согласно выбору владельца; алгоритмы технически независимы. Повышение page budget не устраняет scan cap #8 и не чинит несовместимый accepted prefix или нехватку физического диска. [E5](../add-project-extension-native-call/proposal.md) не зависит от этого owner control, но длительный пилот при storage saturation требует recovery пути.

Не входят GC/retention, VACUUM/checkpoint как обещание освобождения allowance, live подмена пользовательской authority, снижение лимита, unlimited setting, automatic execution после set и rewriting release evidence. Наблюдение issue получено на dev binary; source checks/synthetic tests не объявляются восстановлением исходного Run.

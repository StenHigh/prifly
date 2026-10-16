## Context

Мотивация и приоритет после #8 — [proposal.md](proposal.md). Contracts — deltas [runtime](specs/runtime-resources/spec.md), [CLI](specs/cli-protocol/spec.md), [publication](specs/published-contracts/spec.md) и [terminology](specs/specification-governance/spec.md).

Baseline `437e6a1c`: `StoreOptions.SoftLimitBytes` по умолчанию 512 МиБ, finite range 64 КиБ…64 ГиБ; `Store.softLimitBytes` живёт только в process memory. `StorageUsage` читает page_count/page_size/freelist_count; allocated bytes — page_count × page_size, не physical DB/WAL и не свободный диск. `RequireStorageBudget` отмечает existing ordinary work; savepoints откатывают превышающую mutation, оставляя control/settlement. Existing acceptance-storage tests уже доказывают pending acceptance без повторного producer. Writer `begin` проверяет storage header перед transform; storage version сейчас 9.

Исходный issue описывает dev binary, не qualified release. User authority в этой работе не открывается. Повышение page allowance не исправляет restored scan cap #8: два change независимы по механизму и идут в выбранном владельцем порядке.

## Goals / Non-Goals

**Goals:** observable owner-reviewed durable raise; согласованное enforcement во всех store instances; обычное продолжение того же Run с сохранёнными evidence/Workspace; explicit migration и diagnostic boundary.

**Non-Goals:** auto-grow, unlimited/decrease, GC/retention, reclaim страниц через VACUUM, disk-space guarantee, recovery обходом unknown effects/Stops, новый Run/fork, настройка quota через workflow YAML/local env, изменение Run policy или автоматический drive после set.

## Decisions

### 1. Первый recovery путь — explicit finite raise

Ввести `storage budget show`, `storage budget set --bytes N --reason TEXT --prepare` и применение того же request с `--expected-review-digest DIGEST --command ID`. `doctor --storage` — alternate read-only entry point того же status. Reason обязателен, bounded nonempty UTF-8; bytes — exact positive integer bytes, без float/coercion и env expansion. CLI не выбирает target за владельца.

Set разрешает только target > current effective limit, в existing finite range и с положительным headroom над allocation после самой control mutation. Zero/unlimited/decrease/out-of-range/overflow и новый command с неизменным значением отказываются. Exact duplicate уже принятого command возвращает recorded result прежде повторной проверки «повышает ли target». При 512 МиБ target должен быть явно выбран выше этой величины; 1 ГиБ может служить примером, не default/recommendation автоматически.

Альтернатива: global flag/env `--storage-limit` каждому invocation. Отклонена: разные процессы применяли бы разные guards, setting не durable и recovery требует подмены launch environment. Альтернатива: GC как первый путь. Отклонена: отдельная destructive retention/evidence policy, больший scope и иная приёмка. Поддержку decrease можно добавить позднее отдельным контрактом, здесь она не нужна для восстановления.

### 2. Authority-level numeric setting и audit коммитятся вместе

Добавить durable numeric ceiling в authority storage metadata и closed control record `authority-storage-budget/1` с generation, actor/reason, command identity, before/after и observation. Это authority control, не input WorkflowRevision и не поле каждого Run.

Использовать internal authority mutation по существующему образцу capacity: pure transform рассчитывает control record и scalar update; local store применяет их атомарно в одной transaction и сохраняет receipt. Enforcement читает authoritative numeric metadata в текущей write transaction, control record — provenance того же решения. Проверка целостности обнаруживает рассогласование metadata/control; независимого второго editable config файла нет. Финальная quota достаточность проверяется после записи candidate control metadata/receipt в той же transaction; если target уже не покрывает committed allocation, вся budget mutation откатывается с named refusal.

Mutation имеет control priority выше прежнего soft limit, но не обходит real disk I/O failure и validation. Сбой commit не объявляется success. Она не создаёт execution slots/Run/Attempt и не закрывает pending acceptance. Store не запускает процессы и не читает filesystem/blob из transform; storage usage берётся из transaction-owned state, budget decision — deterministic native code. Runtime DTO/CLI остаются без driver types.

### 3. Review защищает identity и configuration, telemetry проверяется заново

Review digest покрывает authority ID/epoch, current control generation/limit, target/reason, supported semantics и точный storage-upgrade plan при необходимости. Time, absolute machine path и observed usage не являются защитой owner intent: ordinary writes между prepare/set не должны каждый раз делать согласование stale. Их актуальный usage recheck происходит на commit; если выбранного target больше не хватает, named refusal, без auto-grow.

Подлинный owner/local actor определяется existing authority access, не payload. Command ID и protected payload задают existing idempotency; другой request под тем же ID конфликтует. При смене config generation/epoch или несовпадении digest — `storage_budget_review_conflict`, без quota mutation. При target вне rules — `storage_budget_invalid_limit`; недостающий headroom — `storage_budget_target_exhausted`. `storage_budget_exhausted` existing admissions сохраняет code/exit 5; новый owner control не помечает его безусловно retryable.

Exact retry после lost response читает свой receipt, а не заново применяет raise или строит response из уже следующего setting. Unknown commit требует inspect/reconcile по command, а не нового command со случайной mutation. Rejected command IDs следуют существующим receipt правилам: новый review не переписывает старый rejection receipt.

### 4. Все transactions читают committed limit; stale process opts не решают

Проверить все места использования `softLimitBytes`: ordinary Run Apply, logical receipt-only paths, coupled writes, authority writes с quota, telemetry allowance и StorageUsage. Production enforcement после migration читает persisted scalar в той же transaction, которая принимает решение. Уже открытые новые store instances используют новое значение на следующей transaction; предыдущая read transaction честно показывает свой cut, не новое setting поверх старого usage.

`StoreOptions.SoftLimitBytes` становится bootstrap input для новой storage/legacy migration и fixture setup, а не способ перезаписать существующий persisted control при reopen. Несогласованный override для уже configured authority отказывается либо явно игнорируется как bootstrap с documented result; для реализации выбрать отказ `storage_budget_configuration_conflict`, чтобы отсутствие эффекта не скрывалось. Engine передаёт bootstrap только при фактическом создании/migration, а обычное open использует сохранённое значение. В production остаётся прежний default; происхождение default/explicit owner setting видно отдельно.

Existing tests, которые открывают готовый Store с уменьшенным process-local limit ради quota injection, перевести на корректное finite initial fixture budget и controlled saturation. Не оставлять production bypass нового persisted guard ради теста. Guard `RequireStorageBudget` не снимается с control stage, если та создаёт новую обычную работу; settlement reserve сохраняется.

### 5. Storage migration явная и не переписывает Run contracts

Добавить следующую storage version после 9 (10, если номер ещё свободен при реализации). Atomic migration добавляет numeric metadata и control bootstrap provenance с generation 0 без выдуманного owner decision. Для stock legacy path default остаётся 512 МиБ. Исторические process-local overrides невозможно восстановить из отсутствующих данных: status на legacy storage показывает только effective значение данного supported binary с `legacy_default`/bootstrap provenance, не объявляет его историческим фактом. Private dev overrides не угадываются по issue.

Read-only show/prepare на storage 9 не мигрируют: показывают legacy effective setting и planned writer upgrade. Mutation сначала проверяет read-only reviewed request/identity, затем открывает writer по existing migration protocol и revalidates configuration. Проверка review принимает только точно показанный upgrade, а чужое изменение plan/generation требует fresh prepare. Нет бюджетной mutation по устаревшему digest. Если supported writer open уже выполнил ожидаемую schema migration, это служебный upgrade без повышения allowance; ошибку следующего set не выдавать за rollback всего upgrade.

Old writers, уже открытые до upgrade, прекращают последующие writes по existing storageHeader guard; это проверяется executable/version fixture, не только перечислением версии. Updated writers читают persisted limit; old binaries при новом open отказываются по unsupported storage version. Ни old-reader fallback, ни ручной downcast базы не предлагаются. Обновить capability storage-version inventory и migration/verify tests. Run state/read, events, package refs и старые JSON schemas остаются прежними. Restoring backup после upgrade — отдельная restore policy; нельзя потерять новые записи ради возврата старого binary.

### 6. Публикация и operator journey

Отдельные closed schemas: `storage-budget-status/1`, `storage-budget-review/1`, `storage-budget-result/1` и `authority-storage-budget/1`. Status включает authority/epoch, measured allocation/page metrics, effective limit, source/generation и действия. Review добавляет request/digest/compatibility plan; result — recorded before/after/generation/command evidence. Plain doctor foundation/core-doctor/1 и Problem/1 не расширяются. Новые definitions/generated schemas публикуются отдельно; reachability checks валидируют реально emitted responses/record.

При storage refusal code сохраняется; message/engine-authored details по existing Problem conventions содержат доступные observed usage/limit и точный путь `storage budget show`, затем review/set. Ошибка чтения usage не заменяет original refusal guessed numbers; command не должен выполнить второй write ради диагностики уже отказавшего admission. `doctor --claim ID` из #8 и `doctor --storage` — разные mutually exclusive views, common flags не меняют existing plain doctor.

Добавить canonical term Authority storage budget и mappings вместе с кодом, capability `storage_budget_management`, строку author index и `examples/authoring/storage-recovery-guide.md`. Guide показывает реальные CLI поля/profile/storage minima, review-before-set и subsequent manual `run drive RUN_ID`; не придумывает YAML quota field. `examples/troubleshooting.md` различает soft quota, scan cap, per-snapshot cap, disk full и unsupported storage version. Source map ownership не переносится.

### 7. Synthetic recovery доказывает preservation

Публичная fixture создаёт finite small budget из корректного fixture bootstrap, Run с accepted worker и control frontier, затем saturates allowance обычным supported path/fixture controls. Drive отказывает до нового dispatch. Сохраняются Run ID/version frontier, accepted artifact refs/digests/provenance, claim generation и dirty/untracked hashes. Read-only review/show ничего не меняют.

После explicit set и restart новый обычный drive того же Run продвигается без второго producer; выделенный уже открытый driver case видит updated ceiling без restart. Второй сценарий оставляет producer pending acceptance и проверяет admission именно checker, не implicit acceptance. Отдельно проверяются повторное исчерпание нового allowance, control/cancel/settlement above limit, physical write failure, concurrent setting/ordinary writes, stale review, exact duplicate/lost-response и unchanged safety policy. Синтетическое восстановление не является выполнением `run drive` на исходном пользовательском Run.

## Risks / Trade-offs

- [Повышение лишь откладывает saturation] → finite owner choice и повторный guard; никакого обещания неограниченного хранения.
- [Один guard читает stale cached value] → inventory всех local budget consumers и multi-connection tests.
- [Metadata и audit расходятся] → coupled atomic mutation/receipt и verification invariant.
- [Control update сам увеличил allocation выше target] → post-write budget check с rollback candidate setting целиком.
- [Старый process пишет после migration] → write-header fence на каждой transaction и old binary test.
- [Реальный диск заполнен] → honest persistence failure; soft limit raise не обещает физическое место.
- [Read-only prepare мутирует storage] → byte/header/state comparison до/после на legacy и new storage.
- [Rollback binary утратит compatibility] → explicit storage boundary, upgrade binary forward; отсутствие downcast/reset advice.

## Migration Plan

Реализацию брать после #8 согласно выбранному порядку. Сначала failing quota-recovery/compatibility tests, затем storage migration/transactional setting, runtime owner control, CLI/contracts/docs. Targeted checks во время работы и один prescribed final-snapshot gate перед implementation commit. Выход release — existing exact-SHA verify/qualify процедура, отдельно от document validation.

На реальной authority сначала новый compatible CLI read-only status/review, подтверждение finite target, затем exact reviewed set и отдельный ordinary drive выбранного существующего Run после проверки его состояния. Это operational изменение не выполняется planning workflow. Rollback кода не понижает storage version и не удаляет history; для уже migrated authority нужен compatible binary. Повышение обратно не уменьшается в этом срезе; decrease требует будущего отдельного контракта.

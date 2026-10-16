## Context

Мотивация — [proposal.md](proposal.md), contract — [runtime delta](specs/runtime-resources/spec.md) и [CLI delta](specs/cli-protocol/spec.md). На checkout `437e6a1c` `checkLegacyClaimHolders` читает `Store.ReadAll(maxPinScanRuns)`. `ReadAllAt` восстанавливает packed bytes, проверяет digest и собирает все snapshots в память до отказа на >64 МиБ либо >1000 records. Свободный современный claim с пустым RunID также идёт в этот legacy path.

`ReleaseWorktree` сначала берёт authority driver lock, проверяет ownership, затем CAS-ит claims record в `releasing` до filesystem deletion. `prepareClaimRunBinding` связывает claims record и Run атомарно. Однако старый scan возвращает только error, не freshness evidence. Project start в нескольких ветвях игнорирует ошибку rollback `ReleaseWorktree`. `Store` уже имеет ReadCut, snapshots, control/run pins, per-snapshot ceiling 16 МиБ и write-header guards. Общий cut продвигается и на receipts/telemetry: он консервативнее RunVersion.

Наблюдение issue относится к dev binary и authority >64 МиБ при storage usage ниже soft limit. Пользовательская authority и её claim не исследуются и не освобождаются в этом change без отдельного operational запроса.

## Goals / Non-Goals

**Goals:** полное ownership доказательство при ограниченной памяти; pin этого доказательства на coupled binding/release fence; честный cleanup после failed start; synthetic acceptance, достаточная для последующего безопасного operational recovery.

**Non-Goals:** увеличить лимит ReadAll, обрезать scan, пропускать неизвестные Run, доверять OwnerID prefix как отсутствию holder, force release, менять пользовательскую authority/историю, вводить GC, менять storage budget (#7), чинить cancellation (#6) либо реализовать E5.

## Decisions

### 1. Потоковый reader использует одну read transaction

Добавить storage-neutral iterator/callback API чтения latest snapshots на одном consistent current ReadCut. Чтение pinned values, unpack, digest и framing checks остаются внутри read path. Callback runtime декодирует один Run, проверяет supported state и его relevant session claim evidence, после чего snapshot освобождается. Не хранить весь snapshot population или O(N) payload. Рабочая память ограничена per-snapshot ceiling и фиксированным буфером.

Negative evidence «нет запрещающего holder» возвращается только после EOF без ошибки. Positive blocker допускает раннюю остановку, но не выдаётся за complete absence scan; metrics отмечают completeness. Ошибка/timeout/cancel прекращает проверку с unknown ownership. Независимые caps per-document и context cancellation остаются; лимитов на aggregate population 64 МиБ / 1000 records в этом специализированном proof reader нет. Не использовать live `RevisionPage` плюс независимые latest `Read`: такой обход смешивает cuts и допускает пропуск конкурентной смены состояния.

Результат содержит cut, completeness и observed count/restored bytes; данные snapshot не попадают в diagnostic. У bound claim остаётся direct load bound Run. Legacy holder predicates сохраняются: binding не переиспользует уже выданный legacy claim; release допускает лишь holders, для которых доказан existing `claimRunFinished`, без ослабления active/pending/uncertain checks.

Альтернатива: durable holder index. Не выбрана для первого исправления: необходимы backfill, доказательство полноты legacy и сопровождение всех session paths. Потоковый read применяет уже существующий semantic predicate. Пересмотреть индекс после измерений, если O(N) read создаёт неприемлемую задержку.

### 2. Commit проверяет cut, claim version и generation

Вернуть ownership evidence из preparation в binding/release fence. Дополнить внутренние local Command/AuthorityCommand optional expected-read-cut guard; это internal precondition, не public YAML/Run wire field. Guard сравнивается с current authority cut в той же write transaction до transform. Проверять его в обоих commit paths: coupled Run+claims binding и claims-only release fence. Существующие claims record version, exact generation/RunID, directory identity и driver lock сохраняются.

Все команды, меняющие Run/ownership evidence, продвигают общий cut. Поэтому guard ловит не только новый Run, но и изменение уже существующего legacy holder. Один ExpectedRuns или claims record pin недостаточен для второго случая. После свежего fence ordinary admissions видят `releasing` и не входят в удаляемое дерево. File/pinned reads не переносятся внутрь pure write transform.

Receipt duplicate проверяется до freshness guard: точный retry возвращает свой original result без повторной mutation, даже если сама первая команда продвинула cut. Guard не меняет retry identity; immutable operation payload сохраняет command semantics, а internal read proof пересчитывается при неприменённом запросе. Optional omission сохраняет прежние command digests. Проверить это отдельно от CAS rejection receipts.

При stale proof вернуть typed `claim_ownership_conflict` до deletion/admission; caller может повторить всю preparation. Никакой бесконечной автоматической петли. Консервативный cut способен конфликтовать из-за unrelated telemetry; это безопасный ложный отказ и явно описанный trade-off. Тест должен подтвердить полный rescan при новом запросе. Если existing command machinery записала rejection для данного command ID, повтор использует новый command ID по существующему протоколу, не меняет записанный receipt.

### 3. Bounded API остаются bounded и получают typed diagnostics

Сохранить нынешнюю форму и ceiling `ReadAll` для остальных consumers. Заменить plain limit error на typed storage error с reason=count/bytes, observed records/restored bytes и respective limit. На runtime boundary преобразовывать её в `runtime.Fault{Code: authority_scan_budget_exhausted}` без зависимости CLI от SQLite driver types. Exit class 5 и next actions задаются code registry; quota exhaustion не объявляется автоматически retryable.

Число/bytes на отказе — observed lower bound, не полный размер authority. Streaming ownership path не должен возвращать этот aggregate-limit refusal только из-за большой истории. Corrupt/unsupported/unavailable state остаётся отдельным `claim_owner_unproven` либо существующим integrity/compatibility отказом с причиной. Не обещать diagnostic, если сам store не открывается: в таком случае сохраняется исходный storage failure.

### 4. Cleanup helper знает, какой claim создан этой попыткой

Объединить rollback handling всех post-claim failure branches Project start. Переданный continuation/recovery claim и existing idempotently найденный claim не очищаются как новые. При подтверждённом отсутствии admitted Run helper освобождает только newly-created exact ID/generation через `ReleaseWorktree`; local files/branches вручную не удаляются.

`Start` error не всегда доказывает отсутствие commit: сначала проверить original command receipt / derived Run identity через existing authoritative APIs. Если Run создан, сохраняются его claim и original error+Run ID по existing contract. Если outcome неизвестен — сохранить claim и дать inspect/reconcile guidance. Не использовать неуспешный `run list` как доказательство отсутствия Run.

Failure presentation сохраняет original Fault/recorded rejection code и исходную причину. Bounded message добавляет достоверный cleanup result: released либо ID/generation/observed active|releasing|unknown, cleanup code и команды `claim list`, `doctor --claim ID`, затем штатный exact-generation release при доказанной применимости. Unknown ownership/status не заменять guessed values. Если могла не завершиться и package rollback, не заявлять полную отмену side effects; его failure также не скрывать. Детали cleanup не добавлять новыми полями в Problem/1 и не упаковывать explanations как фиктивные violations чужого документа.

### 5. Отдельный diagnostic view сохраняет plain doctor contract

`doctor --claim ID` работает read-only и выдаёт новый closed `claim-ownership-diagnostics/1`: authority identity, claim ID/generation/status/RunID, checked cut, ownership assessment/completeness, observed scan counts/restored bytes, bounded-read ceilings для сравнения, blocker/reason и отдельные logical-page allocated bytes/effective soft limit. Blocker details ограничены необходимыми opaque identities; bodies/paths с secrets не экспортируются.

Это observation, не reusable approval для release. При corruption, unsupported snapshot или прерывании scan доступная часть view отмечается incomplete/unknown; command exit не представляет это как успешное доказательство здоровья. Plain `doctor` сохраняет foundation/core-doctor/1. Existing integrity-check/open failures не обходятся raw SQL; при их возникновении CLI возвращает исходную named причину. Новый диагностический contract и delivery публикуются рядом со старыми artifacts; Run state/read и old bundles не меняются.

Troubleshooting объясняет разницу ReadAll aggregate cap, per-snapshot max и SQLite page budget, а также как смотреть orphan без RunID. Новую строку capabilities не добавлять ради bugfix; если diagnostic получает объявляемый capability ID по conventions реализации, index и author guide должны измениться одновременно. Канонические понятия ReadCut/WorktreeClaim уже есть; glossary bindings не переименовываются.

## Risks / Trade-offs

- [Long read удерживает SQLite snapshot/WAL] → одна streaming transaction с cancellation, bounded memory и measured scan duration; это не latency SLA.
- [Cut меняется из-за unrelated command/sample] → conservative conflict, новый полный proof; отсутствие silent retry/deletion.
- [Proof проверен только в release, но не binding] → обязательные tests обоих coupled commit paths.
- [Новая guard меняет idempotency] → omission прежних payloads, duplicate-before-guard и recorded-rejection retry tests.
- [Cleanup удалит уже admitted tree] → command/Run reconciliation и только newly-created exact-generation rollback.
- [Большая fixture просто занимает диск] → valid packed/unpacked snapshots с проверенными restored sizes; отдельно per-snapshot и aggregate metrics, несколько bounded synthetic cases вместо копии private authority.
- [Неполная diagnosis выглядит PASS] → explicit completeness/unknown и отдельный exit/assertion.

## Migration Plan

Storage schema и saved Run migration не требуются для streaming и optional commit precondition. Сначала baseline regressions, затем reader+guards, runtime wiring, cleanup и diagnostics. На итоговом implementation snapshot выполнить prescribed local gates; release qualification остаётся отдельной процедурой на fixed SHA.

Operational recovery исходного claim проводится только после поставки исправления и отдельного выбора владельца: inspect exact claim/generation и дерево, `doctor --claim ID`, затем existing `claim release`; повторный start — после доказанного release либо известного admitted Run. Planning/fixture PASS не означает, что этот orphan уже очищен. Rollback binary не меняет history, но снова может встретить старый scan cap; force-release, ручной SQL и удаление учтённой worktree не являются rollback strategy.

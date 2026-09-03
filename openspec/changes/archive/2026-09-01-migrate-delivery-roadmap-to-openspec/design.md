## Context

См. [proposal.md](proposal.md). Current source set состоит из
`docs/roadmap/roadmap.md`, `docs/f2-progress.md`, `docs/release.md`,
`docs/rc-scope.md`, `docs/beyond-phase-two.md` и `docs/dependencies.md`.
В нём одновременно находятся будущие milestones, текущий RC scope, release
snapshot и подробная историческая лента implementation slices.

## Goals / Non-Goals

**Goals:**

- Сделать `delivery-roadmap` единственным местом изменения текущего plan,
  phase/status semantics, RC boundary и future queue.
- Сохранить полный milestone inventory и traceability в archived crosswalk,
  не оставляя старые internal record IDs постоянным API документации.
- Оставить historical progress/release evidence доступными через Git/archive,
  но не как второй editable source.

**Non-Goals:**

- Не переквалифицировать release, не изменять status, не запускать gates и не
  редактировать Go, schemas, manifests or evidence.
- Не переносить published JSON Schema/contracts: это отдельная capability.
- Не делать OpenSpec issue tracker, CI dashboard или новый format статуса.

## Decisions

### Current delivery snapshot заменяет, а не пересказывает журнал

Permanent spec хранит целевой двухфазный plan, полный inventory milestones,
значение статусов, current RC boundary, active priority и future queue. Он не
копирует каждый dated implementation slice из `f2-progress.md`: такой журнал
после cutover не является редактируемой нормой и сохраняется exact в Git.
Permanent current snapshot явно называет дату и scope, поэтому исторический
`passed` не превращается в более широкий qualified release.

### Crosswalk покрывает source documents разной природы

Archive содержит: 27 P1/P2 milestones; DCL/OSS/prioritised items; current RC
and first-build boundaries; headings всех historical progress/release sections
с source paths and dates; и future catalogue entries. Для каждого record
crosswalk указывает permanent section либо `historical_git_only` с причиной.
Так удаление legacy release tree не стирает ни факта существования history, ни
не создаёт second source of truth.

### Cutover — одна source-map строка

После exact inventory и permanent validation ownership row переключается на
`openspec/specs/delivery-roadmap/`; legacy source files остаются byte-identical
до общего final cleanup. Derived roadmap CSV/JSON/verifier не становятся новым
OpenSpec contract и будут удалены только вместе с legacy documentation tooling.

## Risks / Trade-offs

- [Текущий status затеряется историей] → snapshot получает date, profile,
  completed/remaining boundary и direct archive link.
- [Исторический record исчезнет] → crosswalk inventory headings + Git history;
  migration verifies every source heading is either current or historical.
- [Roadmap выдаст plan за implementation] → mandatory status/evidence semantics
  and explicit exclusions in every phase/RC section.
- [Dependencies попадут в runtime contract] → inventory remains release scope,
  not a claim of runtime trust or capability.

## Migration Plan

1. Inventory all source headings, milestone records and current-state claims;
   classify each as permanent current plan, archive traceability or historical
   Git-only record.
2. Expand candidate to the full milestone/current RC/future queue contract,
   build exact crosswalk and preserve legacy source bytes.
3. Sync `delivery-roadmap`, switch one ownership row, validate links and retain
   release/history distinction.
4. Archive the change; final cleanup later removes physical legacy sources only
   after every source-map row has completed its own migration.

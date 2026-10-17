## Why

[Issue #11 / E6](https://github.com/StenHigh/prifly/issues/11) требует отдельный повседневный интерфейс задач и результатов ревью. Выпущенные E1 board и E2 native queue дают исходные данные, но нынешний `monitor` обслуживает один frontend независимо от `scan-root`; первый экран Light ещё не существует.

## What Changes

- Отдельный opt-in Light mode команды `monitor`, свой frontend и строго read-only HTTP mux; default full monitor, его auto-start, UI/API и обслуживание сохраняются.
- Карточки native QueueTask, включая очередь до первого Run; группировка продолжений по записанным bindings в пределах authority, самостоятельные fallback-карточки несвязанных Runs и доступная история.
- Понятные этапы по явному mapping закреплённых определений; причины ожидания, несколько параллельных ветвей, partial/cancelled/uncertain и scope завершения видны отдельно.
- Типизированная read projection ревью с candidate/round, gate/report provenance и freshness. `StepResult pass` не подменяет gate. Optional W1/W2/W5 mappings поставляются как данные Core, без изменения consumer packages и без обязательных четырёх reviewers.
- Ограниченные постраничные reads и polling, согласованный source cut, раздельная свежесть authorities, поиск и фильтры, mobile/keyboard, переход к exact Run в full monitor.
- До реализации фиксируются [UI sketch](ui-sketch.md), [read contract и mappings](read-contract.md), [матрица E6-A1–A10](acceptance.md) и измеримые budgets. Локальная инженерная проверка, released qualification и production rollout имеют разные gates.

## Capabilities

### New Capabilities

- `prifly-light`: отдельная доска задач, read projection, доверие к review evidence, UX, производительность и read-only delivery boundary.

### Modified Capabilities

- `cli-protocol`: явный Light mode, отдельный listener/path, optional mapping и full-monitor URL, mode-aware health без изменения default monitor.
- `published-contracts`: отдельные versioned closed Light read/mapping contracts, без изменения historical bundles и saved Run contracts.
- `specification-governance`: канонические определения Light и его декларативной read projection без новых runtime states.
- `delivery-roadmap`: самостоятельный E6/P1 срез с открытым production gate; принятые E1/E2 не переоткрываются.

## Impact

Изменение затрагивает product UI, CLI и read-side runtime/storage; это не только репозиторный процесс. Основные места: `cmd/prifly/monitor*.go`, отдельные `light.*`, `internal/runtime/`, driver-neutral read contract и SQLite reader в `internal/local/`, canonical schemas и новые bundles, `README.md`, `examples/`, tests. Новые LLM-вызовы, scheduler, writable control plane и auth provider не требуются.

Текущие source sets — соответствующие `openspec/specs/<capability>/`; ownership существующих capabilities не переносится. При apply карта `openspec/SOURCE-OF-TRUTH.md` получает только новую строку `prifly-light`, glossary и затронутые entrypoints обновляются вместе. Main specs, historical evidence и consumer workflow bytes на этапе proposal не меняются.

Baseline разбора: локальный checkout `86676dd6d1560216aed2ed233722c3febe9e465a` и issue с заявленной stable 0.13.79; это не подтверждение установленной production версии. Upstream reports просмотрены на `StenHigh/prifly-aif-workflows@3499115956903abd264fbde2be2f432d07bc2c43` исключительно как read-only fixtures. Реализация выполняется обычным OpenSpec apply без `prifly-run` по явному выбору владельца для этой задачи; общая репозиторная политика не меняется. Domain rollout выполняется отдельно после released Core: ожидаемые адреса и auth boundary закреплены в acceptance, домены не зашиваются в Core.

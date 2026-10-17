## Status

Матрица запланированных проверок [issue #11](https://github.com/StenHigh/prifly/issues/11). Development implementation и local replay описаны в [engineering evidence](evidence/engineering.md). A1–A8 проверяются локально на development binary; A9/A10 остаются открытыми до выпуска и отдельно авторизованного production rollout. Статус OpenSpec не заменяет evidence.

| Criterion | Проверка и обязательное evidence | Tasks |
|---|---|---|
| E6-A1 — Настоящий Light | Один fixture set, одновременно два listener, actual browser screenshots Light/full, разные DOM/assets/первый экран, документированный mode; scan-root сам по себе не evidence | 4.1, 5.1, 6.3 |
| E6-A2 — Tasks/Runs | Native queued task до Run; start→partial→continue с одной карточкой; отменённое/повторное продолжение, forks, missing binding/index lag, одинаковые titles и разные authorities; запись actual bindings и истории | 1.2, 2.2, 2.3, 5.2 |
| E6-A3 — Этапы | Shipped feature fixture с mapping и generic non-AIF без него, decision/resource waits, failed/partial/cancelled/uncertain/completed; record phase provenance и scope | 1.3, 2.3, 3.1, 3.4, 5.1 |
| E6-A4 — Review | Typed W1/W2/W5 refs/digests: pass-report+blocked gate, passed/defect/unavailable, warnings, optional skip/disabled/N/A, pending/missing/invalid/stale; raw источники и findings; backend без UI/UX readers | 1.3, 3.1, 3.2, 3.4, 5.2 |
| E6-A5 — Repair/parallel | Начат repair до нового candidate, затем candidate/round mismatch, прежний PASS historical, mixed lens/aggregate gates, multiple current Runs и owner blocker; snapshots+browser observations | 3.3, 3.4, 5.1, 6.3 |
| E6-A6 — Navigation/UX | Actual browser desktop 1440 px/mobile 390 px и keyboard, filter/search/counters/loading/empty/error/stale/expired, late poll, exact Run link с encoded IDs/prefix и возврат к filter/page/scroll | 2.4, 5.2, 5.3, 6.3 |
| E6-A7 — Лёгкость | ≥1000 tasks и ≥1000 Runs, cold/warm/changed/queue/admission/concurrent/source loss; design budgets, response bytes/request waterfall/backend counters/p95/WAL lifetime, полная пагинация и source cut consistency | 2.2, 2.4, 6.4 |
| E6-A8 — Совместимость | Existing full monitor UI/API/entrypoint/auto-start/maintenance, queue CLI/legacy standalone Runs/schemas; before/after SHA-256 manifests consumer workflow packages (включая classic/profiled) и protected evidence, CI gates | 1.1, 2.1, 4.1, 4.2, 6.3, 6.5, 6.6 |
| E6-A9 — Поставка | Released command/config, actual installed binary version/digest, source SHA, local run/health/browser, prefix asset/API routing/full-link, existing proxy authentication/write denial и rollback только Light | 6.1, 7.1, 7.2 |
| E6-A10 — Production | Released Core, отдельный server rollout, owner реальные tasks/review outcomes, оба URL в actual browser, authentication сохранена, exact binary/source SHA и screenshots/наблюдения | 7.3, 7.4 |

Новые engineering/released/production records сохраняются внутри этого change в `evidence/` при выполнении соответствующих задач. Старые archived/release records не редактируются. До actual replay placeholder-файлы с PASS не создаются. Fixture schema validation и mock/browser unit tests не заменяют actual browser observations либо released/production evidence.

## Deployment boundary

Ожидаемый Light URL: **`https://prify-light.stenhigh.net/`** — именно `prify`, без дополнительной `l`. Existing full: **`https://prifly.stenhigh.net/`**. Это owner deployment targets из issue, не константы в Core и не подтверждение текущего состояния сервисов. Domain configuration на этапе подготовки change не меняется.

Порядок: local engineering acceptance → exact-SHA release gates и поставка владельцем → released local replay → отдельно авторизованный server rollout → owner production acceptance. Rollout сохраняет existing authenticated proxy и loopback; direct public listen, отключение auth и новая write plane не допускаются. Proxy перезаписывает Host для loopback upstream; forwarded headers не ослабляют Core Host guard. Проверяется реальная защита без сессии и доступ с owner сессией без сохранения credentials в evidence.

При необходимости rollback удаляет только Light route/service. Full monitor, queue, authority, consumer packages и их версии сохраняются. Наличие зелёного инженерного gate позволяет отметить реализацию в этом scope, но issue/E6 остаются открытыми до A10. В текущем apply выполнена локальная реализация; release, rollout, owner production acceptance и archive не выполняются.

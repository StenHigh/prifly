## 1. Baseline и fixtures

- [x] 1.1 Записать exact source SHA, toolchain/profile и manifest SHA-256 защищённых historical bundles/evidence, embedded full monitor assets и installed/vendored consumer workflow trees до реализации; проверить inventory против Git и включить classic/profiled плюс остальные consumer packages, не меняя их bytes.
- [x] 1.2 Собрать native queue/Run fixtures: queued без Run, partial→cancelled continue→continue, forks, несколько frontier Runs, одинаковые titles, разные authorities, standalone/missing source и отставший queue index; проверить actual queue bindings и ожидаемое число карточек (A2).
- [x] 1.3 Закрепить read-only source fixtures W1/W2/W5 на SHA из read contract, typed reports/gates/requests/candidates и generic non-AIF workflow; проверить schema refs/digests и exact artifact provenance без изменения consumer repos/packages (A3–A5).

## 2. Read contracts и согласованный индекс

- [x] 2.1 Создать closed Light board/task/reviews/report/mapping schemas и driver-neutral DTO по read contract; проверить emitted positive/negative fixtures и unsupported versions через `make schemas-check`, без изменения старых bundles (A8).
- [x] 2.2 Реализовать source-cut reader queue/bindings/selected Runs/admission и ограниченный background index с атомарной публикацией generations, version/digest caches и pagination; concurrency test должен ловить смешанные queue/Run/admission cuts, queue-only/admission-only invalidation и отсутствие queue init/storage writes от read (A2/A7).
- [x] 2.3 Реализовать task grouping/canonical binding fallback, frontier/history и lane/scope projection; fixtures 1.2 должны доказать одну task при continue, независимость standalone forks/authorities и отсутствие ложного task done после succeeded Run (A2/A3).
- [x] 2.4 Добавить bounded query/filter/counters/source coverage, cursor generation vector/expiry, ETag и per-source errors/staleness; проверить поиск записи за 200, обход всех страниц без потерь/дублей на fixed generation, denied/expired cursor, поздний ответ и сохранение healthy sources (A6/A7).

## 3. Typed review projection

- [x] 3.1 Реализовать bounded declarative mapping validation/resolution exact refs, ports, pointers и enum tables; проверить unsupported/conflicting mapping, отсутствие consumer namespace branches в runtime и generic fallback без mapping (A3/A4).
- [x] 3.2 Реализовать candidate/request/round correlation, separate report/gate/check/applicability/freshness и lazy provenance/raw preview; typed tests должны включать pass-report+blocked gate, defect, passed, warnings, optional skip, disabled/N/A, pending, missing/invalid/digest mismatch и невозможность arbitrary ref/path read (A4).
- [x] 3.3 Добавить recorded repair invalidation, parallel/aggregate scope и несколько current Runs; тесты должны отклонять старый PASS после начала repair, round/request/candidate mismatch и mixed outcomes с owner blocker, сохраняя исторический report (A5).
- [x] 3.4 Поставить optional mappings как данные Core для W1/W2/W5 sequential и opt-in parallel, включая enabled predicates, initial-round различие и inner gate correlations; прогнать actual typed fixtures 1.3 и зафиксировать unsupported producer gaps без новых reviewers/readers (A3–A5).

## 4. CLI и read-only listener

- [x] 4.1 Добавить `monitor --ui full|light`, Light-only URL/mapping/base-path options и mode-aware health; CLI tests должны проверить default full, invalid input до bind, сохранённый full auto-start probe и невозможность принять Light health за full (A1/A8/A9).
- [x] 4.2 Добавить отдельный Light allowlist mux/assets с общими loopback/Host/access guards и relative prefix routing; HTTP tests должны проверять GET/HEAD reads, POST/maintenance/queue write rejection без side effects, XSS/CORS/foreign Host, prefix assets/API и отсутствие arbitrary source paths (A8/A9).

## 5. Отдельный frontend

- [x] 5.1 Реализовать первый экран по UI sketch: компактные карточки, attention, mapped phases/generic work, queue/result-received/done scope, все active branches и dynamic review chips; UI tests должны отличать Light DOM/assets от full и проверять typed fixture statuses без false PASS (A1/A3/A4/A5).
- [x] 5.2 Реализовать task/review details, постраничную историю, lazy raw preview и exact full-monitor links через URLSearchParams; проверить source/Run с reserved symbols, configured path/query, filter/page/scroll return и несколько current Run links (A2/A4/A6).
- [x] 5.3 Реализовать filters/counters, loading/empty/error/stale/expired states, bounded conditional polling, query cancellation, theme/mobile/keyboard; actual browser desktop+mobile проверяет focus/scroll, late responses, hidden tab/backoff и источник, пропавший во время чтения (A6).

## 6. Документация и интеграционная приёмка

- [x] 6.1 Обновить glossary с реальными bindings, source map, README и `examples/README.md`/нужный guide `examples/authoring/` с показанными Light mapping полями и минимальным фактическим contract/release; выполнить `TestGlossaryBindings`, `TestEveryDeclaredCapabilityIsInTheAuthorIndex` и OpenSpec validation. Каждой новой declared engine capability соответствует строка author index; Light не навязывает workflow authoring fields.
- [x] 6.2 Добавить воспроизводимые local fixture/launch/browser/performance scripts и включить semantic tests в подходящие CI gates; новые Go tests используют `t.Parallel()` кроме разрешённых AGENTS исключений, проверка — `make test-changed`/`make check-fast` и testguard, без запуска полного e2e на каждом шаге.
- [x] 6.3 Выполнить actual browser comparison Light/full на одном наборе, desktop/mobile/keyboard и exact navigation, сохранить screenshots/observations/request log в новом engineering evidence этого change; покрытие должно явно закрывать A1–A6 и regression full monitor UI/API/entrypoint (A8).
- [x] 6.4 Измерить budgets design на ≥1000 native tasks и ≥1000 Runs: cold/unchanged/one changed Run/queue-only/admission-only/concurrent writes/unavailable source, latency/bytes/requests/decodes/report reads/scan/WAL lifetime и browser behaviour; записать hardware/profile/results, A7 остаётся открытым при провале любого обязательного budget.
- [x] 6.5 Проверить byte/hash manifest 1.1 для existing schemas/bundles/full assets и всех consumer packages, `git diff --check` и отсутствие правок protected archive/release evidence; результат включить в engineering evidence, разрешены только новые declared Light artifacts (A8).
- [x] 6.6 На финальном implementation snapshot перед commit выполнить `make ci-check-once CHECK_CONTEXT=<actual-profile>` (для staged bytes добавить `CHECK_FOR_COMMIT=1`), сохранить точный результат и actual inputs/context; повторять полные ворота только при изменении inputs/контекста либо failure. Проверить `openspec validate add-prifly-light-task-board --strict`.

## 7. Выпуск и отдельный rollout

- [ ] 7.1 Подготовить release на фиксированном SHA, выполнить `python3 scripts/ensure-release-checks.py --sha SHA --ref main --wait` (verify/qualify, race/e2e по release правилам); публикация/tag владельцем через existing release process, проверить реальные release version/binary digest перед следующей задачей.
- [ ] 7.2 На actual released binary воспроизвести documented Light command, loopback run/health/browser, proxy prefix/assets/API/full links, write denial и rollback только Light; сохранить команды, logs, screenshots, version/digest/source SHA в новом released evidence (A9), не выдавая dev build за release.
- [ ] 7.3 Отдельным deployment этапом после release/авторизации rollout настроить Light service и existing authenticated proxy для точного `https://prify-light.stenhigh.net/`, сохранив `https://prifly.stenhigh.net/`; проверить unauthenticated denial, реальный Light/full browser comparison, actual owner tasks/reviews и exact source/Run navigation (A10).
- [ ] 7.4 Сохранить production screenshots/наблюдения, binary/source SHA, действующую auth boundary и owner acceptance; закрывать E6/issue только после A1–A10, затем archive+sync по OpenSpec. При отсутствии production evidence оставить 7.3/7.4 и A10 открытыми, не отмечать «развёрнуто».

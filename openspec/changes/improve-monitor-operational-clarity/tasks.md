## 1. Причина и следующий переход

Общий режим logging: без новых production console/telemetry и без payload в логах. Команды проверки сохраняют exit/status; browser runner сохраняет нужные screenshots/network events в owned output. Основа требований — delta `control-security-ux`, design D1–D3, acceptance A/B.

- [ ] 1.1 Зафиксировать actual base, исходный dirty scope, защищённые JSON/replay bytes и доступные формы list/detail/Light. Проверка: сравнение с actual base по acceptance E; source set Light и прежняя refactor branch сохранены. Только локальное evidence, без production logging.
- [ ] 1.2 Сначала воспроизвести ложное ожидание отчёта и неоднозначную диагностику Full, затем реализовать приватные адаптеры в `cmd/prifly/monitor.js`. Проверка: `node cmd/prifly/monitor_ui_test.cjs`, R01–R06; Board списка не смешивается с деталями другого cut. Зависит от 1.1; logging — только тестовые результаты.
- [ ] 1.3 Добавить `cmd/prifly/light_ui_test.cjs` и улучшить объяснение existing LightCard в `light.js`, сохранив phase/lane/review semantics. Проверка: `node cmd/prifly/light_ui_test.cjs`, R02–R07; срок попытки объяснён, owner не выдуман. Зависит от 1.1; logging — только тестовые результаты.
- [ ] 1.4 Подключить безопасные scoped next-step links и раскрытие остальных причин в обоих UI. Проверка: оба Node suite, R03/R06/R07/R15; неизвестный action имеет fallback, mixed current Runs не выбирают первый Run. Зависит от 1.2–1.3; logging — только traces тестового runner.

## 2. Достоверность чтения

Требования — вторая норма delta, design D4/D6/D7, acceptance R08–R12. Production logging не расширяется.

- [ ] 2.1 Разделить first-load/refresh/error/empty по exact request identity в `cmd/prifly/light.js`. Сначала воспроизвести ложный кеш новых details/filter. Проверка: Light Node suite и browser R08–R11; 304, expired cursor и late response не создают чужой snapshot. Зависит от 1.3; результаты — в test output.
- [ ] 2.2 Согласовать loading/error/stale/recovery в `cmd/prifly/monitor.js` без изменения состояния Run. Учитывать все параметры чтения, включая отдельный page. Проверка: Full Node suite и browser R08–R11; page A не подменяет недоступную page B, один недоступный source не скрывает остальные. Зависит от 1.2; результаты — в test output.
- [ ] 2.3 Воспроизвести изменение driver_live/freshness при прежней Run revision и исправить нужную invalidation без потери focus/scroll. Проверка: Full suite, R12, existing `TestMonitorBoardRefreshesAuthorityFactsAtTheSameRunRevision`. Зависит от 2.2; новых production logs нет.
- [ ] 2.4 Подключить оба Node suite к `make ui-check`, `check`, `ci-check` и static CI; расширить выбор в `scripts/test-changed.py`. Проверка: `make ui-check`, `python3 -B scripts/test_test_changed.py`, `make scripts-check`; unknown/all fallback не теряет suites, gate evidence учитывает новые inputs. Файлы: `Makefile`, `.github/workflows/verify.yml`, scripts/tests; Node 24 в CI. Зависит от 1.2–1.3; вывод только штатных checks.

## 3. Компактная компоновка и навигация

Требования — третья норма delta, design D5/D6, acceptance A и R13–R15. Существующие source/identity/access boundaries сохраняются.

- [ ] 3.1 Уплотнить Full list/details в `monitor.html`, `monitor.css`, `monitor.js`: короткая шапка, disclosure фильтров/metadata, вертикальная мобильная строка. Проверка: Full UI suite и `verify-monitor-board.py` по acceptance C; первый результат 390×844/1440×900, title/status вместе, default view прежний. Зависит от 1.4 и 2.3; screenshots только в test output.
- [ ] 3.2 Уплотнить Light в `light.html`, `light.css`, `light.js`: основной блок до metadata, без больших пустых групп перед поисковым результатом. Проверка: Light suite и `verify-light.py` по acceptance C; честные counters, phase/gate различимы, длинный текст раскрывается. Зависит от 1.4 и 2.1; screenshots только в test output.
- [ ] 3.3 Расширить `test/e2e/verify-monitor-board.py/.cjs` и `test/e2e/verify-light.py/.cjs` для keyboard/focus, скрытых выбранных фильтров, back/popup/poll и proxy prefix; заменить CSS-regex ширины Full наблюдаемым критерием. Добавить в Light replay opt-in `--scenario stops` с отдельным native export и общим owned setup; default performance-набор сохранить. Проверка: Full replay, оба режима Light по acceptance C и R01–R15, обе темы и viewport; новые GET/navigation не вызывают mutation. Зависит от 3.1–3.2; сохранить нужные network events и screenshots без private payload.

## 4. Документация и приёмка

- [ ] 4.1 Обновить `examples/operations/observability.md` и `examples/authoring/light-guide.md`: причины, безопасные переходы, first-read/stale, раскрытие фильтров и границы наблюдения. Проверка: `openspec validate improve-monitor-operational-clarity --strict`, `git diff --check`, сверка описанных действий с browser replay. Зависит от 3.3; glossary/source ownership не меняются, production logging не добавляется.
- [ ] 4.2 Повторить штатный `aif-ui-ux` на exact сохранённом implementation candidate по acceptance D: все 12 checks и дополнительные сочетания. Проверка: native terminal status, typed UI/UX reports, отсутствие required failed/unavailable/blocked и точный release receipt. Зависит от 2.4 и 4.1; новое evidence хранится отдельно от старого, claim/ветка кандидата защищены перед новым start.
- [ ] 4.3 Проверить protected history, JSON/replay bytes и итоговый staged snapshot по acceptance E; выполнить `make ci-check-once CHECK_CONTEXT=<actual-profile> CHECK_FOR_COMMIT=1` и `git diff --check`. Зависит от 4.2; выполнить `make test-changed` либо `make check-fast` после последних содержательных правок. Не закрывать Light E6/roadmap и не делать push; вывод проверки хранить штатным механизмом evidence.

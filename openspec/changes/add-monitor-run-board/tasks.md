## 1. Согласованная проекция

- [ ] 1.1 Добавить компактное атомарное чтение Run snapshot и admission waiting; проверить storage/runtime tests и отсутствие записи при чтении.
- [ ] 1.2 Добавить локальную board-проекцию категорий, причин, действий и всех активных стадий/ветвей; проверить table-driven tests для generic/legacy, decision/resource, partial/failed/cancelled и uncertain.

## 2. Каталог и интерфейс

- [ ] 2.1 Добавить board view в metadata pagination/filtering и обновление nonterminal summaries при изменении admission; проверить HTTP tests с более 200 Run, двумя source, недоступным source и exact revision.
- [ ] 2.2 Добавить карточки, provenance links и переходы к existing details; проверить Node UI tests на multiple branches, fallback, HTML escaping и отсутствие объединения по title.
- [ ] 2.3 Сохранить фильтры, страницу, scroll и старые виды при polling и возврате; проверить живым Playwright browser на CLI monitor с реальными сохранёнными runtime states и цепочкой partial → continue → cancel → continue, включая повторный continue и legacy отсутствие связи.

## 3. Приёмка и поставка

- [ ] 3.1 Обновить пользовательский вход в документацию monitor, зафиксировать E1-A1–A9 evidence; проверить `openspec validate add-monitor-run-board --strict --no-interactive`, `node cmd/prifly/monitor_ui_test.cjs` и `make check-fast`.
- [ ] 3.2 Проверить отсутствие изменений `openspec/changes/archive/**`, прежних release evidence и frozen schemas; выполнить полные ворота `make ci-check-once CHECK_CONTEXT=codex-app-authorized CHECK_FOR_COMMIT=1`, коммит, sync/archive выполненного change и закрытие #4 только после проверки критериев.

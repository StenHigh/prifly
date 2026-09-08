## 1. Каталог и чтение истории

- [x] 1.1 Добавить проверяемое постраничное чтение snapshots без лимита полного telemetry scan; проверить invalid cursor, digest и более 200 Run тестами internal/local и internal/runtime.
- [x] 1.2 Добавить регистрацию roots и фоновое обнаружение с owner/canonical identity, прогрессом и изоляцией ошибок; проверить несколько roots, symlink, foreign owner и недоступный источник в тестах monitor.
- [x] 1.3 Добавить общий HTTP список с поиском/фильтрами/страницами и scoped чтение Run, событий и артефактов; проверить чужой source, методы, Host, ошибки отдельного источника и результат поиска за первой страницей.

## 2. Запуск и интерфейс

- [x] 2.1 Обеспечить автозапуск monitor после создания Run в start/project start/fork/scheduler до drive, с отдельными предупреждениями; проверить hook paths и конкурирующий запуск subprocess в изолированном user config.
- [x] 2.2 Реализовать русский обзор, фильтры и прямые ссылки, системную тему и загрузку/ошибки; проверить браузером и runnable UI checks, включая escaping недоверенных строк.
- [x] 2.3 Реализовать схему pinned workflow со связями и будущими стадиями, выбор вложенного invocation и синхронное дерево instances/attempts; проверить branch/parallel/nested/repeat данные.
- [x] 2.4 Реализовать читаемые задания/контекст, результаты/проверки/артефакты/решения, сохранённые file facts и честные метрики; проверить отсутствие выдуманных значений и исключение chat/tool/stdio интеграций.
- [x] 2.5 Проверить polling 1500 ms, stale response guard и сохранение раскрытия/прокрутки; выполнить сценарий обновления в браузере и UI checks.

## 3. Документация и проверка

- [x] 3.1 Обновить справку, README, SOURCE-OF-TRUTH и текущий delivery backlog без заявления release/P2 qualification; проверить openspec validate add-local-run-monitor --strict.
- [x] 3.2 Выполнить targeted Go tests для monitor/storage/runtime/CLI, make vet refusal-check fmt-check schemas-check и UI checks; записать результаты и границы в evidence.md.
- [x] 3.3 Проверить git diff --check и отсутствие изменений в historical evidence/public bundles; повторить только проверки, затронутые финальными исправлениями.

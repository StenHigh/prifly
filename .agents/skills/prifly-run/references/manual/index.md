# Pri-Fly: справочник эксплуатации

Этот справочник отвечает на вопрос «как выполнить операцию». Читайте нужную страницу, а не весь каталог.
В установленном runner страницы находятся в `references/manual/`. Их версия указана в `references/manual-version.json`.
Поставка соответствует binary, который установил runner. Возможности другой версии сначала проверяйте через `capabilities`.

## Быстрый маршрут

| Задача или симптом | Страница |
|---|---|
| Установка, clone, binary/workflow/runner update, конфликт обновления | [Установка](installation.md) |
| Выбор launch, inputs, profile, questionnaire, prepare, start | [Новый запуск](launch.md) |
| Получить задание, выполнить шаг, записать outputs, отправить отчёт | [Исполнение](hosting.md) |
| Ответить на preflight/runtime question, yield, отчёт о вопросах | [Решения](decisions.md) |
| Continue/resume/reopen/recover; cancelled writer; неизвестные эффекты | [Восстановление](recovery.md) |
| Worktree/checkout, claims, дерево входа/выхода, permitted effects | [Рабочее дерево](workspace.md) |
| Написать или изменить YAML, extension, composition, программу | [Авторинг](authoring.md) |
| ArtifactRef, schema, task/source import, package trust | [Данные и пакеты](artifacts.md) |
| Shared schemas после ошибочного terminal revoke | [Миграция схем](schema-migration.md) |
| Pause/cancel/stops, approvals/grants, lost response, action admission | [Управление](control.md) |
| Monitor/Light, events/timing, stdout/stderr, progress, telemetry | [Наблюдение](observability.md) |
| Queue, capacity, exclusive resource, storage budget, cleanup | [Обслуживание](operations.md) |
| Отказ или неизвестный следующий шаг | [Диагностика](troubleshooting.md) |
| Точная форма команды, все flags и defaults | [Команды](commands.md) |
| Имя из `prifly capabilities` | [Индекс возможностей](capabilities.md) |
| Paths, JSON/exit codes и значения placeholders | [Основы CLI](basics.md) |

Читайте только нужный раздел страницы. Для стандартного host loop достаточно правил skill.
Не загружайте весь справочник ради одного поля: используйте оглавление или поиск по точному имени.

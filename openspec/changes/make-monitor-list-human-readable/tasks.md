## 1. Проверяемые подписи списка

- [x] 1.1 Добавить в компактную проекцию Run fallback с `task.title` только при отсутствии RunBrief; проверить Go-тестами Run с RunBrief, TaskInput и отсутствующим названием.
- [x] 1.2 Добавить optional title в Project execution profile и совместимо закреплять его в новых Run при `project start`; проверить старый Run без поля и новый Run с title Go-тестами и schema checks.
- [x] 1.3 Отрисовать title проекта первой строкой таблицы, а у Run без него — явно помеченное локальное имя authority root; сохранить project identity, workflow, путь и Run ID вторичными данными и проверить `node cmd/prifly/monitor_ui_test.cjs`.

## 2. Приёмка

- [ ] 2.1 Запустить `go test ./cmd/prifly ./internal/runtime`, `node cmd/prifly/monitor_ui_test.cjs`, `make schemas-check`, `openspec validate make-monitor-list-human-readable --strict` и `git diff --check`; подтвердить отсутствие изменений historical Runs и evidence.

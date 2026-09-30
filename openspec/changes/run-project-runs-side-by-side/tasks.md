## 1. Драйвер на Run

- [x] 1.1 `driver-<run>.lock` + разделяемый `driver.lock` authority; `ReleaseWorktree`, очистка — исключительно; живость по файлу Run. Проверка: тест «две программы разных Run одновременно», тест «второй драйвер того же Run отказывает», `go test -race`.

## 2. Ресурсы

- [x] 2.1 `extend.yaml` `resources` (имя, limit, stages), `local.yaml` переопределение лимита; отказ до создания Run для неизвестной стадии. Проверка: `TestProjectResourcesKeepTwoRunsOffOneStage`.
- [x] 2.2 Привязка в Run (новая версия состояния), лимиты в authority, допуск с ресурсами, наследование вызовом. Проверка: тесты «лимит 1 — вторая ждёт и допускается сама», «лимит 2», «вызов».
- [x] 2.3 Держатели ресурса в отказе, `capacity show`, мониторе; `prifly-run` объясняет ожидание. Проверка: тест CLI и `node cmd/prifly/monitor_ui_test.cjs`.

## 3. Запуск

- [x] 3.1 `project start` передаёт код исходной причины и id Run. Проверка: тест с занятым драйвером.
- [x] 3.2 Ёмкость по умолчанию 2 для новых authority. Проверка: тест.

## 4. Документация и ворота

- [x] 4.1 Справочники профиля и расширения, строка возможности, troubleshooting, словарь. Проверка: `TestEveryDeclaredCapabilityIsInTheAuthorIndex`, `TestGlossaryBindings`.
- [ ] 4.2 `make ci-check`, затем на GitHub `verify` и `qualify`.

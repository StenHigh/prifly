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
- [x] 4.2 `make ci-check`, затем на GitHub `verify` и `qualify`.

Выпущено 2026-09-30 в v0.13.71 (тег на `cc3a661` через `scripts/tag-release.py`). Первый `qualify` на `898bd62` упал на e2e: `test/e2e/verify-capacity.py` ждал `driver_already_active` от второго Run и точный вывод `capacity show` без `resources`; исправлено в `cc3a661`, `verify` и `qualify` зелёные, release run 36718915788 опубликовал 6 ассетов.

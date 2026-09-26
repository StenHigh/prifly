## 1. Сохранение деревьев и передача claim

- [x] 1.1 Сначала тест: Run в режиме `worktree` отменён с незакоммиченным файлом, в том же репозитории создаётся новый claim — дерево и ветка отменённого Run остаются; тот же сценарий с исходом `succeeded` освобождает claim как прежде. Затем изменить `releaseSettledClaim`. `go test ./internal/runtime -run 'Test.*Claim' -count=1`.
- [x] 1.2 Передача claim завершённого Run связанному Run в транзакции создания: generation растёт, происхождение читается через fork provenance, файлы не меняются. Отказы: claim освобождён, привязан к другому Run, inode изменился, исходный Run не завершён. Прежние записи claims читаются.

## 2. Объявления в workflow

- [x] 2.1 Checkpoint и `continuation` в авторинге и WorkflowRevision 7: `schemas/core/workflow-revision-v7.schema.json`, authoring-схема, лестница поднимает ревизию по наличию полей; ревизии 1–6 и опубликованные схемы не меняются. Отказы: порт не объявлен, схема не совпадает, не step-стадия, нет схемы checkpoint, вызов с другой схемой, несуществующий или повторный вход, пустые списки, неизвестный вердикт/исход, `failed` в исходах, явная ревизия 6 с полем. `go test ./internal/flow -count=1`, `make schemas-check`.
- [x] 2.2 Показать объявления в `examples/authoring/workflow-authoring-reference.yaml` с минимальной ревизией, строку в `examples/README.md`; `TestEveryDeclaredCapabilityIsInTheAuthorIndex` зелёный.

## 3. Нейтральные продолжение и восстановление

- [x] 3.1 Последний принятый checkpoint выводится из принятых результатов всех вызовов по порядку settlement и показывается в `run status` через новую версию чтения (в `run next` — изменением обработки blocked, которое и так выпускает новую версию ответа next); отвергнутый кандидат не сообщает checkpoint. Тест на настоящем Run с checkpoint в вызываемом workflow и отказ, когда исходный Run не принял ни одного checkpoint.
- [x] 3.2 Сначала тесты отказа на двух нейтральных пакетах (`example:`): `continuation_source_ineligible` (workflow, исход), `continuation_source_incomplete` (стадия, вердикт, порт, checkpoint); затем положительный перенос с provenance и переданным claim, в котором сохранён незакоммиченный файл. Переписать `continuation.go`, `start.go`; удалить литералы. `go test ./internal/runtime -run 'Test.*Continu' -count=1`.
- [x] 3.3 Восстановление: передача claim, данные восстановления без `subject_commit` в новой версии состояния (опубликованная схема прежних версий не меняется), `recovery/1` читается; условие на `review`/`head_commit` удалено; `recover_workspace_released` при освобождённом claim. Тест на workflow без стадии `review`. `go test ./internal/runtime -run 'Test.*Recover' -count=1`.
- [x] 3.4 Guard: тест сканирует не-тестовые `.go` в `internal/` и `cmd/`, печатает число файлов и совпадения, отказывает на `<namespace>:(workflow|package|step)/` вне `core`. Доказать красный до удаления литералов и зелёный после.

## 4. Project CLI

- [x] 4.1 `project continue`: любой launch с объявлением, `project_continue_undeclared` для остальных, передача claim по умолчанию, `--workspace-commit` вместо `--implementation-head`, prepare показывает источник каждого входа и claim; убрать вычисление `implementation` из Git. `project recover`: без выбора commit и условия на `review`. Обновить справку; переписать `TestCLIContinuationFromUnmergedImplementation` и CLI-тесты восстановления на нейтральные пакеты.
- [x] 4.2 Переписать delta specs активных changes `continue-existing-implementation` и `retry-failed-stage-with-new-package` в нейтральной форме; `openspec validate` обоих `--strict`.
- [x] 4.3 Обновить `examples/troubleshooting.md`, раздел «Хост убил `run drive`»: дерево отменённого Run сохраняется, продолжение — `project continue`.

- [x] 4.4 CLI компилирует workflow launch один раз: план, из которого читается объявление продолжения, передаётся в проверку запуска.
- [x] 4.5 Руководство для автора workflow и ИИ-агента в `examples/authoring/`: что такое checkpoint и зачем, как сделать workflow продолжаемым, что делает движок и чего не делает, как проверять дерево первым шагом, как устроены передача claim и хранение деревьев, отказы и что делать с каждым; ссылки из `examples/README.md` и `AGENTS.md`-карты не нужны, если справочник найден через таблицу возможностей.

## 5. Проверка и выпуск

- [ ] 5.1 `make check`, `make race`, `make e2e`, `git diff --check`, `openspec validate declare-continuation-in-the-workflow --strict` — зелёные локально 2026-09-26 (macOS); CI на Linux — после push (`gh run list`).
- [ ] 5.2 Передать сессии пакета: что изменилось и зачем; checkpoint на шагах, меняющих дерево; объявление продолжения; первый шаг проверки дерева и вычисления `implementation`; какие флаги и отказы исчезли.
- [ ] 5.4 Не сломать разработку пилота: пилот обновляет движок только вместе с редакцией пакета, в которой есть объявление продолжения; до этого остаётся на текущем релизе. Сообщить пилоту заранее: новые деревья незавершённых Runs копятся до `claim release`; восстановление старых failed Runs с уже освобождённым деревом даёт `recover_workspace_released`; уже идущие Runs продолжения доводятся без изменений.
- [ ] 5.3 После выпуска пакета на стенде: довести новый Run до partial и до технического отказа, выполнить `project continue --prepare` и `project recover --prepare`; записать номер релиза движка и редакцию пакета.

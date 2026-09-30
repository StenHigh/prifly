## 1. Достоверная monitor-проекция

- [x] 1.1 Добавить единую read-only классификацию узла и activity rows из закреплённых Run/Attempt/Session/Process/decision/wait/fork фактов (`executionRole`, `activityRows` в `monitor.js`, см. design); UI-тестами проверить host Attempt, ожидание допуска и решения, локальную программу, вложенный workflow, fork и неизвестную роль.
- [x] 1.2 Показать активную работу и ожидания из read cut `/api/run` и место в очереди допуска из `/api/capacity` без новых маршрутов API; UI-тестами проверить несколько host Attempt, ready stage, pending decision, очередь допуска и отсутствие ложного утверждения о живом процессе.

## 2. Экран Run

- [x] 2.1 Добавить в карточку узла роль, adapter/executor/host и честную формулировку состояния, а для fork — provenance и переход к исходному Run; проверить `node cmd/prifly/monitor_ui_test.cjs`.
- [x] 2.2 Добавить навигационную секцию «Сейчас» с работой и ожиданиями, которая обновляется вместе с Run и не сбрасывает выбранный узел; проверить UI-тестом и ручным чтением monitor API.

## 3. Приёмка

- [x] 3.1 Запустить `go test ./cmd/prifly ./internal/runtime`, `node cmd/prifly/monitor_ui_test.cjs`, `openspec validate show-live-execution-ownership --strict` и `git diff --check`; подтвердить, что read-only монитор не меняет historical Runs и не добавляет управляющих HTTP операций. 2026-09-30: `go test` ok (cmd/prifly 120 с, runtime 130 с), UI-тест, validate, diff-check зелёные; новых маршрутов API нет.

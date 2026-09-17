## 1. Подключение клона

- [x] 1.1 `cmd/prifly/project.go`: `checkExistingProjectRunners` проверяет присутствие только у названного `--host`; отсутствующие объявленные хосты возвращаются списком. Проверка: тест — клон с профилем из трёх хостов и одним раннером, `project init --host claude-code` создаёт local configuration и называет `missing_hosts`; `go test ./cmd/prifly -run 'Runner|ProjectInit'`.
- [x] 1.2 `project runners add --host NAME` отказывает только из-за конфликта названного раннера. Проверка: тест — в том же клоне `runners add --host codex-cli` создаёт файл; красный до правки (`project_runner_missing`).
- [x] 1.3 Отказ, который остаётся, называет выход в `safe_next_actions` (`project.runners.update`/`project.runners.add`). Проверка: тест на изменённый раннер — код прежний, список действий непустой.

## 2. Рабочая копия и итог

- [x] 2.1 `cmd/prifly/project_start.go`, `internal/runtime/sessions.go`: путь рабочей копии в ответе запуска и в задаче — абсолютный. Проверка: тест — путь из ответа открывается без доработки; `go test ./cmd/prifly ./internal/runtime -run 'Start|SessionTask'`.
- [x] 2.2 `--prepare` печатает имена переменных окружения программ без значений. Проверка: тест — в итоге есть имя, нет значения; `README.md` строка про environment остаётся верной.

## 3. Обещанный документ задачи

- [x] 3.1 `cmd/prifly/main.go`, `internal/runtime/sessions.go`: `session task --all` вручает каждую перечисленную попытку и пишет `task.json` в её рабочую папку; справка называет обе формы точно. Проверка: тест — после `--all` в папке каждой выданной попытки лежит `task.json`, равный документу из вывода; монитор (API `SessionTasks`) ничего не пишет; `go test ./cmd/prifly ./internal/runtime -run 'SessionTask'`.

- [x] 3.2 `project runners update --check` печатает тот же результат и не трогает ни одного файла. Проверка: тест — устаревший раннер, после `--check` файл байт в байт прежний, `updated_hosts` называет его; без флага он заменяется.

## 3b. Граница драйвера

- [x] 3b.1 `cmd/prifly/main.go`, `internal/runtime/driver.go`: `run drive` получает флаг, который ведёт Run через управляющие стадии и вручения и останавливается перед исполнением программы, возвращая управление; без флага поведение прежнее. Проверка: тест — Run, где за управляющей стадией стоит шаг-программа: с флагом драйвер доходит до неё и не запускает (программа не исполнялась, попытка не создана), `run next` показывает `stage_work: program`; без флага программа исполняется как прежде; `go test ./internal/runtime ./cmd/prifly -run 'Drive|Program'`.
- [x] 3b.2 Справка `run drive` и запись в `examples/troubleshooting.md` называют границу и прежнее поведение. Проверка: `python3 -B test/e2e/test_examples.py`.

## 4. Документы и ворота

- [x] 4.1 `examples/troubleshooting.md`: запись «клон не подключается: project_runner_missing у чужого хоста» с новым поведением и обходом для старых сборок (сузить hosts временно). Проверка: `python3 -B test/e2e/test_examples.py`.
- [x] 4.2 Защищённая история не тронута; `make ci-check`, `make e2e`, `make race` зелёные, счётчики записаны в change (общий прогон окна: `../take-a-secret-without-copying-it/evidence.md`, 2026-09-18).

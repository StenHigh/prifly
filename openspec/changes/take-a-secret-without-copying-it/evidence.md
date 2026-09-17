# Проверки

Дата: 2026-09-18. Прогон покрывает дерево с обоими изменениями окна —
`let-a-clone-join-an-existing-project` и это; отдельного выпуска между ними нет.

- `make ci-check GO="$(go env GOROOT)/bin/go"`: PASS. `go test ./...` — cmd/prifly 42.295 с, internal/flow 7.062 с, internal/local 7.302 с, internal/release 1.248 с, internal/runtime 255.107 с; `go vet` и `CGO_ENABLED=0 go vet` без находок; fmt-check — 294 файла, все gofmt-clean; refusal-check — 161 файл, кода отказа внутри текста ошибки нет; staticcheck — 9 пакетов, 0 находок; vuln-check — 9 пакетов, уязвимостей нет; schemas-check — все опубликованные bundle'ы совпали, включая новые `environment-source` (235 112 байт, sha256:159bb4313a65108b215c0e502c0dc8bf821a7480ef1d34b4af21ecf4452d450d) и `execution-bindings-v2` (2445 байт, sha256:4d1dfa3b3a152285d1c7676c2e1db94925c9fa00cac4798682ff6865d9c04139); release-ci-check — контракт на месте.
- Первый прогон ci-check был красным: staticcheck U1000 на `isEnvironmentSourceState` — предикат, который ни один читатель не спрашивает, потому что ни один read-контракт на 32 не ветвится. Удалён, а не отключён.
- `make e2e GO="$(go env GOROOT)/bin/go"`: rc=0, 13 итоговых `outcome`, все `passed`; ни одного `failed`/`refused`. В том числе install verification, 6 тестов примеров, 8 package-кейсов и 1 launch-кейс каталога.
- `make race GO="$(go env GOROOT)/bin/go"`: rc=0, ни одного `DATA RACE`. cmd/prifly 181.370 с, internal/flow 31.001 с, internal/local 8.058 с, internal/release 2.091 с, internal/runtime 727.539 с.
- `openspec validate --all --strict`: 23 passed, 0 failed. `python3 -B test/e2e/test_examples.py`: 6 тестов, OK.

## Что доказали тесты, а не сборка

- `TestDriverEnvironmentSourceReachesTheProgramWithoutEnteringState`: программа получила объявленное значение (`worker-sourced` в рабочей папке попытки), Run идёт под `core-state/32`, и обход **всех** обычных файлов authority root не нашёл значения нигде, кроме файла, который написала сама программа. Проверка проверки: подброшенный `planted-leak` с тем же значением её валит (`the value was written to planted-leak`), то есть обход действительно читает.
- `TestDriverMissingEnvironmentSourceRefusesBeforeStart` (env / file / dotenv): попытка завершается неначатой — `dispatch == nil`, `started == nil`, `process_outcome.started == false`, `worker-ready` не появился, Run `failed`, диагностика `execution_environment_unavailable`, а текст, который получает вызывающий `run drive`, называет переменную.
- `TestEnvironmentSourceReadsExactlyTheDeclaredPlace`: 4 принимающие формы и 7 отказов, включая закомментированный ключ, значение в кавычках и файл больше допустимого; ни один текст отказа не содержит значения.
- `TestExecutionBindingsClosedPayload`: `execution-bindings/1` с полем `environment_from` отказан — опубликованный контракт остался закрытым; `/2` его принимает; `/3` неизвестен.
- `TestStartRefusesADeclaredSourceOutsideTheScopedState`: плоский Run отказывает `unsupported_environment_source`, а не запечатывает объявление под версией, чей контракт для него места не имеет.
- `TestProjectLaunchDigestFollowsTheSourceNotTheValue`: смена значения в файле не меняет `configuration_digest`, смена ключа — меняет.

## Дефекты, найденные собственными тестами

- `project local set` только с `--env-from` уходил в legacy-ветку замены строки и записывал `prifly_executable: ""` — правка в `cmd/prifly/project.go` (условие допуска и условие маршрутизации).
- Относительный путь источника молча резолвился от каталога, в котором запущен инструмент. Теперь отказ: путь называется абсолютно или никак.
- Отказ, рождённый внутри `read()` (значение в кавычках, слишком большой файл), не называл переменную: имя знает только цикл разрешения, и он его теперь и подставляет.

## Найдено рядом и не тронуто

`internal/runtime/driver.go:1155` — единственный `return` между запуском
сторожевой горутины (1084) и `close(watchDone)` (1186). На этом пути горутина
остаётся без хозяина: она живёт до дедлайна попытки, а по `ctx.Done()` вызывает
`requestDriverCancellation`, то есть durable `Restrict{kind: cancel}` для Run,
попытка которого уже завершена неначатой. Дефект существующий; разрешение
источников вынесено **до** запуска сторожа и его не добавляет. Заведено
отдельным изменением `settle-the-driver-watcher-on-every-exit`.

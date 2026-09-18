## 1. Init отвечает за названный хост

- [x] 1.1 `cmd/prifly/project.go`: при существующем профиле `--host`, который профиль уже объявляет, не отказ; создаётся недостающая local configuration и authority, `missing_hosts` называется как прежде. Проверка: тест — клон с профилем из трёх хостов, `project init --host claude-code` создаёт `local.yaml` и не трогает общий YAML и раннеры; красный до правки (`project_profile_conflict`); `go test ./cmd/prifly -run 'ProjectInit|Runner'`.
- [x] 1.2 Отказ `project_profile_conflict` остаётся ровно для хоста, которого профиль не объявляет, и называет исполнимый выход в `safe_next_actions`. Проверка: тест на нераспознанный и на необъявленный хост.

## 2. Квитанция описывает файл

- [x] 2.1 `cmd/prifly/project_local_execution.go`: `allowed_executables` в ответе читается из `local.yaml`, как и `environment`. Проверка: тест — после `--env-from` в ответе остаются ранее разрешённые программы; `go test ./cmd/prifly -run 'Local|EnvironmentSource'`.

## 3. Документы и ворота

- [x] 3.1 `examples/troubleshooting.md`: запись про подключение клона дополнена формой с `--host`. Проверка: `python3 -B test/e2e/test_examples.py`.
- [x] 3.2 `make ci-check`, `make e2e`, `make race` зелёные; счётчики записаны в change (`evidence.md`, 2026-09-18).

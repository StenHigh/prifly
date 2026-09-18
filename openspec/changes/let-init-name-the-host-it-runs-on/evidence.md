# Проверки

Дата: 2026-09-18.

- `make ci-check GO="$(go env GOROOT)/bin/go"`: rc=0. vet прочитал linux и darwin; fmt-check 295 файлов, все gofmt-clean; refusal-check 161 файл; staticcheck 9 пакетов для linux и 9 для darwin, без находок; vuln-check 9 пакетов; все bundle'ы совпали.
- `make e2e GO="$(go env GOROOT)/bin/go"`: rc=0, 13 итогов, все `passed`.
- `make race GO="$(go env GOROOT)/bin/go"`: rc=0, ни одного `DATA RACE`. cmd/prifly 182.248 с, internal/runtime 711.991 с.
- `openspec validate --all --strict`: 24 passed, 0 failed. `python3 -B test/e2e/test_examples.py`: 6 тестов, OK.

## Красное до правки

`TestCLIProjectInitNamesTheHostItRunsOn` — три ветки, все три красные на сборке
до правки с одним и тем же ответом
`invalid_usage: project_profile_conflict … safe_next_actions:["help"]`:

- `declared-and-present` — профиль объявляет `claude-code`, раннер в дереве:
  теперь init создаёт `local.yaml`, называет `missing_hosts` двух чужих хостов,
  общий YAML байт в байт прежний, `.codex/` не появляется.
- `declared-and-absent` — `--host codex-cli`, раннера нет: отказ
  `project_runner_missing`, `safe_next_actions` содержит `project.runners.add`,
  `local.yaml` не создан.
- `undeclared` — профиль объявляет только `claude-code`, назван `codex-cli`:
  отказ `project_profile_conflict`, сообщение называет хост, `safe_next_actions`
  содержит `project.runners.add`.

`TestProjectLocalReceiptDescribesTheFile` — после `--allow-executable shell` и
`--env APP_ENV` вызов только с `--env-from` возвращает квитанцию, в которой
остались и программа, и окружение, и появился источник; значения в ней нет.

## Что подтвердил замер чужой сессии

Холодный заход на опубликованной 0.13.33 прислал сырые конверты. Из трёх
пунктов подтвердились два (оба здесь), третий — `--check` против обычного
`update` — закрыт их же парным замером с `sha256sum`: между вызовами менялся
движок (`prifly update` 0.13.32 → 0.13.33), а не файл, поэтому оба ответа были
верны. Правки не потребовалось; в справочник добавлено разъяснение.

# Базовые правила кода

> Соглашения, снятые с кодовой базы. Специфичные правила областей — `rules.<area>` в `config.yaml`.

## Именование

- Файлы: `snake_case.go`, тесты рядом `*_test.go`; CLI-команды — по одному файлу на семейство (`project_*.go`, `monitor_*.go`).
- Идентификаторы — канонические понятия словаря `openspec/specs/specification-governance/terms.md`; синоним без нового смысла не вводится (`TestGlossaryBindings`).
- Коды отказов — `snake_case` (`project_profile_incomplete`, `unsafe_authority_root`); код рождается там, где принято решение.

## Структура модулей

- `cmd/prifly` — только CLI; предметная логика — `internal/runtime`, модель — `internal/flow`, ОС — `internal/local`.
- Сохранённые JSON-поля и опубликованные схемы не переименовываются; новая форма — новая версия рядом.

## Ошибки

- Отказ — типизированный `runtime.Fault{Code, Cause}`; `errors.New("code: …")` валит `make refusal-check`.
- Ошибки не глотаются: неизвестный исход не становится нулём или успехом.

## Control flow

- Плоский поток: guard clauses и ранний `return`; transform-команды — чистые функции снимка (`internal/purity`).

## Проверки

- Перед «сделано»: точечные тесты затронутого поведения, `git diff --check`; полные ворота (`make ci-check`, `make e2e`, `make race` в фоне, `openspec validate --all --strict`) — для release/product milestone или по слову владельца.
- Новый сторож обязан уметь краснеть: сначала падающий случай, потом зелёный; ворота печатают, сколько входов прочитали.
- Команды запускаются из явного target worktree (`-C`); зелёный в соседнем дереве ничего не доказывает.

## Логирование

- Логов у CLI нет: наблюдаемость — `--json`, `run status`, Problem-конверт на stderr и монитор. Не добавлять печать в stdout поверх одного финального документа.

# Pri-Fly

## Обзор

Pri-Fly — детерминированный локальный движок устойчивых сценариев (workflow
runner) на Go. Он выдаёт задания исполнителям — программам и ИИ-хостам (Codex
CLI, Codex app, Claude Code) — и ведёт запись того, что им было сказано и что
они вернули. Модели он не вызывает и промптами не владеет: сценарии AI Factory
живут отдельно (`StenHigh/prifly-aif-workflows`) и ставятся в проект командой
`prifly project workflows add`.

Продуктовое назначение, установка и быстрый старт — `README.md`. Нормативная
спецификация — только `openspec/specs/` (карта: `openspec/SOURCE-OF-TRUTH.md`).

## Стек

- **Язык:** Go 1.27, один модуль `github.com/stenhigh/prifly`, CGO только для
  драйвера SQLite (`make vet` собирает runtime и CLI с `CGO_ENABLED=0`).
- **Хранилище:** локальная authority на SQLite (`github.com/mattn/go-sqlite3`),
  каталог `~/.prifly/authorities/<sha256 пути репозитория>`.
- **Контракты:** JSON Schema 2020-12 (`santhosh-tekuri/jsonschema/v6`),
  канонический JSON (JCS), YAML authoring (`go.yaml.in/yaml/v3`).
- **Проверки:** `go test` рядом с пакетами, black-box e2e на Python/shell в
  `test/e2e/`, CI — GitHub Actions (`verify.yml`, `release.yml`).
- **Поставка:** подписанные релизы GitHub, `scripts/install.sh`, `prifly update`.

## Что важно знать

- Опубликованные схемы и сохранённые JSON-поля заморожены по digest: новое поле
  публикуется рядом со старым контрактом, а не вписывается в него.
- Отказ несёт код в типизированном `runtime.Fault`, не в тексте ошибки; каждый
  ненулевой exit заканчивается одним Problem-конвертом на stderr.
- Transform-команды authority — чистые функции снимка; guard `internal/purity`.
- Документация и процесс — OpenSpec (`openspec/changes/`, `openspec/specs/`);
  рабочее состояние сессий — `CONTEXT_STATE.md` (ориентир, не норма).

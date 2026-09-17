# Архитектура Pri-Fly

Карта, не спецификация: решения и их основания — в
`openspec/specs/architecture-decisions/spec.md`, словарь понятий — в
`openspec/specs/specification-governance/terms.md`. При расхождении правда там.

## Слои

| Пакет | Роль |
|---|---|
| `cmd/prifly/` | CLI: разбор команд, Problem-конверты и exit-коды, проектный профиль (`project *`), host runners, монитор (`monitor*.go`) |
| `internal/runtime/` | Authority: admission, lifecycle Run/Attempt/Session, локальный протокол, claim рабочих копий и снимки деревьев |
| `internal/flow/` | Версионная детерминированная модель workflow: контракты, схемы, канонизация, компиляция YAML в sealed package |
| `internal/local/` | Локальное хранилище: SQLite store, blob'ы, транзакции и их guard, ошибки драйвера, запуск процессов |
| `internal/purity/` | Guard: transform-команда не читает файлы и не запускает процессы внутри write-транзакции |
| `internal/release/` | Контракт публичной поставки: манифест, подпись, проверка |
| `schemas/` | Опубликованные JSON Schema (`core/`, `foundation/`, `authoring/`); проверяются `make schemas-check` |
| `test/e2e/`, `test/fixtures/` | Black-box проверки собранного CLI и локальные fixture-воркеры |
| `examples/` | Справочники авторинга YAML и `troubleshooting.md` (симптом → причина → поле) |

## Границы

- Authority — единственное место решения; CLI и runtime не зависят от драйвера
  хранения (`make vet` с `CGO_ENABLED=0`).
- Sealed package закрепляет байты контекстов (в том числе host skills) при
  `project compile`/`project start`; хост не угадывается по папкам.
- Исполнитель получает задания через session protocol (`run next`, `session
  task`, `session submit`); движок не вызывает модель.
- Эффекты шага объявляются и измеряются движком (`permitted_effects`, claimed
  workspace, выходные слоты); текст правил их не расширяет.

## Точки входа

| Файл | Назначение |
|---|---|
| `cmd/prifly/main.go` | Диспетчер команд и вывод |
| `cmd/prifly/project.go` | `project init/workflows/questionnaire/runners`, discovery профиля |
| `cmd/prifly/project_start.go` | Запуск объявленного launch: анкета, seal, claim, первый handoff |
| `internal/runtime/model.go` | Модель authority |
| `internal/flow/types.go` | Модель workflow и контрактов |
| `internal/flow/protocol.schema.json` | Источник истины опубликованного контракта |
| `Makefile` | Ворота: `check`, `ci-check`, `race`, `e2e`, `schemas-check` |

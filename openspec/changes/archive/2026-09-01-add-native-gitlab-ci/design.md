## Context

См. `proposal.md`. `.gitlab-ci.yml` отсутствует. `make check` запускает Go
tests, race, vet, formatting и JSON Schema checks; `make e2e` собирает binary
и запускает Python-based end-to-end tests. `go.mod` задаёт Go 1.27.0, а
`go-sqlite3` требует C compiler.

## Goals / Non-Goals

**Goals:**

- Проверять каждый GitLab pipeline теми же двумя актуальными gates, что и
  contributor локально.
- Не полагаться на локальную `.tools/go` директорию или прежнее document
  tooling.
- Сохранять только безопасный, disposable Go cache.

**Non-Goals:**

- Не публиковать release, package, evidence или binary из CI.
- Не вводить custom container image, deployment, external credentials или
  отдельный documentation job.

## Decisions

### Две независимые test jobs

Pipeline содержит `check` и `e2e` jobs в test stage. Это даёт понятный
результат каждому обязательному gate и не передаёт binary между jobs: e2e
собирает его сам, как самостоятельную проверку.

### Официальный Go image и явные developer prerequisites

Jobs используют официальный Debian-based Go image версии из `go.mod`.
`before_script` устанавливает только `python3` и `build-essential`, затем
вызывает Make с полными путями к `go` и `gofmt`. Это не зависит от `.tools/go`
и сохраняет current Makefile contract. E2E-тесты создают temporary directories
в переносимом Unix-пути `/tmp`, а не через macOS-specific `/private/tmp`,
поэтому один и тот же gate переносим между локальной машиной и Linux runner.

Альтернатива — custom CI image — отклонена: пока два стандартных пакета не
оправдывают собственный образ и его обслуживание.

### Кэшировать только Go cache

Кэшируются `.cache/go-build` и `.cache/go-mod`, которые Makefile уже изолирует
в repository. Run state, `bin/`, `dist/` и evidence не кэшируются и не
публикуются как artifacts.

## Risks / Trade-offs

- [Первый pipeline медленнее из-за apt packages] → subsequent Go compilation
  ускоряется cache; custom image рассматривается только по измеренной причине.
- [CI image расходится с declared toolchain] → job печатает `go version`, а
  image version закреплена рядом с pipeline.
- [Зелёный pipeline ошибочно принимают за qualification] → jobs называют
  checks, а не release/qualification, и не публикуют release evidence.

## Migration Plan

1. Добавить `.gitlab-ci.yml` с двумя jobs.
2. Проверить YAML локальным parser и сохранить, что source tree не содержит
   historical document tooling.
3. Push branch и подтвердить GitLab pipeline для обоих jobs.

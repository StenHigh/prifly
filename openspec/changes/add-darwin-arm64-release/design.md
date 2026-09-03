## Context

См. мотивацию в `proposal.md`. Installer и updater уже вычисляют OS/architecture
и выбирают запись signed manifest. Однако GitLab pipeline создаёт лишь один
Linux archive и release builder умеет подписать manifest только из одного
asset. Local state использует `github.com/mattn/go-sqlite3`, поэтому
`CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build` не собирается: Apple binary
нельзя добросовестно получить простым Linux cross-build.

## Goals / Non-Goals

**Goals:**

- Один stable manifest и GitLab Release с `linux/amd64` и `darwin/arm64`.
- Нативная reproducible CI-сборка и минимальная platform qualification для
  Apple Silicon.
- Сохранить существующую проверку подписи, digest, installer и updater.

**Non-Goals:**

- Поддержка всех CPU: `linux/arm64` и `darwin/amd64` остаются отдельными
  будущими решениями.
- Замена SQLite driver, эмуляция macOS на Linux или source-build fallback.
- Автоматическая публикация Release без owner manual gate.

## Decisions

### Объявлять platforms явно, а не обещать «любой Linux/macOS»

Release matrix — `linux/amd64`, `darwin/arm64`. OS без named asset получает
объяснимый отказ. Это согласуется с уже существующей моделью manifest, где
platform — пара OS/architecture, а не расплывчатое название ОС.

Альтернатива — сразу добавить Linux ARM и Intel Mac. Она увеличит обязательную
инфраструктуру и число native qualification без пользовательского требования
к этим architecture.

### Использовать native Apple Silicon GitLab runner

macOS job собирает `darwin/arm64` binary с CGO, запускает короткую native
qualification и передаёт только binary как protected job artifact. Linux
release job принимает оба artifacts, формирует оба archives, один manifest и
одну signature. До наличия trusted protected macOS runner release не может
быть опубликован неполным.

Альтернатива — cross-compile из Linux. Она требует отдельного legal macOS SDK
и cross-C toolchain и не даёт native execution qualification; для текущего
CGO SQLite это хуже и сложнее.

### Расширить builder до набора assets

Release builder получает bounded declared set exact `(OS, architecture,
binary)` inputs, создаёт archives в новом empty output directory, затем
canonicalizes и подписывает один manifest. Он отказывает при duplicate
platform, недостающем обязательном platform или любом failed archive build.
Installer копируется один раз как прежний asset.

Альтернатива — собирать по одному manifest и склеивать JSON в shell. Это
создаёт вторую неподписанную логику и легко расходится с updater contract.

### Проверять topology release CI как данные

Существующий CI contract test дополняется assertions на две build provenance,
два release links и обязательную aggregation. Unit tests создают multi-asset
manifest и проверяют exact selection для обоих platforms. Native macOS job
выполняет target-specific test/build command, который может быть воспроизведён
на runner без Release credentials.

## Risks / Trade-offs

- [Protected macOS runner отсутствует или offline] → pipeline отказывает до
  publication; owner видит job, который требует зарегистрированный protected
  Apple Silicon runner.
- [Появляется неверная platform link или manifest topology] → builder и CI
  contract tests отвергают release до upload.
- [Нативный macOS test отличается от Linux] → platform job сохраняет короткий
  deterministic qualification; общий product gate остаётся на verified source
  tree до protected tag.

## Migration Plan

1. Зарегистрировать protected Apple Silicon macOS GitLab runner с release tag.
2. Изменить pipeline и release builder, добавить tests и documentation.
3. Выполнить product gates на source tree и platform qualification на runner.
4. Создать следующий approved semantic tag; manual jobs создадут оба assets и
   один signed stable manifest.
5. При failure не запускать publication: предыдущий Release и установленный
   binary не меняются.

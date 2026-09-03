## ADDED Requirements

### Requirement: Stable Release публикует точную supported platform matrix

Каждый public stable Release MUST содержать в одном signed release manifest
ровно по одному именованному archive для `linux/amd64` и `darwin/arm64`.
Каждый archive MUST содержать binary `prifly` для той же OS/architecture, а
GitLab Release MUST публиковать оба named assets. Installer и `prifly update`
MUST выбирать только archive, точно совпадающий с OS/architecture текущей
машины; отсутствие exact asset MUST завершаться отказом без fallback, сборки
из исходников или замены установленного binary.

#### Scenario: Apple Silicon macOS устанавливает и обновляет Pri-Fly
- **WHEN** installer или updater выполняется на `darwin/arm64` и latest stable
  manifest содержит корректный signed `darwin/arm64` asset
- **THEN** он выбирает только `prifly-darwin-arm64.tar.gz` и использует его по
  обычным правилам integrity проверки и atomic replacement

#### Scenario: Один из обязательных assets отсутствует
- **WHEN** release build или publication пытается выпустить stable Release без
  `linux/amd64` либо `darwin/arm64` archive
- **THEN** выпуск отказывает до создания или изменения GitLab Release

#### Scenario: Неподдержанная архитектура запрашивает установку
- **WHEN** installer или updater выполняется на platform, которой нет в signed
  release manifest
- **THEN** он сообщает отсутствие exact release asset и не использует binary
  другой architecture

### Requirement: macOS release binary проходит native qualification

`darwin/arm64` archive MUST создаваться из binary, собранного и проверенного
на native Apple Silicon macOS runner с теми же source tree и semantic release
version, что и `linux/amd64` asset. Release pipeline MUST NOT представлять
Linux cross-build как qualified macOS binary.

#### Scenario: macOS runner недоступен
- **WHEN** protected tag pipeline не может получить native `darwin/arm64`
  build artifact
- **THEN** release build отказывает и не публикует неполный signed manifest

#### Scenario: native macOS qualification проходит
- **WHEN** protected Apple Silicon runner собрал release binary и выполнил
  required platform qualification
- **THEN** его exact binary становится единственным источником
  `darwin/arm64` archive в signed manifest

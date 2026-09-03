## MODIFIED Requirements

### Requirement: Поставка использует versioned release assets
Каждый поддержанный public build MUST публиковаться только как asset
семантически versioned, tagged GitHub Release репозитория `StenHigh/prifly`.
Release MUST содержать отдельный manifest с version, platform, archive
identity и digest, а также signature, проверяемую встроенным public key.
`latest` разрешён только для обнаружения новейшего stable tagged Release
через постоянную ссылку `releases/latest/download`; branch, commit, workflow
artifact и произвольный URL MUST NOT быть источником bytes для updater.

#### Scenario: Latest Release указывает на корректный archive
- **WHEN** updater запрашивает обновление для своей OS и architecture
- **THEN** он выбирает только запись signed manifest из latest stable tagged
  Release и получает её named archive

#### Scenario: Manifest или archive подменён
- **WHEN** signature manifest не проходит или digest downloaded archive не
  совпадает с подписанным значением
- **THEN** installation отказывает без замены действующего binary

### Requirement: Release tag использует уже qualified source tree
Перед созданием protected release tag тот же source tree MUST пройти required
product gates. Tag workflow MUST выполнять только owner-controlled сборку и
публикацию signed assets и MUST NOT повторять `verify` или `race`.

#### Scenario: Owner публикует проверенный tag
- **WHEN** owner создаёт protected semantic-version tag для source tree с
  успешными required gates
- **THEN** GitHub Actions запускает только release workflow без второго
  product test batch

### Requirement: Credentials публикации разделены
Private signing key MUST быть secret environment `release`, доступным только
publication job после approve required reviewer владельца. Publisher
credential MUST быть job-scoped `GITHUB_TOKEN` с permission `contents: write`
только в этом job; остальные jobs release workflow MUST работать с
`contents: read` и без signing secret. Создание release tags MUST быть
ограничено ruleset владельца репозитория. Publication job MUST проверять
наличие signing key и public key до сборки assets и давать понятную
диагностику при отказе.

#### Scenario: Publisher credential отсутствует
- **WHEN** publication job запускается без signing key в environment или без
  permission `contents: write` у своего `GITHUB_TOKEN`
- **THEN** job отказывает до создания assets или GitHub Release

#### Scenario: Publisher credential не имеет прав protected tag
- **WHEN** tag создан вне ruleset владельца или required reviewer ещё не
  одобрил environment `release`
- **THEN** ничего не подписывается и не публикуется: workflow либо не
  запускается для такого tag, либо ждёт approve без частичной публикации

#### Scenario: Publisher credential совместим с protected tag
- **WHEN** owner одобряет environment для уже квалифицированного protected
  semantic-version tag
- **THEN** job создаёт GitHub Release только для этого tag с проверенной
  identity (`--verify-tag`) и не трогает другие releases

### Requirement: Первичная установка остаётся явной и непривилегированной
Repository MUST публиковать одну copyable `curl` command для first install.
Она MUST скачивать bootstrap в локальный temporary file перед исполнением,
ставить Pri-Fly только в user-writable каталог и не требовать `sudo`. Bootstrap
MUST не запускать project workflow, не менять project authority и не менять
shell profile без явного отдельного выбора. Документация MUST явно назвать
GitHub HTTPS Release asset trust boundary первой установки; до работающего
binary она MUST NOT обещать cryptographic verification сильнее этого trust
boundary.

#### Scenario: Пользователь запускает documented install command
- **WHEN** supported platform получает bootstrap из official GitHub Release
- **THEN** в пользовательском каталоге появляется executable `prifly`, а
  существующие project files и authority не меняются

#### Scenario: Пользователь не имеет прав на destination
- **WHEN** bootstrap не может записать в выбранный user directory
- **THEN** он сообщает точную причину и не оставляет частично установленный
  executable

### Requirement: Stable Release публикует точную supported platform matrix
Каждый public stable Release MUST содержать в одном signed release manifest
ровно по одному именованному archive для `linux/amd64` и `darwin/arm64`.
Каждый archive MUST содержать binary `prifly` для той же OS/architecture, а
GitHub Release MUST публиковать оба named assets. Installer и `prifly update`
MUST выбирать только archive, точно совпадающий с OS/architecture текущей
машины; отсутствие exact asset MUST завершаться отказом без fallback, сборки
из исходников или замены установленного binary.

#### Scenario: Apple Silicon macOS устанавливает и обновляет Pri-Fly
- **WHEN** installer или updater выполняется на `darwin/arm64` и latest stable
  manifest содержит корректный signed `darwin/arm64` asset
- **THEN** он выбирает только `prifly-darwin-arm64.tar.gz` и использует его по
  обычным правилам integrity проверки и atomic replacement

#### Scenario: Один из обязательных assets отсутствует
- **WHEN** release workflow пытается выпустить stable Release без
  `linux/amd64` либо `darwin/arm64` archive
- **THEN** выпуск отказывает до создания или изменения GitHub Release

#### Scenario: Неподдержанная архитектура запрашивает установку
- **WHEN** installer или updater выполняется на platform, которой нет в signed
  release manifest
- **THEN** он сообщает отсутствие exact release asset и не использует binary
  другой architecture

### Requirement: macOS release binary проходит native qualification
`darwin/arm64` archive MUST создаваться из binary, собранного и проверенного
на native Apple Silicon macOS runner — GitHub-hosted `macos-14` или более
новом arm64 runner — с теми же source tree и semantic release version, что и
`linux/amd64` asset. Release workflow MUST NOT представлять Linux cross-build
как qualified macOS binary.

#### Scenario: macOS runner недоступен
- **WHEN** tag workflow не может получить native `darwin/arm64` build
  artifact
- **THEN** release build отказывает и не публикует неполный signed manifest

#### Scenario: native macOS qualification проходит
- **WHEN** hosted Apple Silicon runner собрал release binary и выполнил
  required platform qualification
- **THEN** его exact binary становится единственным источником
  `darwin/arm64` archive в signed manifest

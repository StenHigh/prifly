Authoritative source set: `openspec/specs/release-distribution/spec.md`
(перенесено). Compatibility path: два release публикуют и legacy signature, и
RFC 8785 signature; старый updater проверяет legacy, новый — canonical;
затем legacy удаляется отдельным release решением.

## MODIFIED Requirements

### Requirement: Поставка использует versioned release assets
Каждый поддержанный public build MUST публиковаться только как asset
семантически versioned, tagged GitHub Release репозитория `StenHigh/prifly`.
Release MUST содержать отдельный manifest с version, platform, archive
identity и digest, а также signature, проверяемую встроенным public key.
Подпись MUST вычисляться по RFC 8785 canonical bytes manifest, чтобы её мог
проверить внешний инструмент без воспроизведения сериализации Pri-Fly; на
переходный период Release MAY дополнительно публиковать прежнюю signature
form. `latest` разрешён только для обнаружения новейшего stable tagged Release
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

#### Scenario: Внешняя проверка подписи
- **WHEN** аудитор проверяет `release-manifest.json` и его canonical signature
  стандартным ed25519 инструментом
- **THEN** проверка проходит без исходного кода Pri-Fly

### Requirement: Первичная установка остаётся явной и непривилегированной
Repository MUST публиковать одну copyable `curl` command для first install.
Она MUST скачивать bootstrap в локальный temporary file перед исполнением,
ставить Pri-Fly только в user-writable каталог и не требовать `sudo`. Bootstrap
MUST скачивать release manifest того же Release и сверять SHA-256 archive с
записанным в нём до установки; несовпадение MUST завершаться отказом без
частичной установки. Bootstrap MUST не запускать project workflow, не менять
project authority и не менять shell profile без явного отдельного выбора.
Документация MUST явно назвать GitHub HTTPS Release asset trust boundary первой
установки; до работающего binary она MUST NOT обещать cryptographic
verification сильнее этого trust boundary.

#### Scenario: Пользователь запускает documented install command
- **WHEN** supported platform получает bootstrap из official GitHub Release
- **THEN** в пользовательском каталоге появляется executable `prifly`, а
  существующие project files и authority не меняются

#### Scenario: Пользователь не имеет прав на destination
- **WHEN** bootstrap не может записать в выбранный user directory
- **THEN** он сообщает точную причину и не оставляет частично установленный
  executable

#### Scenario: Digest archive не совпадает с manifest
- **WHEN** скачанный archive имеет другой SHA-256, чем manifest того же Release
- **THEN** bootstrap отказывает и не создаёт executable

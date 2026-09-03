## Purpose

Определяет безопасную публичную поставку Pri-Fly через GitLab Release,
установку без сборки из исходников и явное обновление установленного binary.

## ADDED Requirements

### Requirement: Поставка использует versioned release assets
Каждый поддержанный public build MUST публиковаться только как asset
семантически versioned, tagged GitLab Release. Release MUST содержать отдельный
manifest с version, platform, archive identity и digest, а также signature,
проверяемую встроенным public key. `latest` разрешён только для обнаружения
новейшего stable tagged Release; branch, commit, CI job artifact и произвольный
URL MUST NOT быть источником bytes для updater.

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
product gates. Tag pipeline MUST выполнять только owner-controlled сборку и
публикацию signed assets и MUST NOT повторять `verify` или `race`.

#### Scenario: Owner публикует проверенный tag
- **WHEN** owner создаёт protected semantic-version tag для source tree с
  успешными required gates
- **THEN** GitLab запускает только manual release jobs без второго product
  test batch

### Requirement: Credentials публикации разделены
Private signing key и credential для GitLab Release API MUST быть разными
protected masked CI variables. Publisher credential MUST быть project-scoped
access token с ролью Developer и GitLab `api` scope и использоваться только
manual publication job.

#### Scenario: Publisher credential отсутствует
- **WHEN** owner запускает publication job без publisher credential
- **THEN** job отказывает до создания или изменения GitLab Release

### Requirement: Первичная установка остаётся явной и непривилегированной
Repository MUST публиковать одну copyable `curl` command для first install.
Она MUST скачивать bootstrap в локальный temporary file перед исполнением,
ставить Pri-Fly только в user-writable каталог и не требовать `sudo`. Bootstrap
MUST не запускать project workflow, не менять project authority и не менять
shell profile без явного отдельного выбора. Документация MUST явно назвать
GitLab HTTPS Release asset trust boundary первой установки; до работающего
binary она MUST NOT обещать cryptographic verification сильнее этого trust
boundary.

#### Scenario: Пользователь запускает documented install command
- **WHEN** supported platform получает bootstrap из official GitLab Release
- **THEN** в пользовательском каталоге появляется executable `prifly`, а
  существующие project files и authority не меняются

#### Scenario: Пользователь не имеет прав на destination
- **WHEN** bootstrap не может записать в выбранный user directory
- **THEN** он сообщает точную причину и не оставляет частично установленный
  executable

### Requirement: Обновление выполняется только по явной команде
`prifly update` MUST быть явной user command и MAY выполнять сеть только во
время её вызова. Она MUST менять только binary, поставленный official
bootstrap, и MUST refuse source build, copied binary или installation без
receipt. Update MUST сохранять existing project authority, packages, locks,
Runs и configuration; отсутствие более новой stable version является успешным
read-only result.

#### Scenario: Source build запрашивает update
- **WHEN** binary был собран из repository, а не поставлен bootstrap
- **THEN** `prifly update` отказывает с diagnostic о supported install route и
  не изменяет files

#### Scenario: Новая версия отсутствует
- **WHEN** signed latest stable Release имеет ту же или более старую version
- **THEN** `prifly update` завершается успешно без network-derived mutation
  установленного binary

### Requirement: Замена binary является atomic и recoverable
Перед заменой updater MUST полностью проверить release manifest, signature,
platform и archive digest, записать replacement на том же filesystem и
атомарно заменить целевой binary только после successful validation. Ошибка,
interruption или incompatible platform MUST сохранять предыдущий исполнимый
binary. Updater MUST NOT обновлять себя автоматически; уже запущенный driver
сохраняет свои загруженные bytes, а subsequent invocation использует новый
binary.

#### Scenario: Скачивание прервано
- **WHEN** сеть или unpack завершается ошибкой до atomic replacement
- **THEN** предыдущая версия остаётся запускаемой, а temporary bytes не
  считаются installation

#### Scenario: Update вызван при активной работе
- **WHEN** другой local workflow driver уже запущен из этой installation
- **THEN** updater атомарно публикует новый binary только для subsequent
  invocations и не меняет bytes уже запущенного driver

## Context

См. [proposal.md](proposal.md). Текущий `scripts/release.py` намеренно создаёт
только local private build и не публикует assets. `prifly` не имеет updater,
а binary из source tree неотличим от произвольной копии. Публичная доставка не
должна добавлять сеть в Run, YAML compiler или authority.

## Goals / Non-Goals

**Goals:**

- Один понятный install command для поддержанной платформы без Go и без root.
- Ручной `prifly update`, который принимает только подписанную поставку
  Pri-Fly и не трогает project state.
- Поставка минимальным числом форматов и без новых runtime dependencies.
- Первый release path, пригодный для дальнейшей публикации GitLab Release.

**Non-Goals:**

- Автообновление, каналы nightly/beta, произвольные download URLs, package
  manager integrations и updater для source build.
- Обновление workflow packages, запусков, authority или configuration.
- Обещание Windows либо любой OS/architecture до отдельной qualified asset.
- Подмена historical release evidence или объявление текущего dev build public
  release.

## Decisions

### Один signed manifest и archive на platform

Release содержит `release-manifest.json`, detached Ed25519 signature и один
`tar.gz` на каждую поддержанную пару OS/architecture. Manifest называет
release version, platform, archive filename, expected binary pathname и SHA-256
archive. Public verification key компилируется в `prifly`; private signing key
находится только в protected GitLab CI variable. Signature проверяется до
скачивания archive, SHA-256 — до unpack.

Ed25519 и SHA-256 уже есть в Go standard library; новых libraries не нужно.
Detached signature сохраняет manifest обычным inspectable JSON. Key rotation
в первой версии требует выпуска binary с новым embedded key: multiple keys и
online key discovery не добавляются без реальной необходимости.

GitLab Release assets имеют stable direct links и latest permalink, поэтому
`latest` выбирает manifest, но manifest/asset остаются versioned в самом
release. Git branch, job artifacts и API token не являются delivery channel.

### Bootstrap является отдельным shell asset с честной первичной границей

README даёт одну command вида `curl … -o temporary-file && sh temporary-file`.
Это намеренно не `curl | sh`: bytes можно увидеть до исполнения. Bootstrap
получает только official GitLab HTTPS Release asset, определяет platform,
скачивает нужный archive и ставит binary в `${HOME}/.local/bin` либо явно
указанный user-writable destination. Он не редактирует PATH или shell profile,
а печатает следующую команду при отсутствующем PATH.

До появления `prifly` bootstrap не может независимо проверить Ed25519
manifest portable shell средствами. Поэтому первая установка честно опирается
на GitLab HTTPS asset; после неё `prifly update` использует compiled key.
Скрипт не получает token, не требует sudo и не запускает project content.
Альтернатива — `curl | sh` — отвергнута как менее inspectable. Полностью
cryptographic bootstrap потребовал бы отдельного trusted verifier или
platform-specific dependency и не нужен для первого minimal path.

### Managed-install receipt отделяет updater от source builds

Bootstrap создаёт небольшой receipt рядом с user installation. В нём только
stable install identity, selected channel и installed asset identity; путь
binary derives from executing binary and receipt, а не от user-supplied URL.
`prifly update` требует valid receipt и matching managed executable. Поэтому
`bin/prifly` в clone, copied file и другие builds получают refusal вместо
неожиданной перезаписи рабочего каталога.

Receipt не является proof of release: trust proof остаётся signed manifest.
Он лишь ограничивает, что updater имеет право заменить. Удаление installation
остаётся обычным удалением user files; uninstall command в этот change не
добавляется.

### Update выполняется в CLI и меняет один файл

`update` обрабатывается до открытия project authority. Оно получает signed
manifest с latest GitLab Release, сравнивает semantic versions без downgrade,
скачивает compatible archive во temporary file на filesystem installation,
проверяет content, extracts only expected `prifly` regular file и делает
atomic rename. Existing process продолжает исполнять already-mapped bytes;
следующий invocation видит replacement. Ошибка до rename удаляет temporary
file best-effort и оставляет старый executable.

Локальная упаковка и CI используют один manifest/archive builder, чтобы tests
не расходились с publish route. Первая supported asset — только platform,
которая проходит named release qualification; bootstrap и updater отказывают
остальным до download. Добавление platform — новый asset plus qualification,
не silent cross-compile claim.

### Publication требует явного tag pipeline

Product gates выполняются на release candidate до создания тега. Owner создаёт
protected semantic-version tag только для уже qualified source tree; tag
pipeline намеренно содержит только manual release jobs и не повторяет
`verify` или `race`. Он создаёт archive/manifest/signature и GitLab Release
asset links. Публикация остаётся manual/owner-controlled, не происходит на
обычном push. Private signing key и отдельный protected masked project access
token с ролью Developer разделены: token нужен только `release-publish` для
GitLab Release API. Если любой credential отсутствует или signing key не
соответствует compiled public key, job fails before release creation.

## Risks / Trade-offs

- [Первая shell-installation доверяет GitLab HTTPS] → README называет это
  явно, bootstrap download остаётся файлом, а subsequent update проверяется
  signature.
- [Private signing key] → хранить только protected masked CI variable;
  тесты используют отдельный test key и не принимают production secret.
- [Publisher token] → отдельный protected masked project token с минимальной
  ролью Developer; revoke/rotate его при подозрении на раскрытие.
- [Release asset отсутствует для platform] → bootstrap/updater сообщает
  unsupported platform, а не ищет source build или другой binary.
- [Interrupted update] → temporary write и atomic rename сохраняют прежний
  binary; test моделирует interruption до rename.
- [Release job стоит CI minutes] → только tag/manual publication, обычные
  branches используют существующие fast gates.

## Migration Plan

1. Добавить builder, signed manifest contract, installer и updater вместе с
   offline fixtures/локальным HTTP test server.
2. Добавить protected release configuration и tag-only publication job, но не
   публиковать release пока owner не назначит production signing key и
   separate publisher token.
3. Проверить first supported platform на named build, затем создать первый
   GitLab Release и обновить README с exact command.
4. Откат выпуска: снять latest tag/release asset и выпустить новый signed
   fixed version. Уже installed binary не обновляется сам; no forced rollback.

## Open Questions

- Owner должен перед первой публичной публикацией создать protected
  `PRIFLY_RELEASE_SIGNING_KEY` и matching `PRIFLY_RELEASE_PUBLIC_KEY`; это
  operational prerequisite, не меняющий выбранный contract.

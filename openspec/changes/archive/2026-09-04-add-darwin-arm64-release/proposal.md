## Why

Публичная поставка сейчас выпускает только `linux/amd64`. Пользователь Apple
Silicon получает корректное сообщение об отсутствии архива, хотя installer и
updater уже умеют выбирать asset по OS и architecture.

Нужен настоящий подписанный `darwin/arm64` binary в том же tagged Release, а
не сборка из исходников на машине пользователя. «Любой Linux/macOS» не должно
быть неточным обещанием: поддержка всегда задаётся точной парой OS/architecture.

## What Changes

- Добавить в public stable Release второй supported platform:
  `darwin/arm64`, сохранив `linux/amd64`.
- Собирать macOS binary нативно на protected Apple Silicon runner, потому что
  текущий SQLite driver требует CGO; не выдавать Linux cross-build за macOS
  qualification.
- Выпускать один подписанный manifest с двумя archive assets и публиковать оба
  в GitLab Release.
- Проверять platform matrix, содержимое manifest, installer и updater без
  fallback на другой binary.
- Обновить публичную документацию точным списком поддержанных платформ.

## Capabilities

### New Capabilities

_Нет._

### Modified Capabilities

- `release-distribution`: public stable Release получает обязательный
  `darwin/arm64` asset наряду с `linux/amd64`; installation и update выбирают
  exact supported platform.

## Impact

- `.gitlab-ci.yml`, release asset builder и его тесты;
- protected Apple Silicon macOS runner в GitLab как внешняя release
  инфраструктура;
- installer/update acceptance tests и `README.md`;
- product runtime API, YAML и existing released binaries не меняются.

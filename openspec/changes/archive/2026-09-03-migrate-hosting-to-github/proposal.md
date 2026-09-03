## Why

Pri-Fly хостится на GitLab, где darwin/arm64 release собирается только на
runner владельца, а публикация зависит от отдельного Maintainer-токена и
Package Registry. GitHub даёт hosted Apple Silicon runners, Releases с
постоянной ссылкой на latest asset и job-scoped publisher credential, а
каталог сценариев и AI Factory packages уже живут на GitHub. Репозиторий
переезжает целиком; GitLab остаётся нетронутым до удаления, а новая история
начинается свежим коммитом в отдельной рабочей копии.

## What Changes

- Module path `gitlab.com/stenhigh/prifly` → `github.com/stenhigh/prifly` во
  всех Go-источниках и `go.mod`; published JSON contracts и schemas не
  меняются.
- `prifly update` и `scripts/install.sh` читают assets с
  `https://github.com/StenHigh/prifly/releases/latest/download/`; формат
  manifest, ed25519-подпись и имена assets прежние.
- `.gitlab-ci.yml` заменяется GitHub Actions: `verify` (branches и PR,
  `make ci-check` + `make e2e`, теги исключены), `race` (manual), `release`
  (tag `vX.Y.Z`: linux/amd64 на ubuntu, darwin/arm64 нативно на hosted
  `macos-14`, затем один job за environment `release` с required reviewer
  подписывает manifest и создаёт GitHub Release через job-scoped
  `GITHUB_TOKEN` с `contents: write`).
- **BREAKING** для процесса: publisher credential больше не отдельный
  долгоживущий token, а `GITHUB_TOKEN` publication job; manual-гейт —
  approve environment; protected tag — ruleset `v*`.
- `test/e2e/verify-release-ci.py` проверяет контракт `release.yml` и
  `verify.yml` вместо `.gitlab-ci.yml`.
- README (бейджи, install-команда, clone URL), SECURITY.md (private
  vulnerability reporting, модель secrets) и authoring references ссылаются
  на GitHub.
- Каталог `prifly-workflows` и `prifly-aif-workflows` (installer URL в CI,
  ссылки) переводятся на GitHub после первого GitHub release.

Runtime, authoring YAML contract, sealed packages и сохранённые Runs не
меняются. Существующие установки `v0.5.0` продолжают смотреть на
GitLab-permalink и не увидят новых версий; переустановка новой командой —
единственный путь, bridge-release на GitLab не делается.

## Capabilities

### New Capabilities

_Нет._

### Modified Capabilities

- `release-distribution`: поставка через GitHub Release, environment-gated
  publication с job-scoped credential, native darwin qualification на
  GitHub-hosted Apple Silicon runner.

## Impact

Изменяются `go.mod`, import-пути всех Go-файлов, `internal/release/release.go`
(base URL), `scripts/install.sh`, `.github/workflows/*`, удаляется
`.gitlab-ci.yml`, обновляются `test/e2e/verify-release-ci.py`, README,
SECURITY.md, `examples/authoring/*` и OpenSpec spec `release-distribution`.
Требуются действия владельца на GitHub: создать репозиторий `StenHigh/prifly`,
secret `PRIFLY_RELEASE_SIGNING_KEY` в environment `release` с required
reviewer, variable `PRIFLY_RELEASE_PUBLIC_KEY`, ruleset на теги `v*`. Активный
change `harden-authority-reliability-and-performance` меняет те же требования
release-distribution и должен быть перебазирован на новый текст.

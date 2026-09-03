## 1. Отвязка кода

- [x] 1.1 Переименовать module path в `github.com/stenhigh/prifly` (`go.mod`, все `*.go`), перевести `DefaultReleaseBaseURL` и `scripts/install.sh` на `https://github.com/StenHigh/prifly/releases/latest/download`, обновить authoring references; проверить `go build ./...`, `go vet ./...`, `gofmt -l` и `rg gitlab` вне `openspec/` и `CONTEXT_STATE.md`.

## 2. GitHub Actions

- [x] 2.1 Добавить `.github/workflows/verify.yml` (branches/PR, `make ci-check` + `make e2e`, `tags-ignore`), `race.yml` (workflow_dispatch) и `release.yml` (tag `vX.Y.Z`, linux на ubuntu, darwin нативно на `macos-14`, publication job за environment `release` с `contents: write`, `gh release create --verify-tag`); удалить `.gitlab-ci.yml`; проверить `python3 -B test/e2e/verify-release-ci.py` с новым контрактом.

## 3. Документация и spec

- [x] 3.1 Обновить README (бейджи, install-команда, clone URL), SECURITY.md (private vulnerability reporting, модель secrets и `GITHUB_TOKEN`) и delta `release-distribution`; проверить `openspec validate migrate-hosting-to-github --strict`.
- [ ] 3.2 Прогнать `make ci-check`, `make e2e` и `make race` в рабочей копии `Pri-Fly/github`; проверить `git diff --check` и что published contracts и archive не изменены.

## 4. GitHub (после подтверждения владельца)

- [x] 4.1 Свежий initial commit в `Pri-Fly/github`, создать публичный `StenHigh/prifly`, push `main`; проверить `gh repo view` и зелёный `verify` (`8bb73d0` + fix fixture `project-launch`, run `33758316245` success).
- [ ] 4.2 Настроить environment `release` с required reviewer, variable `PRIFLY_RELEASE_PUBLIC_KEY`, ruleset на теги `v*`; owner добавляет secret `PRIFLY_RELEASE_SIGNING_KEY`; проверить через `gh api`.
- [ ] 4.3 Tag `v0.6.0`, approve publication, проверить installer с GitHub на darwin/arm64 и linux/amd64 (`prifly version`, `prifly update` = up to date).
- [ ] 4.4 Перевести `prifly-workflows` и `prifly-aif-workflows` (README-ссылки, installer URL в CI) на GitHub; проверить зелёный CI AIF-репозитория.
- [ ] 4.5 Архивировать GitLab-проект с README-указателем на GitHub; обновить `CONTEXT_STATE.md` и память.

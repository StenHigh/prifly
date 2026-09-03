## 1. Отвязка кода

- [x] 1.1 Переименовать module path в `github.com/stenhigh/prifly` (`go.mod`, все `*.go`), перевести `DefaultReleaseBaseURL` и `scripts/install.sh` на `https://github.com/StenHigh/prifly/releases/latest/download`, обновить authoring references; проверить `go build ./...`, `go vet ./...`, `gofmt -l` и `rg gitlab` вне `openspec/` и `CONTEXT_STATE.md`.

## 2. GitHub Actions

- [x] 2.1 Добавить `.github/workflows/verify.yml` (branches/PR, `make ci-check` + `make e2e`, `tags-ignore`), `race.yml` (workflow_dispatch) и `release.yml` (tag `vX.Y.Z`, linux на ubuntu, darwin нативно на `macos-14`, publication job за environment `release` с `contents: write`, `gh release create --verify-tag`); удалить `.gitlab-ci.yml`; проверить `python3 -B test/e2e/verify-release-ci.py` с новым контрактом.

## 3. Документация и spec

- [x] 3.1 Обновить README (бейджи, install-команда, clone URL), SECURITY.md (private vulnerability reporting, модель secrets и `GITHUB_TOKEN`) и delta `release-distribution`; проверить `openspec validate migrate-hosting-to-github --strict`.
- [x] 3.2 Прогнать `make ci-check`, `make e2e` и `make race` в рабочей копии `Pri-Fly/github`; проверить `git diff --check` и что published contracts и archive не изменены (ci-check и e2e локально зелёные; полный race локально упирался в нагрузку машины — `schema_timeout`, `database is locked`, 30-минутный лимит, точечный прогон упавших тестов `ok`; полный `race.yml` на GitHub-runner run `33764104892` success за 19 мин).

## 4. GitHub (после подтверждения владельца)

- [x] 4.1 Свежий initial commit в `Pri-Fly/github`, создать публичный `StenHigh/prifly`, push `main`; проверить `gh repo view` и зелёный `verify` (`8bb73d0` + fix fixture `project-launch`, run `33758316245` success).
- [x] 4.2 Настроить environment `release` с required reviewer, variable `PRIFLY_RELEASE_PUBLIC_KEY`, ruleset на теги `v*`; owner добавляет secret `PRIFLY_RELEASE_SIGNING_KEY`; проверить через `gh api` (сделано: environment `release` с reviewer StenHigh, variable `521250b2…35b4`, восстановленный по подписи манифеста v0.5.0, ruleset `release tags` id 22194431; ждёт secret владельца).
- [x] 4.3 Tag `v0.6.0`, approve publication, проверить installer с GitHub на darwin/arm64 и linux/amd64 (`prifly version`, `prifly update` = up to date) (run `33769043603` success после approve владельца; installer с `releases/latest/download` дал `0.6.0` на этом Mac и в контейнере ubuntu:24.04 linux/amd64, `prifly update` проверил signed manifest: `updated:false`).
- [x] 4.4 Перевести `prifly-workflows` и `prifly-aif-workflows` (README-ссылки, installer URL в CI) на GitHub; проверить зелёный CI AIF-репозитория (каталог `f19aa70`, AIF `fdea851`, AIF verify run `33770025084` success с Pri-Fly 0.6.0 из GitHub).
- [x] 4.5 Архивировать GitLab-проект с README-указателем на GitHub; обновить `CONTEXT_STATE.md` и память (GitLab `main` `117e2ff` с указателем, проект `stenhigh/prifly` архивирован read-only, релизы `v0.2.0`–`v0.5.0` остаются доступны).

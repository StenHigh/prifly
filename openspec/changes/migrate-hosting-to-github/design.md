## Context

См. `proposal.md`. GitLab-привязка сидит в пяти местах: константа base URL
updater-а и installer, `.gitlab-ci.yml` (darwin на self-hosted runner
владельца, publish через `glab` с Maintainer-токеном), тест контракта CI,
спека `release-distribution` и документация. Формат release (manifest +
ed25519 signature + два tar.gz + install.sh) от хостинга не зависит.

## Goals / Non-Goals

**Goals:**

- Полная отвязка от GitLab в отдельной рабочей копии до первого push на
  GitHub; GitLab-репозиторий не меняется до удаления.
- Та же модель безопасности релиза: подпись отдельным ключом, manual-гейт
  публикации, protected tags, native darwin qualification.
- Никаких изменений runtime, published contracts и сохранённых Runs.

**Non-Goals:**

- Bridge-release на GitLab для существующих установок.
- Перенос issues, MR и CI-истории GitLab.
- Изменение формата release assets или updater protocol.

## Decisions

### Отдельная рабочая копия и свежая история

Копия `HEAD` (`git archive`) в `Pri-Fly/github`, `git init`, toolchain по
symlink `.tools`. Все правки, гейты и OpenSpec change выполняются там; первый
push — один свежий коммит. Так GitLab остаётся рабочим до удаления, а история
GitHub не наследует GitLab-специфичные коммиты.

### Module path `github.com/stenhigh/prifly`

Строчный путь, как принято в Go; GitHub резолвит регистр owner-а. Замена
механическая (`sed` по `*.go` и `go.mod`), ldflags в workflows используют
новый путь. Published schemas и `$id` не содержат module path, поэтому
`schemas-check` не меняется.

### Release workflow: hosted macOS, environment как manual-гейт

`build-darwin-arm64` на `macos-14` повторяет квалификацию GitLab-job
(`uname`, `version`, `init`, `doctor`); `build-linux-amd64` на ubuntu.
Publication job получает `contents: write` и signing secret только за
environment `release` с required reviewer — это эквивалент `when: manual`
плюс раздельных credentials: build jobs идут с `contents: read` и без secret.
`gh release create --verify-tag` публикует assets в GitHub Release, а
`releases/latest/download/<asset>` даёт updater-у постоянную ссылку.
Альтернатива — PAT владельца как publisher token — отклонена: `GITHUB_TOKEN`
job-scoped и не требует ротации.

### Контракт CI остаётся тестом

`verify-release-ci.py` проверяет строки `release.yml`/`verify.yml`: hosted
arm64 runner, native build, оба asset, единственный `contents: write`,
environment, `--verify-tag`, отсутствие secret в build jobs, `tags-ignore` в
verify. YAML-парсер не добавляется.

### Существующие установки

Binary `v0.5.0` содержит GitLab base URL; после переезда `prifly update` там
видит прежнюю версию и честно сообщает «обновлений нет». Документация
называет переустановку новой командой; GitLab-релизы остаются как есть до
удаления проекта.

## Risks / Trade-offs

- [Hosted macOS runner недоступен или платный] → для public repository
  бесплатен; при недоступности release workflow падает без частичного
  manifest.
- [Ошибка в secrets/variables при первом релизе] → job проверяет их до
  сборки assets; owner исправляет и перезапускает workflow.
- [`harden-authority-reliability-and-performance` меняет те же требования]
  → его delta перебазируется на новый текст release-distribution.
- [Ссылки каталога и AIF-репозитория на GitLab] → обновляются после первого
  GitHub release одним коммитом в каждом.

## Migration Plan

1. Код, workflows, тест контракта, документация, spec sync — в рабочей копии;
   `make ci-check`, `make e2e`, `openspec validate --strict`.
2. Свежий коммит, создание `StenHigh/prifly`, push `main`.
3. Owner: environment `release` с required reviewer, secret
   `PRIFLY_RELEASE_SIGNING_KEY`, variable `PRIFLY_RELEASE_PUBLIC_KEY`,
   ruleset на теги `v*`; часть можно сделать через `gh api`.
4. Tag `v0.6.0` → release workflow → approve → проверка installer с GitHub на
   обеих платформах.
5. Перевести каталог и AIF-репозиторий на GitHub-ссылки; архивировать
   GitLab-проект с README-указателем.

Rollback до шага 2 — удалить рабочую копию; GitLab не затронут.

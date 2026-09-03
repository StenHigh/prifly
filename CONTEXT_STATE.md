# CONTEXT_STATE

Рабочее состояние репозитория для следующей сессии. Нормативная правда —
в `openspec/` (см. `openspec/SOURCE-OF-TRUTH.md`); этот файл только
ориентирует.

Обновлено: 2026-09-03. Это GitHub-копия Pri-Fly (`StenHigh/prifly`),
начатая свежей историей из GitLab-дерева `main` = `27fa58c`. GitLab-проект
остаётся нетронутым до удаления.

## Переезд на GitHub (change `migrate-hosting-to-github`)

- Module path `github.com/stenhigh/prifly`; updater и `scripts/install.sh`
  читают `https://github.com/StenHigh/prifly/releases/latest/download/`.
- CI — GitHub Actions: `verify.yml` (branches/PR, теги исключены),
  `race.yml` (manual), `release.yml` (tag `vX.Y.Z`; darwin/arm64 нативно на
  hosted `macos-14`; publication job за environment `release` с
  `contents: write`; `gh release create --verify-tag`).
  `test/e2e/verify-release-ci.py` проверяет этот контракт.
- Spec `release-distribution` синхронизирована с delta change; README и
  SECURITY.md указывают на GitHub.
- Репозиторий `StenHigh/prifly` создан (initial `8bb73d0`), `verify` и
  manual `race` на GitHub-runner зелёные. Настроены environment `release`
  (required reviewer StenHigh), variable `PRIFLY_RELEASE_PUBLIC_KEY`
  (восстановлен проверкой подписи манифеста v0.5.0), ruleset `release tags`
  (`v*`, bypass только admin).
- Первый релиз с GitHub: tag `v0.6.0`, run `33769043603` после approve
  владельца; installer с `releases/latest/download` проверен на darwin/arm64
  и linux/amd64, `prifly update` читает signed manifest с GitHub. Каталог и
  `prifly-aif-workflows` (README, installer URL в CI) переведены на GitHub.
- Ещё не сделано: архивирование/удаление GitLab-проекта — решение владельца
  (task 4.5). Существующие установки `v0.5.0` смотрят на GitLab и обновлений
  не увидят — переустановка новой командой. Рабочая копия для дальнейшей
  работы — `Pri-Fly/github`; `Pri-Fly/isolated` (GitLab) заморожена.
- Полный `make race` на этой машине под нагрузкой (load ≈ 8) даёт
  `schema_timeout`/`database is locked`; доверять GitHub `race.yml`.

## Открытые changes (`openspec list`)

- `migrate-hosting-to-github` — задачи 4.x ждут действий владельца.
- `harden-authority-reliability-and-performance` — 0/51; его delta
  release-distribution написана под GitLab-текст, нужен rebase.
- `add-run-decision-catalog` — 18/20; открыты 4.2 (ручное наблюдение в
  Codex и Claude Code) и 6.3 (bounded live pilot).
- `add-native-host-question-ux` — 6/7 (task 2.3 — ручное наблюдение UI).
- Завершённые, но не заархивированные: `consolidate-current-delivery-backlog`,
  `add-darwin-arm64-release`, `add-project-workspace-mode`,
  `fix-protected-release-publication`, `add-project-workflow-options`,
  `add-release-installer-and-update` — перед удалением любого их требования
  архивировать без sync.

## Нюансы

- Гейты запускать по абсолютному пути: `make -C /Users/sh/PhpstormProjects/Pri-Fly/github …`;
  `.tools` — symlink на toolchain соседней GitLab-копии.
- Параллельный запуск `make ci-check` и `make race` на холодном кэше даёт
  ложный `schema_timeout` в `internal/runtime`; гонять последовательно.
- `aif-classic/workflow.yaml` (внешний repo) ссылается на
  `.prifly/workflows/aif-classic/…`; staged copy валидируется под финальным
  именем папки.
- macOS `rename` не заменяет пустой каталог; swap в `update` резервирует имя.
- `git ls-remote URL tag` возвращает tag object; peel через паттерн `tag^{}`.
- Тот же `id@version` с другими bytes = `project_start_package_identity_conflict`.
- Таблица glossary bindings принимает только `internal/` источники.
- `gh` account StenHigh (scope `workflow` есть); SSH-ключ — `dzianis-87`,
  push StenHigh-репозиториев по HTTPS с `!gh auth git-credential`.

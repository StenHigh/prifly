# CONTEXT_STATE

Рабочее состояние репозитория для следующей сессии. Нормативная правда —
в `openspec/` (см. `openspec/SOURCE-OF-TRUTH.md`); этот файл только
ориентирует.

Обновлено: 2026-09-03. Это рабочая копия Pri-Fly (`StenHigh/prifly`),
начатая свежей историей из GitLab-дерева `main` = `27fa58c`. GitLab-проект
`stenhigh/prifly` архивирован (read-only, README указывает на GitHub).

## Переезд на GitHub (change `migrate-hosting-to-github`, заархивирован)

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
- GitLab-проект архивирован (его `main` `117e2ff` с указателем на GitHub);
  релизы `v0.2.0`–`v0.5.0` там остаются доступны. Существующие установки
  `v0.5.0` смотрят на GitLab и обновлений не увидят — переустановка новой
  командой. Рабочая копия — `Pri-Fly/github`; `Pri-Fly/isolated` —
  замороженный GitLab-клон, нужен только как источник `.tools`.
- Полный `make race` на этой машине под нагрузкой (load ≈ 8) даёт
  `schema_timeout`/`database is locked`; доверять GitHub `race.yml`.

## Открытые changes (`openspec list`)

- `harden-authority-reliability-and-performance` — 0/51; delta
  release-distribution перебазирована на GitHub-текст spec.
- `add-run-decision-catalog` — 18/20; открыты 4.2 (ручное наблюдение в
  Codex и Claude Code) и 6.3 (bounded live pilot).
- `add-native-host-question-ux` — 6/7 (task 2.3 — ручное наблюдение UI).
- Завершённые, но не заархивированные: `consolidate-current-delivery-backlog`,
  `add-darwin-arm64-release`, `add-project-workspace-mode`,
  `fix-protected-release-publication`, `add-project-workflow-options`,
  `add-release-installer-and-update` — перед удалением любого их требования
  архивировать без sync.

## Следующие шаги

- Релиз `v0.7.0` (2026-09-03) выпущен из GitHub Actions на коммите `d19d537`:
  именованные отказы приёма отчёта, предпроверка до записи candidate, захват
  деревьев по объявленным bindings. Change `fix-assisted-submit-diagnostics`
  заархивирован, spec `cli-protocol` синхронизирована. Обе pilot-сессии
  оповещены.
- Следующий change — `improve-cli-discoverability` по очереди ниже.
- `harden-authority-reliability-and-performance` готов к
  `/openspec-apply-change` (0/51; delta `release-distribution` перебазирована
  на GitHub-текст, `openspec validate --strict` зелёный).
- Ручные наблюдения владельца: `add-run-decision-catalog` 4.2 и 6.3,
  `add-native-host-question-ux` 2.3; после них — архив с sync.
- Установки `v0.5.0` не обновятся сами: переустановка командой из README;
  GitLab-проект владелец скоро удалит, заметок о переходе не делать.
- Очередь после `fix-assisted-submit-diagnostics` (по отчётам pilot-сессий,
  2026-09-03): `improve-cli-discoverability` — `prifly schema` без аргумента
  выдаёт список имён, `submission_schema_ref` в SessionTask, алиасы
  `core:schema/...`; `authority_not_found` вместо `not_found` при пустом
  `--project`; `session task` без удерживаемой передачи отвечает отдельным
  кодом (`no_active_handoff`) с `run.explain`/`run.drive`, а не `not_found`;
  `--help`/`-h` на подкомандах, `--version`, `help <topic>`; `invalid_usage`
  показывает полученное значение (`received: …`, пути с пробелами);
  `project local set --executable`; `prifly schema` печатает список имён;
  описание пары `result_schema_ref.id` против `schema_version: const "1"`;
  `description` для
  `WorkspaceTreeLocation.path` и `SessionSubmission.result` в generated
  schemas; именованные каталоги вместо `context/skills/{00,01}`; не создавать
  пустой `outputs/` в claim; `run status` считает выходы Run и шагов
  раздельно; `waiting_host` через versioned bump `CoreNextView` (enum уже без
  `waiting_decision`); `prifly update` печатает адрес проверенного manifest.
  Затем `pin-skill-reference-trees`: источник контекста-каталог, закрепляемый
  как tree manifest, плюс отказ при запечатывании навыка с неразрешимой
  ссылкой. Обходной путь до него: каждый `references/*.md` объявляется
  отдельным `context_refs` (список).
- Ответы пилотам 2026-09-03: состояние совместимо между релизами (v0.7.0 не
  менял ни одну опубликованную схему, ci-check сверяет байты); `external_write`
  отказывает как граница профиля (`start.go` и контракт assisted-шага), P2-09
  не имеет горизонта — её предпосылка P2-08 тоже не принята; ActionIntent и
  ActionAdmission существуют, доставки нет, и класс эффекта шага обязан
  совпадать с намерением, поэтому промежуточной формы публикации сегодня нет;
  подтверждение человеком делается через Decision Bridge с объявленным
  runtime-решением, отдельного порта вопросов не будет.
- Для бэклога `prifly-aif-workflows`: шаги 5-6 `aif-improve` должны объявить
  runtime-решение и вызывать мост; `references/**` закрепить отдельными
  `context_refs`.
- В шаблон runner-навыка добавить: `decision_bridge: true` — способность
  поднять запрос, а не открытый вопрос.

## Нюансы

- GitLab `stenhigh/prifly` (id 85838592) архивирован: push туда невозможен,
  `glab` остаётся залогинен (unarchive — `glab api -X POST
  projects/85838592/unarchive`). В `Pri-Fly/isolated` ничего не менять.

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

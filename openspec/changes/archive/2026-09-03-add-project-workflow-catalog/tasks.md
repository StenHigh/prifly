## 1. Contract и словарь

- [x] 1.1 Добавить в `terms.md` термины Workflow repository, Workflow catalog и Workflow folder origin с границами относительно Registry, SealedPackage, TrustDecision и SourceSnapshot и Go-именами в тексте терминов (таблица bindings принимает только `internal/` источники, а типы живут в `cmd/prifly`); проверить `go test ./internal/runtime -run TestGlossaryBindings`.
- [x] 1.2 Расширить reader `project.yaml` необязательным `packages.NAME.origin` с закрытым списком полей и валидацией `commit`/`digest`; обновить `schemas/authoring/project-profile-v2.schema.json` и `project-profile-authoring-reference.yaml`; проверить accepted/rejected origin в `go test ./cmd/prifly` и editor contract в `python3 -B test/e2e/test_examples.py`.
- [x] 1.3 Добавить `schemas/authoring/workflow-catalog-v1.schema.json`, запись в `manifest.json`/README и `examples/authoring/workflow-catalog-authoring-reference.yaml`; проверить `python3 -B test/e2e/test_examples.py`.

## 2. Установка из репозитория

- [x] 2.1 Реализовать git helper для project-команд: typed argv, `--` перед URL, ограниченный env, `GIT_ALLOW_PROTOCOL=https:ssh:file`, timeouts; разбор `SOURCE` с отказом до сети для относительного пути, userinfo, ведущего `-` и `ext::`; проверить негативные случаи в `go test ./cmd/prifly` без сети.
- [x] 2.2 Реализовать shallow fetch по ref/commit во временный каталог, разрешение default branch и discovery папок по marker с bounded depth; проверить одну/несколько/ноль папок и `--path` на локальном bare-репозитории в `go test ./cmd/prifly`.
- [x] 2.3 Реализовать staged copy (`Lstat`, только regular files, лимиты, отказ symlink/gitlink), структурную проверку `readProjectWorkflowFolder` в temp-root с финальным именем папки (folder-relative пути decision catalog), digest дерева без `extend.yaml` и atomic rename в `.prifly/workflows/NAME/`; проверить отказы и отсутствие частичной папки в `go test ./cmd/prifly`.
- [x] 2.4 Реализовать правку `project.yaml` через `yaml.v3` Node API (packages + origin + launch, temp + rename, откат папки при ошибке) и отказы `project_workflow_exists`/`project_workflow_package_conflict`; проверить сохранение комментариев и init-template в `go test ./cmd/prifly`.
- [x] 2.5 Собрать команду `project workflows add SOURCE` с результатом `project-workflow-add/1` (package identity, references, origin, launch, next steps) и help; проверить полный сценарий add → `project workflows` → `project compile` в `go test ./cmd/prifly`.

## 3. Каталог

- [x] 3.1 Реализовать строгий parser `catalog.yaml` (`prifly-workflow-catalog/1`, лимиты, имена, категории, относительный repository → отказ); проверить accepted/rejected документы в `go test ./cmd/prifly`.
- [x] 3.2 Реализовать `project workflows search [QUERY] [--category] [--catalog]` с детерминированным `project-workflow-catalog/1` и `add NAME` через запись каталога с проверкой pinned `commit`; встроить default `https://github.com/StenHigh/prifly-workflows.git` с переопределением `--catalog`; проверить фильтры, default/override, `project_workflow_catalog_entry_unknown` и `project_workflow_commit_mismatch` на локальном каталоге в `go test ./cmd/prifly`.

## 4. Lifecycle

- [x] 4.1 Реализовать `project workflows update NAME [--ref]`: origin обязателен, drift по digest → `project_workflow_modified` со списком путей, `ls-remote` read-only success, перенос `extend.yaml`, atomic swap, обновление origin, флаги `extend_upstream_changed` и `package_version_unchanged`; проверить каждый исход в `go test ./cmd/prifly`.
- [x] 4.2 Реализовать `project workflows remove NAME` с удалением папки, package и её launches без обращения к authority; проверить в `go test ./cmd/prifly`.

## 5. Host UX и документация

- [x] 5.1 Добавить в runner `prifly-run` раздел поиска и установки одним native вопросом, включить прежний exact runner в accepted-previous для `project runners update`; проверить init, clone-check и update в `go test ./cmd/prifly`.
- [x] 5.2 Обновить `README.md`, `examples/README.md` и help CLI описанием репозиториев сценариев, каталога, origin и границы trust; проверить `prifly help` и ссылки документов.

## 6. Приёмка

- [x] 6.1 Добавить в `test/e2e/verify-authoring.py` offline-случаи: `add` из локального Git-репозитория → `project workflows` показывает launch → `project compile` проходит; `search` и `add NAME` по локальному каталогу, `update` read-only, `remove`; проверить `make e2e`.
- [x] 6.2 Выполнить `make check`, `make e2e`, `openspec validate add-project-workflow-catalog --strict` и `git diff --check`; отдельно убедиться, что `openspec/changes/archive/`, published contracts, sealed `PackageOrigin` и historical evidence не изменены.

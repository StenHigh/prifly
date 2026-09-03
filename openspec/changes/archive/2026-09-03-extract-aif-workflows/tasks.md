## 1. Внешний repository

- [x] 1.1 Создать публичный `StenHigh/prifly-aif-workflows` с `aif-classic/`, `aif-fanout/`, README, `tests/test_folders.py`, `tests/verify.py` и GitHub Actions; проверить `python3 -B tests/test_folders.py` и `python3 -B tests/verify.py --binary <released prifly v0.5.0>` локально и `gh repo view`.
- [x] 1.2 Опубликовать tag `v1.0.0` (commit `6543137`); проверить `git ls-remote --tags` и что CI repository прошёл на `main` (`c650872`, те же папки плюс workflow file; run `33745012530` success).

## 2. Каталог

- [x] 2.1 Перевести записи `aif-classic` и `aif-fanout` в `StenHigh/prifly-workflows` на новый repository, `ref: v1.0.0` и pinned commit; проверить `prifly project workflows search` и `add aif-classic` во временном проекте с текущей сборкой.

## 3. Pri-Fly

- [x] 3.1 Архивировать завершённые `align-aif-classic-and-fanout` и `add-workspace-artifact-bindings` без sync (их delta уже в main); проверить `openspec list` и неизменность main specs (`git diff --stat openspec/specs` пуст до применения этого change).
- [x] 3.2 Удалить `examples/aif-classic`, `examples/aif-fanout`, `TestCLIProjectCompileSealsAIFWorkflowFolders` и `AIFWorkflowFolderTest`; добавить нейтральный тест выбора `--package-profile` и отказа неизвестного profile; проверить `go test ./cmd/prifly` и `python3 -B test/e2e/test_examples.py`.
- [x] 3.3 Обновить `examples/README.md`, `test/README.md` и `README.md`: ссылки на каталог и внешний repository вместо локальных папок; проверить `rg -n 'aif-classic|aif-fanout' examples test/README.md README.md` — остаются только упоминания каталога и внешнего repository.

## 4. Приёмка

- [x] 4.1 Выполнить `make check`, `make e2e`, `openspec validate extract-aif-workflows --strict` и `git diff --check`; отдельно убедиться, что `openspec/changes/archive/`, published contracts и historical evidence не изменены, а удалённые требования не вернулись в main specs.

## Context

См. `proposal.md` — Why. Факты, которые определяют подход:

- `examples/aif-classic` и `examples/aif-fanout` в HEAD байт-в-байт совпадают
  с tag `v0.5.0` Pri-Fly (latest stable release), поэтому внешний repository
  может проверяться против опубликованного binary без сборки из исходников.
- `aif-classic/workflow.yaml` называет свои decision files repository-relative
  путями `.prifly/workflows/aif-classic/decisions/...`: имя папки в repository
  и имя установки обязаны совпадать.
- `TestCLIProjectCompileSealsAIFWorkflowFolders` — единственный тест Pri-Fly,
  который проверяет `project compile --package-profile` и отказ
  `project_compile_unknown_profile`; остальные его проверки (tree bindings,
  decision catalog, exclude/settings, questionnaire) уже покрыты нейтральными
  тестами `internal/flow`, `internal/runtime` и `cmd/prifly`.
- Changes `align-aif-classic-and-fanout` и `add-workspace-artifact-bindings`
  завершены (все tasks закрыты, их delta уже в main specs), но не
  архивированы. Их delta ADD-ят те же требования, которые этот change
  удаляет: поздний archive с sync вернул бы их.

## Goals / Non-Goals

**Goals:**

- Pri-Fly содержит только нейтральные authoring references и fixtures; AI
  Factory packages развиваются в `StenHigh/prifly-aif-workflows`.
- Новый repository устанавливается через каталог без правок: папки в корне,
  `path: aif-classic`, tag `v1.0.0` с pinned commit.
- Проверки package переезжают вместе с ним и выполняются против released
  Pri-Fly; engine-покрытие Pri-Fly не уменьшается.

**Non-Goals:**

- Не менять содержимое самих папок AIF, их версии или decision inventory.
- Не переносить engine-требования (tree bindings, run decisions) во внешний
  repository.
- Не переписывать archived changes и historical evidence.

## Decisions

### Layout внешнего repository

`aif-classic/` и `aif-fanout/` лежат в корне, README описывает установку через
каталог и вручную для Pri-Fly `v0.5.0`, требования к host skills, правило
версий и backlog, перенесённый из roadmap. Альтернатива `workflows/aif-*`
отклонена: лишний уровень без пользы, а `path` каталога всё равно явный.

### Проверки — black-box через released Pri-Fly

`tests/test_folders.py` переносит статический контракт из
`test_examples.py`; `tests/verify.py` переносит проверки Go-теста как
black-box сценарий: временный Git-репозиторий, `project init`, копия папок,
stub skills для трёх host roots, questionnaire, sealed decision catalog,
профили Fast/Full/Ultra, `exclude`/`settings`, порядок classic route, read-only
gates, parallel fan-out, неизменность `extend.yaml` и authority. GitHub Actions
ставит Pri-Fly официальным installer (`PRIFLY_INSTALL_DIR` не нужен, default
`~/.local/bin`) и запускает оба скрипта. Проверено локально против `v0.5.0`.
Альтернатива — сборка Pri-Fly из исходников в CI — отклонена: repository
должен подтверждать совместимость с тем, что реально ставят пользователи.

### Версии и каталог

Tag repository `v1.0.0` соответствует `package.version: 1.0.0` обеих папок.
Каталог указывает `ref: v1.0.0` и `commit` этого tag; README требует bump
`package.version` при любом изменении папки, потому что тот же `id@version` с
другими bytes — identity conflict при `project start`.

### Pri-Fly: удаление и нейтральное покрытие

Удаляются папки, Go-тест и Python-класс; добавляется нейтральный
`TestCLIProjectCompileSelectsDeclaredPackageProfile`: fixture с двумя
profiles компилируется с `--package-profile`, неизвестный profile получает
`project_compile_unknown_profile`, `extend.yaml` остаётся неизменным.
Документация (`examples/README.md`, `test/README.md`, README) ссылается на
каталог и внешний repository.

### Порядок архивации

Сначала архивируются `align-aif-classic-and-fanout` и
`add-workspace-artifact-bindings` без sync (их delta уже в main), затем
применяется этот change, затем архивируется он сам с sync. Так удалённые
требования не возвращаются.

## Risks / Trade-offs

- [CI внешнего repository зависит от доступности GitLab Release] → installer
  тот же, что у пользователей; падение CI честно показывает недоступность.
- [Новый authoring contract Pri-Fly ломает внешний package] → README
  внешнего repository требует проверку против latest release и bump версии;
  Pri-Fly больше не обещает совместимость чужого package.
- [Open tasks `add-run-decision-catalog` 5.2 и 6.3 ссылаются на `aif-plan` и
  live pilot] → они используют внешний repository как fixture; текст tasks
  не меняется этим change.
- [`examples/README.md` терял описание установки AIF] → его заменяет ссылка
  на каталог и repository.

## Migration Plan

1. Создать repository, push, tag `v1.0.0`; проверить `tests/verify.py`
   против released `v0.5.0`.
2. Обновить `catalog.yaml` и README каталога, push; проверить `search` и
   `add aif-classic` во временном проекте.
3. Архивировать два завершённых AIF changes без sync.
4. Удалить папки и тесты, добавить нейтральный тест, обновить документацию и
   specs; `make check`, `make e2e`.
5. Архивировать этот change с sync; закоммитить.

Rollback: `git revert` возвращает папки и тесты; внешний repository остаётся
самостоятельным и каталог может указывать на любой из источников.

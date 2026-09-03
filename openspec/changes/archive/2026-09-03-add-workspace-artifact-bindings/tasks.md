## 1. Версионированный tree contract

- [x] 1.1 Добавить StepDefinition v5 и typed declared Workspace tree binding с sealed WorkspaceTreeManifest в flow model, protocol schemas и compiler validation; проверить focused `go test ./internal/flow -run 'WorkspaceTree|Authoring|Protocol' -count=1` с valid Fast/Full/Ultra, duplicate, JSON, path, policy и effect negative cases.
- [x] 1.2 Добавить YAML lowering и local editor schema/reference для explicit `workspace_trees`, сохранив v1–v4 definitions; проверить `go test ./internal/flow -run 'Authoring|WorkspaceTree' -count=1` и public `project compile` positive/negative corpus.

## 2. Runtime и assisted host

- [x] 2.1 Ввести следующую Core state/read capability и `assisted-session/4` SessionTask/SessionSubmission schemas с typed tree bindings, output-only capture location, legacy reader guards и capability inventory; проверить `go test ./internal/runtime -run 'Session|Compatibility|Workspace' -count=1` и generated-schema validation для old/new DTO.
- [x] 2.2 Реализовать confined tree materialization и atomic output capture в claimed RepositoryWorkspace до normal result intake; проверить focused runtime tests для Fast/Full/Ultra byte-for-byte provenance, missing entry, pre-handoff drift, traversal/symlink rejection, capture-policy escape и unchanged legacy assisted handoff.

## 3. Канонический AI Factory пример

- [x] 3.1 Перевести `examples/aif-classic` с JSON `summary/tasks` schema на declared native plan manifest; plan создаёт output-only plan tree, improve и implement используют input/output tree, а implement возвращает final checked-plan manifest. Проверить compile fixtures и тест, где second improve и implement получают exact prior captured native tree, включая Ultra `index.md` и phase files.
- [x] 3.2 Добавить package-level default-layout/profile compatibility check и честное README guidance для Fast, Full и Ultra; проверить three default happy paths и explicit refusal для non-default AI Factory paths, не добавляя AI Factory dependency в Core.

## 4. Документация, проверка и границы

- [x] 4.1 Обновить `terms.md`, Go/JSON glossary bindings, authoring references и current delivery backlog; проверить `TestGlossaryBindings`, `openspec validate add-workspace-artifact-bindings --strict` и что backlog отделяет manifest bridge от live pilot qualification.
- [x] 4.2 Выполнить targeted Go, authoring corpus, `git diff --check` и один final `make check`; убедиться, что historical release evidence и `openspec/changes/archive/` не изменены, а результат записать в change tasks без заявления о закрытии product gate.

## Проверка

2026-09-03: focused flow/runtime/CLI tests, generated-schema check, strict change
and main-spec validation, `git diff --check` and one final `make check` passed.
Historical release evidence and `openspec/changes/archive/` were not changed.
This completes the manifest bridge change, not live-pilot qualification or a
product release gate.

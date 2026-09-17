## 1. Контракт шага

- [ ] 1.1 `internal/flow/types.go`, `schema.go`: StepDefinition v8 — binding без `output_port` допустим только в v8; embedded schema v8 требует `capture` и хотя бы один из `input_port`/`output_port`. Проверка: новый тест в `internal/flow` — v7 с binding без `output_port` отказан, v8 принят; `go test ./internal/flow -run WorkspaceTree`.
- [ ] 1.2 `internal/flow/compile.go` `checkWorkspaceTrees`: materialize-only binding только при `effects.class: none`, binding с `output_port` только при `workspace_write`; сообщение отказа называет обе стороны. Проверка: тест на оба отказа и на принятую пару; `go test ./internal/flow -run WorkspaceTree`.
- [ ] 1.3 `internal/flow/authoring.go` + `schemas/authoring/step-v2.schema.json` + `examples/authoring/step-authoring-reference.yaml`: `prifly-step/1` lower-ит форму без `output_port` в v8. Проверка: `internal/flow/authoring_test.go` — YAML read-only шага с `workspace_trees: [{input_port, capture}]` даёт StepDefinition v8; `go test ./internal/flow -run Authoring`.
- [ ] 1.4 `cmd/schema-gen`, `schemas/core/step-definition-v8.schema.json`, `scripts/check-schema.py`: новый bundle; v5–v7 байт в байт прежние. Проверка: `make schemas && make schemas-check` печатает совпадение всех bundle'ов, `git diff --stat schemas/core` называет только новый файл.

## 2. Runtime

- [ ] 2.1 `internal/runtime/start.go`, `compatibility.go`: v8 в допустимых версиях шага; materialize-only binding проходит `workspace_tree_manifest_contract_mismatch`-проверку по входному порту. Проверка: `go test ./internal/runtime -run 'Start|Compatib'`.
- [ ] 2.2 `internal/runtime/workspace_trees.go`, `driver.go`, `effects.go`: для read-only шага с materialize-only binding'ом — материализация в claim-worktree, отпечаток после неё, при submit сверка с ним, после settle снятие entries; захвата нет, output port не заполняется. Проверка: новый тест по образцу `TestWorkspaceTreeSessionPassesExactNativePlanToImproveAndImplement` — plan захвачен implement'ом, read-only шаг получает файл, отчёт без изменений принят, после settle файла нет; второй тест — host правит materialized entry → `effect_not_permitted` с путём; `go test ./internal/runtime -run WorkspaceTree`.
- [ ] 2.3 `internal/runtime/sessions.go`: `repository_workspace`/`workspace_mode` в SessionTask read-only шага с таким binding'ом; без binding'а — как прежде. Проверка: тест `SessionTask` для обоих случаев; `go test ./internal/runtime -run SessionTask`.
- [ ] 2.4 `internal/runtime/workspace_tree_guide.go`: `workspace-tree-guide/2`, запись без `output_port` и текст «порт не объявлять»; submission с таким портом — named refusal. Проверка: `go test ./internal/runtime -run Guide`.

## 3. Документы и ворота

- [ ] 3.1 Глоссарий `openspec/specs/specification-governance/terms.md`: Workspace tree binding — materialize-only форма, v8; `TestGlossaryBindings` зелёный.
- [ ] 3.2 `examples/authoring/step-authoring-reference.yaml`, `schemas/authoring/README.md`, `examples/troubleshooting.md` (симптом `schema_invalid … requires output_port, capture` → форма v8). Проверка: `openspec validate --all --strict`.
- [ ] 3.3 Защищённая история не тронута: `git diff --stat openspec/changes/archive schemas/core/step-definition-v[5-7].schema.json` пуст; `make ci-check`, `make e2e`, `make race` в фоне — все зелёные; счётчики ворот записаны в change.
- [ ] 3.4 Кандидат-сборка передана пакетчику до тега; их ворота и стенд на ней зелёные; форма записи binding'а совпала с их проводкой `plan` в verify/review.

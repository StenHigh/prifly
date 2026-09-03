## 1. Контракт и authoring каталога

- [x] 1.1 Добавить versioned YAML/JSON schemas для `DecisionDefinition`, `DecisionSheet`, request, answer и ledger; обновить authoring manifest и fixtures, проверив editor validation и `make schema-check`.
- [x] 1.2 Реализовать confined чтение exact `decision_catalog` paths из Project workflow folder, canonical validation destinations/conditions/sensitivity и sealing в package manifest; покрыть допустимые nested files, duplicate/unknown IDs, traversal и попытку изменить graph тестами `go test ./cmd/prifly`.
- [x] 1.3 Сохранить `extend.yaml.profile` как reviewed default и добавить explicit package-profile override в project compilation; проверить Fast, Full и Ultra byte contracts, invalid profile и отсутствие изменения tracked source через `go test ./cmd/prifly`.

## 2. Предзапусковой выбор конкретного Run

- [x] 2.1 Добавить read-only typed launch-questionnaire output, который возвращает package profiles, conditional preflight entries, defaults/recommendations и requiredness без package import, Workspace claim или Run creation; проверить no-mutation test в `go test ./cmd/prifly`.
- [x] 2.2 Расширить `project start` typed `--package-profile` и preflight-answer input с declared precedence; проверить, что explicit Full/Ultra compile-ятся до Run, не меняют `extend.yaml`, а missing/unknown selection не создаёт state в `go test ./cmd/prifly`.
- [x] 2.3 Создать canonical Decision Sheet и включить выбранный profile/answers/sources в sealed launch receipt и Start inputs; покрыть schema-invalid/stale default, canonical digest и reproducible result тестами `go test ./cmd/prifly ./internal/runtime`.
- [x] 2.4 Добавить dependency-aware `when`: typed predicate предыдущего preflight answer, deterministic declaration order и refusal cycles/unknown predecessor; проверить ветку `roadmap_linkage → roadmap_milestone` без graph mutation в `go test ./cmd/prifly ./internal/runtime`.

## 3. Durable decision lifecycle

- [x] 3.1 Ввести persisted decision catalog/ledger events, pending request state и versioned public state/schema migration без переписывания старых Runs; проверить old-state read, incompatible-reader refusal, restart/replay fixtures через `go test ./internal/runtime ./cmd/schema-gen`. Qualified backup/restore остаётся вне текущей capability.
- [x] 3.2 Реализовать универсальный versioned Decision Bridge: typed `DecisionRequest/1` от совместимой Attempt и `run decision answer` с request digest, typed schema and Run CAS; проверить два package через один protocol, duplicate, conflict, late answer, unknown ID and no hidden successor dispatch в `go test ./internal/runtime ./cmd/prifly` (`TestDecisionBridgeServesTwoPackagesWithOneProtocol`; попутно исправлена command identity answer, чтобы исправленный ответ после schema refusal не отбивался как conflict, а поздний — сообщался как `decision_not_pending`).
- [x] 3.3 Расширить assisted-session protocol versioned Decision Sheet, bridge capability и resume delivery для того же Attempt; проверить reconnect/restart, second authorized host, unsupported unbridged executor and that prior external delivery is not replayed in `go test ./internal/runtime`.
- [x] 3.4 Реализовать attended/autonomous admission policy: automatic choice возможен лишь для explicitly allowed `ordinary` entry; scope-changing, approval-like и unknown choices wait. Проверить ledger source и forbidden auto-choice тестами `go test ./internal/runtime`.

## 4. CLI и native host experience

- [x] 4.1 Добавить CLI read/answer surfaces и human-readable final decision ledger без раскрытия unauthorized secret answer bytes; проверить structured outputs and diagnostics в `go test ./cmd/prifly`.
- [ ] 4.2 Обновить generated Codex и Claude `prifly-run` templates: один conditional questionnaire, итоговая confirmation page, launch with typed values, wait/reconnect and decision answer flow. Проверить exact template/unit cases, затем вручную наблюдать по одному Fast/Full/Ultra dialog в обоих supported hosts.
- [x] 4.3 Добавить explicit `project runners update`, который atomically обновляет только exact known generated templates и отказывает modified runner; проверить normal, stale and modified runner cases через `go test ./cmd/prifly`.

## 5. Совместимость `aif-classic`

- [x] 5.1 Добавить readable `decisions/` tree в `aif-classic` и complete pinned inventory известных вопросов `aif-plan`, `aif-implement` и `aif-commit`, включая Fast/Full/Ultra and conditional roadmap/milestone branches; проверить compilation и inventory snapshot через `make examples`.
- [x] 5.2 Передать sealed Decision Sheet и выбранный profile в session handoff (после `extract-aif-workflows` package `aif-classic` живёт в `StenHigh/prifly-aif-workflows`, а его adapter сам вызывает `aif-plan` с profile); проверить нейтральным focused integration test `TestCLIProjectStartSealsSelectedProfileIntoHandoff`, что capture layout handoff совпадает с выбранным profile и что запуск без `--package-profile` использует reviewed default, а не profile предыдущего Run.
- [x] 5.3 Привязать `aif-classic` к универсальному bridge без AI Factory knowledge в Core; raw upstream native questions не выдавать за DecisionRequest. Проверить unsupported diagnostic for unbridged runtime decision and document exact supported upstream skill revision in `make examples`.

## 6. Документация, доказательства и закрывающая проверка

- [x] 6.1 Обновить glossary, OpenSpec source specs and единый delivery backlog с active high-priority change; проверить links, terminology bindings and that historical evidence/manifests were not rewritten using `make check`.
- [x] 6.2 Обновить authoring reference and Project/CLI documentation with one questionnaire, `--package-profile`, autonomous boundary, wait/resume and runner upgrade examples; validate referenced YAML and commands with `make docs-check` or the repository's current documentation gate.
- [ ] 6.3 Выполнить focused Go tests, `make schema-check`, `make examples`, `make check` и full release gate; записать exact pass/fail counts and preserve a bounded live-pilot record that distinguishes preflight proof from dynamic-bridge qualification.

## Verification record — 2026-09-03 (second pass, after 3.2 and 5.2)

- Passed: 20 focused decision tests across `internal/runtime` and
  `cmd/prifly` (bridge with two packages, duplicate/stale/late/invalid answers,
  profile-to-handoff), `make ci-check` (`internal/runtime` 362.464 s),
  `make race` (`internal/runtime` 1615.371 s, `cmd/prifly` 29.394 s),
  `make e2e` (6 wrapper tests, 7 authoring cases, 1 launch case, workflow
  catalog case, CLI/Core/Context verifications), OpenSpec validation of the
  change.
- Fixed on the way: the answer command identity now includes the expected Run
  version and the value digest, so a corrected answer after a schema refusal is
  accepted and a late answer reports `decision_not_pending`.
- Still not completed: manual observation of the questionnaire in Codex and
  Claude Code (4.2) and the bounded live pilot with dynamic-bridge
  qualification (6.3). The tests above prove preflight and bridge contracts,
  not those interactions.

## Verification record — 2026-09-03 (first pass)

- Passed: focused compatibility tests (7), full `internal/runtime` regression
  (274.854 s), `make check`, and `make examples`.
- `make examples`: 7 Python examples, 7 authoring cases, 7 CLI cases, Core
  verification (169 commands), Context verification (75 commands, 10 cases).
- `make check`: normal tests, `go test -race` (runtime: 984.206 s), `vet`,
  schema drift check and release-CI contract all passed. OpenSpec validation:
  16 passed, 0 failed.
- Not completed: the bounded live pilot in both native hosts and the dynamic
  bridge qualification of raw upstream AI Factory questions. Preflight and
  sealed-context coverage must not be presented as proof of those interactions.

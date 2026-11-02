## 1. Контракт и воспроизведение

- [ ] 1.1 Зафиксировать исходный commit и inventory затронутых closed carriers: profile, questionnaire, summary/start, persisted/read claims, nested continuation/recovery. Проверка: таблица версий и digest старых schemas в evidence change.
- [ ] 1.2 Добавить отказной regression для project-owned input с неизменённым workflow и закрытой task schema. Проверка: новый focused test сначала падает на текущем Core; тест проверяет отсутствие claim/Run/preflight effects.
- [ ] 1.3 Добавить profile `/5`, declarations, schema path guards и binding `naming_input`. Проверка: positive/negative tests для namespace collision, extra/mixed fields, 16 inputs, unused declaration, traversal, symlink и remote `$ref`.
- [ ] 1.4 Обновить словарь и Go/JSON mappings по delta `specification-governance`; отметить новый source delta в карте источников без переноса ownership. Проверка: `go test ./... -run '^TestGlossaryBindings$'` и `openspec validate add-project-owned-worktree-naming-inputs --strict`.

## 2. CLI и reviewed snapshot

- [ ] 2.1 Добавить `--naming-input NAME=FILE`, conditional questionnaire и typed validation. Проверка: missing/duplicate/unknown NAME, wrong schema/value/pointer, null, number и отсутствие prose inference; отказы до claim.
- [ ] 2.2 Встроить полный canonical snapshot в prepare/start review до preflight и authority effects. Проверка: changed value, unused JSON field, schema и declaration дают stale refusal; whitespace canonical equivalent сохраняет digest.
- [ ] 2.3 Сохранить read-only prepare и существующий preview allocation. Проверка: authority/registry/claims/Git topology до и после равны; конкурентный start выбирает допустимый suffix без stale review по availability.
- [ ] 2.4 Сохранить отдельные workflow inputs и compile identity. Проверка: task не получает metadata; refs и compiled bytes одинаковы при изменении только naming values; checkout/no-Workspace не требуют naming, явные неприменимые values отказываются.

## 3. Claim и совместимые формы

- [ ] 3.1 Добавить bounded immutable snapshot в новый claim command payload и `authority-claims/5`. Проверка: restart round-trip сохраняет schemas/values/digests, duplicate command не расходует имя, изменённый snapshot даёт idempotency conflict.
- [ ] 3.2 Передавать snapshot в continue/resume/recover без чтения текущих naming files. Проверка: изменённые/удалённые profile policy, schema и value file не меняют claim/path/branch; generation повышается штатно.
- [ ] 3.3 Обработать continuation с новым claim от commit и отказ naming override. Проверка: новый claim использует source snapshot/base и collision policy; legacy source без snapshot сохраняет прежнее поведение.
- [ ] 3.4 Опубликовать новые closed schemas и DTO всех carriers из 1.1 с queue/composition coexistence. Проверка: реальные outputs проходят новые формы; старые schemas отвергают новые поля; legacy paths выбирают прежние versions; старый reader отказывает до effects.
- [ ] 3.5 Сохранить transform purity, storage-driver boundary и typing отказов. Проверка: `make test-changed`, `make refusal-check`, `make vet`; guard `internal/purity` проходит на новых payload paths.

## 4. Справочники и исполняемые примеры

- [ ] 4.1 Обновить author reference, operations commands и launch handbook; добавить capability и строку в `examples/README.md`. Проверка: справочник показывает schema/value files, новый flag, minimum contracts; `TestEveryDeclaredCapabilityIsInTheAuthorIndex` проходит.
- [ ] 4.2 Добавить consumer journey с pinned официальным `aif-classic v1.57.0`: install/update/prepare/start. Проверка: upstream inventory остаётся exact, consumer profile/schema/extend/project files сохраняются; start создаёт ожидаемое дерево без изменения task schema.
- [ ] 4.3 Добавить generic native CLI probe с exact executable argument и synthetic session reports. Проверка: probe проходит без LLM, tracker и приложения; выполняет stale/invalid paths и допустимое продолжение, штатно завершает/останавливает свои Runs.
- [ ] 4.4 Связать пять критериев issue №19 с конкретными tests/evidence. Проверка: таблица покрытия отделяет source-built probe, official package journey и ещё не выполненный released-Core gate.

## 5. Итоговые ворота и released приёмка

- [ ] 5.1 Выполнить итоговые source checks из явного claim worktree: `make ci-check-once CHECK_CONTEXT=codex-app-escalated CHECK_FOR_COMMIT=1` для staged bytes, `openspec validate add-project-owned-worktree-naming-inputs --strict`, `git diff --check`. Проверка: evidence относится к тому же snapshot и фактическому permission profile.
- [ ] 5.2 Проверить сохранность истории относительно commit из 1.1: `git diff --exit-code BASE -- openspec/changes/archive` и отсутствие modified/deleted старых published schemas/release evidence. Проверка: новые schema files допустимы, прежние bytes совпадают с inventory.
- [ ] 5.3 Подготовить release handoff с exact candidate SHA и probe command. Проверка: названы обязательные verify/qualify gates и owner actions; локальные tests не объявляются разрешением push/tag/release.
- [ ] 5.4 После публикации владельцем совместимого Core выполнить native probe и official package journey на exact released executable. Проверка: evidence содержит version/digest, commands, expected/actual claim и upstream integrity; до этого пункт и released критерий issue остаются открытыми.

## 1. Baseline aif-classic

- [ ] 1.1 Зафиксировать Pri-Fly SHA, compiler/stable binary versions, identity и origin AI Factory, host skills hashes и project settings. Сверить upstream и installed folder, перечислить local drift. Проверка: `acceptance.md` содержит точные refs и file inventory; локальный `extend.yaml` не выдан за upstream.
- [ ] 1.2 Провести baseline четырёх reader tasks только для `aif-classic` по `readability-protocol.md`. Проверка: записаны время, верность, подсказки, порядок и знакомство с preview; pending не заменён успехом.

## 2. Основная работа: aif-classic

- [ ] 2.1 Создать disposable копию закреплённого внешнего package и отдельно применить выбранные local settings. Оформить root, `steps/verify.yaml`, `workflows/verify-once.yaml`, `workflows/verify-batch.yaml` по preview. Проверка: parsed values всех четырёх YAML-объектов совпадают; tracked `.prifly/` Pri-Fly не изменён; `first_match`, captures и repeat boundaries сохранены.
- [ ] 2.2 Извлечь полные decoded bytes bridge в `instructions/aif-verify-bridge.md`, заменить text на source и добавить README-навигацию. Проверка: metadata, длина/SHA-256 content и завершающий LF совпадают; ссылки и карты сверены с graph. Missing source и escape проверены существующим neutral покрытием; при пробеле добавлен только необходимый neutral CLI test в Pri-Fly, без aif-specific fixtures.
- [ ] 2.3 Скомпилировать baseline/candidate через `prifly project compile --repository TEMP --package aif-classic --host codex-cli --output NEW_DIR` с одинаковыми skills и profile values. Повторить для fast/full/ultra и местных settings. Проверка: definitions совпадают после ограниченного owned-ref mapping; external refs, schemas и context bytes не исключены из сравнения; identities обеих сборок записаны.
- [ ] 2.4 Проверить verify cases из design детерминированными session replies без платных моделей, начиная с owner-only и exhausted repeat. Проверка: baseline/candidate имеют одинаковые bindings, вызовы fix и outcomes; новая implementation доходит до следующего round и downstream; прежний дефект оформлен отдельно. Package-specific harness и tests принадлежат disposable upstream checkout/внешнему repository.

## 3. Короткая контрольная проба CSV

- [ ] 3.1 Во временной копии существующего `csv-report` раскрыть только bindings и routes `stages.parse` по контрольному preview. Проверка: parsed value всего workflow совпадает; читатель находит следующий шаг и отказ; обе формы компилируются и дают прежний sample output. Команды и короткое наблюдение записаны в `acceptance.md`. Tracked CSV, steps, README, worker и schema не меняются; новых CSV tests и отдельного UX-пилота нет.

## 4. Приёмка читаемости aif-classic

- [ ] 4.1 Провести четыре candidate reader tasks только для `aif-classic`; удалить пробные edits перед semantic checks. Проверка: таблица замеров заполнена; знакомство, порядок и повторения раскрыты; чистые hashes восстановлены.
- [ ] 4.2 Сопоставить результаты с gate и записать решение отдельно для block YAML, карты и Markdown source. Проверка: `acceptance.md` содержит принятие/доработку/отклонение с фактическими числами; несопоставимый замер не отмечен PASS. Для CSV достаточно результата задачи 3.1, без требования измеренного ускорения.

## 5. Handoff и интеграционная проверка

- [ ] 5.1 Сохранить точный diff принятого AI Factory candidate и manifest baseline/skills/settings для repository workflows. Проверить compile и применимые package tests против зафиксированного latest stable Pri-Fly, отдельно от development binary. Проверка: воспроизводимые команды, versions и результаты в `acceptance.md`; без upstream commit/release/update и без объявления, что package уже поставлен. Недоступная stable-проверка оставляет эту задачу открытой.
- [ ] 5.2 Выполнить `make test-changed`, `openspec validate improve-workflow-authoring-readability --strict` и `git diff --check`; записать результаты. Проверить отсутствие diff к BASELINE в `examples/workflows/csv-report`, `.prifly`, `schemas`, `internal/runtime`, `internal/flow`, `openspec/changes/archive` и historical release evidence/manifests. Product diff Pri-Fly допускает только необходимый neutral source test. Перед предусмотренным коммитом staged snapshot проходит `make ci-check-once CHECK_CONTEXT=ACTUAL_PROFILE CHECK_FOR_COMMIT=1`; race/e2e qualify и релиз не запускаются.

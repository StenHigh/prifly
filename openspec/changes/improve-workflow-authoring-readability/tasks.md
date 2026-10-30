Принятие от 30 октября 2026 года: владелец выбрал review без обязательных ручных замеров.
Ответ «Принимай» и исходные формулировки заменённых задач сохранены в `evidence/owner-review.json`.
Чекбоксы ниже относятся к согласованным критериям. Они не означают, что прежний reader pilot выполнен.

## 1. Baseline aif-classic

- [x] 1.1 Зафиксировать Pri-Fly SHA, compiler/stable binary versions, identity и origin AI Factory, host skills hashes и project settings. Сверить upstream и installed folder, перечислить local drift. Проверка: `acceptance.md` содержит точные refs и file inventory; локальный `extend.yaml` не выдан за upstream.
- [x] 1.2 Зафиксировать способ приёмки, выбранный владельцем после сравнения до/после. Проверка: сохранены точный вопрос и ответ «Принимай»; ручные baseline задания явно отмечены как не выполненные; решение по review не выдано за измеренный UX-результат.

## 2. Основная работа: aif-classic

- [x] 2.1 Создать disposable копию закреплённого внешнего package и отдельно применить выбранные local settings. Оформить root, `steps/verify.yaml`, `workflows/verify-once.yaml`, `workflows/verify-batch.yaml` по preview. Проверка: parsed values всех четырёх YAML-объектов совпадают; tracked `.prifly/` Pri-Fly не изменён; `first_match`, captures и repeat boundaries сохранены.
- [x] 2.2 Извлечь полные decoded bytes bridge в `instructions/aif-verify-bridge.md`, заменить text на source и добавить README-навигацию. Проверка: metadata, длина/SHA-256 content и завершающий LF совпадают; ссылки и карты сверены с graph. Missing source и escape проверены существующим neutral покрытием; при пробеле добавлен только необходимый neutral CLI test в Pri-Fly, без aif-specific fixtures.
- [x] 2.3 Скомпилировать baseline/candidate через `prifly project compile --repository TEMP --package aif-classic --host codex-cli --output NEW_DIR` с одинаковыми skills и profile values. Повторить для fast/full/ultra и местных settings. Проверка: definitions совпадают после ограниченного owned-ref mapping; external refs, schemas и context bytes не исключены из сравнения; identities обеих сборок записаны.
- [x] 2.4 Проверить verify cases из design детерминированными session replies без платных моделей, начиная с owner-only и exhausted repeat. Проверка: baseline/candidate имеют одинаковые bindings, вызовы fix и outcomes; новая implementation доходит до следующего round и downstream; прежний дефект оформлен отдельно. Package-specific harness и tests принадлежат disposable upstream checkout/внешнему repository.

## 3. Короткая контрольная проба CSV

- [x] 3.1 Выполнить короткую техническую CSV-пробу во временной копии: раскрыть только bindings и routes stages.parse. Проверка: parsed value всего workflow совпадает; pass ведёт к validate, остальные verdicts и on_error ведут к rejected; обе формы компилируются и дают прежний sample output. Ручное наблюдение владельца не проводилось и после выбранной приёмки по review не является обязательным. Tracked CSV и его worker, schema и README не меняются; новых CSV tests и UX-пилота нет.

## 4. Приёмка читаемости aif-classic

- [x] 4.1 Предъявить владельцу реальные baseline/candidate фрагменты и получить решение по review без обязательных reader tasks. Проверка: сравнение сохранено в owner-review.html; ответ пользователя зафиксирован; hashes всех семи файлов совпали с handoff; ручные задания и времена не выдуманы.
- [x] 4.2 Зафиксировать принятие отдельно для block YAML, README-карты и Markdown source по решению владельца. Проверка: все три части приняты, критерии плана согласованы с выбранным способом; измеренного ускорения не заявлено; исходный протокол замеров сохранён как необязательный исследовательский вариант.

## 5. Handoff и интеграционная проверка

- [x] 5.1 Сохранить точный diff принятого AI Factory candidate и manifest baseline/skills/settings для repository workflows. Проверка: compile и применимые package tests прошли на закреплённом stable Pri-Fly 0.13.81 отдельно от development binary; команды, версии и результаты сохранены; принятие владельцем записано в evidence. Upstream commit/release/update и установка не выполняются; принятый handoff не выдан за поставленный package.
- [x] 5.2 Выполнить `make test-changed`, `openspec validate improve-workflow-authoring-readability --strict` и `git diff --check`; записать результаты. Проверить отсутствие diff к BASELINE в `examples/workflows/csv-report`, `.prifly`, `schemas`, `internal/runtime`, `internal/flow`, `openspec/changes/archive` и historical release evidence/manifests. Product diff Pri-Fly допускает только необходимый neutral source test. Перед предусмотренным коммитом staged snapshot проходит `make ci-check-once CHECK_CONTEXT=ACTUAL_PROFILE CHECK_FOR_COMMIT=1`; race/e2e qualify и релиз не запускаются.

## 1. Высший приоритет — разные настройки одного workflow без конфликтов

- [x] 1.1 Ввести распознавание profile `/3` и его b1 build-key/provenance contract из design, сохранив `/2` legacy compilation; обновить glossary и добавить golden vectors для порядка файлов, путей, profiles, context/supporting bytes, catalog и manifest metadata. Проверка: одинаковый вход даёт одинаковые refs, изменение только description/question даёт новую сборку, encoding проходит Version schema.
- [x] 1.2 Применить общую identity-сборку `/3` к package и owned closure в обоих входах compile/start, перепривязать refs и root lookup; проверка: compile/start возвращают одинаковый root, provenance schema/mapping совпадает с exports, изменённая mapping отвергается, внешние refs не изменены, legacy package без provenance читается прежним способом.
- [x] 1.3 Добавить один neutral lifecycle regression A → B → A в одной authority с import/start, settings/exclude/insert и изменением context; проверка: разные сборки сосуществуют, возврат настроек переиспользует прежние refs, default YAML не меняется.
- [x] 1.4 Проверить старый активный Run после нового варианта и restart, tamper rejection и revoked reimport; проверка: прежние bytes сохранены, новый вариант не наследует trust, отзыв не снимается. Не использовать отдельную authority на каждый вариант.
- [x] 1.5 Обновить сообщения compile/start/update и документацию author version против compiled version; проверка: пользователь видит понятную версию сценария и exact сборку, новые fixture checks проходят через public CLI. Сохранить результат focused тестов и отдельный commit этого среза.

## 2. Высший приоритет — полноценный запуск без Git и ИИ

- [ ] 2.1 Расширить `/3` optional hosts и явным подключением runner, сделать его default fresh init, сохранив reader `/2`; проверка: init без host не создаёт Git/AI folders, clone/copy bootstrap не переписывает shared YAML и frozen runners, context-capable authority готова и без host, editor schemas принимают обе версии.
- [ ] 2.2 Отделить обнаружение Project root от Git во всех общих init/list/questionnaire/compile/start путях; проверка: public CLI работает в обычной папке и не вызывает Git там, где он не нужен; repository download сохраняет свои проверки.
- [ ] 2.3 Ввести versioned Start/state/read/envelope путь без RunBrief для workflows без такого input, сохранив admitted start intent/inputs/policy и прежние brief contracts; проверка: no-brief Run создаётся и читается после restart без фиктивного artifact, old reader отказывает unsupported version, прежние Runs сохраняют digest и brief semantics.
- [ ] 2.4 Описать YAML execution bindings, local executable allow mapping и отдельный versioned exact-ref payload; проверка: shapes/editor examples покрывают steps и checks, unknown selectors, чужие refs, unsafe paths и необъявленные executable отвергаются. Установленные файлы сами не исполняются.
- [ ] 2.5 Провести bindings через общий resolver validation/Start к существующему executor pinning, не меняя `EffectiveConfiguration` и authority config; проверка: два packages/версии step с разными programs в одной authority, restart, unchanged config bytes и конфликт повторного command ID с другим binding.
- [ ] 2.6 Вычислять нужные host/workspace по compiled contracts и связать neutral Start с Project; проверка: command-only source не требует host/brief/claim, host-bound context без host и Git-запись без workspace choice отказывают до mutation; режимы `/2` не меняются.
- [ ] 2.7 Собрать переносимый YAML пример CSV → validation → report на существующем managed worker contract; проверка: реальный public start без Git/ИИ создаёт output с ожидаемым содержимым, его можно прочитать после restart. Разделить учебный example и тестовые fixtures, не требовать AIF/сеть.
- [ ] 2.8 Сохранить инструкции локальной настройки программы и запуска для нового пользователя, явно назвать существующий worker contract и отсутствие sandbox; проверка: пройти пример по README без ручного редактирования authority JSON. Записать focused gates и отдельный commit среза.

## 3. Высокий приоритет — общие вопросы и смешанные шаги

- [ ] 3.1 Удалить AIF-процессные правила из текущего generic runner, сохранить frozen templates; проверка: neutral one-step launch не получает reviewers/improve/commit, tests старых runner upgrades проходят без изменения исторических hashes.
- [ ] 3.2 Расширить существующую questionnaire optional runtime предответами, условиями и summary возможных ожиданий; проверка: read-only preview не создаёт package/claim/Run, invalid/stale selection отвергается, runtime answer проходит ту же validation, что Start.
- [ ] 3.3 Показать summary до первого dispatch и сохранить его в final result; проверка: worker marker ещё отсутствует в момент предъявления summary, изменённые sources/bindings/answers пересчитываются до эффекта, неизвестный skill question не получает скрытого ответа.
- [ ] 3.4 Проверить no-Git command → assisted `effects:none` → command с настоящим handoff, typed decision wait, restart и ответом в том же Run; проверка: Git claim отсутствует, порядок outputs верен, завершающая команда не исполняется до принятого ответа/result.
- [ ] 3.5 Обновить generic host инструкции и ограничения local-owner trust; выполнить конечные UI observations Codex и Claude по незавершённым tasks существующих changes, связав evidence вместо копирования задач. Проверка: source actor не выдан за доказанную личность человека; отсутствующее наблюдение остаётся открытым. Сохранить отдельный commit среза.

## 4. Интеграция — внешний AIF на тех же правилах

- [ ] 4.1 Во внешнем `prifly-aif-workflows` добавить compatibility check против exact candidate после явной миграции fixture Project на `/3`: Classic Fast → Full → Ultra → default → Fast в одной authority, custom setting/exclude/insert и разные host skill bytes; проверка: import/start проходят, старые Runs и tracked defaults сохранены. Core fixture не импортирует AIF.
- [ ] 4.2 Перенести необходимые прикладные указания из прежнего runner в AIF YAML/contexts и проверить Classic/Fanout; проверка: canonical порядок/циклы, изменённый plan между improve rounds, вопросы и notes сохранены без AIF-ветвей в Core.
- [ ] 4.3 Выполнить один ограниченный живой AIF pilot на согласованной небольшой задаче; записать binary/package/host versions, решения и итоговый artifact/commit. Проверка: наблюдался реальный путь, а не только compile; task 6.3 decision-catalog связывается с этим evidence лишь при совпадении её критериев.
- [ ] 4.4 На собранном candidate один раз выполнить `make check` и `make e2e`, а внешние проверки — в AIF repo; записать точные итоги и scope. Не считать scripted host живым UI, не повторять одинаковые дорогие gates после docs-only правки; новый release/version согласовывается отдельно.
- [ ] 4.5 Синхронизировать delta, glossary, published contracts, editor references, README и текущую очередь; проверка: `openspec validate --all --strict --no-interactive`, `TestGlossaryBindings` при изменении карты, `git diff --check`. `git diff --name-only 5b5c4ca -- openspec/changes/archive` должен быть пустым: historical evidence не меняется. Формальные P1/P2 gates и deferred backlog остаются незакрытыми.

## 5. Правило проверки каждого среза

- [ ] 5.1 Перед закрытием каждого среза запустить конкретные добавленные/затронутые Go tests через `.tools/go/bin/go test ./cmd/prifly ./internal/runtime ./internal/flow -run '<точные имена>' -count=1`, сузив packages до затронутых; проверить, что тесты действительно исполнились, а не дали `no tests to run`. Записать команды, счётчики и границы доказательства; публикацию коммитов группировать, чтобы не расходовать CI на каждый промежуточный шаг.

## Срез 1 — проверка реализации, 2026-09-05

Один in-memory render используется для build identity и sealing в обоих
CLI входах. b1 различает package и всю owned closure; refs не переписываются
в literal/default/schema instance data. Закрытая provenance schema проверяет
root, полные author/compiled mappings и derivation versions. Обновлены
glossary, текущие требования только этого среза, README и editor reference;
полная синхронизация оставшихся delta остаётся задачей 4.5.

Нейтральный public CLI fixture использует одну authority. A остаётся реально
активным в ожидании host; B, повтор A и варианты settings/exclude/insert/context
проходят session submit + drive до succeeded. Повторное открытие authority
сохраняет bytes прежних definitions/context и handoff A. Переименование и
перенос leaf YAML в глубокую known directory, а также другой host label с
идентичными skills воспроизводят A. Отдельные import-only варианты меняют
только description или только текст вопроса; оба устанавливаются рядом.
Проверены обычный trust admission нового exact manifest, tamper,
identity-conflict и невозможность снять revocation повторным import.

Focused команды (рабочая директория — корень GitHub checkout):

```sh
GOCACHE=/private/tmp/prifly-neutral-go-build GOTOOLCHAIN=local .tools/go/bin/go test ./cmd/prifly -run '^(TestCLIProject|TestProjectBuild|TestProjectWorkflow|TestProjectProfileOrigin)' -count=1 -json
GOCACHE=/private/tmp/prifly-neutral-go-build GOTOOLCHAIN=local .tools/go/bin/go test ./cmd/prifly ./internal/runtime -run '^(TestAuthoringDocumentsAreServedAndMatchTheDistributedFiles|TestGlossaryBindings)$' -count=1 -json
GOCACHE=/private/tmp/prifly-neutral-go-build GOTOOLCHAIN=local .tools/go/bin/go test ./cmd/prifly -run '^TestCLIProjectCompiledVariantsLifecycle$' -count=1 -v
GOCACHE=/private/tmp/prifly-neutral-go-build GOTOOLCHAIN=local .tools/go/bin/go test ./cmd/prifly -run '^TestProjectBuildProvenanceRejectsIdentityTampering$' -count=1 -v
make fmt-check refusal-check
openspec validate --all --strict --no-interactive
git diff --check
git diff --name-only 5b5c4ca -- openspec/changes/archive
```

Первый прогон: **27/27 top-level, 15/15 subtests**, 12.557 s. Контрактный:
**2/2 top-level, 8/8 subtests**, cmd 1.337 s, runtime 0.552 s. После добавления
metadata-only import assertions повторён только lifecycle: **1/1**, 5.395 s.
После добавления unknown-field assertion повторён только provenance test:
**1/1 + 4/4 subtests**, 0.640 s. Итого **29 уникальных top-level tests и
24 subtests**, без отказов; повторные запуски не посчитаны как новые тесты.
OpenSpec: **19/19**. Formatting/refusal/diff checks чистые; archive diff пуст.

**Что в этот срез не входит:** optional Git/host/RunBrief, executable bindings
и их supporting files, neutral runner, новые вопросы/preview, внешний AIF
и живое наблюдение Codex/Claude. Поддержанные сегодня source files покрыты
через exact context bytes; contract новых supporting files вводится в 2.4 и
должен включить свои bytes в build key, не меняя ключ старого входа. Fixture
остаётся Git-проектом с brief и scripted host, имеет capacity 2 и явно
освобождает пока безусловно созданный, но не используемый `effects:none`
claim. Это проверка coexistence/history, а не квалификация no-Git/no-AI или
UI. `make check`/`make e2e` отложены до candidate по 4.4; release, установка
на компьютер и обновление полигона здесь не выполняются. 5.1 остаётся
открытой как обязательство следующих срезов.

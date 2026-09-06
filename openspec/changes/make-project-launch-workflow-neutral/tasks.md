## 1. Высший приоритет — разные настройки одного workflow без конфликтов

- [x] 1.1 Ввести распознавание profile `/3` и его b1 build-key/provenance contract из design, сохранив `/2` legacy compilation; обновить glossary и добавить golden vectors для порядка файлов, путей, profiles, context/supporting bytes, catalog и manifest metadata. Проверка: одинаковый вход даёт одинаковые refs, изменение только description/question даёт новую сборку, encoding проходит Version schema.
- [x] 1.2 Применить общую identity-сборку `/3` к package и owned closure в обоих входах compile/start, перепривязать refs и root lookup; проверка: compile/start возвращают одинаковый root, provenance schema/mapping совпадает с exports, изменённая mapping отвергается, внешние refs не изменены, legacy package без provenance читается прежним способом.
- [x] 1.3 Добавить один neutral lifecycle regression A → B → A в одной authority с import/start, settings/exclude/insert и изменением context; проверка: разные сборки сосуществуют, возврат настроек переиспользует прежние refs, default YAML не меняется.
- [x] 1.4 Проверить старый активный Run после нового варианта и restart, tamper rejection и revoked reimport; проверка: прежние bytes сохранены, новый вариант не наследует trust, отзыв не снимается. Не использовать отдельную authority на каждый вариант.
- [x] 1.5 Обновить сообщения compile/start/update и документацию author version против compiled version; проверка: пользователь видит понятную версию сценария и exact сборку, новые fixture checks проходят через public CLI. Сохранить результат focused тестов и отдельный commit этого среза.

## 2. Высший приоритет — полноценный запуск без Git и ИИ

- [x] 2.1 Расширить `/3` optional hosts и явным подключением runner, сделать его default fresh init, сохранив reader `/2`; проверка: init без host не создаёт Git/AI folders, clone/copy bootstrap не переписывает shared YAML и frozen runners, context-capable authority готова и без host, editor schemas принимают обе версии.
- [x] 2.2 Отделить обнаружение Project root от Git во всех общих init/list/questionnaire/compile/start путях; проверка: public CLI работает в обычной папке и не вызывает Git там, где он не нужен; repository download сохраняет свои проверки.
- [x] 2.3 Ввести versioned Start/state/read/envelope путь без RunBrief для workflows без такого input, сохранив admitted start intent/inputs/policy и прежние brief contracts; проверка: no-brief Run создаётся и читается после restart без фиктивного artifact, old reader отказывает unsupported version, прежние Runs сохраняют digest и brief semantics.
- [x] 2.4 Описать YAML execution bindings, local executable allow mapping и отдельный versioned exact-ref payload; проверка: shapes/editor examples покрывают steps и checks, unknown selectors, чужие refs, unsafe paths и необъявленные executable отвергаются. Установленные файлы сами не исполняются.
- [x] 2.5 Провести bindings через общий resolver validation/Start к существующему executor pinning, не меняя `EffectiveConfiguration` и authority config; проверка: два packages/версии step с разными programs в одной authority, restart, unchanged config bytes и конфликт повторного command ID с другим binding.
- [x] 2.6 Вычислять нужные host/workspace по compiled contracts и связать neutral Start с Project; проверка: command-only source не требует host/brief/claim, host-bound context без host и Git-запись без workspace choice отказывают до mutation; режимы `/2` не меняются.
- [x] 2.7 Собрать переносимый YAML пример CSV → validation → report на существующем managed worker contract; проверка: реальный public start без Git/ИИ создаёт output с ожидаемым содержимым, его можно прочитать после restart. Разделить учебный example и тестовые fixtures, не требовать AIF/сеть.
- [x] 2.8 Сохранить инструкции локальной настройки программы и запуска для нового пользователя, явно назвать существующий worker contract и отсутствие sandbox; проверка: пройти пример по README без ручного редактирования authority JSON. Записать focused gates и отдельный commit среза.

## 3. Высокий приоритет — общие вопросы и смешанные шаги

- [x] 3.1 Удалить AIF-процессные правила из текущего generic runner, сохранить frozen templates; проверка: neutral one-step launch не получает reviewers/improve/commit, tests старых runner upgrades проходят без изменения исторических hashes.
- [x] 3.2 Расширить существующую questionnaire optional runtime предответами, условиями и summary возможных ожиданий; проверка: read-only preview не создаёт package/claim/Run, invalid/stale selection отвергается, runtime answer проходит ту же validation, что Start.
- [x] 3.3 Показать summary до первого dispatch и сохранить его в final result; проверка: worker marker ещё отсутствует в момент предъявления summary, изменённые sources/bindings/answers пересчитываются до эффекта, неизвестный skill question не получает скрытого ответа.
- [x] 3.4 Проверить no-Git command → assisted `effects:none` → command с настоящим handoff, typed decision wait, restart и ответом в том же Run; проверка: Git claim отсутствует, порядок outputs верен, завершающая команда не исполняется до принятого ответа/result.
- [ ] 3.5 Обновить generic host инструкции и ограничения local-owner trust; выполнить конечные UI observations Codex и Claude по незавершённым tasks существующих changes, связав evidence вместо копирования задач. Проверка: source actor не выдан за доказанную личность человека; отсутствующее наблюдение остаётся открытым. Сохранить отдельный commit среза.

## 4. Интеграция — внешний AIF на тех же правилах

- [x] 4.1 Во внешнем `prifly-aif-workflows` добавить compatibility check против exact candidate после явной миграции fixture Project на `/3`: Classic Fast → Full → Ultra → default → Fast в одной authority, custom setting/exclude/insert и разные host skill bytes; проверка: import/start проходят, старые Runs и tracked defaults сохранены. Core fixture не импортирует AIF. (сделано 2026-09-06: `tests/compatibility.py` в `prifly-aif-workflows` `e263720`, CI репозитория зелёный; пять запусков в одной authority, четыре различные сборки, пятая совпадает с первой по `build_key`; `verify.py` сохраняет прежнюю гарантию `before == after` по `package list`, то есть Core fixture по-прежнему не импортирует AIF)
- [x] 4.2 Перенести необходимые прикладные указания из прежнего runner в AIF YAML/contexts и проверить Classic/Fanout; проверка: canonical порядок/циклы, изменённый plan между improve rounds, вопросы и notes сохранены без AIF-ветвей в Core. (сделано 2026-09-06: пять
      прикладных указаний, удалённых из generic runner, разобраны по файлам
      пакета. «Предложенное гейтом действие не команда» — в четырёх мостах и,
      сильнее прозы, структурно: маршрут ветвится по вердикту и `blocking`, а
      `suggested_next` никто не читает. «Другие объявленные вопросы» — покрыто
      объявленным `improve_apply` и поимённо названными родными диалогами.
      «Тихий успешный выход» намеренно не переносился: у каждого шага объявлен
      `result_schema_ref` и выходы `required_for: [pass]`, поэтому молчаливый
      успех недостижим и получает именованный отказ приёма. TaskInput как
      provider-neutral граница выражена схемой `task.yaml` с
      `additionalProperties: false`. Слово `commit` относилось к файлам самого
      Pri-Fly, а не к шагу фиксации, и убрано вместе с нейтрализацией. Пятое
      указание — две reviewer-задачи и отдельные сессии — действительно
      осталось без дома и добавлено в `aif-review-bridge` выпуском пакета
      `v1.11.0` (`9ecb72d`, каталог `27b2b3e`); в `aif-verify-bridge` его нет
      намеренно, у того навыка нет инструментов подсессии. Графом проверены
      канонический порядок, обе петли, `next_bindings.plan.from ==
      iteration_output` и отсутствие AIF-ветвей в Core. Четыре ворот пакета
      против выпущенного `prifly 0.10.0` прогнаны независимо на этой машине:
      `test_versions`, `test_folders`, `verify.py`, `compatibility.py` — все
      зелёные. Наблюдение прогона остаётся за живым pilot задачи 4.3)
- [ ] 4.3 Выполнить один ограниченный живой AIF pilot на согласованной небольшой задаче; записать binary/package/host versions, решения и итоговый artifact/commit. Проверка: наблюдался реальный путь, а не только compile; task 6.3 decision-catalog связывается с этим evidence лишь при совпадении её критериев.
- [x] 4.4 На собранном candidate один раз выполнить `make check` и `make e2e`, а внешние проверки — в AIF repo; записать точные итоги и scope. Не считать scripted host живым UI, не повторять одинаковые дорогие gates после docs-only правки; новый release/version согласовывается отдельно. (сделано 2026-09-06: локальная половина — `make check` PASS 986.72 с и `make e2e` в [записи выпуска](release-0.10.0.md), включая честно записанный отказ первого прогона `make e2e` на устаревшей фикстуре каталога и его исправление; внешняя — четыре ворот `prifly-aif-workflows` против опубликованного `prifly 0.10.0`, зафиксированы в их `e263720` с зелёным CI. Scripted host живым UI не считается; версия 0.10.0 согласована владельцем отдельно)
- [x] 4.5 Синхронизировать delta, glossary, published contracts, editor references, README и текущую очередь; проверка: `openspec validate --all --strict --no-interactive`, `TestGlossaryBindings` при изменении карты, `git diff --check`. `git diff --name-only 5b5c4ca -- openspec/changes/archive` должен быть пустым: historical evidence не меняется. Формальные P1/P2 gates и deferred backlog остаются незакрытыми. (сделано 2026-09-06 в `9238d8f`, подробности — в записи среза 4: мерж двусторонний, все 26 блоков дельты совпадают с main, песочная архивация даёт нулевые added/modified/removed. `openspec validate --all --strict --no-interactive` 20 passed 0 failed; `TestGlossaryBindings` PASS; `make schemas-check` 46/46; `git diff --check` чист; `git diff --name-only 5b5c4ca -- openspec/changes/archive` пуст; branch `verify` 34033376071 success. Формальные P1/P2 gates и deferred backlog не закрывались)

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

## Срез 2 — нейтральный managed запуск, 2026-09-05

Fresh Project `/3` не требует Git, host или отдельного RunBrief. Явные hosts
подключаются через init/add; copy bootstrap сохраняет shared YAML и frozen
runners. Обычные compile/start берут local authority из `local.yaml`, если
владелец не передал прежний raw `--project`. `/2` сохраняет Git/host/brief,
default worktree и файловую передачу configuration overrides. Для `/3`
обязательность inputs проверяет compiled contract после defaults/settings;
чтение source listing не подменяет эту проверку. Unknown/duplicate inputs,
отсутствие host, workspace choice, Git или разрешённой программы отвергаются
до registration/claim/Run.

`execution_bindings` в root YAML описывает собственные steps/checks, argv,
supporting files и явные пределы. Supporting bytes читаются через открытые
directory handles с проверкой identity и запретом symlinks; файлы не запускаются
во время compilation. Exact compiled refs и supporting bytes включены в
inert `execution-bindings.json`, manifest и b1. Старый b1 golden без новых
bindings сохранён. Логическая программа разрешается через local allow map и
явный `--allow-execution`; registry/global executor config не переписываются.
Runtime использует один resolver для validation/Preview/Start и существующие
PinnedExecutor/PackageLock. Проверены две exact версии step, две программы,
automatic checks, разные supporting bytes, restart и конфликт retry с изменённым
payload в одной authority.

Реальный осмотр уточнил две границы первоначального плана. ExecutionEnvelope
`/1` уже не содержит brief_ref и действительно доставлен no-brief worker;
новый envelope ради номера не понадобился. Вместо него versioned Start `/2`
выбирает state/read `/26`, которые не создают пустой Brief/artifact; прежние
schema/read contracts сохранены. Для CheckDefinition понадобился новый
PackageManifest `/2`: прежний manifest `/1` не допускал kind check. Compiler
использует manifest `/2` только с checks, остальные packages сохраняют `/1`.
Отдельный тест доказывает YAML compile → import → reopen exact check closure;
исполнение checks доказано runtime lifecycle, не выдано за CSV-операцию.

Учебный `examples/workflows/csv-report/` содержит YAML graph и обычный Node
worker, не AIF и не compiler script. Единственный сквозной Run выполнил
parse → validate → report, экспортировал bytes `Rows: 3` / `Total: 24` и был
прочитан после повторного открытия authority. Git отсутствует в PATH и папке,
AI directories/brief/decisions/claim отсутствуют, shared/local/global config
не меняются. Пример использует настоящий `process.execPath`, потому что shell
shim менеджера Node не обязан работать в очищенном worker environment.
Перед успешным Run отдельно проверены разрешение запуска, local allow map и
missing input после настройки allow map; каждый отказ оставил packages пустыми.
Доставка source в локальную `.prifly/` — обычное копирование YAML и декларация
package/launch, а не ручная правка authority JSON. Git downloader не расширялся.

Итоговая focused команда (один прогон после правок, из GitHub checkout):

```sh
GOCACHE=/private/tmp/prifly-neutral-go-build GOTOOLCHAIN=local .tools/go/bin/go test ./cmd/prifly ./internal/runtime -run '^(TestCLIProject.*|TestProject.*|TestNeutralAuthoringReferencesMatchServedSchemas|TestCheckAuthoringSchemaMatchesFullDefinition|TestAuthoringDocumentsAreServedAndMatchTheDistributedFiles|TestNeutralStart.*|TestStartInputPreflightUsesReadonlyAdmissionValidation|TestExecutionBindings.*|TestPackageCheckImportsKeepVersionAndIdentityBoundaries|TestStartExactRetrySnapshotAndSourceDrift|TestCoreInputConfiguration|TestSourceRuntimeRejectsShapeOnlyDescriptors|TestFullContextNativeExecutionUsesPinnedSources|TestCallConfigurationDoesNotReplaceExplicitAbsence|TestContextFieldsNeverExtendOlderStateContracts|TestPublicSchemasMatchActualReadViewsAndRejectExtensions|TestGlossaryBindings)$' -count=1 -json
GOCACHE=/private/tmp/prifly-neutral-go-build GOTOOLCHAIN=local make schemas-check GO=.tools/go/bin/go
python3 test/e2e/test_examples.py EditorContractTest
make fmt-check refusal-check GO=.tools/go/bin/go
openspec validate --all --strict --no-interactive
git diff --check
git diff --name-only 5b5c4ca -- openspec/changes/archive
```

Результат: **62/62 top-level, 143/143 subtests, 0 skipped, 0 failed**.
cmd/prifly 18.135 s, runtime 9.755 s; промежуточные повторы не прибавлены к
счётчику. Schema gate: **44/44** наборов совпали без `--write`, старые
закреплённые hashes сохранены. Editor Python gate: **1/1**. OpenSpec:
**19/19**. Formatting/refusal/diff checks чистые, archive diff пуст.

**Что в этот срез не входит:** общий host runner без отраслевых правил,
полный summary вопросов до dispatch, mixed command → assisted → command,
живые UI observations Codex/Claude, внешний AIF package/pilot и полный
candidate `make check`/`make e2e`. Эти работы остаются в срезах 3–4, вместе с
полной синхронизацией main specs по 4.5; здесь актуализированы словарь,
published/editor contracts, README и concrete design. Shell-команды не
превращаются автоматически в native workers, managed scratch не объявляется
sandbox. Release, установка на компьютер и полигон не менялись. Старые
release evidence не редактировались. 5.1 остаётся открытой для следующих срезов.

## Срез 3 — общая анкета и mixed protocol, 2026-09-05

Текущий runner больше не содержит число reviewers, improve/review/commit
правила или обязательный task/brief для обычного workflow. Пять исторических
форм для трёх hosts сохранены byte-for-byte; все 15 hashes и upgrade проходят.
Profile `/3` использует одну анкету с optional runtime предответами и
read-only `--prepare`, затем exact `--expected-launch-digest`. `/2` сохраняет
старый запуск и отдельный catalog digest, без ложного checked summary и
неявной миграции. Current runner явно различает эти контракты.

Questionnaire и Start используют одну validation; форма может быть неполной.
Missing predecessor показан условным, известный false в AND не превращён в
unknown. Package-profile answer и разрешённый policy default применяются до
зависимых вопросов. Typed `false` не теряется. Найденное прежнее игнорирование
`launch_input` исправлено: `/3` связывает JSON answer с обычным input до
schema/scope validation, конфликты источников отвергаются. Public CLI Run
действительно получил значение 2 вместо default 3; runtime-перезапись уже
закреплённого input не объявляется поддержанной.

Summary содержит selected package/root, требования, digests inputs/config,
программы и argv, ответы с источниками и known wait reasons. Он предъявляется
до мутаций/dispatch в stderr и остаётся в `project-start/3.launch_summary`;
stdout сохраняет один final JSON. Изменение source/support/input/answers/local
binding между prepare и Start отвергается до package/claim/Run. Проверены
отказ при ошибке вывода и замена executable symlink внутри summary callback.
В callback не было Run, пакета, claim и worker workspace; после нормального
старта три настоящих Node worker оставили marker в своих Attempt workspaces
и дали отчёт Total 24. Review digest не зависит от command ID и installation
того же exact package. Public Run view по-прежнему скрывает executors;
read-only runtime comparison проверяет private pinned digests без раскрытия.

Отдельный no-Git mixed Run выполнил native parse → assisted effects:none →
native report. Typed request остановил тот же Attempt, invalid string answer
не изменил snapshot. После reopen integer 2 попал в decision_context и ledger;
Drive не запускал завершающий шаг ни до ответа, ни до принятого session result.
После host result native report выдал Total 48. Git claim и Brief отсутствуют.
Это scripted host и настоящий CLI/session protocol, не поведение модели/UI.

Focused команды (GitHub checkout, warm cache; повторы не складываются в новые
тесты):

```sh
GOCACHE=/private/tmp/prifly-neutral-go-build GOTOOLCHAIN=local .tools/go/bin/go test ./cmd/prifly ./internal/runtime -run '^(TestCLIProject.*|TestProject.*|TestHelpNamesTheQuestionnaireFlags|TestCLIHelpDoesNotDenyImplementedCoreOperators|TestExecutionReviewChecksPinnedBytesWithoutDisclosure|TestExecutionBindingsExactVersionsChecksAndRestart|TestGlossaryBindings)$' -count=1 -json
GOCACHE=/private/tmp/prifly-neutral-go-build GOTOOLCHAIN=local .tools/go/bin/go test ./cmd/prifly -run '^(TestCLIProjectStartClaimsDeclaredWorkspaceAndStopsAtHostHandoff|TestCLIHelpDoesNotDenyImplementedCoreOperators|TestCLIProjectDecisionInputReachesRun|TestHelpNamesTheQuestionnaireFlags|TestProjectRunnerTextIsPinned|TestProjectFrozenRunnerTextIsPinned|TestProjectCurrentRunnerIsWorkflowNeutral)$' -count=1 -json
GOCACHE=/private/tmp/prifly-neutral-go-build GOTOOLCHAIN=local .tools/go/bin/go test ./cmd/prifly -run '^(TestHelpNamesTheQuestionnaireFlags|TestCLIHelpDoesNotDenyImplementedCoreOperators)$' -count=1 -json
make fmt-check refusal-check GO=.tools/go/bin/go
python3 test/e2e/test_examples.py EditorContractTest
openspec validate --all --strict --no-interactive
git diff --check
git diff --name-only 5b5c4ca -- openspec/changes/archive
```

Итог по последним результатам каждого теста: **57/57 top-level, 99/99
subtests, 0 skipped, 0 оставшихся failures**. Первый общий прогон: runtime
4.566 s, cmd 16.032 s; 54 теста прошли, три остановились на старых ожиданиях
версии анкеты/справки и missing ignore-file нового fixture. После исправлений
повторены только затронутые проверки и current/frozen pins: 2.994 s.
Дополнительно одна новая проверка справки выявила отсутствующую ссылку на
`questionnaire --prepare` именно в короткой помощи start; исправлена и
проверена двумя help tests за 0.633 s. Промежуточные отказы не скрыты, полного
повторного прогона ради справки не было. Editor **1/1**, OpenSpec **19/19**,
format/refusal/diff чистые, archive diff пуст. Wire/schema bytes не менялись.

**Что в этот срез не входит:** task 3.5 остаётся открытой: инструкции и
local-owner caveat реализованы, но живые UI observations не завершены. Claude
Desktop/CLI обнаружены; попытка read-only доступа к UI остановилась на
системных Accessibility/Screen Recording permissions. Повторных ожиданий и
запуска модели не было; finite dialog Codex также не наблюдался. Поэтому
`add-native-host-question-ux` 2.3 и `add-run-decision-catalog` 4.2/6.3 не
закрыты и не заменены scripted proof. Actor — provenance, не удостоверенный
отдельный человек; неизвестный native вопрос требует явной остановки/вопроса,
а не придуманного bridge record. Внешний AIF compatibility/pilot, полные
candidate gates, полная синхронизация main specs, Release и полигон остаются
в срезе 4. При mixed-тесте также замечен прежний wire-нюанс Go
`SessionSubmission`: request нужно отправлять без поля result, поскольку
`result:null` не равен отсутствию result; тест использует допустимый wire,
старый DTO/wire контракт этим срезом не переписан. 5.1 остаётся обязательством
следующих срезов. Все принятые уточнения находятся также в design этого change.

## Отдельный выпуск 0.10.0 — 2026-09-06

Владелец утвердил version 0.10.0. Подготовка и точные результаты core-local
release gates находятся в [записи выпуска](release-0.10.0.md).
Неисполненные UI observations и внешние AIF tasks остаются открытыми;
публикация binary не подменяет их приёмку и не закрывает весь change.

## Срез 4 — синхронизация и внешние ворота, 2026-09-06

Дельта и main specs сведены двусторонне: там, где main был точнее
(алгоритм build key `b1`, `prifly-build-provenance/1`, «а не тихой заменой»),
основой взят main, а из дельты дописана недостающая конкретика. Все 26 блоков
дельты совпадают с main байт-в-байт; архивация, прогнанная в песочнице, даёт
`specsUpdated: false` и нулевые added/modified/removed, то есть перенос ничего
не потеряет и не продублирует.

Устранено противоречие спека и кода: `run-decisions` больше не утверждает про
предответ владельца «выбор сделал человек, только раньше». Источник `actor`
означает происхождение ответа, а не доказанное присутствие отдельного
человека: при общем OS principal local owner и агент неотличимы. Формулировка
согласована с runner (`cmd/prifly/project.go:159-161`) и словарём. Правка
внесена и в дельту, чтобы change документировал её сам.

Вынужденная правка вне этого change: два блока MODIFIED в
`add-native-host-question-ux`. После синхронизации main получил сценарии,
которых в них нет, и `validate --strict` падал; кроме того его проза, будучи
применённой после нейтрального change, вернула бы обязательные Git и host
roots. Прочего в том change не менялось. Отдельно зафиксировано
предсуществующее расхождение, не вызванное этой синхронизацией: блок
«Политика отсутствия человека…» в `add-run-decision-catalog` лежит под ADDED,
хотя требование уже есть в main, и при архивации даст `already exists`.

Внешние ворота задачи 4.4 прогнаны против опубликованного кандидата 0.10.0
(`prifly 0.10.0`, установка с `releases/latest/download`): в
`prifly-aif-workflows` `test_versions.py`, `test_folders.py`, `verify.py` и
новый `compatibility.py` — все четыре зелёные. `verify.py` до этого падал по
двум причинам, обе не дефекты движка: нейтральный `project init` больше не
записывает `hosts`, из-за чего `project compile --host` отказывал
`project_compile_unknown_host`, и анкета перешла на `project-questionnaire/3`,
где `preflight` перечисляет только применимые к выбранному profile решения.
Счётчики заменены картой применимости по всем девяти решениям, обращения по
индексу — обращениями по `id`; перевёрнутый ассерт про `improve_apply`
сохранён. Правки отревьюены владельцем того репозитория и зафиксированы в `e263720`
с зелёным CI; он дополнительно проверил границу собственной мутацией — удалил
`gate_warnings` из каталога решений, и `verify.py` упал, назвав недостающее
решение. Пакет при этом не менялся: `v1.10.0` остаётся выпущенным. Задачи 4.1
и 4.4 этим закрыты; 4.2 ждёт ответа о том, где живут прикладные указания,
удалённые из generic runner.

Проверки этого среза: `openspec validate --all --strict --no-interactive`
20 passed 0 failed; `git diff --check` чист; `git diff --name-only 5b5c4ca --
openspec/changes/archive` пуст; `TestGlossaryBindings` PASS;
`make schemas-check` 46/46. Go-код не менялся, поэтому обязательство 5.1
адресных тестов к этому срезу не применяется. Формальные P1/P2 gates,
deferred backlog, живые UI observations (3.5) и живой AIF pilot (4.3)
остаются незакрытыми.

## Правка контракта анкеты после среза 4 — 2026-09-06

Пилот нашёл в `project-questionnaire/3` три списка решений, где идентификатор
назван по-разному: `id` в `preflight` и `runtime` (поле опубликованного DTO
`run-decision/1`) и `decision_id` в `decision_states`. Разбор по `.id` давал
девять `null` вместо отказа — тихая пустота вместо ошибки, тот же класс, что
`request_digest` квитанции против `pending_request_digest`. Наше собственное
письмо к релизу вело в эту яму: оно советовало читать по `id` и одновременно
называло `decision_states` источником полного перечня.

Приведено к одному имени `id` с повышением версии: `project-questionnaire/4`
и `project-launch-summary/3` — структура состояний решений встроена в оба
ответа. `decision_id` не оставлен рядом: два имени под одной версией дают два
неразличимых по объявлению документа, а старый читатель должен падать на
проверке версии, а не молча получать пустые значения. Решение согласовано с
владельцем пакета, единственным известным потребителем этого поля.

Текст раннера называет версию сводки, поэтому его рендер изменился. Прежний
рендер заморожен цепочкой вперёд от `BeforeTiming`, добавлен седьмым в
`projectKnownRunnerSkills`, три digest-пина обновлены. Проверено вживую, а не
только тестом: бинарник с `6b0e2f9` установил раннер со сводкой `/2`, новый
бинарник заменил его через `project runners update` на всех трёх хостах.

Контракт анкеты до этого не был нормирован ни одним требованием — жил в коде и
словаре. Добавлено требование «Анкета называет решение одним именем во всех
списках» одинаково в `cli-protocol` и в дельту. Новая проверка
`questionnaireIdentifiersAgree` читает опубликованный JSON, а не поля Go:
прежние тесты ходили через структуру и расхождения имён увидеть не могли —
именно поэтому дефект дожил до пилота. Негативно проверена возвратом тега.

Ворота: `go test ./cmd/prifly` PASS; `TestGlossaryBindings` PASS;
`openspec validate --all --strict --no-interactive` 20 passed 0 failed;
`make schemas-check` 46/46; `git diff --check` чист; `gofmt -l` пуст.
Изменение затрагивает выпущенный контракт, поэтому пилот и пакет получат его
только со следующим релизом; версия релиза согласовывается отдельно.

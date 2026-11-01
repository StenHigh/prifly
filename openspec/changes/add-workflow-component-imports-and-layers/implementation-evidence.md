# Core implementation evidence

Change реализует compile-time composition. Он не меняет runtime operators,
сохранённые Runs, старые schemas или реальные sources smsplace/backend.
`prifly-run` для разработки не использовался.

## Проверки поведения

1. `TestProjectComposition*` проверяет imports, namespaces, owned exports,
   nested checks/contexts, single/diamond parents, settings, profiles, conflicts,
   inert staging и фактические compile/prepare/start JSON contracts.
2. `TestProjectBuildDeterminismAndDataBoundaries` сохраняет точный старый b1 vector.
3. Baseline HEAD `94b7eb2ccad4828b9e9db363ce2236f6427b4e5f` собран отдельно.
   `baseline-refusals.json` записывает четыре отказа нового syntax без effects.
4. По `protected-files.json` проверены 777 frozen schema/history hashes.
   Additive editor manifest entries не относятся к frozen schemas.
5. Public CLI harness выполняет required/not-required/unknown для sequence,
   parallel и nested typed Choice. Дополнительно он проверяет partial,
   unavailable, default/unknown Choice, freshness, limits, consent,
   all/wait, cancellation, candidate recheck и continue.

```text
source composition → native validation → reviewed prepare → one Run/invocation tree
```

Выполненные команды и воспроизводимый итоговый прогон:

```sh
go test ./cmd/prifly -run 'TestProjectComposition|TestEngineNamesNoWorkflowOfItsOwn|TestProjectBuildDeterminismAndDataBoundaries' -count=1
go test ./internal/runtime -run 'TestGlossaryBindings|TestEveryDeclaredCapabilityIsInTheAuthorIndex' -count=1
python3 -B -m unittest discover -s scripts -p test_light_projection.py
python3 scripts/check-schema.py --go /home/dev/.local/bin/go
openspec validate add-workflow-component-imports-and-layers --strict --no-interactive
git diff --check
go build -o /tmp/prifly-composition-cli ./cmd/prifly
python3 -B test/e2e/verify-workflow-composition.py --binary /tmp/prifly-composition-cli --report /tmp/prifly-composition-final-report.json
make ci-check-once GO=/tmp/prifly-go1.27.3/go/bin/go CHECK_CONTEXT=codex-local-linux-amd64-go1.27.3-cgo1
```

`make test-changed` прочитал все затронутые пакеты. Первый batch обнаружил
статические compiler-owned workflow IDs. Исправление использует namespace автора;
`TestEngineNamesNoWorkflowOfItsOwn` после исправления проходит. Итоговые полные
ворота проверяют исправленный snapshot. Их authoritative log/receipt создаёт
`ci-check-once` в `.cache/`, а не этот текст.

Проверка на установленном Go 1.27.2 остановилась на `vuln-check` с GO-2026-6809.
До неё прошли Go tests, vet, formatting, refusal guard и staticcheck для Linux/Darwin.
Исправленный source snapshot дополнительно проверяется временным Go 1.27.3.
Официальный archive проверен по SHA256 из `https://go.dev/dl/?mode=json`:
`65efbc80fdc1fa2cd2631cad5b481777a68e8725f34e89f50fc6ac714e379b40`.
`go.mod`, `.tools/go` и project pins не изменены. Security toolchain update
остаётся отдельной работой; этот change не скрывает отказ стандартного Go 1.27.2 gate.

Регрессионная проверка `TestProjectCompositionDoesNotResolveQualifiedRefsInsideData`
фиксирует data boundary: только top-level `refs` workflow/step разрешает import aliases.
Поле `refs` в JSON Schema, configuration default или другом instance data сохраняет своё значение.

## Сохранённые границы

Sequence accepted subtree переносится по существующим recovery rules.
Parallel в accepted prefix даёт `recover_topology_unsupported`; явный
`--from-stage extra-checks` допускает повтор всей группы. Этот change не
расширяет native recovery contract. Изменённый imported child не переносит старый PASS.

Resource record продолжает называть root stages. После переноса contribution
в группу прежний resource stage требует явного consumer override. Unknown stage
отказывает до effects. Compiler не расширяет resource scope молча и не повышает limits.

Все CLI scenarios синтетические. Report не доказывает реальный browser lifecycle,
Git commit, внешний MR, остановку внешнего actor или published-binary qualification.
Релиз Core, package release и пилот smsplace остаются разными gates.

Итоговый `ci-check-once` на Go 1.27.3 завершился с exit 0. Log: `/tmp/prifly-composition-final-ci.log`.
Полный CLI harness также завершился с exit 0: `/tmp/prifly-composition-verified-report.json`.
Он подтвердил 11 applicability routes, 11 refusals, 5 limit cases, 2 Choice controls,
2 join controls, unchanged/changed prefix, explicit parallel replay, candidate recheck и cancellation.
Environment использовал execute-only mode из-за unsupported tool symlink; reusable receipt не заявляется.
Commit/staging и release qualification не выполнялись. Перед commit требуется staged-bytes gate.

## Контекст интеграции после v0.13.83

Предыдущие команды и результаты выше относятся к сохранённому самостоятельному snapshot. Они не доказывают совместимость текущего объединённого snapshot. v0.13.83 уже обновил project toolchain до Go 1.27.3; прежний toolchain blocker больше не описывает актуальную базу.

Интеграция сохраняет опубликованные point `/4` и все прежние schemas. Composition `/3` явно отказывает на sidecar любого участвующего package и effective `/4` до flattening. Library import без sidecar игнорирует standalone provider `/4`. Семь новых негативных случаев сначала воспроизвели проблему; после исправления они проходят вместе с прежними points/composition tests.

Совместный candidate `42c94b32089fb71dabad18920463742d195079140f8b040d31baa55a71e17dec` прошёл оба полных public CLI harness. Points report содержит 11 cases и 12 Runs. Composition report содержит 11 routes, 11 refusals, 5 limits, 6 freshness cases, 2 joins, 2 Choice controls, 2 inherited-group cases и 5 technical-error cases. Replay, candidate recheck и cancellation также прошли. Отчёты: `/tmp/prifly-points-round3-native.json`, `/tmp/prifly-composition-round3-native.json`. Все сценарии синтетические.

Независимое ревью и regression-first проверки исправили origin supporting files слоёв, потерю inherited groups, повторные group IDs и origins, фактическую глубину groups и source DAG. Generated wrappers сохраняют technical failure без outcome. Внешний Call применяет объявленный `arrange.on_error`. Неизвестный Choice predicate и предметный rejected сохраняют отдельные маршруты.

Cache regression проверяет source depth 8/9 в обоих порядках parents и imports. Cached node хранит высоту своего полного dependency subtree. Обзор сохраняет объявленные parent names и import targets, даже когда несколько записей проекта используют одну folder. Одинаковая пара alias/target объединяется; другой объявленный target даёт typed отказ. Red/green logs: `/tmp/compose-cache-all-red.log`, `/tmp/compose-cache-green.log`, `/tmp/compose-cache-native-green.log`.

Candidate после cache fixes: `7b75077ba83fef5d62a6d650b578bc194fac3e5d6279372d577f411df82261cc`. Targeted CLI tests прошли за 13.097s. Для этого exact binary используются `/tmp/prifly-composition-checked-native.json` и `/tmp/prifly-points-checked-native.json`. Полные локальные ворота создают `/tmp/compose-checked-ci.log`; staged-bytes gate перед commit создаёт `/tmp/compose-staged-ci.log`. Authoritative receipt остаётся в `.cache/verification/success.json`.

Проверены 897 опубликованных schemas и historical files относительно `389808611e40f17d5bbcedebf4580e0db93eb06b`. Их bytes не изменены. Три additive inventory files сохраняют прежние записи и добавляют новые contracts. Исходные 53 файла, их modes и stash сохранены отдельно.

Release gates 8.1–8.2 для composition остаются открытыми. Этот локальный commit не означает published-binary qualification, release UI/UX-пакета или reviewed pilot smsplace.

## Проверка объявлений настроек по ancestry

Candidate `16306e166b3701b48664be43d781a306f2b41bbeef0d0020978ad66a4651f3b3` добавляет исправление наследования настроек. Resolver объединяет собственные объявления уникальных ancestors. Объявление потомка заменяет объявление своего ancestor. Разные объявления независимых родителей требуют решения consumer. Объекты заменяются целиком; пустой exclude явно очищает список. Правила refs и provenance settings сохраняются.

Regression-first проверки воспроизвели ложный конфликт в direct и diamond ancestry при обоих порядках родителей. Проверки охватывают resources, settings, preflight answers, runtime, model records, decision policy, package profiles, exclude и refs. Red log: `/tmp/compose-options-red.log`. Targeted tests текущего snapshot прошли за 15.562s: `/tmp/compose-ready-targeted.log`.

Текущий exact binary проверяется через native settings scenarios: `/tmp/compose-ready-settings-native.log`, `/tmp/prifly-composition-ready-settings.json`. Они проверяют оба порядка родителей, независимый явный сброс и решение consumer. Повтор полного points harness использует `/tmp/prifly-points-ready-native.json`. Полная composition matrix предыдущего candidate остаётся отдельным результатом; её bytes и результаты не приписываются новому binary. Полные ворота текущего source snapshot записываются в `/tmp/compose-ready-ci.log`; перед commit обязательна проверка staged bytes.

## Положительное владение программами при composition

Независимое ревью воспроизвело назначение программы provider чужому компоненту по совпавшему ID. До flattening каждый program теперь сохраняет допустимые document aliases. Source programs используют local и явно inherited inventory. Layer programs сохраняют только resolved targets. Runtime execution reader получает только выбранные собственные компоненты. Неизвестный owned target даёт typed `project_execution_invalid`; собственный невыбранный private program пропускается до чтения supporting files. `Name` и `Names` сохраняют dedup exact dependencies.

Red logs: `/tmp/compose-ownership-red.log`, `/tmp/compose-ownership-native-red.log`. Green tests проверяют чужие step/check, wrong kind, private collision с иными bytes/version, точный provider/check binding, inherited program и shared exact refs с одинаковой либо конфликтующей программой. Существующий layer-relative test также проходит. Log: `/tmp/compose-ownership-green.log`. Консервативный source fingerprint может содержать невыбранный private definition; это не включает его программу в исполняемую closure.

Предыдущий candidate `16306e166b3701b48664be43d781a306f2b41bbeef0d0020978ad66a4651f3b3` завершил полный local gate и strict OpenSpec validation: `/tmp/compose-ready-ci.log`, `/tmp/compose-ready-openspec.log`. Его native settings и полный points harness также прошли. Эти результаты не приписываются последующему ownership fix. Новый candidate и полный composition report фиксируются отдельно в `/tmp/prifly-composition-owned-native.json`; локальные gates используют `/tmp/compose-owned-ci.log`. Финальная проверка staged bytes остаётся обязательной.

Ownership candidate SHA256: `89b3fb217d849cd8542210b32e0c125466d1ffb2805344aa2d6b4389530eebce`. Все targeted composition/points/Call/build tests прошли за 8.573s: `/tmp/compose-owned-targeted.log`. Публичный CLI regression использует `/tmp/compose-ownership-native-green.log`.

Дополнительная regression сначала обнаружила потерю второго supporting path при identical shared binding: `/tmp/compose-shared-provenance-red.log`. Исправление сохраняет оба paths/hashes без повторного payload. Текущий candidate SHA256: `f831dc7f318cc72210e71ad197472dd0febaaf8a6e1bdf3eaf04375faadb423e`. Итоговые targeted tests: `/tmp/compose-owned-complete-targeted.log`. Полный composition report и local gate ниже относятся к этому candidate. Предварительные запуски candidate `89b3fb21` остановлены до завершения; PASS для них не заявляется.

## Повторное явное подключение layer

Оба полных native harness завершились успешно на exact candidate `f831dc7f318cc72210e71ad197472dd0febaaf8a6e1bdf3eaf04375faadb423e`. Composition report дополнительно содержит 4 settings cases и 2 program ownership cases: `/tmp/prifly-composition-owned-native.json`. Points report содержит 11 cases и 12 Runs: `/tmp/prifly-points-owned-native.json`. Replay, cancellation и остальные synthetic controls также прошли.

Следующее независимое ревью воспроизвело повторное явное подключение одного слоя у разных parents. Спецификация workflow/context требует отдельного отказа. Проверка global normalized layer inventory теперь выполняется до merge. Cached common ancestor не читает свои layers повторно. Library mode не читает standalone layers. Red/green: `/tmp/compose-explicit-layers-red.log`, `/tmp/compose-explicit-layers-green.log`. Сценарии проверяют обе очередности siblings, ancestor/descendant, normalized paths, settings-only и structural layers, common ancestor и оба порядка effective/library cache.

Последующий candidate с этим ограниченным guard проверяется отдельным native regression: `/tmp/compose-explicit-layers-native-green.log`. Полный composition report `f831dc7f` не заявляется результатом другого binary. Для окончательного source snapshot используются `/tmp/compose-layer-guard-ci.log` и итоговый staged-bytes gate. Release-only gates остаются открытыми.

Layer guard candidate SHA256: `9e148fdd6a3a357d26e266950dad4c5f1376bd26890a2734a326a40c3327ea79`. Targeted composition/points/Call/build tests прошли за 9.617s: `/tmp/compose-layer-guard-targeted.log`.

## Итоговый локальный снимок интеграции

Полные локальные gates для layer guard завершились с exit 0. Fingerprint: `5023720aac0deb967359eb33536486c5154adca57bdbb397b8ebb3dbe4208c91`. Log: `/tmp/compose-layer-guard-ci.log`. Strict OpenSpec: 40/40. Public CLI подтвердил два отказа на explicit sibling layers и successful common-ancestor Run: `/tmp/prifly-composition-explicit-layers-native.json`. Независимое заключительное ревью не содержит findings.

Окончательный commit включает эту отметку прогресса. Перед его созданием `make ci-check-once CHECK_FOR_COMMIT=1 CHECK_CONTEXT=codex-app-authorized` проверяет staged bytes с Go 1.27.3. Log: `/tmp/compose-staged-ci.log`. Authoritative receipt хранится в `.cache/verification/success.json`. Временный Run plan не входит в Git.

Интеграция сохраняет исходные 53 файла с modes и stash, а также 897 protected released файлов. Локальный `main` остаётся на опубликованном v0.13.83. Этот commit не закрывает release gates 8.1–8.2 и не означает продуктовую приёмку F2 или smsplace.

## 1. Контракты и воспроизводимый дефект

- [x] 1.1 Сверить версии StepDefinition, authoring, assisted session, state/read и затронутые readers с опубликованным inventory; сохранить точную карту в design и добавить regression нынешнего позднего ответа: ответ сохранён, результат отклонён по deadline. Проверка: адресный тест воспроизводит отказ без настоящего ожидания.
- [x] 1.2 Добавить новый authoring marker и versioned `session_limits` с конечным active default и бессрочным wait default; закреплять настройки в definition, не менять legacy compilation. Проверка: compiler tests для defaults, индивидуальных значений, новой revision, неверного executor, нуля, отрицательных чисел и переполнения; прежние fixture digests неизменны.
- [x] 1.3 Ввести необходимые новые session/state/read editions с сохраняемыми остатком, фазой ожидания, observations и поколением доставки; связать initial envelope, answer context и effective timing проверяемыми bytes. Проверка: round-trip/reopen, отказ stale или подменённой delivery и чтение old editions без переписывания frozen schemas.

## 2. Время и повторный допуск

- [x] 2.0 Закрепить исключительное использование claim за Run через versioned authority record и атомарную admission; проверять владельца и при legacy admissions, сохранить binding во время пауз и restart. Исправить release: запрет незавершённого/uncertain владельца, durable fence до cleanup, честное восстановление прерванного release. Проверка: два Run/один claim, capacity 1 после парковки, чужая/stale generation, legacy holders, cancel/release/admission race и сохранность checkout файлов.
- [x] 2.1 Реализовать закрытие активного интервала при допустимом вопросе и сохранение остатка; не допускать пополнения при ответе, automatic/preanswer, повторной команде, restart или rollback. Проверка: детерминированные переходы с управляемыми Observation, включая active expiry до вопроса и точную границу finite wait.
- [x] 2.2 При безопасной передаче управления парковать ту же Attempt: fence старую delivery, освободить execution slot и исключить её из executing parallelism, сохранив pins/outputs/claims. Проверка: capacity 1 позволяет другому Run работать; неизвестный in-flight effect даёт отказ до освобождения, второй singleton question не портит свою delivery.
- [x] 2.3 Разделить сохранение ответа и повторный admission через существующую очередь; выдавать новую delivery той же Attempt только после capacity, ControlPin, revocation и resource checks, с прежними inputs и остатком. Проверка: занятый slot не теряет ответ и не тратит бюджет; Pause/Stop и истёкшие права не обходятся, claim не продлевается и не освобождается автоматически.
- [x] 2.4 Убрать блокирование независимой работы и управляющих команд открытым вопросом. Проверка: sibling может завершиться, join ждёт припаркованную ветвь, cancellation/recovery доступны раньше idle/waiting_decision, новый вопрос не требуется для продвижения уже отвеченной Attempt.

## 3. Истечение, отмена и понятный интерфейс

- [x] 3.1 Обрабатывать active/wait expiry и cancel открытого вопроса с сохранением причины закрытия; отвергать late answer/result без hidden retry и не применять managed-only false ProcessOutcome к переданной host работе. Проверка: cancel/answer race, effects:none без обязательств завершается отменой, unknown effect остаётся в recovery; legacy deadline не получает продление.
- [x] 3.2 Показать в launch questionnaire/summary оба срока, defaults и контекст вопроса; различать «ответ сохранён» и «работа может продолжиться», объяснять capacity/control/claim recovery допустимыми командами. Проверка: CLI snapshots и runner instruction tests, включая legacy Run, не повторяют известный ответ и не обещают auto-wakeup или физическую остановку host.

## 4. Сквозная проверка и согласование документации

- [x] 4.1 Проверить составным regression: runtime через 10 минут работы, вопрос, две недели управляемого времени, reopen, ответ, допустимое продолжение с 50 минутами и exact bytes принятого artifact; существующий public CLI mixed — реальные managed→assisted→managed, те же yield/answer/readmission и exact итоговый отчёт. Проверка: прежние Run/Attempt, реальные admitted transitions и outputs, без настоящего долгого ожидания, подмены snapshot или нового полного бюджета. Дополнить отказ при неподтверждённом Git claim. Не выдавать два отдельных доказательства за один многонедельный mixed/OS Run.
- [x] 4.2 Выполнить один короткий public CLI mixed Run на named source binary с человеческим ответом и итоговой программой; сохранить версии, digest, точный итог и наблюдённые границы в этой записи. Проверка: повторное чтение completed/succeeded и экспорт ожидаемого отчёта; native UI обоих hosts и недели реального времени не объявлять проверенными.
- [x] 4.3 Выполнить адресные Go tests затронутых packages, race-проверки переходов, `make schemas-check`, `git diff --check` и `openspec validate --all --strict --no-interactive`; записать точные counts и результаты. Полные `make check` и `make e2e` выполнить один раз на общем candidate при завершении `make-project-launch-workflow-neutral`, не повторять на каждом срезе. Проверка защиты: `git diff f1c9ebd85a971ab0394ea19f49e089a666dc5864 -- openspec/changes/archive` пуст; old public bundles проверены в 1.3.
- [x] 4.4 Синхронизировать реализованные delta specs, glossary bindings, полный YAML reference, examples и roadmap; запустить `TestGlossaryBindings` при изменении карты и editor/reference checks для новых YAML полей. Проверка: документация разделяет готовую возможность, сохранённые legacy сроки и незакрытые UI/AIF gates; абзац «Что в этот срез не входит» явно оставляет managed process >1 часа, auto-wakeup, автоматическое обновление claims, Release и обновление полигона вне этой работы. Коммит с объяснением причин и push без переписывания истории.

## Planning verification — 2026-09-06

Подготовлены proposal, пять delta specs, design и 13 задач реализации.
`openspec validate --all --strict --no-interactive`: 20 passed, 0 failed.
`git diff --check`: pass; historical archive относительно `f1c9ebd` не изменён.
Реализовано 0/13: план и проверка документов не означают исправление runtime.

Что в этот срез не входит: код, новые бинарники, миграция старых Runs,
Release, полигон и product qualification. Дорогие code/CI gates для этого
docs-only planning среза не запускаются; результаты прежнего короткого
живого теста и его ограничения сохранены в design.

## Implementation checkpoint — 2026-09-06

Выполнено 1/13. `TestLegacyAssistedDecisionAcceptsLateAnswerButRejectsResult`
через обычные API, настоящий SQLite и `testing/synctest` проводит 10 минут
работы, две недели ожидания и restart. Старый ответ сохраняется, deadline не
меняется, поздний результат отклоняется. `TestLegacyAssistedRunsShareInstallationClaim`
подтверждает отдельный пробел: два разных Start при capacity 2 одновременно
получают один checkout claim. Ни один тест не объявляет эти дефекты нормой
для нового контракта.

Проверка двух diagnostic regression: 2/2 PASS, package 1.455 s. Команда:
`GOCACHE=/private/tmp/prifly-neutral-go-build GOTOOLCHAIN=local .tools/go/bin/go test ./internal/runtime -run '^TestLegacyAssisted(DecisionAcceptsLateAnswerButRejectsResult|RunsShareInstallationClaim)$' -count=1 -timeout=45s -v`.
OpenSpec strict all: 20 passed, 0 failed; `git diff --check`: pass.

Parking/admission остановлены по правилу OpenSpec apply: выявлен пропуск
дизайна, запрошено согласование защиты рабочей копии (подробности в design).
Задача 1.2 подготовлена локальным незакоммиченным черновиком в flow authoring,
project compiler и editor sources; runtime сроки ещё не реализованы. В том
же черновике общий `validateAssistedStep` явно отказывает
`unsupported_session_limits`, чтобы новый YAML не исполнялся со скрытым старым
часом. Проверка этого guard вместе с двумя regression: 3/3 PASS, 1.507 s.
Mirror schemas, документация authoring и runtime integration ещё не завершены,
поэтому 1.2 не отмечена выполненной и черновик не публикуется как capability.

Адресная проверка authoring-черновика: 8 top-level tests и 24 named subtests
PASS, package 0.525 s; дополнительно 9 table checks внутри editor test.
Команда: `GOCACHE=/private/tmp/prifly-neutral-go-build GOTOOLCHAIN=local .tools/go/bin/go test ./internal/flow -run '^(TestSessionLimits|TestConciseStepYAMLLowersSafeDefaults|TestStepAuthoringRejectsUnsafeSurface|TestWorkspaceTreeAuthoringAndValidation)' -count=1 -v`.
Project/b1 revision regression и frozen fixture-digest gate ещё не выполнены.

Что в этот срез не входит: исправление ожидания/отмены/claim exclusivity,
новые session/state editions, успешный двухнедельный timed Run, Release,
обновление установленного бинарника и полигона. Коммит этого checkpoint
содержит только diagnostic tests и запись зависимости; незавершённый authoring
остаётся в рабочем дереве для продолжения после решения владельца.

## Approved continuation — 2026-09-06

Владелец подтвердил защиту рабочей папки. Добавлена task 2.0; текущий объём
1/14, прежние counts выше относятся к своим checkpoints. Причина остановки
снята, продолжается apply того же change. Истёкший lease по-прежнему не
освобождает claim; автоматический filesystem cleanup не добавлен.


## Implementation complete — 2026-09-06

Выполнено 14/14 задач этого change. Новый `prifly-step/2` закрепляет
`session_limits` в StepDefinition/6; прежний /1 компилируется без смены
контрактов и digest. Новый шаг по умолчанию имеет час активной работы,
но объявленный вопрос не ограничен сроком. Это не глобальный час на workflow:
после ответа используется только сохранённый остаток той же Attempt.
Новые editions: state/read/27, assisted-session/6, request/record/2,
launch-summary/2; старые сохранённые Runs не мигрируются.

Парковка атомарно освобождает execution slot и закрывает прежнюю delivery,
но не освобождает рабочую папку. Ответ сначала сохраняется; отдельный drive
проверяет ограничения, отзыв пакета и права на claim, затем выдаёт новую
версию доставки. Claim связан с одним Run в authority-claims/3; release
сначала сохраняет запрет нового допуска и лишь затем выполняет cleanup.
Неопределённые изменения рабочей папки сохраняются для recovery, а не
выдаются за отменённый незапущенный процесс. Новых сервисов и зависимостей нет.

### Проверки реализации

- Основная runtime-выборка: 29 top-level tests и 59 named subtests PASS,
  0 failed, package 14.627 s; включая TestGlossaryBindings с 8 subtests.
  Команда: `go test ./internal/runtime -run '^(TestGlossaryBindings|TestTimed|TestTiming|TestSessionTiming|TestDecisionYield|TestLegacyAssisted)' -count=1 -timeout=90s -json`.
- Основная race-выборка: 22 top-level tests и 51 named subtests PASS,
  0 failed, package 62.576 s. Команда: `go test -race ./internal/runtime -run '^(TestTimed|TestTimingWire|TestTimingState|TestSessionTiming|TestClaim|TestLegacyHandedClaim|TestResolveParkedTimed)' -count=1 -timeout=120s -json`.
- Добавленный затем regression реального отзыва пакета:
  `TestTimedDecisionResumeRechecksRevokedPackage` — 1/1 PASS, package 1.731 s;
  отдельно с `-race` — 1/1 PASS, package 5.324 s. Ответ принят, но
  продолжение получает `package_revoked` без изменения Run, занятия слота,
  новой delivery или пополнения оставшихся 50 минут.
- Финальное ревью: `TestCancelledTimedSessionSettlesSavedReport` и
  `TestMixedTimedRunKeepsLegacyDecisionVisible` — 2/2 PASS, package 2.708 s;
  та же выборка с `-race` — 2/2 PASS, 6.418 s. Сохранённый report завершается
  при отмене, а вопрос legacy /5 внутри Run /27 остаётся доступен человеку.
  После этих правок оба public CLI mixed tests повторены: 2/2 PASS, 3.767 s.
- Финальная claim-выборка с `TestClaimReleaseCannotOverlapDriverPreparation`:
  7/7 PASS, package 3.700 s; та же выборка с `-race` — 7/7 PASS, 7.758 s.
  Release под блокировкой драйвера отказывает без изменения claim и checkout;
  после освобождения блокировки та же команда успешно завершает release.
- Компилятор project/b1: 2 tests и 12 vectors PASS, 1.212 s; flow
  compatibility-выборка: 3 tests PASS, 0.608 s. Authoring boundary-выборка
  ранее: 8 tests и 24 named subtests PASS, 0.525 s; editor defaults —
  9 table checks. Editor/reference check: 1/1 PASS, 0.006 s.
- Public CLI mixed: timed и legacy варианты, 2/2 PASS, package 4.444 s.
  Реальные native parse → assisted answer → native report, без Git;
  точный итог `Total: 48`. Runner pins и revision checks: 3/3 PASS,
  0.888 s. Сгенерированный runner прошёл skill-creator quick_validate.
- Заключительная анкета/runner-выборка: 5 top-level tests и 3 named host
  subtests PASS, 0 failed, package 1.218 s. Команда:
  `go test ./cmd/prifly -run '^(TestCLIProjectSessionLimitsPrepareShowsPinnedPolicies|TestProjectCurrentRunnerIsWorkflowNeutral|TestProjectFrozenRunnerTextIsPinned|TestProjectRunnerTextIsPinned|TestProjectRunnerUpdateReplacesEveryReleasedRunner)$' -count=1 -timeout=60s -json`.
- `make schemas-check`: PASS, 46 schema profiles; прежние public bundles
  неизменны. Новый timed-session bundle: 232943 bytes,
  SHA-256 `7f2174a237c62dc4dd25adabbb6efd3b0eb8fc986e6386cd90b2c0c6164de4a5`;
  StepDefinitionV6: 9853 bytes,
  SHA-256 `9759c5a010382c0362eaca77849206d19240042af5c3c1d17602996cb26c6707`.
- `openspec validate --specs --strict --no-interactive`: 16 passed,
  0 failed; `openspec validate --all --strict --no-interactive`:
  20 passed, 0 failed. Пять delta specs синхронизированы в main specs;
  glossary, YAML reference, authoring inventory и roadmap согласованы.
- `git diff --check`: PASS. Diff исторического archive относительно
  `f1c9ebd85a971ab0394ea19f49e089a666dc5864` пуст.

Все Go-команды использовали Go 1.27.0, `GOTOOLCHAIN=local` и общий
`GOCACHE=/private/tmp/prifly-neutral-go-build`. Counts разных выборок
пересекаются: они не складываются в число уникальных тестов.

### Длительное ожидание и короткий живой запуск

Runtime regression проходит 10 минут работы, открытый вопрос, две недели
управляемого времени, закрытие и повторное открытие Engine, ответ и ещё
49 минут работы с сохранённым 50-минутным остатком. Принимаются точные bytes
результата той же Attempt. Отдельный вариант с workspace_write после двух
недель честно отказывает в продолжении из-за истёкшего claim, не теряя ответ.
Другой regression проверяет длительное waiting_admission и capacity 1;
parallel regression — работу sibling и ожидание join. Это реальные API и
SQLite transitions; сохранённые snapshots и timestamps не подменяются.

Один смешанный OS workflow внутри testing/synctest не удалось использовать:
живой listener удерживал fake time, тест остановлен своим 60-секундным
лимитом. Вместо нового production clock seam использованы два отдельных
доказательства: runtime с управляемыми неделями и настоящий короткий mixed
через CLI. Они не называются единым многонедельным mixed запуском.

Короткий живой тест завершён на darwin/arm64 source binary:
SHA-256 `a33bd9bfe5e00a105f16ea1f73cf0e048463bbd78797005025f9010284407cd3`,
HEAD `fd9b0627ff5ba6a1f053ca40cd6e606e63a24fc8` плюс source diff на момент сборки.
Список 314 source/module файлов до и после сборки совпал, его SHA-256
`01fb3c1d1372600e807090aec5fb85d2d1b592cfcfedd586145a68cadb447e41`.
Run `run:a21717ba2ee9eb2b75846e8a3e3d4737db25229a5e5902801c1bad119ec2aae8`
повторно прочитан отдельным CLI: completed/succeeded, 3 Attempt,
0 diagnostics, Next terminal, claims []. Host использовал ранее согласованный
ответ 2, не выдавая его за новый вопрос в native UI. Остаток 3583997 ms
совпадает при парковке, после ответа и drive; delivery 1 → 2 принадлежит
той же Attempt и тем же inputs. Native report экспортировал 37 bytes:

```text
Pri-Fly CSV report
Rows: 3
Total: 48
```

SHA-256 отчёта:
`534dfc0d3202e8fd64080aaa1d06e8ef9695fabf1201abf602d1fafd485d46ab`.
Локальный transcript и build manifest сохранены в
`/private/tmp/prifly-live-timed.V2ttR5/`; эта временная копия не заменяет
воспроизводимые тесты и приведённую здесь запись. Первая подготовка Start
попала под запрет Unix socket песочницы, а последующий drive — за её
10-секундный managed admission deadline. Тот Run сохранён как rejected,
process_outcome.started=false. Успешный Run создан отдельной явной командой
полностью вне песочницы; неудачная подготовка не скрыта и не исправлялась
изменением authority.

После этой живой сборки финальное ревью исправило два пограничных пути в
driver/next (отмена после сохранения report и legacy-вопрос в mixed Run),
а также сериализацию release с подготовкой рабочей папки. Эти дополнения
проверены адресными regression/race tests; ранее собранный бинарник не
выдаётся за сборку с этими последними изменениями.

Отдельно найден прежний дефект `prepareWorkspaceTrees`: возвращаемый cleanup
input tree использует уже закрытый `os.Root`, поэтому при отказе допуска
подготовленные файлы могут остаться. Он не создаёт обход новой claim
exclusivity и не исправлялся вместе со сроками; самостоятельная задача
`workspace-tree-preparation-rollback` добавлена в текущий roadmap.

Что в этот срез не входит: managed OS process дольше часа, auto-wakeup или
автоматический запуск host, автоматическое продление/восстановление claim,
новая квалификация native Codex/Claude UI и AIF, Release, обновление
установленной программы и пользовательского полигона. Полные `make check`
и `make e2e` оставлены для общего candidate
`make-project-launch-workflow-neutral`. Новое поведение включается явно
в YAML; исторические Runs и release evidence не переписываются.

# План исправления #15: сохранить версию summary при именовании worktree

Ветка: `prifly/prifly-aif-classic`
Дата: 2026-11-02
Исходный commit: `2060fc9286cff2924e198b0294ba9a2814ad0e90`

## Original Request

Исправить https://github.com/StenHigh/prifly/issues/15 в рамках опубликованных контрактов. На v0.13.85 composition /5 + worktree_naming меняет project-launch-summary/7 на /5 при повторной проверке start и ошибочно отказывает project_start_stale_launch. Сначала воспроизвести падающим integration test; затем сохранить требуемую версию summary. Проверить prepare+start с naming, настоящий named claim, legacy /4/named launch, реальный config/input/naming drift refusal. Не ослаблять digest, effects, consent или source freshness; не менять опубликованные схемы и historical evidence. Выполнить штатные локальные ворота и локальный commit. После завершения Run владелец поручил merge и выпуск нового Core, published replay; затем downstream release и consumer #9; затем принятие GitLab Draft MR !1417 в staging. Не включать downstream или private код в Core commit.

## Настройки

- Профиль: `full` из `decision_sheet.package_profile` и `core:package_profile`.
- Тесты: да (`plan_tests=true`). Сначала новая падающая регрессия.
- Логирование: `minimal`. CLI сохраняет один JSON-документ и штатный Problem-конверт.
- Документация: нет (`plan_docs=false`). Новая capability и новый контракт не появляются.
- Ограничения: исправить только #15 внутри опубликованных контрактов. Сохранить настоящие freshness-отказы.
- Политика предупреждений ворот: `ignore`. Ошибки проверок остаются блокирующими.
- Реализация и локальный commit принадлежат следующим native шагам Run.
- Merge, release, published replay, downstream, consumer #9 и GitLab MR !1417 выполняются после Run по отдельному поручению владельца.
- Claim владеет веткой. План не создаёт ветку и не запускает push.

## Roadmap Linkage

Milestone: `none`.
Это исправление существующего контракта. Оно не закрывает продуктовую приёмку или этап delivery-roadmap.
Источник: `openspec/specs/delivery-roadmap/spec.md`; файл остаётся read-only.

## Requirements Reconciliation

Приоритет: названный OpenSpec change → текущие source sets → контекст проекта.
Task не называет change. Исправление восстанавливает существующий смысл; новый OpenSpec change не требуется.
Task и [issue #15](https://github.com/StenHigh/prifly/issues/15) согласуются с текущими спецификациями.
Research отсутствует и не влияет на план.

| Правило или сочетание | Источник | Проверка |
|---|---|---|
| Unified folder `/3` и extension `/5` дают compile `/4`, summary `/7`, start `/7` | `openspec/specs/cli-protocol/spec.md`, «Unified compiler exposes versioned source and point provenance» | Native prepare/start; фактический JSON проходит `ProjectLaunchSummaryV7` и `ProjectStartV7` |
| Profile `/4` наследует остальные возможности и добавляет naming | `openspec/specs/workflow-and-context/spec.md`, «Project объявляет общий шаблон имени disposable worktree» | Unified + naming создаёт настоящий claim с ожидаемыми path, branch и naming metadata |
| Prepare не резервирует имя; занятость кандидата не меняет согласованную политику | `openspec/specs/cli-protocol/spec.md`, «Prepare показывает пространство имён без reservation» | Два prepare дают один digest и `reserved:false`; следующий start выбирает допустимый suffix |
| Изменение naming declaration или разрешённого input запрещает старый review до effects | Тот же раздел; «Unified review remains bound to exact prepared inputs» | Негативные integration cases проверяют `project_start_stale_launch`, отсутствие нового Run и claim |
| Повторная проверка замечает machine-local config drift после публикации summary | `cmd/prifly/project_start.go`, блок повторного review; `project_launch_summary_test.go` | Writer hook меняет naming или config после публикации; start отказывает до claim |
| Legacy extension `/4`, обычный named launch и named handover сохраняют версии и смысл | `openspec/specs/published-contracts/spec.md`, naming version boundary и additive closed schemas | Existing legacy/unified/named tests; legacy named остаётся `/5`, unified без naming остаётся `/7` |
| Digest, effects, consent и source freshness остаются обязательными | Task input; `openspec/specs/cli-protocol/spec.md`, exact prepared inputs | Реальные input/source/config изменения отказывают; allow-execution не обходится |

Карта владения: `openspec/SOURCE-OF-TRUTH.md`, разделы naming и unified composition.
Specs, сохранённые JSON-поля, схемы и historical evidence не требуют изменений.

## Причина и граница исправления

`projectPrepareAndStart` сначала выбирает версию summary по доступным metadata.
Composition повышает её до `/6` либо `/7`.
Повторный review копирует summary, затем naming ошибочно понижает версию до `/5`.
`projectCallExtensionReview` хеширует версию вместе с остальными полями.
Поэтому неизменённый launch получает ложный `project_start_stale_launch`.

```text
prepare → summary /7 → start → повторное naming → сохранить /7 → сравнить digest → claim
                                              ↘ настоящий drift → typed refusal
```

Минимальная правка удаляет только понижение версии после повторного разрешения naming.
Повторное чтение профиля, вычисление naming и сравнение digest остаются на своих местах.
Advisory preview остаётся вне digest.
Существующий handover использует записанный claim и не вычисляет новое имя.

## Задачи

### Этап 1. Доказать регрессию

- [ ] Task 1: Добавить native integration test сочетания unified composition и naming.

Файл: `cmd/prifly/project_unified_test.go`.
Основа: `writeUnifiedImportFixture`, `TestProjectUnifiedEmitsClosedCompileAndLaunchViews` и `validateUnifiedView`.
Сохранить representative imported point/public Call из fixture, который использует опубликованный пример `examples/nested-extension-points`.
Включить profile `/4`, folder `/3`, extension `/5`, host `codex-app` и workspace `worktree`.
Добавить `worktree_naming` с проверяемыми bindings и ожидаемой основой имени.
Prepare должен возвращать `/7`, naming и preview без reservation.
Проверить стабильный digest повторного prepare и отсутствие claims до start.
Start с тем же digest должен создавать Run и настоящий named claim.
Проверить реальный каталог, ветку `prifly/<base>` и naming metadata claim.
Проверить реальные JSON через `ProjectLaunchSummaryV7` и `ProjectStartV7`.
Без исправления этот тест должен падать на ложном stale refusal.
Сохранить точную команду и ненулевой результат до правки runtime-кода.
При успешном старте завершить fixture Run штатным cancel, как существующий тест.
Новый безопасный тест использует `t.Parallel()`; при process environment mutation объяснить исключение.
Логирование: только test diagnostics через `t.Fatalf`; production stdout не менять.
Зависимости: нет. Источники: первые три строки таблицы требований и issue #15.

### Этап 2. Исправить выбор версии

- [ ] Task 2: Сохранить ранее выбранную версию summary при повторном разрешении naming.

Файл: `cmd/prifly/project_start.go`, блок `currentSummary.WorktreeNaming` в `projectPrepareAndStart`.
Удалить присваивание `currentSummary.SchemaVersion = "project-launch-summary/5"` из повторной проверки.
Сохранить `readProjectProfile`, `projectResolveWorktreeNaming`, `projectCallExtensionReview` и проверку digest до claim.
Не копировать reviewed naming вместо повторного вычисления.
Не исключать SchemaVersion или naming policy из digest.
Не менять порядок effects, consent, imports, dispatch или сохранённые contracts.
Сначала повторить падавшую регрессию, затем общую целевую команду ниже.
Логирование: новые production logs не добавлять; штатный typed refusal сохраняет код.
Зависимости: Task 1 должен доказать красный результат до этой правки.

### Этап 3. Сохранить настоящие отказы и совместимость

- [ ] Task 3: Проверить freshness на сочетанном пути и границы прежних launches.

Файлы: `cmd/prifly/project_unified_test.go`, при необходимости `project_worktree_naming_test.go` и `project_launch_summary_test.go`.
Проверить изменения template, binding и разрешённого input после prepare.
Каждый случай должен давать `project_start_stale_launch` без нового Run, claim и worktree.
Проверить добавление, изменение и удаление naming после публикации summary внутри start.
Для этого использовать существующий `projectSummaryWriter`, без package-global hooks.
Хотя бы один post-publication config drift также должен отказывать на сочетанном пути.
Использовать реальные fixture bytes; не подменять digest сравнением с самим собой.
Existing tests сохраняют отказы для executable, supporting files, translations и resources.
Повторный command ID должен сохранять Run, claim, path и branch без расходования suffix.
Занятое первое имя допускает следующий suffix при прежнем review digest.
Unified без naming сохраняет `/7`; ordinary named launch сохраняет `/5`.
Legacy extension `/4` и recorded named handover сохраняют прежнее поведение.
Использовать имеющиеся baseline tests; не переписывать fixtures прежних опубликованных схем.
Логирование: test diagnostics именуют mutation и обнаруженный effect; production logs не добавлять.
Зависимости: Task 2. Источники: оставшиеся строки таблицы требований.

### Этап 4. Проверить итоговый снимок и передать native commit шагу

- [ ] Task 4: Выполнить целевые проверки и штатные ворота итогового снимка.

Файлы: изменённые `cmd/prifly/*_test.go`, `cmd/prifly/project_start.go` и этот план.
Во время реализации использовать targeted Go tests либо `make test-changed` / `make check-fast` по правилам AGENTS.
Полный batch выполнить один раз для итогового снимка перед commit с указанным ниже CHECK_CONTEXT.
После stage native commit шаг проверяет staged bytes через `CHECK_FOR_COMMIT=1` на том же снимке.
Reuse допустим только по правилам `ci-check-once`; новый input или failure требует нового успешного gate.
Native commit шаг создаёт один локальный Conventional Commit: `fix: preserve launch summary version with worktree naming`.
Эта planning сессия не выполняет implementation, commit, merge, push или release.
Логирование: сохранить полные результаты команд и exit status в evidence последующих шагов; не выдавать baseline за проверку исправления.
Зависимости: Tasks 1–3. Run заканчивается локальным commit на claim-ветке.

## Проверки

Команды выполняются из claimed workspace:
`/home/dev/.prifly/projects/prifly-core/.prifly/work/claims/prifly-aif-classic`.

Целевая команда уже проверена на исходном снимке. Новые тесты должны иметь префикс `TestProjectUnified`, чтобы она их включала.

```bash
go test ./cmd/prifly -run '^(TestProjectUnified.*|TestCLIProjectNamedWorktreePrepareStartAndFreshness|TestCLINamedClaimResumeKeepsNamesAfterPolicyRemoval|TestCLIProjectLaunchSummaryBeforeDispatch|TestProjectExtensionPointsProgramFilesAndResourceReviewAreSealed|TestTheReviewedSummaryCoversTheTableTheRunWillSeal)$' -count=1
```

Штатные final gates из `decision_context.gate_checks`:

```bash
make ci-check-once GO="$(go env GOROOT)/bin/go" CHECK_CONTEXT=codex-app-authorized
openspec validate --all --strict --no-interactive
git diff --check
```

`CHECK_CONTEXT=codex-app-authorized` обозначает фактический host permission profile.
Он не даёт дополнительных разрешений.
Final gate после реализации ещё не выполнен.
Race и полный e2e принадлежат отдельному release qualify на фиксированном SHA.

## Проверено при планировании

- Targeted baseline: PASS, `cmd/prifly`, 39.582s; сочетанный новый тест ещё не создан.
- Отдельные existing named/unified integration tests: PASS, 8.733s.
- OpenSpec strict: PASS, 40 passed, 0 failed.
- `git diff --check`: PASS на исходном снимке.
- Baseline full batch остановлен по указанию host. Это INTERRUPTED, не PASS.
- Полный batch требуется только на итоговом снимке перед native commit.

Число изменённых файлов и строк следует получать из текущего Git diff при review.
План не закрепляет статистику над подвижным HEAD.

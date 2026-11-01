## 1. Контракты и совместимость

- [x] 1.1 Зафиксировать JSON/authoring version matrix для новых markers и views. Проверка: таблица old/new outputs и negative schema cases.
- [x] 1.2 Сохранить baseline legacy b1 vectors и protected-file hashes. Проверка: неизменённые fixtures дают прежние bytes и contracts.
- [x] 1.3 Добавить первые red fixtures для imports, parents, layers и groups. Проверка: baseline CLI отвергает новые markers до effects.

## 2. Публичные exports и imports

- [x] 2.1 Добавить folder `/2` parser для exports и root imports. Проверка: valid workflow/step exports проходят; private/wrong-kind/unknown отказываются.
- [x] 2.2 Разрешать qualified aliases по объявленным project packages. Проверка: sibling без импорта, missing source и origin drift отказываются без fetch.
- [x] 2.3 Собирать reachable provider closure с schemas, contexts, checks и программами. Проверка: consumer работает без копий supporting source files.
- [x] 2.4 Изолировать provider namespaces и проверять dependency cycles/identity conflicts. Проверка: одинаковые short names допустимы; разные bytes одной identity отказываются.
- [x] 2.5 Разделить source imports и standalone provider extend. Проверка: nested library import работает, чужие overrides и launch preflight не исполняются.

## 3. Родители, общая база и слои

- [x] 3.1 Добавить single-base derived folder с собственной root/package identity. Проверка: двухуровневое наследование сохраняет source graphs и явные settings.
- [x] 3.2 Добавить `/3` extend и плоский ordered layers list. Проверка: порядок базы/layers/extend воспроизводим, nested/repeated layers отказываются.
- [x] 3.3 Реализовать ограниченную таблицу merge и source provenance. Проверка: scalar/object/list overrides совпадают с design, неизвестные keys отказываются.
- [x] 3.4 Разрешать layer/supporting paths только внутри repository. Проверка: shared путь работает; symlink, escape и превышение depth/count/bytes отказываются.
- [x] 3.5 Подключить общий delivery layer к двум synthetic consumers. Проверка: tests/MR workers существуют в одном provider source, оба графа используют их.
- [x] 3.6 Проверять structural conflicts и `workflow: root`. Проверка: duplicate stage, removed route и conflicting references называют оба source keys.
- [x] 3.7 Добавить multiple-parent `extends` и bounded ancestry DAG. Проверка: scalar/list формы читаются, self-cycle и unrelated/ambiguous bases отказываются.
- [x] 3.8 Разрешать общую exact основу и dedup по source provenance. Проверка: diamond fixture исполняет classic/tests/commit один раз.
- [x] 3.9 Объединять уникальные parent contributions до применения графовых вставок. Проверка: дополнения обоих parents сохраняются, независимые одинаковые имена не сливаются.
- [x] 3.10 Добавить явное разрешение parent setting conflicts. Проверка: без consumer override отказ, перестановка parents не выбирает другое значение.

### Последовательные, параллельные и условные дополнения

- [x] 3.11 Добавить `arrange` для contribution selectors на общем переходе. Проверка: missing/unknown/duplicate contributions и незаданный порядок отказываются.
- [x] 3.12 Понижать sequence в обычные стадии и child workflows. Проверка: tests один раз, затем оба дополнения по порядку, commit только после успеха.
- [x] 3.13 Понижать parallel в native branches и all/wait join. Проверка: обе проверки исполняются, первый PASS не завершает группу, failure не скрывает живую ветвь.
- [x] 3.14 Поддержать choice через native typed predicates. Проверка: parent добавляет развилку после tests без копии classic; default/unknown routes явные.
- [x] 3.15 Проверить вложенную группу с обязательной проверкой и альтернативами. Проверка: mandatory applicability исполняется всегда, optional branch следует predicates.
- [x] 3.16 Проверить scopes outputs и control outcomes групп. Проверка: sibling bindings и missing typed wrappers отказываются, partial child не становится успешной группой.
- [x] 3.17 Проверить whole-closure budgets, resources и cancellation групп. Проверка: нет скрытого повышения capacity или sequential fallback; обязательные ветви settled до публикации.

## 4. Native lowering и приёмка результата

- [x] 4.1 Добавить qualified step/Call extension targets с явным stage ID. Проверка: local v2 scope неизменён, imported targets используют обычные native stages.
- [x] 4.2 Добавить `/3` root-input и stage-output bindings. Проверка: task/implementation проходят без пересказа; missing/late/incompatible ports отказываются.
- [x] 4.3 Поддержать qualified project-scoped settings импортированного workflow. Проверка: schema/scope соблюдены; private settings и program overrides отказываются.
- [x] 4.4 Проверить shared Run, claim, resources, effects и whole-closure budgets. Проверка: invocation tree один; превышение допуска не создаёт скрытый Run.
- [x] 4.5 Проверить outcomes и гарантии exports. Проверка: partial/rejected не превращаются в success, success-only artifact недоступен на иных routes.

## 5. Читаемый обзор, freshness и update

- [x] 5.1 Добавить краткий composition overview и `project compile --explain`. Проверка: текст показывает parents, общий ancestor один раз, groups и settings без полного JSON.
- [x] 5.2 Опубликовать новые compile/prepare/provenance schemas. Проверка: реальные emitted fixtures проходят новые closed schemas, legacy outputs неизменны.
- [x] 5.3 Связать ancestry/contributions/groups/layers/imports/files с reviewed digest. Проверка: изменение parent, порядка sequence, parallel join или predicate вызывает stale-start до effects.
- [x] 5.4 Проверить provider update и сохранение local files. Проверка: consumer extend/shared layers не меняются, старый Run читает прежние pins.
- [x] 5.5 Проверить continue/recover для imported subtree. Проверка: unchanged prefix переносится; incompatible prefix отказывает либо допускает законный whole-child replay.
- [x] 5.6 Проверить read-only границу compile/prepare и диагностику. Проверка: нет programs/network/trust/Run/claim; Problem содержит typed code и source context.

## 6. Обязательная применимость и сквозной reference scenario

- [x] 6.1 Добавить synthetic applicability provider с required/not-required/unknown. Проверка: applicability исполняется ровно один раз на каждый вход в приёмку.
- [x] 6.2 Добавить smsplace-подобный consumer classic с delivery layer. Проверка: required выполняет reader; not-required сохраняет причину без reader Attempt.
- [x] 6.3 Покрыть unknown и недоступную обязательную проверку. Проверка: ни commit, ни synthetic publication не исполняются на этих путях.
- [x] 6.4 Покрыть интерфейсное изменение PHP и фактический candidate. Проверка: Core не использует filename эвристику, child получает исходную задачу и exact evidence.
- [x] 6.5 Покрыть repair/freshness и interruption. Проверка: новый candidate повторно проходит applicability; cancel не выдаёт cleanup или старый PASS.
- [x] 6.6 Запустить public CLI harness с вложенным program и shared delivery step. Проверка: один Run, explicit execution consent, scoped policy и воспроизводимый отчёт.
- [x] 6.7 Добавить два родительских workflow с общей smsplace-подобной базой. Проверка: consumer наследует UI/UX и аудит без копии classic, tests/MR не дублируются.
- [x] 6.8 Прогнать sequence/parallel/choice комбинации после tests на public CLI. Проверка: порядок/выбор виден в invocation tree, неуспешные обязательные checks запрещают commit/MR.

## 7. Документация и итоговая проверка Core

- [x] 7.1 Обновить glossary, brief и source map в том же implementation change. Проверка: `TestGlossaryBindings` и отсутствие второго нормативного source set.
- [x] 7.2 Обновить capability inventory и `examples/README.md`. Проверка: `TestEveryDeclaredCapabilityIsInTheAuthorIndex` проходит в обе стороны.
- [x] 7.3 Добавить reference и runnable примеры imports/layers, single/diamond parents и groups. Проверка: minimum Core version, порядок исполнения, conflicts и skip явно показаны.
- [x] 7.4 Проверить все deltas и whitespace. Команды: `openspec validate add-workflow-component-imports-and-layers --strict --no-interactive`, `git diff --check`.
- [x] 7.5 Проверить protected history и legacy schemas по baseline hashes 1.2. Проверка: нет изменений archive evidence, старых bundles и saved Run bytes.
- [x] 7.6 Выполнять `make test-changed` или `make check-fast` по затронутому поведению. Проверка: targeted tests и legacy vectors проходят.
- [x] 7.7 Пройти итоговые ворота один раз перед implementation commit. Команда: `make ci-check-once CHECK_CONTEXT=<actual-profile>`; Перед будущим commit staged bytes проверить с `CHECK_FOR_COMMIT=1`; старое поручение не создавало commit. Новое поручение после v0.13.83 разрешает один отдельный локальный commit.

## 8. Выпуск Core и отдельный handoff

Эти gates выполняются после implementation commit. Отсутствие ещё не выпущенного binary не блокирует commit и не считается успешным published replay.

- [ ] 8.1 Для exact release SHA пройти existing verify/qualify gates по AGENTS.md. Проверка: `ensure-release-checks.py --sha SHA --ref main --wait` подтверждает SHA.
- [ ] 8.2 Повторить generic CLI harness на опубликованном binary. Проверка: version/SHA256, diamond dedup, sequence/parallel/choice и три applicability routes; synthetic evidence помечено явно.
- [x] 8.3 Подготовить отдельное поручение workflows repository. Проверка: описаны public export, task/candidate correlation, unknown/skip, browser lifecycle и package qualification.

Реализацию можно вести по этапам: imports/один parent, затем multiple parents/groups. До выполнения 3.7–3.17 и 6.7–6.8 полная capability не считается готовой. Неподдержанный syntax должен явно отказываться, а промежуточный опубликованный subset — сохранять честную version boundary.

Внешние этапы не входят в Core checklist: изменение и выпуск UI/UX-пакета, затем reviewed pilot в smsplace. Они требуют отдельных поручений владельца. Этот change не разрешает их выполнять автоматически.

## 9. Интеграция с выпущенным v0.13.83

- [x] 9.1 Восстановить change и уточнить нормативную границу `/3` и `/4` до изменения кода.
- [x] 9.2 Разрешить пересечения parser/compiler; явно отвергать mixed effective forms и sidecar до flattening. Library import без sidecar игнорирует standalone `/4`.
- [x] 9.3 Сохранить обе серии regression tests, published schemas, identity и additive inventories. Проверить mixed refusals, ownership и execution consent.
- [x] 9.4 Запустить оба native harness на одном candidate, подключить composition к будущему release e2e и проверить path selection.
- [x] 9.5 Пройти итоговые локальные gates, security/review/challenge/verify и создать один локальный commit без push, merge или release. Исходный worktree и stash сохранить.

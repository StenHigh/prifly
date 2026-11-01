# Техническая проверка кандидата aif-classic

Кандидат сохраняет поведение исходника. После отдельного package fix итоговые локальные ворота прошли; исходный failure сохранён ниже.
Человеческая приёмка завершена по **review владельца**: после «позже» получен ответ «Принимай».
Владелец явно разрешил критерии без обязательных ручных замеров. Все три части приняты; reader pilot не выполнялся.
Эта запись не утверждает улучшение скорости чтения и не закрывает продуктовые gates roadmap.

## Закреплённые входы

| Вход | Значение |
|---|---|
| Pri-Fly repository baseline | `6de187d22435447c464b9b41269cd8b2aec50d2e` |
| Repository workflows | `v1.53.0`, `c85306050df060dc4b381ba7512173806250b3a6` |
| aif-classic | `1.47.0` |
| Проверенный latest stable CLI | `0.13.81`, darwin/arm64; собран Go 1.27.2 |
| Development Go | `1.27.3` |
| CLI SHA-256 | `bdbca429a41be9673e9d7db6c979302f086a6b507291e81118d314d42d09b66f` |
| Release archive SHA-256 | `5c9e120d5632a8a44d51024606bbc3af3f22b8b43bda19680546ba3f81fd6f00` |
| Project extend SHA-256 | `44deb007e0faa23ec5d6a88b01bf01322c3805905d1a63cda9677e32378f45fe` |

Установленный `/Users/sh/.local/bin/prifly` побайтно совпал с binary официального release archive.
SHA-256 архива совпал с опубликованным manifest.
Проверки внешнего package выполнялись этим stable binary, отдельно от development tests.
Версии закреплены на момент начала эксперимента; последующие releases не меняют baseline.

Полный inventory файлов upstream и host skills находится в [baseline-manifest.json](evidence/baseline-manifest.json).
Для `codex-cli` использованы те же bytes `.agents/skills`, явно скопированные в его declared `.codex/skills`.
Для `codex-app` те же bytes лежали в declared `.agents/skills`.

## Установка baseline и local drift

Старый installed package 1.40.0 имел шесть изменённых upstream-файлов.
Владелец разрешил резервную копию и чистую установку последней версии.
Копия старого package и project profile сохранена до update:

```text
/Users/sh/PhpstormProjects/Pri-Fly/github/.cache/authoring-readability/backups/20261030-144418/
```

Native update установил release `v1.53.0`; повторная проверка вернула `up_to_date: true`.
Все canonical файлы совпадают с upstream; исключение — сохранённый project `extend.yaml`.
Hosts и launch сохранены. Подготовительный коммит — `6de187d`; он не является поставкой readability candidate.

Для эксперимента созданы отдельные baseline/candidate копии repository workflows.
Кандидат не установлен в tracked `.prifly/` Pri-Fly.

## Реальное изменение

[Точный product patch](handoff/aif-classic-readability.patch) меняет семь файлов:

1. `workflow.yaml` — блочная запись contracts, bindings, routes и limits.
2. `steps/verify.yaml` — блочная запись контракта verify.
3. `workflows/verify-once.yaml` — явные predicates, bindings и routes.
4. `workflows/verify-batch.yaml` — явные repeat boundaries и bindings.
5. `contexts/aif-verify-bridge.yaml` — `source` вместо `text`.
6. `instructions/aif-verify-bridge.md` — новый файл с прежними bytes.
7. `README.md` — карта маршрута и ссылки на исходники.

Раскрыты 64 flow mappings. Короткие sequences сохранены.
Parsed values всех четырёх YAML graphs равны исходным значениям.
Порядок `first_match`, captures, schema revisions, resumable и review-challenge сохранены.
Относительные README-ссылки разрешаются в существующие файлы.

Bridge содержит **9938 UTF-8 bytes**, включая завершающий LF.
SHA-256: `8772b0ef8121540cceb00cd912ba005ef434ed7fe467fde6e88913262a1fd24c`.
Identity `aif:context/aif-verify-bridge@1.16.0` и media type сохранены.
[Manifest handoff](evidence/handoff-manifest.json) фиксирует patch и hashes изменённых файлов.
`git apply --check` прошёл; после применения bytes всех семи файлов совпали с кандидатом.

```text
Baseline → блочный YAML + README + Markdown → exact compile → native route probes
                                                            ↓
                                                  owner review: принято; замеры не выполнялись
```

## Технические результаты

| Проверка | Результат | Граница доказательства |
|---|---|---|
| Upstream `tests/verify.py`, обе формы | PASS: 8 packages, 126 components, 83 steps | Compile gate; stub host skills; без Runs |
| Paired compile с реальными host skills | PASS: 12 пар | 2 hosts × 2 settings × 3 profiles |
| Identity и sealed bytes | Полное равенство | Package, build key, все component refs и bytes; нормализация не нужна |
| Upstream `tests/compatibility.py`, обе формы | PASS: 5 launches, 4 distinct builds | До первого assisted handoff; без ответа модели |
| Native verify routes, обе формы | Совпали все 8 случаев | Детерминированные ответы, реальные Runs и claim worktrees |
| Neutral `context.source` tests | PASS | Inline/source equality, CRLF/UTF-8, missing, traversal, внешняя symlink |
| Намеренная порча bytes | Ожидаемый FAIL | Guard обнаружил добавленный `drift`; правильные данные затем прошли |
| CSV sample, обе формы | PASS | Одинаковый build key, `succeeded` и точные report bytes |

[Все 12 build identities и component hashes](evidence/paired-builds.json) сохранены без удаления refs, schemas или digests.
Settings проверены раздельно: canonical `extend.yaml` и ваш сохранённый local `extend.yaml`.
Profiles: `fast`, `full`, `ultra`. Подмена capture layouts не выполнялась.

Результаты compatibility: [baseline](evidence/baseline-compatibility.json), [candidate](evidence/candidate-compatibility.json).
Neutral guard сначала дал ожидаемый FAIL на испорченном файле; [RED record](evidence/source-guard-red.json) сохраняет его причину.
Затем `go test ./cmd/prifly -run '^TestCLIProjectContextSource' -count=1` завершился с exit 0.
Production compiler и runtime не менялись.

## Матрица verify

| Случай | Наблюдение в обеих формах |
|---|---|
| owner-only | Оба blocking flags true; fix не вызван; root `partial` |
| exhausted | Лимит 1; один fix; повторного verify нет; root `partial` |
| disabled | Verify и fix не вызваны; implementation дошла до security и review-challenge |
| clean | Один verify; далее security и review-challenge |
| repaired | Verify → fix → verify; исправленный head прочитан повторным verify, security и review-challenge |
| fail | Root status `failed`, без outcome; [прежний дефект](handoff/existing-verify-outcome-gap.md) |
| no_work | Root status `failed`, без outcome; [прежний дефект](handoff/existing-verify-outcome-gap.md) |
| blocked | Fix не вызван; root `partial` |

[Machine-readable results](evidence/routes.json) содержат порядок bridges и проверки входов implementation.
В успешных путях проверка остановилась у review-challenge. Это не полный end-to-end development Run.
Все временные Runs завершены или отменены; созданные claims и monitor registrations освобождены.

## Короткая CSV-проба

Во временной копии раскрыты только `stages.parse.input_bindings` и `stages.parse.on`.
Parsed value всего workflow совпал. Обе формы скомпилированы и выполнены установленным stable CLI.
Каждый Run завершился `succeeded` и выдал:

```text
Pri-Fly CSV report
Rows: 3
Total: 24
```

[CSV result](evidence/csv-control.json) фиксирует одинаковый build key и SHA-256 отчёта.
Tracked CSV, worker, schemas и README не менялись. Новые CSV tests не добавлялись.
Техническое чтение переходов: parse.pass → validate; fail/needs_revision/no_work и on_error → rejected.
Наблюдение владельца по CSV не проводилось. По выбранной приёмке оно не является обязательным gate.

## Человеческая приёмка

Владелец сначала отложил reader pilot ответом «позже». Затем он получил сравнение реальных файлов до/после.
На предложение принять три части по review без обязательных замеров он ответил **«Принимай»**.
[Evidence](evidence/owner-review.json) содержит точный вопрос, ответ, hashes и исходные формулировки заменённых задач.
[Сохранённое сравнение](owner-review.html) показывает принятый кандидат.

| Часть | Решение | Основание |
|---|---|---|
| Block YAML | Принято | Решение владельца по review |
| Markdown source | Принято | Решение владельца по review; exact bytes подтверждены |
| README-карта | Принято | Решение владельца по review; ссылки сверены с YAML |

| Ранее запланированное измерение | Фактический статус |
|---|---|
| Четыре baseline reader tasks | Не проводились |
| Четыре candidate reader tasks | Не проводились |
| Время, ошибки, подсказки | Не измерялись |
| Отдельное CSV-наблюдение владельца | Не проводилось |

Критерии обновлены по прямому решению владельца. Эти измерения не получили PASS.
Исходная методика сохранена в `readability-protocol.md` как необязательный вариант.
Принят проверенный handoff; upstream поставка и установка не выполнялись.

## Воспроизведение и интеграция

Команды внешнего repository и переносимые probes — в [handoff/README.md](handoff/README.md).
Кандидат не меняет author versions: bump и release принадлежат отдельной upstream поставке.
Race, полный e2e, платные модели, upstream commit/release/update не запускались.
Принятие владельца снимает блокер implement. Дальнейшие verify/review/commit назначает native workflow; результат не подменяется одними command checks.

Итоговые локальные проверки и защищённые пути фиксируются в [local-checks.json](evidence/local-checks.json).


## Первоначальный блокер checkpoint-коммита

`make ci-check-once CHECK_CONTEXT=codex-app-authorized CHECK_FOR_COMMIT=1` завершился с exit 2.
Набор Go tests обнаружил сбой `TestNamedWorktreeConcurrentCLIProcessesFromPublishedExample`.
Он получил `invalid_input` при параллельных `project start`, до admission Run.
Остальные исполненные Go packages прошли; следующие цели `ci-check` после test не запускались.

Отдельная проверка повторила тест десять раз в каждой форме:

| Источник | Падения / попытки | Exit |
|---|---|---|
| Чистый `git archive 6de187d` | 2 / 10 | 1 |
| Текущий claim | 1 / 10 | 1 |

Это доказывает наличие сбоя до readability edits. Корневая причина пока не установлена.
[Machine-readable evidence](evidence/worktree-gate-failure.json) фиксирует команды, среду и hashes логов.
Полные логи лежат в `.cache/authoring-readability/`; они не выдаются за зелёный gate.

Правило `AGENTS.md` требует зелёный staged gate перед коммитом. Checkpoint-коммит не создан.
На этом снимке задача 5.2 была открыта, progress составлял 5/11.
После package fix технические проверки выполнены: **6/11**; человеческие задачи и принятие handoff открыты.
Изменения остаются staged в claim branch; native implement Attempt остаётся активным.
Владелец выбрал подготовку отдельного [OpenSpec change](../fix-concurrent-named-worktree-start/proposal.md).
На том диагностическом этапе его реализация ещё не началась.


## Последующее поручение: имена проекта и диагностика

После фиксации baseline владелец попросил включить читаемые имена Pri-Fly.
В claim `.prifly/project.yaml` теперь содержит profile `/4`, title `Pri-Fly` и основу `prifly-aif-classic`.
Это отдельно разрешённое изменение конфигурации. Оно не меняет readability patch внешнего package.
Прежнее evidence отсутствия `.prifly/` diff относится к снимку до этого поручения.
На том этапе задача 5.2 оставалась открытой. Конфигурация пока не закоммичена и не перенесена в main.

[Диагностика](../fix-concurrent-named-worktree-start/acceptance.md) установила причину concurrent start:
коллизия `os.Rename` при публикации одной package identity двумя процессами.
На том этапе production fix и зелёный staged batch отсутствовали.


## Последующее выполнение package fix

Гонка публикации пакета исправлена отдельным change `fix-concurrent-named-worktree-start`.
Production files этого fix не выдаются за часть readability candidate.
Полные локальные ворота совокупного snapshot прошли; прежние отрицательные batch сохранены.
Readability 5.2 закрывает выполненную техническую проверку с этими отдельно разрешёнными изменениями.
Человеческие 1.2/4.1/4.2, human-часть 3.1 и принятие handoff 5.1 остаются открытыми.
Отдельный stress failure подготовки worktree остаётся в evidence package fix.


## Возврат после исправления реестра Git

Отдельный worktree fix завершён: пункт 3.1 прошёл 50/50, полный staged batch зелёный, change — 9/9.
[Результат](../fix-concurrent-named-worktree-start/acceptance.md) сохраняет прежние отрицательные batches.

Основной кандидат уже реализован в disposable upstream copy. На текущем исправленном CLI повторены:

| Проверка | Результат |
|---|---|
| 12 пар сборок: hosts × settings × profiles | PASS, полные identities и bytes равны |
| Восемь пар маршрутов verify | PASS, исходы и bindings совпадают |
| Семь файлов candidate | PASS, hashes совпадают с прежним handoff |

[Evidence текущего CLI](evidence/current-runtime.json) отделено от прежней проверки stable binary 0.13.81.
Новый бинарник собран только в cache; установленный CLI не заменён. Платные модели не вызваны.
Результат fail/no_work остался прежним в обеих формах; его исправление не входит в readability.

На том снимке следующим шагом была отложенная человеческая проверка.
Ниже записано последующее решение владельца; прежние замеры не восстановлены задним числом.
Изменения не закоммичены и не перенесены в main. Текущий implement Attempt остаётся активным.


## Принятие владельцем

Ответ «Принимай» завершает приёмку трёх частей по review без обязательных UX-замеров.
В `tasks.md` закрыты **11/11 согласованных задач**. Исходные измерительные задачи явно заменены решением владельца.
Функциональные проверки, stable CLI, byte equality и короткая CSV-проба не ослаблены.
[Manifest принятого handoff](evidence/handoff-manifest.json) сохраняет прежний patch и семь hashes.

Прежний зелёный полный batch проверял реализацию. После этих документальных изменений итоговый staged snapshot проверяется отдельно.
Заключительный receipt и лог находятся в `.cache/verification/` и `.cache/authoring-readability/full-owner-accepted.log`.
Релиз, установка кандидата, merge в main и push не входят в это принятие.

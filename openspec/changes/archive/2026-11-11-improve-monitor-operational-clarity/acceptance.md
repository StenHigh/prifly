# Приёмка улучшения Full и Light

Это план проверок, не результат будущей реализации. Статус исходного аудита не переносится на новый candidate.

## Основание

Аудит: `run:20e1fe11189ed2e7d87c03e21c8d7225fe366b8130286e9ce7cb99b23897bc32`.
Исходный HEAD: `60931dc6f77770d748ff4ff0fc58b8c177b74dfa`.
Candidate digest: `sha256:e8c9dfbf28b3c1b09229ae44f7f695fd1cd66bdfc975829f97222a6a52eaf361`.
Материалы: `.cache/codex-reports/monitor-ux-2026-11-11/` в исходном checkout.

| Проверенное наблюдение | Воспроизведение без прежних IDs |
|---|---|
| Full: uncertain соседствует с ожиданием отчёта | Native uncertain fixture → подробности → «Сейчас» |
| Light: первое чтение обещает сохранённые данные | Новый browser context → перехват GET API до первой навигации |
| Причина ресурса слабее общего статуса | Resource fixture → список и подробности |
| Full: мобильная таблица 820 px при контейнере 356 px | 390×844 → поиск resource fixture → строка результата |
| Light: пустые группы вытесняют результат | 390×844 → поиск единственной карточки |

Неназванные legacy Runs, unmapped phases и исправленные ошибки selectors не считаются дефектами продукта. Отдельные состояния проверки и gate считаются правильным поведением.

## A. Двенадцать исходных браузерных проверок

Сохранить check IDs и оба lens. Пути ниже относятся к одному owned origin: Full `/`, Light `/light/` за prefix. `ui` использует реальные screenshots и визуальный осмотр; `ux` выполняет действия в браузере.

| Check ID / scenario | Lens / viewport | Действие и критерий |
|---|---|---|
| `full_desktop` / `full-desktop` | ui, 1440×900 | Первый экран и подробности resource fixture; причина заметна до технических панелей |
| `full_mobile` / `full-mobile` | ui, 390×844 | Название, состояние и основной переход доступны вместе; нет горизонтальной прокрутки страницы |
| `light_desktop` / `light-desktop` | ui, 1440×900 | Первый экран, причина, фаза и приёмка различимы; общая фаза не маскирует препятствие |
| `light_mobile` / `light-mobile` | ui, 390×844 | Первая карточка видна; пустые группы не вытесняют результат поиска |
| `full_stop` / `full-stop` | ux, 1440×900 | Найти human wait, resource, failed, uncertain, partial; определить причину и доступный следующий переход |
| `full_results` / `full-results` | ux, 1440×900 | Поиск, фильтр, результаты, цепочка продолжений и возврат сохраняют scope и положение |
| `full_keyboard` / `full-keyboard` | ux, 1440×900 | Tab/Shift+Tab/Enter/Escape, пустой поиск, ошибка первого чтения и восстановление |
| `full_mobile_flow` / `full-mobile-flow` | ux, 390×844 | Найти uncertain, прочитать причину и открыть соответствующие подробности без горизонтального поиска статуса |
| `light_stop` / `light-stop` | ux, 1440×900 | Те же пять состояний; срок попытки объяснён, неизвестное не выдаётся за причину |
| `light_results` / `light-results` | ux, 1440×900 | Поиск, history/review, typed gate, exact Full popup и возврат сохраняют задачу/source/Run |
| `light_keyboard` / `light-keyboard` | ux, 1440×900 | Клавиатура, пустой поиск, первый сбой GET, восстановление; нет ложного кеша |
| `light_mobile_flow` / `light-mobile-flow` | ux, 390×844 | Найти остановленную работу, прочитать причину и перейти к существующей диагностике |

Для проверки первого экрана использовать scale 100%, один найденный результат с обычным коротким названием и стандартные сообщения без длинных test payloads. Проверить также первоначальный список без фильтра. Длинный текст проверяется отдельно: доступное раскрытие, полный accessible name и отсутствие горизонтального переполнения. Снимки сохраняют viewport, состояние фильтров и выбранный объект.

## B. Регрессии на сочетаниях состояний

| ID | Вход / сочетание | Наблюдаемый результат | Где проверять |
|---|---|---|---|
| R01 | disconnected + issued, при наличии и отсутствии principal | Причина потери связи важнее обычного ожидания; неизвестный host не выдуман | Full Node + browser |
| R02 | uncertain + unresolved effect; cancelled и stopping отдельно | Нет обещания безопасного повтора; отмена в процессе не объявлена законченной | Оба Node + browser |
| R03 | Working branch + decision/resource/signal либо актуальная disconnected Attempt другой ветви | Видны все причины и scope; чужая ошибка не становится причиной выбранной Attempt, но текущее препятствие другой ветви сохраняется; category/lane/status прежние | Оба Node + native fixtures |
| R04 | Legacy без Board/причины/owner | Явный unknown/fallback, без обязательного нового поля | Оба Node |
| R05 | Исторический failed после retry, другая StageActivation, неизвестный код | Старый Diagnostic не становится текущей причиной; код остаётся доступным; актуальные причины других ветвей проверяются отдельно в R03 | Full Node + Light scoped reason |
| R06 | Completed/partial → cancelled continuation → next continuation | Отдельные исходы, exact source-scoped links, без выбора latest | Оба browser |
| R07 | StepResult pass + blocked gate; unmapped и mixed current Runs | Нет общей зелёной приёмки и произвольной ссылки первого Run | Light Node + typed-review/queue fixtures |
| R08 | Нет успешного snapshot → GET error | Ошибка первого чтения, без «данные сохранены» и бесконечной загрузки | Оба browser в новых contexts |
| R09 | Board A success → filter/view/order/page/cursor B error; Full page A success → page B error; board success → detail B error | Нет snapshot B и ложного утверждения о сохранённых B; поздний A отвергнут | Оба browser с route interception |
| R10 | Текущий snapshot success → refresh error → восстановление | Тот же снимок stale, статус Run прежний; после восстановления ошибка исчезает | Оба browser |
| R11 | 304 с соответствующим snapshot / без него; HTTP 200 с stale source | Неизменённый snapshot используется только в своём scope; source freshness сохранена | Node request harness + browser |
| R12 | Прежняя Run revision, изменились driver_live/capacity/freshness | Отображаемые признаки обновлены без false host death и без потери focus/scroll | Full harness + browser |
| R13 | Длинное название, HTML-like payload, длинная причина | Текст экранирован, доступен полностью, страница не переполняется | Оба browser, desktop/mobile |
| R14 | Скрытый выбранный фильтр, поздняя страница, темы, back/popup/poll | Фильтр продолжает действовать; focus, page, scroll и exact route сохранены | Оба browser |
| R15 | Только выбор/раскрытие/GET; bounded summary/list | Нет новых mutation requests, N+1 details/artifacts и изменений authority | Network log + existing Go guards |

R01–R15 расширяют покрытие исходных checks, а не заменяют их искусственным новым положительным набором. Синтетический fault injection помечается в evidence. Native fixtures подтверждают реальные формы данных и routing.

## C. Команды реализации и целевого replay

Выполнять из actual implementation worktree. Использовать существующие Go, Node, Playwright и Chromium. Пути ниже — параметры конкретного host; отсутствие инструмента не считается успешной проверкой.

После добавления UI suite:

```sh
node cmd/prifly/monitor_ui_test.cjs
node cmd/prifly/light_ui_test.cjs
make ui-check
python3 -B scripts/test_test_changed.py
make test-changed
```

Для быстрых интеграционных Go-проверок сохранить `TestMonitorBoardRefreshesAuthorityFactsAtTheSameRunRevision`, `TestMonitorBoardPagesAllRunsAndKeepsSourceScopedProvenance`, `TestLightSeparateReadOnlyUIAndPrefix`, `TestLightPagesKeepGenerationAndQueryAcrossThousandCards`, `TestLightHealthCannotSatisfyFullMonitorProbe` и `TestLightTypedReportAndGateCannotInventPass`.

Целевой браузерный replay — отдельный запуск существующих scripts, не полный release e2e. `MONITOR_BIN` собирается из того же candidate; output, fixtures и caches находятся вне него. `MONITOR_GO`, `MONITOR_PLAYWRIGHT`, `MONITOR_CHROMIUM` задаются фактическими найденными путями. `MONITOR_ARTIFACTS` — новый owned каталог.

```sh
python3 test/e2e/verify-monitor-board.py \
  --binary "$MONITOR_BIN" --target "$MONITOR_ARTIFACTS/full" \
  --go "$MONITOR_GO" --playwright-module "$MONITOR_PLAYWRIGHT" \
  --chromium "$MONITOR_CHROMIUM"
```

Light replay ожидает четыре native fixture authorities. Их получают существующими opt-in тестами, не импортируют из private authority:

```sh
PRIFLY_LIGHT_PERF_FIXTURE_DIR="$MONITOR_ARTIFACTS/light/authorities" \
PRIFLY_LIGHT_FIXTURE_DIR="$MONITOR_ARTIFACTS/light/authorities" \
"$MONITOR_GO" test ./cmd/prifly ./internal/runtime \
  -run '^(TestLightExportPerformanceFixtures|TestLightExportLongHistoryFixture|TestLightNativeTypedReviewRun|TestCLIProjectQueuePartialCancelContinueKeepsTaskAndRunHistory)$' \
  -count=1

python3 test/e2e/verify-light.py \
  --binary "$MONITOR_BIN" --target "$MONITOR_ARTIFACTS/light" \
  --playwright-module "$MONITOR_PLAYWRIGHT" --chromium "$MONITOR_CHROMIUM"
```

Этот набор не содержит все пять причин из `light_stop`. Для них нужен отдельный экспорт, который не смешивается с performance authorities:

```sh
PRIFLY_MONITOR_FIXTURE_DIR="$MONITOR_ARTIFACTS/light-stop/authorities" \
"$MONITOR_GO" test ./cmd/prifly ./internal/runtime \
  -run '^TestMonitorBoardExportFixtures$' -count=1
```

Оба пакета экспортируют через одну env. `runtime-fixtures.json` содержит human/parallel, resource, failed и uncertain; `cli-fixtures.json` содержит partial и продолжения. `light-projection` для этих причин не требуется; отсутствие title или mapped phase остаётся явным.

Задача 3.3 добавляет `--scenario stops` в существующие `verify-light.py/.cjs`. Этот режим пока не реализован. Его будущая команда:

```sh
python3 test/e2e/verify-light.py --scenario stops \
  --binary "$MONITOR_BIN" --target "$MONITOR_ARTIFACTS/light-stop" \
  --playwright-module "$MONITOR_PLAYWRIGHT" --chromium "$MONITOR_CHROMIUM"
```

Режим использует общий owned setup, отдельный registry и подключает все экспортированные sources к обоим мониторам. Light получает фактический Full URL. Run IDs берутся из fixture JSON; source/key — из реального API. Проверка использует поиск или пагинацию: 252 generic Runs могут вытеснить нужную карточку с первой страницы. Режим не требует performance mapping либо 2003 карточки. Default режим сохраняет прежние проверки бюджета и typed review.

Scripts могут создавать и изменять только свои тестовые authorities. Browser fault injection блокирует чтение, не меняет backend. Закрываются только созданные процессы.

## D. Повторный aif-ui-ux

1. Сохранить итоговый candidate и implementation branch вне возможной автоочистки следующего claim.
2. Подготовить input `configuration` по установленной schema: те же 12 IDs, оба lens, exact base/candidate, свежий owned environment.
3. Указать реальные setup/cleanup, инструменты и фактический model profile; не копировать прежний `/tmp`, owner/session или digest.
4. Выполнить native questionnaire/prepare и проверить launch summary, effects, input digests, naming и source workspace.
5. Вести тот же Run через `run next`, issued tasks и штатные программы до terminal outcome.
6. Проверить отчёты, candidate coverage и точный cleanup receipt; сохранить новое evidence отдельно от исходного аудита.

Условия приёмки: все 12 checks выполнены; нет required failed/unavailable/blocked; сценарии B покрыты с соответствующим результатом. Любые warnings рассматриваются по фактическому pinned gate policy, не переименовываются вручную в pass. Неизвестная причина, когда данных действительно нет, является правильным выводом, а не основанием отключить check.

## E. Защищённые границы и итоговые ворота

В начале реализации сохранить actual base SHA и перечень исходных dirty files. Сравнение ведётся с этой основой, не только с исходным аудитом `60931dc6`. Уже существующие изменения установки runner/workflow не присваиваются этому срезу.

Продуктовые файлы `internal/runtime/`, `internal/flow/`, `internal/local/`, `internal/projectwire/`, опубликованные `schemas/` и нетестовые Go-файлы CLI остаются неизменными. Новые тесты могут читать существующие DTO/fixtures; контракт не расширяется ради удобства UI. При выявленной нехватке данных применяется unknown либо отдельная задача владельцу контракта.

Проверить отсутствие изменений в `openspec/changes/archive/**`, прежних release evidence/manifests, старых schema bundles и source set активного Light change. Сопоставить сохранённые JSON/replay fixtures побайтово с actual base; не перезаписывать ожидаемые bytes для зелёного теста. Ожидаемый diff этого среза ограничен frontend, тестами/gate wiring, текущими справочниками и собственными planning artifacts.

```sh
openspec validate improve-monitor-operational-clarity --strict
make ci-check-once CHECK_CONTEXT=<actual-profile> CHECK_FOR_COMMIT=1
git diff --check
```

`<actual-profile>` заменяется реальным permission profile; он не даёт разрешений сам по себе. Staged bytes должны совпадать с проверенным итоговым снимком. При других inputs/context/toolchain reuse не считается свежим успехом. Race и полный e2e выполняются перед релизом, не подменяют эти целевые сценарии.

Проверка документа не закрывает roadmap, Light E6 release/production или продуктовый gate. Push, release и перенос прежнего рефакторинга в этот этап не входят.

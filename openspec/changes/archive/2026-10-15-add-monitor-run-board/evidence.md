# E1: инженерная приёмка доски Run

Источник задачи: https://github.com/StenHigh/prifly/issues/4. Проверяется
дополнительный локальный вид существующих Runs. Task queue и task identity не
входят в этот change; фазы P1/P2 и release qualification им не закрываются.

## Проверенный build и fixtures

- Development binary `/tmp/prifly-issue4-filter-fixed`, SHA256
  `eea41b6c19d6646b7e3bbbd75473da89dcbf2390f6c0a5234b1fcb02df327a97`.
- Реальные сохранённые authority созданы CLI/runtime API текущего дерева:
  `TestMonitorBoardExportFixtures` в `cmd/prifly` и `internal/runtime`.
  Они включают project continue, повторный continue от того же partial,
  отдельные ветви, runtime decision, exclusive-resource refusal, failed,
  uncertain и generic foundation history из 252 Runs.
- Копии authority читаются readonly; их история и relocation guards не
  переписываются. Для живого изменения CLI создаёт отдельный authority на его
  окончательном пути и исполняет локальный fixture shell step.
- Chromium с Playwright, реальный HTTP monitor, семь источников, 267 исходных Runs и 268 после live-arrival проверки.
  Итог `/tmp/prifly-issue4-filter-green/summary.json`: `outcome=passed`,
  `console_page_errors=[]`, `console_errors=[]`. Там же screenshot доски,
  параллельных ветвей, прежней таблицы и полный `trace.zip`.

## Критерии issue

| Критерий | Проверка и результат |
|---|---|
| E1-A1 | `TestMonitorBoardDistinguishesRecordedWorkAndWaiting`, terminal/unknown cases: pending без записанного начала не стал работой; status/outcome сохранены. Browser показывает категории из metadata. PASS. |
| E1-A2 | Generic legacy workflow без Git и отдельный timed workflow без title; fallback явно подписан. 252 одноимённых Runs не объединяются. PASS. |
| E1-A3 | Browser различает decision/resource, partial/cancelled/failed/uncertain; ссылки открывают workflow/facts/journal. Input check не выдаётся за admission Step (`TestAResourceKeepsTwoRunsFromOverlappingOnAStage`). PASS. |
| E1-A4 | Реальные `reader-left` и `reader-right` видны одновременно с pending decision. `TestMonitorBoardShowsEveryParallelBranchWhileDecisionWaits` и browser. PASS. |
| E1-A5 | CLI fixtures: partial → tail → cancelled → next; повторный tail от исходного partial. Карточки сохраняют собственные исходы, parent и все прямые children. HTTP тест исключает cross-source relation; legacy отсутствие provenance явно показано. PASS. |
| E1-A6 | Browser ломает только свой fixture config: прежняя карточка stale, здоровый source остаётся читаемым. Успешное чтение и существующее удаление source сохраняют прежнюю семантику. PASS. |
| E1-A7 | Поиск/фильтр по полному каталогу, страница 5, metadata >200, 50 отдельных карточек одинакового title. На доске не запрашиваются `/api/run` и artifacts. PASS. |
| E1-A8 | Atomic snapshot+admission/resources и concurrent writer test; release ресурса виден при прежних RunVersion/EventSeq. Browser проверяет live transition целой карточки, поздний HTTP response, три detail tabs и возврат с view/filter/page/scroll, включая обновление скрытого списка при появлении реального нового Run. PASS. |
| E1-A9 | Frozen bundles, прежние archived changes и release evidence не менялись. Foundation Runs и storage v3 читаются без rewrite; optional local `board` не меняет public RunView или YAML contracts. PASS. |

## Воспроизводимые проверки

Обычные unit/HTTP/UI проверки выполняются в текущем claim worktree. Имена
проверок приведены в таблице и `tasks.md`; регрессии до исправления сохранены
в `/tmp/prifly-issue4-*-red.log` (неактивированный wait, input check/resource,
каталог и CSP таблицы). После исправления targeted Go tests и Node UI PASS.

Browser replay:

```sh
python3 -B test/e2e/verify-monitor-board.py \
  --binary /tmp/prifly-issue4-filter-fixed \
  --go /srv/projects/prifly/core/.tools/go/bin/go \
  --target /tmp/prifly-board-replay-new \
  --playwright-module /tmp/w2-browser/node_modules/playwright \
  --chromium /home/dev/.cache/ms-playwright/chromium-1223/chrome-linux64/chrome
```

Без `--fixtures` replay сам генерирует состояния через opt-in Go fixtures;
путь target должен отсутствовать или быть пустым. `--fixtures DIR` копирует
ранее сгенерированные состояния для повторного UI прогона. Dependencies не
скачиваются. Ни credentials, ни authority владельца не участвуют.

Полные ворота завершены на инженерном снимке (см. ниже). Canonical
verify/security/review/commit и sync/archive следуют после этой реализации;
их фактические результаты фиксируются после завершения Run. Этот раздел не заменяет принятые
StepResults и не объявляет будущую поставку или внешний host pilot пройденным.

`make check-fast GO=/srv/projects/prifly/core/.tools/go/bin/go` — PASS: все затронутые пакеты (в этом diff — полный пакетный набор), Node UI, 374 gofmt inputs, 191 refusal-check inputs. `TestParallelTestsTouchNoProcessWideState` — PASS после переноса новых parallel tests в отдельный файл без записи package state.

Дополнительная E1-A8 регрессия: при real Run arrival во время открытых подробностей scroll колонки сбрасывался с 600 до 0. Позиции grid/колонок теперь сохраняются при уходе и восстанавливаются при возврате; полный Chromium replay повторно PASS, console errors и page errors пусты. Первый полный batch обнаружил только неиспользуемые helpers прежней обработки `storage_busy`; они удалены, итоговый batch повторно PASS.

Полные ворота `make ci-check-once GO=/srv/projects/prifly/core/.tools/go/bin/go CHECK_CONTEXT=codex-app-authorized CHECK_FOR_COMMIT=1` — PASS, input fingerprint `01cd2e41c1645df7c1051fecacda2b7ce07e9c5bbf5f0d12c5bf27f8cbaaf8d1`, log `/tmp/prifly-issue4-ci-engineering-final.log`: Go tests, vet/staticcheck Linux+Darwin, 374 gofmt inputs, 191 refusal-check inputs, vuln scan, schemas/release/scripts gates, 686 runtime tests distributed exactly once, modules verified. Новая отметка инженерной готовности не заменяет последующие canonical stages и доставку change.


Review обнаружил дополнительную E1-A8 гонку: поколение запросов менялось
только через debounce (200 мс), поэтому старый HTTP ответ мог установить
пустую доску под уже новым фильтром. Регрессия с реальным пустым HTTP ответом,
приостановленным browser clock и observer каждого добавленного grid падает на
candidate2 (`/tmp/prifly-issue4-filter-clock-red.log`). После синхронного
увеличения generation на input тот же replay PASS: observer видит `[1]`,
устаревшая пустая доска не устанавливается. Полный Chromium replay на
filter-fixed также PASS: 268 Runs, console/page errors пусты; Node UI PASS.
`LOG_LEVEL=debug` даёт `[FIX:monitor-filter]` только в тестовом Node harness;
production monitor не получает дополнительных console logs.

Полные ворота после fix-round также PASS: fingerprint
`5ec5fc8551dd4639b77cc78676163b86979b2b4c258d51536dc12021e645c08d`,
log `/tmp/prifly-issue4-filter-ci.log`; CLI tests 130.021s, остальные Go packages,
vet/staticcheck, vuln/schema/release/scripts и modules verified.

Canonical Run `run:20c44358e3d3cc9ca50c73bca03198805612ff9cf46395aff9a6b6c37c3d41e1`
завершился terminal после принятых verify/security/review и одного fix-round.
Implementation commit `e199d617ed41f898829a44ccec891868f4ac77d7` создан штатным commit Step и влит
fast-forward в main. Окончательный staged gate PASS, fingerprint
`baa4b9a8dad9b3ace67cb25e7bb6af445bda61fdfa53201acb675aa928f0ead4`,
log `/tmp/prifly-issue4-filter-ci-final.log`. Sync переносит три ADDED
requirements в local-run-monitor с сохранением всех прежних scenarios;
архивирование удаляет только plan этого Run.

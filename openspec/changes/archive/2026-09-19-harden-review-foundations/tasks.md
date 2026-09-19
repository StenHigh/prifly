## 1. Срез и наблюдаемость

- [x] 1.1 `internal/local/store_read.go`: `ReadEventsOfType` получает верхнюю
  границу `through` и ставит `seq<=through` во все страницы запроса; callers
  обновлены. Сначала падающий тест: срез → новый `state.changed` → чтение по
  срезу не видит новое событие (`go test ./internal/local -run ReadEventsOfType`).
- [x] 1.2 `internal/runtime/engine.go`: `hydrateTransitions(ctx, r, through)`;
  `View` передаёт `read.Snapshot.EventSeq`, телеметрия (`telemetry.go`) —
  `snapshot.EventSeq` выбранного cut; snapshot-переходы legacy сохраняются.
  Проверка: регресс «отчёт на cut → коммит перехода → отчёт на том же cut
  байт-в-байт прежний» (`go test ./internal/runtime -run 'Telemetry|View'`).
- [x] 1.3 Timing/read view называют неполность прочитанную историю вместо
  тихого обрыва; существующие тесты зелёные
  (`go test ./internal/runtime -run 'Timing|View'`). Падал красным на
  `TestATruncatedHistoryIsNamedInsteadOfReportedAsAbsent`: граница
  проверялась только между страницами, поэтому одна страница в 500 событий
  перескакивала её целиком и `maxRecordedTransitions` не ограничивал ничего.

## 2. Эффекты workspace program-шага

- [x] 2.1 `internal/runtime/effects.go`: ошибка `workspaceMark` до исполнения
  на claimed worktree — именованный отказ (не `measured=false`); после
  исполнения ошибка сравнения — отказ приёма, не пустая строка. «Не
  репозиторий» остаётся отдельным осознанным случаем. Проверка: два новых
  падающих-затем-зелёных теста (`go test ./internal/runtime -run 'Effects|ProgramStep'`).
- [x] 2.2 Коды отказов внесены в глоссарий и troubleshooting.md;
  `TestGlossaryBindings` зелёный; `refusal-check` не находит код в тексте.

## 3. Телеметрия команды и allowance

- [x] 3.1 `internal/local/store_samples.go`, `store.go`: savepoint вокруг
  `insertSamples` в `recordCommandSamples`; при `ErrSampleLimit` — откат
  пакета, команда коммитится. Сначала падающий тест границы
  (`go test ./internal/local -run Sample`).
- [x] 3.2 Существующие `TestStoreSampleBudgetAfterActualSQLiteAllocation` и
  `TestTelemetrySamplesRecordedWithCommand` зелёные; счётчики записаны.
  Красный до правки: `TestOverBudgetCommandSamplesDoNotCommitWithTheirCommand`
  — «an over-allowance batch committed with its command: 10 samples kept»,
  600 КиБ диагностики под разрешением в 256 КиБ.

## 4. CLI-режим открытия

- [x] 4.1 `cmd/prifly/main.go`: `claim create-set` в `mutatingCommands`;
  оба направления в `TestEveryMutatingCommandOpensForWriting`. Сначала
  падающий тест на create-set.
- [x] 4.2 E2E: успешный атомарный `claim create-set` из двух repository
  через собранный CLI (`test/e2e`); reject read-only открытия называет режим.

## 5. Сопровождающее

- [x] 5.1 `internal/local/store_bench_test.go`: `fillBenchHistory` подаёт
  CAS при версии 0, валит бенчмарк на первом же отказе и проверяет до
  таймера, что база держит 1200 Runs, 1200 событий и не меньше 9 МиБ.
  Измерено: старая фикстура — applied=0, rejected=1200, runs=0
  (`not_found: run does not exist` на каждой команде). Новый срез и
  контроль старой фикстуры на той же машине — в proposal.md; архивные
  числа 2026-09-04 не тронуты.
- [x] 5.2 `cmd/prifly/project_preflight.go`: `Setpgid`, `Cancel` бьёт по
  группе, `WaitDelay` ограничивает дренирование.
  `TestPreflightTimeoutEndsTheProgramGroupAndDoesNotWaitOnIt` красный до
  правки: «the refusal waited 30.010365291s on a program whose deadline was
  500ms» — внук держал трубу вывода, и `Run` ждал трубу, а не программу.
  После: тест зелёный за 0.96 с, внук мёртв (`kill(pid, 0)` → ESRCH).
  `go vet` прочитан на darwin и linux.
- [x] 5.3 `internal/runtime/sessions.go`: `sessionTaskFrom(ctx, r, view,
  attemptID)` — проекция из уже прочитанного Run; `SessionTask` остался
  тонкой обёрткой с прежней сигнатурой. Было n+1 чтений Run на n handoff'ов,
  каждое со своим срезом. `TestListingHandoffsAgreesWithReadingOneByName` —
  фикстура держит 3 handoff'а одновременно (4 чтения → 1) и проверяет, что
  задача в списке байт-в-байт та же, что выдаётся по имени attempt'а.
- [x] 5.4 `internal/runtime/engine.go`, `versions.go`: у `versionContract`
  появилась колонка `Next`, `nextVersionFor` заменил вторую копию лестницы
  внутри `Next` — шестнадцать последовательных `if`, каждый перезаписывал
  предыдущий, и следом цепочка из четырнадцати `else if` в обратном порядке.
  Эквивалентность доказана против дословно скопированной старой цепочки на
  всех 32 состояниях и на трёх неизвестных версиях, и только после зелёного
  сравнения цепочка удалена. `TestEveryStateNamesItsNextContract` держит
  лестницу руками написанной таблицей и валит сборку, когда новая state
  version не назвала свой next-контракт. Action-логика `Next` не тронута:
  `go test ./internal/runtime ./cmd/prifly` — 250.9 с и 41.2 с, rc=0.
- [x] 5.5 Семантические коды проекта — типизированные отказы вместо
  `usageError`. Замер до правки: **382 сайта**, **105 разных кодов**, и все
  они уходили на провод как `code: "invalid_usage"` с настоящим кодом внутри
  прозы. Конвертированы все 382 через `refusal(code, detail)`; читатель
  ничего не теряет, потому что отказ всегда JSON-конверт на stderr, так что
  `code` и `message` приходят вместе в любом случае.
  `TestProjectRefusalsCarryTheirCodeInTheEnvelope` ассертит декодированный
  `Problem.code`, непустой `message`, отсутствие кода внутри своего же
  предложения, exit-класс и непустой `safe_next_actions`.
  `refusal-check` расширен на `usageError` с кодопрефиксом; доказано, что
  гейт умеет падать — подложенный `usageError("guard_probe_code: …")` он
  нашёл и завалил сборку. `make ci-check` rc=0 (162 файла прочитано).
  Два следствия записаны намеренно:
  `safe_next_actions` для `project_*` без своей записи по-прежнему `["help"]`,
  как было у `invalid_usage`; и `exitForCode` получил `project_` раньше
  подстрочных правил — иначе восемь кодов с «conflict» в имени стали бы
  классом 3 («состояние authority не то, что вы предполагали»), хотя это
  расхождение двух строк в project.yaml, то есть класс 2. Побочно
  `project_profile_conflict` и `project_runner_conflict` перешли с 3 на 2,
  и это исправление: они и раньше означали то же самое.
- [x] 5.6 `scripts/check-schema.py`: одна сборка `schema-gen` во временный
  каталог, дальше один exec на бандл вместо `go run` на каждый. Замер
  warm-cache (три прогона подряд, Apple M1): **8.70 / 8.61 с → 3.38 / 3.37 с**,
  то есть гейт стал быстрее в 2.6 раза и отдаёт около 5.3 с на каждом прогоне.
  Байты не изменились: 57 бандлов сошлись теми же sha256, что в ci-check
  на `460cb82`.
- [x] 5.7 Ранний выход watcher'а (driver.go): остановка и join сразу после
  старта; тест, что ранний отказ до `RunProcess` не оставляет живого тикера.

## 6. Документы и ворота

- [x] 6.1 `openspec validate --all --strict` — 22 passed, 0 failed;
  `git diff --check` чист; словарь не менялся в разделах 1 и 3.
- [x] 6.2 Focused-тесты каждого раздела прогнаны через явный target
  worktree (`/Users/sh/PhpstormProjects/Pri-Fly/github`, `-count=1`), счётчики
  записаны выше по разделам. Полные ворота по слову владельца — дважды:
  на `460cb82` (разделы 1–4) и на конвертации отказов (раздел 5):
  `make ci-check` rc=0 — vet прочитан на linux и darwin, fmt-check 301 файл,
  refusal-check 162 файла, staticcheck 9 пакетов × 2 платформы, 57 бандлов
  схем сошлись; `make e2e` rc=0; `make race` rc=0 — `cmd/prifly` 178.7 с,
  `internal/local` 7.6 с, `internal/runtime` 718.7 с, гонок нет.
- [x] 6.3 Защищённая история не тронута: `git diff --name-only
  --diff-filter=MD 5b5c4ca -- openspec/changes/archive` пуст (134 файла
  добавлено новыми архивами, ни один прежний не изменён и не удалён).
  Формулировка уточнена: сам список не пуст и не должен быть — архив растёт.

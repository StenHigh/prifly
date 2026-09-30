## 1. Движок

- [x] 1.1 `DecisionRecord.Requested` для записей с `attempt_id` в Run на 41; запись при запросе, перенос в журнал при ответе. Проверка: `go test ./internal/runtime -run Decision`.
- [x] 1.2 `host_pickup`, `host_work`, `decision_wait` на узле assisted-Attempt, ревизия `core-timing/4`; unavailable с причинами для старых Run. Проверка: тесты на фикстурах «работа 17 мин с ожиданием 5», «Run до 41», «открытая сессия», «повторная выдача».
- [x] 1.3 Суммы `*_sum` на узлах и `idle` на корне; `session take` записывает взятие один раз. Проверка: `go test ./internal/runtime -run 'NamesWhereItsTimeWent|UnrecordedPhases|Taking|NeverTaken'`.
- [x] 1.4 `run status`: строка `time` со сводом. Проверка: тест CLI.

## 2. Монитор

- [x] 2.1 Блок «Куда ушло время» в обзоре Run: категории с долей и таблица стадий с числом исполнений, качество у каждой величины. Проверка: `node cmd/prifly/monitor_ui_test.cjs` + ручной просмотр на Run `aif:workflow/classic`.
- [x] 2.2 Фазы Attempt в блоке шага. Проверка: тот же тест.

## 3. Документация и ворота

- [x] 3.1 `examples/troubleshooting.md` (как читать время Run), словарь метрик в `terms.md`, help `run timing`. Проверка: `TestGlossaryBindings`.
- [x] 3.2 `make ci-check`, `make e2e` зелёные; прежние опубликованные bundles и числа ревизии 3 не изменились (`make schemas-check`, тесты timing); `git diff --stat -- openspec/changes/archive/` пуст.

Ворота 2026-09-30: `make ci-check` и `make e2e` зелёные на одном дереве трёх change (со второго прогона: первый поймал неотформатированный `main.go` и ожидание `core-timing/3` в `test/e2e/verify-context.py`).

Выпущено 2026-09-30 в v0.13.69 (тег на `f867bd3`): GitHub `verify` 36642047819 и `race` 36642084242 зелёные на этом коммите, `release` 36643402546 опубликовал 6 ассетов после подтверждения окружения `release`.

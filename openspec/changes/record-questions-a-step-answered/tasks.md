## 1. Протокол и хранение

- [x] 1.1 `AnsweredQuestion`, `QuestionReport`, `SessionSubmission.AnsweredQuestions`, `Attempt.QuestionReport`, `SessionTask.QuestionReport`; проверка формы, пределов, `decision_id` по каталогу Run и `port` по входам шага; отказы `answered_questions_missing` / `answered_questions_invalid` через `flow.Problem` с путём поля. Проверка: `go test ./internal/runtime -run Question`.
- [x] 1.2 `core-state/41` + `core-read/41`: выбор при старте для scoped Run с assisted-шагом, строка в `versions.go`, `compatibility.go`, достижимость. Проверка: тест достижимости версий и `TestGlossaryBindings`.
- [x] 1.3 Run на 40 и ниже: отчёт без поля принимается, с полем — отказ, что версия его не держит. Проверка: тест.

## 2. Опубликованные схемы

- [x] 2.1 `cmd/schema-gen` → `question-report.schema.json` в `internal/runtime` и `schemas/core`, `scripts/check-schema.py`; прежние bundles byte-identical. Проверка: `make schemas-check`.

## 3. Чтение

- [x] 3.1 `run status`: строки `question` на каждый вопрос и `questions … none` для пустого отчёта. Проверка: `go test ./cmd/prifly -run TestRunStatusPrintsTheQuestionsEachStepAnswered`.
- [x] 3.2 Слой шаблона runner `prifly-run` с текстом о вопросах, закреплённые digest'ы прежних вариантов не меняются. Проверка: `go test ./cmd/prifly -run Runner`.

## 4. Документация

- [x] 4.1 `examples/README.md` строка возможности, справочник шага/сессии, `troubleshooting.md` (`answered_questions_missing`), `terms.md` (Question report), help `session submit`. Проверка: `TestEveryDeclaredCapabilityIsInTheAuthorIndex`, `TestGlossaryBindings`.

## 5. Ворота

- [x] 5.1 `make ci-check` и `make e2e` зелёные; `git diff --stat -- openspec/changes/archive/` пуст — прошлые release evidence не тронуты.

Ворота 2026-09-30: `make ci-check` и `make e2e` зелёные на одном дереве трёх change (со второго прогона: первый поймал неотформатированный `main.go` и ожидание `core-timing/3` в `test/e2e/verify-context.py`).

Выпущено 2026-09-30 в v0.13.69 (тег на `f867bd3`): GitHub `verify` 36642047819 и `race` 36642084242 зелёные на этом коммите, `release` 36643402546 опубликовал 6 ассетов после подтверждения окружения `release`.

## Находки первого боевого Run (пилот, 0.13.69) — исправлено в 0.13.70

- [x] 7.1 `basis: decision` с `decision_id: core:package_profile` отклонялся: хост видит решение только по ключу `decision_context`, а проверка знала лишь id каталога. Теперь принимается и ключ, под которым ответ передан этой попытке; запись хранит id каталога; отказ перечисляет и id, и переданные ключи. Проверка: `TestADecisionIsNamedByTheKeyItWasHandedUnder`; текст `prifly-run` называет оба имени, прежний текст заморожен.
- [x] 7.2 `session submit --template` не говорил, что `answered_questions` обязателен. Поле по-прежнему не заполняется шаблоном; при `question_report: required` в stderr печатается, что и где добавить. Проверка: ассерт в `TestSessionTaskLeavesTheHandoffAndItsReportShapeInReach`, красный без подсказки.

Выпущено 2026-09-30 в v0.13.70 (тег на `272ec70` через `scripts/tag-release.py`: verify и qualify зелёные на этом коммите), release run 36652499849 опубликовал 6 ассетов.

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

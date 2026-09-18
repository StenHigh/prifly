# Проверки

Дата: 2026-09-18.

- `make ci-check GO="$(go env GOROOT)/bin/go"`: rc=0 (fmt-check 297 файлов, refusal-check 162, staticcheck 9 пакетов × 2 платформы без находок, vuln-check чист, все bundle'ы совпали, включая новый `workflow-revision-v5` — 17 154 байта, sha256:f07e0663e557d02a0a468a670c81aedcf0f94835bbeb27e2d037d6c00b9e8c4f).
- `go test ./...`: зелёный целиком.
- `openspec validate --all --strict`: 23 passed. `python3 -B test/e2e/test_examples.py`: 6 тестов, OK.

## Повтор, объявленный автором графа

- `TestDeclaredRetryTakesTheStageAgainWithoutTheOwner`: программа, падающая первый раз и проходящая второй, доводит Run до `completed`; попыток ровно две, и диагностика первой сохранена с текстом «the stage is taking its declared retry».
- `TestDeclaredRetryIsSpentAndThenTheRunFails`: программа, падающая всегда, при бюджете 1 даёт ровно две попытки, после чего Run `failed` — как без бюджета.
- `TestTechnicalRetriesNeedTheStepAuthorsPermission` (`internal/flow`): бюджет на шаге с `retry_class` `never`, `deduplicated` или `reconcile_required` отказан компиляцией `unsupported_retries`, и отказ называет класс; с `pure` и `idempotent` тот же граф компилируется. Проверены обе стороны — первая версия теста молча пропускалась (`t.Skip`), потому что шаг фикстуры оказался повторяемым, и это ничего не доказывало.

## Повтор по слову владельца

- `TestReopenRunsTheBrokenStageAgainAndKeepsWhatWasDone`: Run, сломавшийся на программной стадии из-за пустого источника (ровно форма боевого случая), после исправления источника и `run reopen` доходит до конца; число завершённых стадий до и после совпадает — завершённое заново не исполнялось.
- `TestReopenRefusesARunThatReachedAnOutcome`: завершённый Run отказывает `not_a_broken_run`.
- `TestReopenTakesASettledRunOnceAndOnItsCurrentVersion`: устаревшая ожидаемая версия отказана; повторный вызов на уже открытом Run отказан.

## Что изменилось против первоначального замысла

Вторая половина планировалась как «fork с точкой входа». Ей нужно поле в состоянии (какие стадии унаследованы и от кого), а список версий состояния упёрся в предел 32. Перечитали отклонённый вариант 3 и нашли, что отклонение было слишком широким: «Run, достигший outcome» и «Run, сломавшийся, не ответив» — разные вещи. Первый переоткрывать нельзя, и команда его отказывает. Второй ответа не дал вовсе, и вернуть его стадию ближе к resume. Новых полей в состоянии нет.

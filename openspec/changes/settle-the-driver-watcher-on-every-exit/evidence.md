# Проверки

Дата: 2026-09-18.

- `make ci-check GO="$(go env GOROOT)/bin/go"`: rc=0. cmd/prifly 41.413 с, internal/runtime 240.319 с; vet прочитал linux и darwin; fmt-check 295 файлов, все gofmt-clean; refusal-check 161 файл; staticcheck 9 пакетов для linux и 9 для darwin, без находок; vuln-check 9 пакетов; все bundle'ы совпали.
- `make race GO="$(go env GOROOT)/bin/go"`: rc=0, ни одного `DATA RACE`. cmd/prifly 176.409 с, internal/runtime 701.044 с.
- `make e2e GO="$(go env GOROOT)/bin/go"`: rc=0, 13 итогов, все `passed`.
- `openspec validate --all --strict`: 22 passed, 0 failed.

## Воспроизведение, которое сначала не воспроизвелось

`TestDriveTakesItsWatcherWithItOnAnEarlyFailure` (`internal/runtime/driver_watcher_test.go`)
подменяет путь записанного claim'а на чужой, так что `processWorkspaceBoundary`
падает `local.ErrIntegrity` при живом контексте вызывающего, и проверяет, что
после завершения вызова Run не получает отмену.

Первая версия теста была **зелёной на сломанной сборке**, и причина этого —
часть находки, а не случайность. Фикстура `programAfterWriteFixture` ведёт
ошибку шага-программы в `finish`, поэтому тот же отказ делал Run terminal, а
`requestDriverCancellation` молчит для terminal Run. Осиротевший сторож там
никого не отменяет — он только живёт до дедлайна попытки.

Вред начинается там, где Run переживает неудачную попытку. Фикстура получила
параметр `recoverable`: объявленный `on_error` ведёт на следующий ассистируемый
шаг. После этого дефект воспроизвёлся дословно на сборке до правки:

```
drive=stored content failed integrity verification status=ready cancel=false ready=[]
a finished drive call cancelled the Run afterwards: foreground driver interrupted
```

Run жив и ждёт своего хоста; отмену присылает вызов драйвера, который
закончился. После правки тот же тест зелёный, а `TestAProgramStepIsHandedTheWorkspaceAndHeldToItsEffects`
(обе ветки, `recoverable: false`) остался прежним.

## Что именно проверяет тест

- попытка завершена неначатой: `dispatch != nil` (отказ случился **после**
  записи отправки, то есть на том самом выходе, а не на более раннем с тем же
  кодом), `started == nil`, `process_outcome.started == false`;
- последняя диагностика — `workspace_validation_failed`;
- Run не terminal и без запрошенной отмены в момент возврата из `Drive`;
- в течение секунды после завершения контекста вызова отмена не появляется;
  до правки она появлялась за миллисекунды с причиной
  «foreground driver interrupted».

## 1. Доказать безопасный выбор

- [x] 1.1 Добавить fixture с несколькими compiled editions, local entries и пределом 512; проверить точный budget и минимальный выбор по времени импорта, а не по строке версии, через `go test ./internal/runtime -run 'Test.*Package.*Retire' -count=1`.
- [x] 1.2 Покрыть защиту новой/предыдущей edition, non-terminal Run, dependent package, revoked/quarantined status и недостатка места; тот же targeted тест должен вернуть объяснимый `dependency_limit` без мутации.

## 2. План и атомарное применение

- [x] 2.1 Ввести read-only preview с exact refs, counts, reasons и review digest; проверить, что `project ... --prepare` не меняет package list, inventory и Run count в CLI integration test.
- [x] 2.2 Согласовать применение retirement, trust новой edition и Run admission на одной сериализованной границе без файловых чтений внутри transform; проверить гонку с запуском Run и повтором command ID через `go test ./internal/runtime ./internal/local -run 'Test.*Package.*Retire' -count=1`.
- [x] 2.3 Покрыть сбой между импортом и Run creation: не оставлять скрыто изменённый trust, не терять receipts/bytes; проверить crash/replay fixture и `make refusal-check`.

## 3. CLI, совместимость и приёмка

- [x] 3.1 Добавить versioned CLI preview/result с текущим и итоговым бюджетом, изменёнными и защищёнными editions; проверить stale digest и прежние JSON readers targeted `go test ./cmd/prifly -run 'Test.*Package.*Retire' -count=1`.
- [x] 3.2 Сверить обновлённые правила с `workflow-and-context`, `runtime-resources`, `cli-protocol` и словарём, обновив карту терминов только если появился новый смысл; выполнить `TestGlossaryBindings`, `make schemas-check` и `openspec validate auto-retire-unused-package-editions --strict`.
- [x] 3.3 На fixture и копии реального authority проверить, что после retirement старый terminal Run и его event/evidence bytes неизменны, а незавершённый Run продолжает разрешать свои pins; выполнить `git diff --check` и затронутые Go tests.

## Реализация (2026-09-29, в составе `make-run-reading-plain-for-any-model`)

- План: `Engine.PlanRegistryRetirement` (`internal/runtime/registry_retirement.go`).
  Выбирает самые старые по `Imported` доверенные редакции того же пакета;
  защищает самую новую из прежних, редакции с зависимыми пакетами и те, что
  держит незавершённый Run. Если места не хватает — `dependency_limit` с
  перечнем оставшихся редакций и причин. Ничего не пишет.
- Применение: план передаётся в `ImportPackage` (новая редакция) или в
  `SetPackageStatus` (восстановление выведенной или повторное подтверждение
  доверенной) и исполняется в той же транзакции записи пакетов. Транзакция
  закреплена версией записи и новым `AuthorityCommand.ExpectedRuns` (число
  Runs при планировании): Run, созданный после плана, даёт `runs_changed`, и
  не меняется ничего. Частичного состояния нет: вывод и доверие новой
  редакции — одна запись.
- Проверки: `TestTheOldestEditionIsWithdrawnSoTheNewOneFits` (порядок по
  импорту, а не по строке версии; самая новая прежняя сохраняется; байты на
  месте; повтор команды ничего не меняет), `TestAPlanIsRefusedWhenARunAppearedAfterIt`
  (красный без проверки `ExpectedRuns`), `TestAPlanThatCannotMakeRoomNamesWhatStays`,
  `TestAuthorityCommandRejectsAChangedRunCount` (хранилище),
  `TestProjectStartWithdrawsOldEditionsToFitTheRegistry` (сквозной CLI:
  держатель-Run не выводится; `--prepare` показывает план и ничего не меняет;
  старт с отпечатком выводит ровно старейшую; Run на выведенной редакции
  читается; красный без передачи плана в импорт).
- Отклонения. 2.3: компенсации после сбоя между установкой и созданием Run
  нет. Вывод и новая редакция записываются одной транзакцией, поэтому сбой
  после неё оставляет согласованное состояние, видимое в `package list` с
  причиной и командой `package restore` в `status_reason`. 3.1: план
  добавлен полем `registry_retirement` в `project-launch-summary/3`, а не
  новой версией: так же, как раньше `registry_budget` (документ CLI без
  закреплённой схемы). Отпечаток ревью покрывает план: изменившиеся Runs или
  пакеты дают `project_start_stale_launch`.

## 1. Доказать безопасный выбор

- [ ] 1.1 Добавить fixture с несколькими compiled editions, local entries и пределом 512; проверить точный budget и минимальный выбор по времени импорта, а не по строке версии, через `go test ./internal/runtime -run 'Test.*Package.*Retire' -count=1`.
- [ ] 1.2 Покрыть защиту новой/предыдущей edition, non-terminal Run, dependent package, revoked/quarantined status и недостатка места; тот же targeted тест должен вернуть объяснимый `dependency_limit` без мутации.

## 2. План и атомарное применение

- [ ] 2.1 Ввести read-only preview с exact refs, counts, reasons и review digest; проверить, что `project ... --prepare` не меняет package list, inventory и Run count в CLI integration test.
- [ ] 2.2 Согласовать применение retirement, trust новой edition и Run admission на одной сериализованной границе без файловых чтений внутри transform; проверить гонку с запуском Run и повтором command ID через `go test ./internal/runtime ./internal/local -run 'Test.*Package.*Retire' -count=1`.
- [ ] 2.3 Покрыть сбой между импортом и Run creation: не оставлять скрыто изменённый trust, не терять receipts/bytes; проверить crash/replay fixture и `make refusal-check`.

## 3. CLI, совместимость и приёмка

- [ ] 3.1 Добавить versioned CLI preview/result с текущим и итоговым бюджетом, изменёнными и защищёнными editions; проверить stale digest и прежние JSON readers targeted `go test ./cmd/prifly -run 'Test.*Package.*Retire' -count=1`.
- [ ] 3.2 Сверить обновлённые правила с `workflow-and-context`, `runtime-resources`, `cli-protocol` и словарём, обновив карту терминов только если появился новый смысл; выполнить `TestGlossaryBindings`, `make schemas-check` и `openspec validate auto-retire-unused-package-editions --strict`.
- [ ] 3.3 На fixture и копии реального authority проверить, что после retirement старый terminal Run и его event/evidence bytes неизменны, а незавершённый Run продолжает разрешать свои pins; выполнить `git diff --check` и затронутые Go tests.

# Source verification

Отказная regression выполнена до реализации; см. `regression.md`.
Focused naming tests прошли: declarations, paths, flags, canonical freshness,
исходные workflow refs, no-effects, persisted snapshot, resume/continue/recover,
полные 128 KiB через queue + unified composition и native queue replay.

`make refusal-check vet` прошёл: typed refusals и driver boundary linux/darwin.
Первый `make test-changed` обнаружил пропуски handbook inventories; они исправлены.
Два нагрузочных отказа (`TestStoreConcurrentCAS`, telemetry command requests)
прошли по три раза отдельно без изменения соответствующего кода.
Итоговый полный gate фиксируется локальным `ci-check-once` evidence.

112 прежних `*.schema.json` совпали с baseline. Archive не изменён.
Source-built generic и pinned official journey прошли; JSON evidence приложен.
Released-Core gate не исполнялся и остаётся открытым.

Полный Go test batch прошёл. Staticcheck нашёл лишнее начальное присваивание
в новом test helper; присваивание удалено. Отдельная red→green regression
закрепляет независимость fingerprint от порядка naming flags.
Полный `ci-check-once CHECK_CONTEXT=codex-app-escalated CHECK_FOR_COMMIT=1`
прошёл после этих исправлений: Go tests, vet linux/darwin, fmt, refusal guard,
staticcheck linux/darwin, vulnerability scan, schema generators, release/script
checks, shard inventory и `go mod verify`. Go toolchain — 1.27.3.
`openspec validate --all --strict --no-interactive`: 41 passed, 0 failed.
`git diff --check` прошёл. После обновления только progress/evidence документов
финальный staged snapshot проходит тот же gate; Go использует применимый cache.
Legacy replay и native/official probe измерили один exact executable.

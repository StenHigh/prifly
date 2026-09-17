## 1. Сторож заканчивается с вызовом

- [x] 1.1 `internal/runtime/driver.go`: выход после запуска сторожевой горутины закрывает `watchDone`, дожидается `watchExited` и присоединяет `watchErr` к возвращаемой ошибке. Проверка: тест — вызов драйвера, у которого `processWorkspaceBoundary` отказал, возвращает управление без живой горутины; `go test ./internal/runtime -run 'Drive'`.
- [x] 1.2 Отмена не приходит от закончившегося вызова. Проверка: тест, красный на текущей сборке — после отказа границы и завершения контекста вызова Run не получает `cancel_requested`, а `run status` не показывает причину «foreground driver interrupted».

## 2. Ворота

- [x] 2.1 `make ci-check`, `make e2e`, `make race` зелёные; счётчики записаны в change (`evidence.md`, 2026-09-18).

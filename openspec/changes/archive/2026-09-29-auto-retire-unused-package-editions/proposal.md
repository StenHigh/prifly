## Why

Каждая сборка Project package добавляет десятки определений в доверенный Registry. В SMSPlace очередная редакция continuation превысила общий предел 512 (516 записей), хотя старые terminal Runs уже хранят собственные pinned definitions; ручной `package remove` пришлось выполнять перед восстановлением Run. При обычных обновлениях это будет повторяться.

## What Changes

- При подготовке установки новой редакции показывать точный бюджет Registry и кандидатов на перевод в существующий `PackageStatus=Removed`: только старые редакции того же package, не удерживаемые незавершённым Run, зависимым package или явно выбранной редакцией.
- Если новой редакции не хватает места, атомарно выводить минимально достаточное число таких кандидатов из доверенного разрешения в рамках подтверждённого запуска/импорта; новую и одну предыдущую редакцию сохранять для простого отката. Если без них места не хватает — прежний `dependency_limit`, без частичной установки.
- Не удалять sealed package bytes, записи статуса, command receipts, pinned Run definitions, artifacts или историческое evidence. Показывать фактические изменения и отказ при устаревшем плане; ручные `package remove`/`restore` остаются доступны.
- Это изменение поведения продукта Pri-Fly, не правило OpenSpec как runtime и не общая очистка диска или истории Runs.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `workflow-and-context`: lifecycle нескольких редакций package и безопасный перевод устаревшей редакции в `Removed`.
- `runtime-resources`: бюджет Registry, доказательство отсутствия держателей и атомарность очистки при установке.
- `cli-protocol`: read-only preview, review digest и результат автоматического освобождения места.

## Impact

`internal/runtime/packages.go`, `package_lifecycle.go`, `context_resources.go`, локальная authority transaction/pin, `cmd/prifly/project_start.go` и package CLI; новые targeted и совместимые тесты. Существующие сохранённые Runs и опубликованные старые JSON-редакции не переписываются.

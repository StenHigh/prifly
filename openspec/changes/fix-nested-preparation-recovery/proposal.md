## Why

Штатный `project recover --prepare` отказывает для технического сбоя во вложенном workflow до admission: один отказ распространяется через несколько `call/repeat`, а planner требует единственный root step с Attempt. Даже явный выбор ранее принятой root стадии не помогает. Это блокирует сохранённую работу, хотя контракт допускает восстановление с корневой точки остановки.

## What Changes

- Определять одну доказуемую корневую точку технической остановки через recorded invocation/caller связи; propagated failures одного сбоя не считать независимыми отказами.
- Поддержать `step` с preparation failure без Attempt и корневые `call/repeat`, которые повторяются целиком. Без выданной Attempt не оценивать и не переносить её якобы результат.
- Разрешать выбор ранее принятой root стадии при доказанной естественной точке, сохраняя проверки history, префикса, текущих ограничений и claim.
- Проверить runtime/CLI регрессии и обе read-only prepare на сохранённом SMSPlace Run. Подготовить проверенный локальный CLI; recovery START выполняет исходный хост.

## Capabilities

### New Capabilities

Нет новых понятий, полей YAML/JSON или самостоятельных capabilities.

### Modified Capabilities

- `domain-execution`: уточнить техническую точку восстановления при nested preparation failure и границу независимой неоднозначности существующего требования возобновления.

## Impact

Изменение затрагивает продуктовый runtime: `internal/runtime/recovery.go`, recovery history/trace consumers и regression tests; при необходимости CLI tests и справочники существующего recovery. `engine.go`/`brokenStage()` остаются контрактом прежнего `reopen`, если для исправления не требуется отдельное согласованное изменение этого маршрута.

Ownership не меняется: нормативный source set — `openspec/specs/domain-execution/spec.md`; до sync действует delta этого change. Сохранённые Runs, опубликованные schemas, drift/effect checks и прежнее release evidence не переписываются. Предыдущий Pri-Fly claim с готовым verification reuse и SMSPlace claim сохраняются. Release, теги, push и продуктовая работа SMSPlace в scope не входят.

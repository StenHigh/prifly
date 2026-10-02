## Why

SMSPlace #163 остановился на втором возобновлении `aif-classic`: Run B имеет принятый префикс от A и текущее рабочее дерево, но `project continue --prepare` отказывает `resume_prefix_empty`. Дефект воспроизведён на Pri-Fly 0.13.74 и на независимом программном workflow без ИИ; модель не может продолжить через отказ authority.

## What Changes

- Исправить общий planner восстановления/возобновления: проверять и переносить унаследованные доказательства вместе с принятыми стадиями текущего source Run, до выбранной точки возобновления.
- Сохранять происхождение результатов, bindings и единственного текущего владельца дерева при цепочке A → B → C → D, без фиктивных Attempts и повторного исполнения ранних стадий.
- Переносить дерево таким, каким его оставили шаги, включая tracked и untracked изменения; prepare остаётся read-only.
- Сохранить отказы на несовместимые контракты, отсутствующее evidence, unresolved execution, stale review и недоступный claim. Уточнить выбор унаследованной root-стадии через `--from-stage`.
- Согласовать CLI-объяснение и runner guidance: настоящий пустой префикс отличается от недоступного унаследованного evidence; отказ не должен предлагать потерять сохранённую работу.
- Зафиксировать change в едином delivery backlog и подготовить безопасную проверку на SMSPlace B после поставки.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `domain-execution`: композиция перенесённого префикса при повторном восстановлении/возобновлении, точка отсечения и неизменность исходных Runs.
- `cli-protocol`: обозримый план цепочки, проверяемое происхождение переноса и точные отказы/инструкции оператору.
- `delivery-roadmap`: приоритет исправления повторного возобновления и граница приёмки на сохранённом пилоте.

## Impact

Изменение затрагивает product runtime, а не только процесс репозитория. Основные места: `internal/runtime/recovery.go`, создание Run и bindings в `internal/runtime/start.go`, Project CLI, существующие resume/recovery fixtures, runner guidance и справочники авторинга/troubleshooting.

Нормативная ownership не меняется: current source sets — `openspec/specs/domain-execution/spec.md`, `openspec/specs/cli-protocol/spec.md`, `openspec/specs/delivery-roadmap/spec.md`; до archive уточнения принадлежат delta specs этого change. Согласовать общий planner с активным `retry-failed-stage-with-new-package`, не переписывая его evidence и не закрывая его задачи этим исправлением.

Новые зависимости и YAML-поля не нужны. Существующие public/state editions читаются с прежним смыслом; если для provenance потребуется новое wire-поле, публикуется новая edition рядом со старой. Historical Runs, schemas и release evidence не изменяются. Ремонт SQLite, изменение SMSPlace workflow, автоматическое обновление packages и выполнение бизнес-задачи не входят в реализацию движка.

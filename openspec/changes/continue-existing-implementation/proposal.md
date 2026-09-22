## Why

Run может честно закончиться `partial`, хотя работа в claimed worktree уже
исправлена и закоммичена после его последнего gate. Исторический Run нельзя
изменять, а новый `aif-classic` начинает с `implement`: корректный `no_work`
там ведёт в `abandoned` и заставляет повторять уже сделанную работу.

Нужен отдельный continuation Run: он принимает доказуемо существующую
Implementation, проверяет её актуальность в новом claimed workspace и проходит
только хвост качества `verify → fix (при необходимости) → review → commit`.

## What Changes

- Добавить Project continuation launch для `aif-classic`, который не вызывает
  warmup, plan, improve или implement.
- Добавить Project continuation с существующим `ForkProvenance`: он
  связывает новый Run с source Run и sealed `task`, `handoff`, `plan`, а
  Implementation создаёт из текущего Git. Historical Run остаётся
  неизменным; raw `run fork` не получает этого расширения.
- Перед первым gate зафиксировать актуальный HEAD и changed files нового
  claimed workspace как новую Implementation, проверяя ancestry исходной
  Implementation, и привязать claim к новому Run при его создании.
  Неподходящий или незакоммиченный workspace должен отказать.
- Дать проектному launcher и generated runner одну явную команду continuation,
  чтобы host не извлекал artifact refs из JSON вручную и не запускал обычный
  `aif-classic` по ошибке.
- Сохранить обычный `aif-classic` и существующие historical Runs без изменения
  их graph, outcome или evidence.

## Capabilities

### New Capabilities

- `project-workflow-continuation`: создание нового Project Run из точных
  артефактов завершённого source Run и принятой реализации в текущем commit.

### Modified Capabilities

- `workflow-and-context`: Project workflow authoring получает явный хвостовой
  continuation route и typed входы без скрытого пропуска стадий.
- `domain-execution`: новый Run сохраняет provenance source Run и принимает
  только проверяемую Implementation, не меняя historical lifecycle.
- `cli-protocol`: launcher получает безопасную typed-команду для continuation
  вместо ручного извлечения refs из состояния Run.

## Impact

- `cmd/prifly/`: Project continuation command, prepare/start summary и
  generated `prifly-run` guidance.
- `internal/runtime/` и `internal/flow/`: валидация provenance и authoring
  continuation tail; сохранённые historical Run JSON не мигрируются.
- `prifly-aif-workflows`: `aif-classic` и derived `aif-profiled`, новые
  package versions, tests и release commit.
- SMSPlace: source Run
  `run:176bdd49611198acd06a0a5a96e161f22a6d79eb50c4addfdf0e36aea76db1ac`
  становится первым acceptance scenario; его исходный `partial` не меняется.

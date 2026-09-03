## Why

Семантика исполнения и единый вход выбранной задачи пока живут в legacy-главе
`docs/spec/02-domain-execution.md` и `docs/task-input.md`. Чтобы OpenSpec стал
единственным нормативным источником до первого release candidate, этот смысл
нужно перенести в одну читаемую capability без старых внутренних номеров в
постоянной спецификации.

## What Changes

- Создать постоянную OpenSpec capability `domain-execution` с requirements и
  scenarios, эквивалентными правилам областей, идентичности, immutable
  ресурсов, исполнения, состояний, эффектов, человеческого допуска и
  управления графом Pri-Fly.
- Включить в capability контракт `TaskInput/1`: неизменный intake внешней
  задачи до создания RunBrief, подтверждение владельцем и нейтральность к
  источнику задачи.
- Сохранить связь со всеми существующими acceptance cases в архивной legacy
  coverage crosswalk этого change; старые `DOM-*` и `AC-*` IDs не попадут в
  постоянный spec.
- После проверки replacement переключить строку `Исполнение и вход задачи` в
  `openspec/SOURCE-OF-TRUTH.md` на OpenSpec и оставить legacy source
  неизменённым до final cleanup.

## Capabilities

### New Capabilities

- `domain-execution`: предметная семантика Pri-Fly от intake задачи до
  завершения Run, включая жизненные циклы, admission, effects, evidence,
  workflow control и совместимость protocol.

### Modified Capabilities

- Нет.

## Impact

Изменяется только ownership нормативной документации. Go runtime, CLI,
пользовательский YAML, JSON wire contracts, acceptance evidence и historical
manifests не меняют поведения.

## Why

Правила пакетов, workflow, context и YAML authoring пока находятся в двух
legacy-документах. Чтобы OpenSpec стал единственным нормативным источником до
первого release candidate, их нужно перенести в одну capability, не сохраняя
в постоянном spec прежние внутренние номера требований.

## What Changes

- Создать постоянную OpenSpec capability `workflow-and-context` с правилами
  package lifecycle, точных зависимостей, workflow composition, typed bindings,
  context/evidence boundaries и YAML frontend `prifly-workflow/1`.
- Сохранить связи legacy requirements, примеров и acceptance cases в архивной
  coverage crosswalk; старые `PKG-*`, `WF-*`, `CTX-*` и `*-AC-*` IDs не
  попадут в постоянный spec.
- После полной сверки replacement переключить строку `Сценарии, пакеты,
  контекст и YAML authoring` в `openspec/SOURCE-OF-TRUTH.md`; legacy source
  set останется неизменным до final cleanup.

## Capabilities

### New Capabilities

- `workflow-and-context`: packages, workflows, contexts и YAML authoring как
  проверяемые contracts Pri-Fly.

### Modified Capabilities

- Нет.

## Impact

Изменяется только ownership нормативной документации. Go runtime, CLI,
пользовательский YAML, sealed JSON packages, acceptance evidence и historical
manifests не меняют поведения.

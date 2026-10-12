## Why

Реализация issue #3 влита в `main`, но provider 429 остановил оставшиеся живые
Claude UI-проверки. Владелец явно отложил их и запросил отдельный follow-up,
чтобы архивирование реализации не превращало непроверенные случаи в PASS.

## What Changes

- После восстановления доступа проверить настоящий Claude `AskUserQuestion`
  для declared finite preflight ответа, страницы по фактическому лимиту tool
  с Next/Back/Defer и отдельный dismissal без ответа/default.
- Сохранить воспроизводимые prompts, host/model/mode/tool contracts, candidate
  hashes, реальные calls/replies и независимые manifests repository/authority.
- Записать PASS/FAIL/BLOCKED отдельно по каждому случаю. Исправления продукта,
  если они потребуются, оформлять отдельным согласованным change.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

Нет. Это только процесс приёмки существующего поведения; `skip_specs: true`.
Нормативная ownership ни одной capability не меняется. Источники требований:
[cli-protocol](../../specs/cli-protocol/spec.md),
[control-security-ux](../../specs/control-security-ux/spec.md) и
[workflow-and-context](../../specs/workflow-and-context/spec.md).

## Impact

Только fixtures и новая observation evidence после отдельного разрешения на
проверку. Runtime, schemas, workflow graph, runner instructions и Go baseline
этим follow-up не меняются. Тестовый product Run, package import, claim и start
не нужны. Провайдер, модель и режим пользователя не переключаются ради теста.

Исходное состояние — [архив реализации](../archive/2026-10-12-add-project-launch-advice/observations.md):
commit `d0cb89a568eddef08634d2dafba3da55c8cda82b`, завершённый Run
`run:274c1d674dbef5d7d4740702de340aacc5b04da8629bb443a8356ee4a8354fd7`,
finite attempt resume `0c2dff97-c875-48aa-bb10-9504b8ae8b17` без native вопроса.
Архивные evidence сохраняют прежние результаты; будущие наблюдения записываются
здесь. Статус сейчас DEFERRED: новые проверки не запускались.

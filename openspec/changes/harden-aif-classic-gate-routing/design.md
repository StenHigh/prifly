## Context

См. `proposal.md`. В current package repair loop уже существует внутри
`verify-once` и `review-once`, однако выход `needs_revision` bypass-ит choice
и становится terminal `partial`. Один реальный Codex host выбрал именно этот
выход, хотя передал gate с исправимыми findings.

## Goals / Non-Goals

**Goals:**

- Сделать repair route устойчивым к понятной, но неверной классификации
  findings как `needs_revision`.
- Сохранить отдельный terminal path для отсутствующего gate, owner-only
  blockers и безопасно неустранимых findings.
- Проверять оба gate loops как package behavior.

**Non-Goals:**

- Не менять Core verdict semantics, `step-result` schema или historical Runs.
- Не требовать subagent/fork от host, который не предоставляет такого механизма.
- Не добавлять новые retries, фонового driver либо model-specific behavior.

## Decisions

### Route usable gate verdicts к existing choice

В `verify-once` и `review-once` handler `needs_revision` будет вести к
`decide`, как и `pass`. Choice уже различает owner-only blocker, repairable
blocker и clean result по typed gate fields; это самый малый shared guard.

Альтернатива — требовать от host только `pass` — отклонена: она сохраняет
одну ошибку формулировки как terminal state. Альтернатива — заменить
`needs_revision` в runtime — отклонена: verdict является сохранённым фактом
Attempt и не должен подменяться engine.

### Уточнить adapter, но не полагаться на него

Bridge contexts и README будут использовать один язык: gate returns artifact,
engine selects next Attempt, host does not invoke a skill outside its task.
Текст предотвращает ошибку, а граф предотвращает её последствия.

### Сделать host control loop явным

`prifly-run` будет содержать короткую таблицу действий по `run next`: control
и program продвигаются `run drive`, assisted_session — ровно одной выданной
Attempt, waiting/terminal не мутируются. После каждого session submit host
снова читает `run next`; он не считает выдачу Attempt запуском следующего шага
и не останавливается между узлами. Separate session используется только когда
платформа реально даёт способ её создать; иначе host выполняет Attempt сам и
честно сообщает unavailable provenance.

Альтернатива — требовать Codex subagent для каждого шага — отклонена: такая
команда не существует на всех host platforms, а ложный результат хуже
односессионного исполнения.

### Проверить authoring source через compiled route

Regression check создаёт/компилирует package и исполняет minimal assisted
session fixture, в которой gate возвращает `needs_revision` с typed blocking
artifact. Проверка утверждает выдачу fix Attempt и повторный gate; отдельный
case подтверждает terminal owner-only path. Это проверяет sealed graph, а не
только строки YAML.

## Risks / Trade-offs

- [Host вернёт `needs_revision` без gate] → output contract остаётся required;
  submission не принимается как usable result.
- [Host маскирует реальную неспособность проверить] → bridge сохраняет правило:
  `needs_revision` без established gate означает inconclusive terminal path.
- [Package bytes меняются] → поднять authoring package/component versions;
  `origin` остаётся provenance импортированного upstream source и не выдаётся
  за новую external release. Старые Runs остаются pinned на прежних bytes.

## Migration Plan

1. Добавить regression fixture и зафиксировать текущий terminal bypass.
2. Изменить package YAML и pinned prose, поднять authoring versions.
3. Скомпилировать package, выполнить fixture и existing project checks.
4. Проверить runner instruction текстовым scenario для control/program/assisted
   action и fallback без subagent.
5. Новые Runs seal-ят новую package revision; созданный Run не переписывается.

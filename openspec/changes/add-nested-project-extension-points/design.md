## Context

См. proposal.md. Released v0.13.82 с SHA-256 `3b073126921bca3e9d78b7649b51bd20527fd70cf4de44e0d440b5428b7712e5` воспроизводит success/repair baseline. В repair reader вызывается дважды, root measurement один раз. Nested target получает `project_extension_unknown_workflow`. Exact consumer fixture source: `95ea2cb77b69570c24891694e8e23bcf2d9102cb`.

Existing primitives сохраняют artifacts, но не предоставляют authoring point. Settings меняют только input defaults. Call refs статические; unresolved project hook проверяется даже за disabled Choice. Package reference и alias нельзя подменить. Это ограничение composition, не transport.

## Goals / Non-Goals

**Goals:** package-author управляет границей подключения; project-author выбирает реализацию и явные outputs. Поддержаны root и вложенный owned local workflow, включая body native repeat/call. Один point заменяет только entry либо один direct route.

**Non-Goals:** arbitrary patch, импорт пакетов, provision, test selector, новый runner/cache, runtime operator, nested-stage resume. Consumer issue #9 отдельно определяет пригодность report.

## Decisions

### Закрытый sidecar и независимая версия

`extension-points.yaml` рядом с workflow.yaml использует `authoring: prifly-extension-points/1`. Он принадлежит пакету. `prifly-extension/4` явно включает новый branch; `/3` не занимается, поскольку отдельный change использует его для композиции.

```yaml
authoring: prifly-extension-points/1
points:
  before-read:
    workflow: probe:workflow/round
    entry: read
    input_bindings: {carrier: $inputs.carrier}
    consumers:
      reader: {stage: read, input: carrier}
      repair: {stage: repair, input: carrier}
      clean: {stage: clean, output: carrier}
    routes:
      step:
        on: {pass: read, needs_revision: blocked, blocked: blocked, fail: stopped, no_work: stopped}
      call:
        on: {succeeded: read, partial: blocked, rejected: stopped}
        on_error: stopped
  after-repair:
    workflow: probe:workflow/round
    between: {from: repair, to: fixed}
    input_bindings: {carrier: $stages.repair.carrier}
    consumers:
      repeated: {stage: fixed, output: carrier}
    routes:
      step:
        on: {pass: fixed, needs_revision: blocked, blocked: blocked, fail: stopped, no_work: stopped}
      call:
        on: {succeeded: fixed, partial: blocked, rejected: stopped}
        on_error: stopped
```

```yaml
authoring: prifly-extension/4
extensions:
  - id: measure-before-read
    point: before-read
    step: measure
    output_bindings: {reader: carrier, repair: carrier, clean: carrier}
  - id: measure-after-repair
    point: after-repair
    call: measure-child
    output_bindings: {repeated: carrier}
```

Point routes — fixed package-owned `on`, optional `on_error` и step-only `impossible_verdicts`. Project entry не принимает workflow/between/input_bindings/on либо неизвестные keys. Каждый consumer alias обязан иметь ровно один producer output. Consumer input либо finish output должен уже существовать. Один target port меняется не более одного раза. Project implementation должен жить в `project/steps/` либо `project/workflows/`; call closure использует обычные declared refs.

`enabled: false` полностью сохраняет passthrough. Отсутствующее расширение оставляет original entry/routes/bindings. Manifest всё равно проверяется и входит в identity, когда присутствует.

### Compile до sealing

Manifest выбирает workflow по полному author ID; отсутствующая или неоднозначная цель отказывается. Target item ждёт compiled step/call, затем compiler вставляет native stage с ID extension.id, фиксированными inputs/routes и явными consumer bindings. Parent получает новый child digest обычным substitution. Cycle detection учитывает target -> implementation. Native compiler проверяет ports, schema, producer availability, verdicts/outcomes, graph cycle и guaranteed outputs.

Entry point применим только к exact текущему entry. Between point требует ровно один direct route. Несколько выбранных points на один anchor или consumer отказываются. Native Call/Repeat/Choice/Parallel internals не патчатся; target — отдельный local WorkflowRevision.

### Identity, effects и recovery

Manifest, selected/disabled entries, новый graph и program supporting bytes входят в build key; read paths включают sidecar. Prepare/start проверяют прежний review digest. Новая compilation не меняет package files либо pinned Runs. Read-only reader сохраняет effects:none. Measurement получает только свои declared effects и execution consent.

Partial сохраняет native meaning. Обещанный output обязателен на соответствующем route; error/cancel не выдумывает export. Repeat.next_bindings читает carrier принятой body iteration. Вложенные report/log refs сохраняют public ArtifactRef identity даже после удаления producer Attempt directory.

Reviewed continue работает на root frontier: child call/repeat выполняется повторно целиком. Accepted root prefix переносится штатно, source Run сохраняется. Nested-stage resume не поддерживается. Изменившийся accepted prefix получает обычный refusal; stale output не переносится из провалившегося frontier.

## Risks / Trade-offs

- Дополнительные версии authoring → отдельные closed schemas; legacy bytes и unknown `/3` остаются прежними.
- Гарантии output на partial/error → native validation плюс runtime missing-output tests; без fallback на прежний carrier.
- Новая dependency может создать cycle → добавить edge до compile, а не ждать generic unresolved values.
- Synthetic fixture не квалифицирует настоящий provision/release → явно указать границу в example/evidence.

## Migration Plan

Новая возможность opt-in. Старые extend files не требуют миграции. Выпуск binary и downstream consumer acceptance выполняет владелец отдельно после локального коммита.

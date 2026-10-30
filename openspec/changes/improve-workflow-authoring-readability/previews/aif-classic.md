# Предложение для aif-classic

Кандидат подготовлен на `aif-classic` 1.47.0 из release repository `v1.53.0`.
Полный проверенный diff — [handoff](../handoff/aif-classic-readability.patch).
Технические результаты — [acceptance.md](../acceptance.md). Владелец принял кандидат по review без UX-замеров; см. evidence/owner-review.json.

## Структура выбранного участка

```text
aif-classic/
  README.md                         обзор root и отдельная карта verify
  workflow.yaml                     прежний root graph
  steps/verify.yaml                 прежний step contract
  workflows/
    verify-once.yaml                 один gate и возможный fix
    verify-batch.yaml                выключатель и bounded repeat
  contexts/aif-verify-bridge.yaml    metadata и source вместо text
  instructions/
    aif-verify-bridge.md             новый файл; прежние bytes инструкций
```

Показан участок, не полный inventory package. Остальные файлы остаются на своих
местах. `contexts/aif-verify.yaml` продолжает читать выбранный host skill.
`extend.yaml` потребителя не заменяется настройками этого preview.

## Контракт steps/verify.yaml

После — полное содержимое выбранного файла, кандидат без изменения поведения:

```yaml
authoring: prifly-step/2
id: aif:step/verify
version: 1.16.0
title: Run read-only aif-verify over the implementation
kind: worker
inputs:
  implementation:
    schema_ref: '{{schema_implementation}}'
  plan:
    schema_ref: '{{workspace_tree_manifest}}'
outputs:
  gate:
    schema_ref: '{{schema_gate-result}}'
    required_for: [pass, needs_revision, blocked]
executor:
  adapter_ref: '{{assisted_adapter}}'
  operation: session
instructions_ref: "{{context_aif-verify-bridge}}"
context_refs: ["{{context_aif-verify}}"]
effects:
  class: none
  retry_class: pure
session_limits:
  active_timeout_ms: null
result_schema_ref: "{{step_result_schema_v2}}"
# Read-only materialisation: the engine lays the plan out where the pinned
# skill's own discovery looks and takes it back after the step. No
# `output_port`, so nothing is captured and a write to it is refused.
workspace_trees:
  - input_port: plan
    capture: "{{plan_capture}}"
```

`active_timeout_ms: null` не заменяется default. Read-only plan capture и
отсутствие `output_port` сохраняются. Файл по-прежнему выбирает session executor.

## Контекст вместо длинного text

После — полное содержимое `contexts/aif-verify-bridge.yaml`:

```yaml
id: aif:context/aif-verify-bridge
version: 1.16.0
media_type: text/markdown; charset=utf-8
source: .prifly/workflows/aif-classic/instructions/aif-verify-bridge.md
```

Новый Markdown содержит все decoded bytes старого `text: |`. Проверенный
baseline: 9938 UTF-8 bytes, завершающий LF,
SHA-256 `8772b0ef8121540cceb00cd912ba005ef434ed7fe467fde6e88913262a1fd24c`.
Это digest содержимого, не digest всего context resource или package.
Identity bridge 1.16.0 сохранена. Полные bytes сравниваются также после compilation.

Начало Markdown показано только для review. Это фрагмент, не весь будущий файл:

```markdown
# Pri-Fly adapter for the pinned aif-verify skill

The following supporting context is the pinned upstream `aif-verify` skill.
Verify the implementation against what the plan promised. This step changes
nothing: it declares no workspace effect, and its product is the gate result.
```

Инструкции не пересказываются. Frontmatter и новые data fields не добавляются.
Обычный Markdown preview и diff становятся доступны независимо от YAML editor.

## Важная ветка verify-once

После — фрагмент `stages.decide`; прочие поля и stages сохраняются:

```yaml
decide:
  kind: choice
  selection: first_match
  branches:
    - id: owner
      predicate:
        op: eq
        left: $stages.verify.gate#/blocking_owner_only
        right: true
      next: owner-decides
    - id: blocking
      predicate:
        op: eq
        left: $stages.verify.gate#/blocking
        right: true
      next: fix
  default: clean
```

Комментарий исходника о причине `first_match` сохраняется при реализации;
здесь он не повторён. Нельзя переставлять branches или заменять selection.
Owner-only требует решения владельца и не превращается в автоматический fix.

В `verify-batch` отдельно раскрыть `initial_bindings`, `next_bindings` и
`on_complete`. Сохранить `max_iterations: 8`, default `verify_round_limit: 3`,
`continue_on: [partial]` и `on_limit: exhausted`. Предел source graph и выбранное
значение configuration показываются отдельно.

## Навигация README

| Что изменить или проверить | Где искать |
|---|---|
| Общий путь от warmup до commit | `workflow.yaml`: entry и stages |
| Инструкции bridge verify | `instructions/aif-verify-bridge.md` через context.source |
| Подключённый aif-verify skill | `contexts/aif-verify.yaml`: host_skills source |
| Передачу implementation и plan | `steps/verify.yaml` и input_bindings вызывающего workflow |
| Почему требуется владелец | `workflows/verify-once.yaml`: decide и owner-decides |
| Когда повтор заканчивается | `workflows/verify-batch.yaml`: round и terminal stages |
| Выбранное число повторов проекта | Его `extend.yaml.settings`; default — input configuration callee |

Карта verify должна показывать bypass, fix и исчерпание лимита. Краткий happy
path root не заменяет эти правила. Проверка полного graph включает неизменённые
improve, security, review и commit; их логика в этом срезе не переписывается.

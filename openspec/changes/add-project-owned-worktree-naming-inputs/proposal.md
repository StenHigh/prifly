## Why

Issue [#19](https://github.com/StenHigh/prifly/issues/19) обнаружил ограничение profile `/4`: именование читает только inputs выбранного workflow. Поэтому официальный `aif-classic v1.57.0` нельзя снабдить отдельными source/ID без изменения upstream graph или закрытой task schema.

## What Changes

- Ввести opt-in `prifly-project-profile/5` с типизированными входами внутри `worktree_naming.inputs`. Их схемами владеет проект-потребитель.
- Передавать значения отдельным `--naming-input NAME=FILE`. Существующий `--input` сохраняет назначение входов workflow.
- Добавить binding `naming_input` с JSON Pointer. Существующие `value` и `input` сохраняют значение.
- Показывать схемы, значения и источники naming inputs в questionnaire/prepare. Закреплять их digests в reviewed launch до claim effects.
- Сохранять исходные naming inputs вместе с metadata claim. Continue/resume используют сохранённые данные без новых defaults.
- Добавить executable пример install/update/prepare/start и native probe без LLM. Проверку на released Core учитывать отдельно от source tests.

Изменение затрагивает продукт: Project CLI, profile reader, review и metadata claim. Native workflow operators и task schemas остаются прежними.

## Capabilities

### New Capabilities

Нет. Изменение расширяет существующее именование Project worktree.

### Modified Capabilities

- `workflow-and-context`: project-owned naming declarations, typed inputs и независимость от upstream folder.
- `cli-protocol`: отдельный флаг, questionnaire, reviewed prepare/start и диагностика.
- `runtime-resources`: сохранение naming evidence в claim и наследование при продолжении.
- `published-contracts`: новые closed authoring/Project/claim формы и совместимость старых readers.
- `specification-governance`: точное определение naming input и соответствия Go/JSON.

Нормативное владение не переносится. Source sets остаются в соответствующих `openspec/specs/` по `openspec/SOURCE-OF-TRUTH.md`; этот change содержит только deltas.

## Impact

Затрагиваются `cmd/prifly/project*`, `internal/projectwire`, claim metadata в `internal/runtime`/`internal/local`, новые published schemas и author-facing references. Новые registry, runner, tracker adapter и универсальная task metadata не вводятся.

Приёмка включает неизменённый официальный package, stale refusal, отказы до claim, update с сохранением consumer files и продолжение исходного claim. Старые profiles, pinned Runs, schemas и archived evidence сохраняют bytes и meaning.

## Non-goals

- Не добавлять source/ID в task schema каждого workflow и не копировать development graph.
- Не извлекать ID из prose, queue metadata, URL или внешнего tracker.
- Не менять правила allocation, collision suffix, branch prefix или owner merge/push.
- Не объявлять release qualification по результату локального теста.

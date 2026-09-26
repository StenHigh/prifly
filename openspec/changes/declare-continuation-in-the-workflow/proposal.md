## Why

`project continue` и `project recover` работают только для одного пакета:
движок сравнивает workflow ID с `aif:workflow/classic` и
`aif-continuation:workflow/classic-continuation`, ищет результаты по именам
стадий `warmup`, `implement`, `review` и портам `task`, `handoff`, `plan`,
`implementation`, читает `base_commit`/`head_commit` из содержимого
артефакта и сам считает `changed_files` через Git. Это нарушает действующее
требование `product-model` «Core не закрепляет предметные имена» и концепцию
продукта: движок исполняет, workflow командует. Другой workflow, включая
запрос заказчика о продолжении после принятого `blocked`, этими командами не
продолжить и не восстановить.

Кроме того, незакоммиченная работа теряется: при каждом новом claim в
репозитории движок автоматически освобождает деревья всех завершённых Runs,
удаляя каталог и ветку, каким бы ни был их исход.

Исправление идёт первым: от него зависит изменение обработки `blocked`.

## What Changes

- **Checkpoint.** Workflow объявляет схему своего checkpoint — точки, с
  которой можно продолжить, — и стадии, чей выход её сообщает. Шаг
  сообщает checkpoint обычным выходом; автор делает его обязательным для
  нужных вердиктов, включая `fail` и `blocked`. Движок не читает содержимое
  checkpoint и не знает, commit это или нет; последний принятый checkpoint
  Run виден в его представлении.
- **Объявленное продолжение.** Workflow продолжения объявляет, от каких
  workflow и исходов он продолжает и откуда берёт каждый вход: вход
  исходного Run, принятый выход стадии корневого вызова или последний
  checkpoint. Проверки состояния (например, что дерево совпадает с
  checkpoint) делает первый шаг этого workflow, объявленный автором.
- **Передача claim.** Run продолжения или восстановления забирает claim
  рабочего дерева исходного Run: тот же каталог и та же ветка, поэтому
  незакоммиченные файлы сохраняются без участия движка. Передача
  записывается. Если claim уже освобождён, продолжение создаёт новый claim,
  восстановление отказывает.
- **Сохранение деревьев.** Автоматическое освобождение при новом claim
  снимает только деревья Runs с исходом `succeeded`,
  `completed_with_waivers` или `no_work`; деревья Runs с исходом `partial`,
  `rejected`, технически отказавших и отменённых остаются до передачи или
  явного `claim release`.
- **Нейтральное восстановление.** Условие на стадию `review` и сверка
  `head_commit` удаляются; восстановление работает в переданном дереве, как
  `run reopen`.
- **BREAKING:** из `internal/` и `cmd/` удаляются все литералы
  идентификаторов, стадий и портов пакета и вычисление `implementation` из
  Git. Launch без объявления продолжения отказывает в `project continue`;
  пакет AI Factory выпускает редакцию с объявлением и первым шагом проверки.
  `--implementation-head` заменяется нейтральным `--workspace-commit`: новый
  claim от указанного commit вместо передачи.
- Объявления несёт WorkflowRevision 7; ревизии 1–6, опубликованные схемы и
  сохранённые Runs не меняются.
- Guard-тест валит сборку, если в не-тестовом коде `internal/` или `cmd/`
  появляется идентификатор workflow, пакета или шага вне пространства
  `core:`.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `product-model`: «Новый сценарий выражается существующими контрактами»
  получает проверяемое следствие для продолжения и восстановления.
- `workflow-and-context`: авторинг checkpoint и объявления продолжения в
  WorkflowRevision 7, компиляция и отказы.
- `runtime-resources`: передача claim связанному Run и правило
  автоматического освобождения.
- `domain-execution`: продолжение и восстановление переносят только
  объявленное и не читают содержимое артефактов.
- `cli-protocol`: `project continue` и `project recover` для любого
  объявившего launch, `--workspace-commit`, отказы.

## Impact

Изменение затрагивает product runtime. Ownership нормативных источников не
меняется: все пять capability остаются в `openspec/specs/**` по
`openspec/SOURCE-OF-TRUTH.md`.

- Код: `internal/flow` (ревизия 7, схема, компиляция), `internal/runtime`
  (`continuation.go`, `recovery.go`, `start.go`, `claim_run.go`,
  `worktrees.go`, представление Run), `cmd/prifly`
  (`project_continuation.go`, `project_recovery.go`, `project_start.go`,
  справка), `schemas/core/workflow-revision-v7.schema.json`, authoring-схема,
  `examples/authoring/`, `examples/README.md`, `examples/troubleshooting.md`,
  guard-тест.
- Активные changes `continue-existing-implementation` и
  `retry-failed-stage-with-new-package` содержат требования с именами
  AI Factory; их delta specs переписываются в нейтральной форме в рамках
  этого change.
- Внешние: новая редакция `prifly-aif-workflows` (checkpoint, объявление
  продолжения, первый шаг проверки), затем пилот. Порядок выпуска: движок →
  пакет → пилот.
- Вне объёма: capability `aif-classic-workflow` в `openspec/specs/`
  описывает внешний пакет внутри репозитория движка; её перенос — отдельное
  решение владельца.

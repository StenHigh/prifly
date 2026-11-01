# Композиция workflow

Нужен Core build с capability `workflow_composition`, Project profile `/3`
или `/4`, folder `/2` и extension `/3`. Сохранённые Runs и старые схемы не меняются.
[Справочник полей](../authoring/workflow-composition-authoring-reference.yaml)
показывает новые markers, exports, imports, parents, layers и groups.

```text
                 classic + delivery layer
                    /              \
          classic-ui             classic-audit
          UI/UX Call             audit contribution
                    \              /
                  smsplace: arrange
                         |
       tests → обязательная приёмка → commit → DraftMR
```

1. Зарегистрируйте источники в `project.yaml.packages`.
2. Экспортируйте собственный workflow или step из provider `workflow.yaml`.
3. Укажите родителя через `extends`, а source export через `imports`.
4. Добавьте extension на точный переход. При пересечении объявите `arrange`.
5. Проверьте `project compile --explain` и reviewed prepare перед запуском.

Для smsplace основной extension выглядит так. Сигнатуру должен отдельно
выпустить пакет `aif-ui-ux`; этот пример не утверждает, что export уже опубликован.

```yaml
authoring: prifly-extension/3
imports: {ui: aif-ui-ux}
layers: [../../layers/delivery.yaml]
extensions:
  - id: interface-review
    workflow: root
    between: {from: tests, to: commit}
    call: ui.review-for-change
    input_bindings:
      task: $inputs.task
      implementation: $stages.review.implementation
    on: {succeeded: commit, partial: abandoned, rejected: abandoned}
    on_error: abandoned
```

Вызов `review-for-change` обязателен. Provider проверяет task и exact candidate,
затем записывает required, not-required с причиной либо unknown. Required запускает
UI/UX reader. Not-required обходится без reader Attempt. Unknown и недоступная
обязательная проверка запрещают commit/MR. Изменение PHP тоже может затронуть интерфейс.
Core не определяет применимость по расширению файла.

Общий delivery layer может импортировать tests и DraftMR из одного provider.
Два consumer используют одни source workers. Нельзя одновременно оставить прежнюю
вставку tests/MR в consumer и добавить её вторую копию через слой.

`extends: [classic-ui, classic-audit]` объединяет родителей с одной exact основой.
Общая основа и общие additions включаются один раз. Порядок родителей не разрешает
конфликт settings и не задаёт порядок проверок. Для одного перехода нужен sequence,
parallel либо choice. Mandatory UI/UX следует размещать вне альтернатив choice.

Слои применяются после основы и до consumer extend. Настройка input заменяется
целиком. Model profile заменяется целой записью host/profile, resource — целой
записью. Несогласованные значения родителей требуют явного consumer override.
Пути layers относительны объявляющему документу. Symlinks, выход за repository,
повторения и вложенные layers запрещены. Пределы: depth 8, 32 source folders,
32 layer files; дополнительные native/file limits остаются обязательными.

Source import включает только выбранный export и reachable dependencies.
Standalone extend и launch preflight provider не участвуют. `extends` отдельно
наследует effective settings и extensions. Обновление provider меняет следующий
review digest. Старый Run продолжает читать прежние pins.

Existing recovery rules остаются действующими. Sequence subtree переносится при
неизменном accepted prefix. Native parallel в переносимом prefix пока даёт
`recover_topology_unsupported`; явный `--from-stage extra-checks` повторяет всю группу.
После группировки resource record должен называть существующую root-стадию.
Compiler отказывает при прежнем имени перенесённого contribution; scope не меняется молча.

Сквозной synthetic пример запускается из checkout:

```sh
go build -o /tmp/prifly-composition ./cmd/prifly
python3 -B test/e2e/verify-workflow-composition.py \
  --binary /tmp/prifly-composition --report /tmp/composition-report.json
```

Harness создаёт временные repository и authority. Он проверяет три applicability
исхода для sequence, parallel и вложенного typed choice. Shared tests/DraftMR —
настоящие program Attempts с явными allowlist и consent. Их работа синтетическая.
Он не запускает реальный браузер, Git commit или внешний MR. Разработка этого change
и выполнение harness не требуют `prifly-run`.

Composition `/3` сохраняет отдельную границу от выпущенных extension points `/4`.
Root, parent или imported provider с `extension-points.yaml` дают typed
`project_composition_extension_points_unsupported` до объединения документов.
Effective `/4` и point entries в layers дают тот же отказ. Library import без
sidecar игнорирует standalone provider `extend.yaml`, включая `/4`.
Выбор импортированных points требует отдельного совместимого контракта.

Именованная parent group сохраняется целиком при повторном `arrange`.
Повторный group ID даёт `project_extension_stage_conflict`. Один source contribution
не может одновременно входить в разные inherited groups или в группу и отдельную
ветвь: это `project_composition_duplicate_contribution`. Вынесите общую группу
в общего ancestor и наследуйте её один раз. Effective depth групп, включая
унаследованные группы и безымянные wrappers, ограничена восемью уровнями.

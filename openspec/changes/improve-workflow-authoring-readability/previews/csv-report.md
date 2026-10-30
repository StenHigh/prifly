# CSV report — только короткая контрольная проба

Это существующий пример из Pri-Fly, а не новый workflow. Исполняемые файлы
и README в репозитории не меняются. Основное предложение — [aif-classic](aif-classic.md).

## Один переход до и после

Во временной копии `workflow.yaml` меняется только оформление `stages.parse`.
Остальные stages, steps, schemas, execution bindings и worker сохраняются.

До — существующий фрагмент:

```yaml
parse:
  kind: step
  step_ref: "{{step_parse}}"
  input_bindings: {csv: $inputs.csv}
  on: {pass: validate, fail: rejected, needs_revision: rejected, no_work: rejected}
  on_error: rejected
```

После — тот же объект в блочной записи:

```yaml
parse:
  kind: step
  step_ref: "{{step_parse}}"
  input_bindings:
    csv: $inputs.csv
  on:
    pass: validate
    fail: rejected
    needs_revision: rejected
    no_work: rejected
  on_error: rejected
```

## Одна контрольная задача

Покажите человеку оба фрагмента и попросите найти следующий шаг и остановку
при отказе. Запишите короткое наблюдение: стало понятнее, без разницы или хуже.
Отдельные четыре задания и таблица времени для CSV не нужны.

Проверьте равенство parsed value всего workflow, compile обеих форм и прежний
sample Run. Ожидаются `succeeded` и точные bytes отчёта:

```text
Pri-Fly CSV report
Rows: 3
Total: 24
```

Этого достаточно для контрольной пробы. Новые CSV tests, матрица отказов,
изменения `steps/*.yaml`, README и отдельная CSV-приёмка в scope не входят.
Команды и результат пробы попадут в общий `acceptance.md` при реализации.

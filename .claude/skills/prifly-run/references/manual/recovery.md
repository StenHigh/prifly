[Индекс](index.md) · [Решения](decisions.md) · [Рабочее дерево](workspace.md)

# Продолжение и восстановление

Сначала прочитайте `run next`, `run status` и `run explain` текущего Run.
Technical failure, принятый `fail`, `partial`, `blocked`, pause и unknown effects требуют разных действий.
Exit процесса не выбирает правильную процедуру вместо authority.

## Как выбрать операцию

| Состояние и цель | Операция | Что сохраняется |
|---|---|---|
| Paused/stopped существующий Run | Разрешённый release, затем `run resume` и drive | Тот же Run и sealed package |
| Технически failed stage без итогового outcome | `run reopen` по next action | Принятые стадии прежнего Run |
| Finished Run, следующий объявленный workflow | `project continue` другим declared launch | Declared carried inputs и source tree |
| Finished Run с `resumable`, повтор тем же workflow | `project continue` тем же launch | Accepted prefix и original inputs |
| Технический сбой, исправлен package | `project recover --prepare` | Совместимый accepted prefix и execute frontier |
| Неизвестен внешний effect | Owner inspection, затем `run resolve` | История obligation; успех работы не заявляется |
| Низкоуровневое ветвление exact sealed refs | `run fork --file REQUEST.json` | Отдельный linked Run; это не project continuation shortcut |

## Continue другим workflow

Выполните обязательные [updates до prepare](installation.md).
`run next` законченного Run перечисляет `continuations`. Это список declarations, а не разрешение admission.
Выберите реально объявленный launch нужного workflow.

```sh
"$PRIFLY_BIN" project continue --repository "$repository" --launch "$LAUNCH" \
  --source-run "$SOURCE_RUN" --host codex-app --prepare --json
"$PRIFLY_BIN" project continue --repository "$repository" --launch "$LAUNCH" \
  --source-run "$SOURCE_RUN" --host codex-app \
  --expected-launch-digest "$REVIEW_DIGEST" --allow-execution --json
```

Покажите происхождение carried inputs, исходное дерево со всеми оставленными файлами, claim и exact package.
Применяйте одинаковые reviewed options к prepare/start. Не извлекайте ArtifactRefs вручную из внутренних JSON/SQLite.
`project_continue_undeclared` требует другого объявленного launch или остановки.
`project_continue_active_child` называет уже активного ребёнка: сначала изучите его.
`--allow-duplicate-continuation` на обоих вызовах допускает только намеренно независимую работу, явно рассмотренную в summary.

## Resume тем же workflow

Если список continuations включает workflow самого Run, same-launch continue может возобновить его.
Workflow должен объявлять `resumable`; без declaration будет `resume_undeclared`.
Original inputs берутся из source Run. Не передавайте новые `--input` для подмены задачи.
Без explicit override используются sealed profile, policy и answers источника.
`--from-stage STAGE` выбирает более раннюю принятую корневую стадию, включая inherited prefix; nested stage не является legal root.
Prepare показывает recovery frontier, reused stages и дерево. Снова проверьте digest перед start.
Повторная цепочка resume переносит inherited и local accepted stages вместе.

Source должен быть текущим держателем claim. Не возвращайтесь к старому ancestor и не reset-ите его tree.
`resume_prefix_empty` означает реально пустой reviewed prefix; dirty bytes сначала сохраняются по решению владельца.
`recover_evidence_unavailable` означает недоступное ancestry evidence; предположение о прежнем успехе его не заменяет.

## Reopen и recover

```sh
"$PRIFLY_BIN" --project "$authority_root" run reopen "$RUN" \
  --expected-version "$RUN_VERSION" --reason 'Исправлена техническая причина' --json
"$PRIFLY_BIN" --project "$authority_root" run drive "$RUN" --json
```

Reopen не переводит принятый бизнес-исход в технический сбой и не применяет новый package автоматически.
Для исправленного package используйте reviewed recovery:

```sh
"$PRIFLY_BIN" project recover --repository "$repository" --launch "$LAUNCH" \
  --source-run "$SOURCE_RUN" --host codex-app --prepare --json
"$PRIFLY_BIN" project recover --repository "$repository" --launch "$LAUNCH" \
  --source-run "$SOURCE_RUN" --host codex-app \
  --expected-launch-digest "$REVIEW_DIGEST" --allow-execution --json
```

Nested call/repeat и preparation failure проверяются по execute frontier и pinned evidence.
Поддержка конкретного recovery пути зависит от установленной версии; проверяйте capabilities и diagnostic.
Руководство [continuation-guide](../examples/authoring/continuation-guide.md) фиксирует минимальные revisions и known recovery fixes.

## Cancelled writer и неизвестные эффекты

```text
cancel/interruption -> inspect actual effects -> owner resolve
                                                |
                                                v
                         drive settlement -> review recovery -> execute again
```

External-write программа без принятого результата может стать `uncertain` с `external_write_unreconciled`.
`idempotent` означает авторское обещание безопасного повторного обеспечения состояния; Pri-Fly не проверяет его истинность.
`deduplicated`, `reconcile_required` и `never` требуют разрешить неизвестное обязательство перед повтором.
`pure` несовместим с external_write.

```sh
"$PRIFLY_BIN" --project "$authority_root" run resolve "$RUN" --attempt "$ATTEMPT" \
  --outcome applied --reason 'Владелец проверил внешнее состояние' --json
```

`applied|not_applied` выбирает владелец после inspection. Не угадывайте значение по отсутствию stdout.
Resolution удостоверяет effect, а не успешное выполнение задачи. Затем drive выполняет settlement.
Поддержанный same-launch continue/recover повторит writer и нужные following quality stages.
Отмена source Run остаётся в истории.

`workspace_tree_output_exists` означает занятую capture path. Сохраните partial bytes отдельно и получите явное решение очистки именно этой path.
Не удаляйте файлы автоматически и не принимайте partial plan за готовый.
`workspace_tree_input_drift` требует exact sealed input manifest; наличие похожего файла недостаточно.
Если takeover уже создал failed child, текущим source становится этот держатель claim.

См. также [контроль](control.md), [симптомы](troubleshooting.md), [blocked guide](../examples/authoring/blocked-guide.md).

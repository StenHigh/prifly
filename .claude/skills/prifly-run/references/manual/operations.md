[Индекс](index.md) · [Наблюдение](observability.md) · [Диагностика](troubleshooting.md)

# Очередь, ресурсы и хранение

## Локальная очередь задач

Queue включается только явным `project queue`; это не фоновый scheduler.
TaskInput bytes, revision, owner и generation относятся к конкретной заявке.
Claim-next сохраняет владение заявкой, но не выбирает launch и не начинает Run.

```sh
"$PRIFLY_BIN" project queue add --repository "$repository" --input TASK.json --priority 0 --command-id ADD_ID --json
"$PRIFLY_BIN" project queue list --repository "$repository" --json
"$PRIFLY_BIN" project queue claim-next --repository "$repository" --owner SESSION --command-id CLAIM_ID --json
"$PRIFLY_BIN" project queue show --repository "$repository" --task-id TASK_ID --json
```

При selection передавайте все четыре значения из текущего queue record:
`--task-id`, `--task-owner`, `--task-revision`, `--task-claim-generation`.
Частичная selection отказывается. Не берите generation из другого task или прежнего owner.
Questionnaire/prepare/start связывают выбранный task с reviewed запуском обычным путём.
Не заменяйте `request_bytes` пересказом и не добавляйте необъявленный task port.

```sh
"$PRIFLY_BIN" project queue update --repository "$repository" --input UPDATED_TASK.json \
  --task-id TASK_ID --task-owner SESSION --task-revision N --task-claim-generation G \
  --priority 1 --command-id UPDATE_ID --json
"$PRIFLY_BIN" project queue complete --repository "$repository" \
  --task-id TASK_ID --task-owner SESSION --task-revision N --task-claim-generation G \
  --reason TEXT --command-id COMPLETE_ID --json
```

`release` использует ту же selection и reason, возвращая владение по контракту.
`complete` означает локальное завершение обработки, а не обещание merge/deploy.
При неизвестном dispatch используйте `show`, receipt и explicit `reconcile --intent-id ID --resolution not_started|effects_resolved`.
Resolution выбирается по recorded phase и проверенным фактам. Не повторяйте dispatch из-за таймера или потерянного ответа.
Все queue mutations требуют stable `--command-id`; list/show остаются reads.

## Capacity и exclusive resources

```sh
"$PRIFLY_BIN" --project "$authority_root" capacity show --json
"$PRIFLY_BIN" --project "$authority_root" capacity set --capacity N --reason TEXT --json
```

Действует меньшее из machine и workflow limits. Capacity разрешает число Attempt, а не даёт право на extra effects.
Project `resources` задаёт named resource, limit и корневые стадии; root call держит ресурс вместе с descendants.
Machine-local limit может дополнительно ограничить resource.
`resource_busy` называет holder Run; ничего нового не исполнилось.
Сообщите holder, дождитесь его settlement и drive тот же Run. Не поднимайте limit самостоятельно ради продолжения.
Capacity conflict, resource wait и claim conflict — разные причины.

## Storage status и reviewed budget

```sh
"$PRIFLY_BIN" --project "$authority_root" storage budget show --json
"$PRIFLY_BIN" --project "$authority_root" doctor --storage --json
"$PRIFLY_BIN" --project "$authority_root" storage budget set --bytes 1073741824 \
  --reason 'Обоснованное увеличение' --prepare --json
"$PRIFLY_BIN" --project "$authority_root" storage budget set --bytes 1073741824 \
  --reason 'Обоснованное увеличение' --expected-review-digest "$REVIEW_DIGEST" \
  --command "$COMMAND_ID" --json
```

Пример числа — 1 GiB, не рекомендуемый универсальный budget.
Budget — finite logical-page ceiling, а не свободное место OS. Диапазон target: 64 KiB..64 GiB.
Prepare read-only показывает target/current budget и review digest; apply требует тот же target/reason и stable command.
Изменившийся review отказывается. Поддержанный путь повышает budget; не обещайте произвольное уменьшение.
Apply не запускает Run заново. После успешного обслуживания продолжите existing Run разрешённым drive/recovery.
`doctor --storage` выводит diagnostic даже при ошибке; неизвестное состояние не называется здоровым.

## Integrity и claims

```sh
"$PRIFLY_BIN" --project "$authority_root" doctor --json
"$PRIFLY_BIN" --project "$authority_root" doctor --claim "$CLAIM" --json
```

Doctor читает integrity и текущие ограничения; он не даёт разрешение вручную менять authority state.
Storage migration и saved Run semantics имеют разные версии.
Старый writer не становится совместимым только потому, что новый read-only клиент открыл authority.

## Очистка через monitor

Full monitor показывает размер blocks, blobs и свободное место на диске authority root.
APFS clones/snapshots могут разделять blocks; оценка не является точным обещанием освобождения физических bytes.
Review удаления перечисляет exact candidates и объём; изменившийся план отказывается.
Pending/uncertain executions, unresolved effects, claims и dependent sources защищены.
Пакеты, shared bytes, настройки, audit и receipts сохраняются по контракту обслуживания.
Внешние repositories и цели symlink не удаляются.

```text
закрыть writers -> review candidates -> подтвердить exact plan -> cleanup -> VACUUM
```

Обслуживание требует exclusive access. Активный CLI/driver блокирует cleanup; операция его не прерывает.
Перед cleanup обновите clients и закройте старые writers, которые не участвуют в cooperative lock protocol.
VACUUM требует дополнительного места и может сообщить отдельную ошибку.
Удаление необратимо и не удаляет backup. Не заменяйте reviewed cleanup командой `rm -rf authority`.

См. также [monitor](observability.md), [storage recovery guide](../examples/authoring/storage-recovery-guide.md), [control/receipts](control.md).

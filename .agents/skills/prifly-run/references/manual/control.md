[Индекс](index.md) · [Данные](artifacts.md) · [Наблюдение](observability.md)

# Управление, неизвестный результат и полномочия

## Остановка и продолжение

```sh
"$PRIFLY_BIN" --project "$authority_root" run pause "$RUN" --reason TEXT --command-id ID --json
"$PRIFLY_BIN" --project "$authority_root" run cancel "$RUN" --reason TEXT --command-id ID --json
"$PRIFLY_BIN" --project "$authority_root" run stop "$RUN" --kind pause --reason TEXT --command-id ID --json
```

Pause и cancel задают разные ограничения. `stop --kind pause|cancel` использует явный kind.
Stop не доказывает, что внешний actor прекратил работу или что эффект отсутствует.
Driver interruption требует inspection записанного Run перед повтором.
Program process group получает SIGTERM, затем SIGKILL после grace; cleanup самого процесса также входит в эту группу.
Authority не обещает физически остановить произвольную внешнюю систему.

```sh
"$PRIFLY_BIN" --project "$authority_root" run release "$RUN" --expected-epoch N --stop ID:GENERATION --reason TEXT --json
"$PRIFLY_BIN" --project "$authority_root" run resume "$RUN" --expected-version N --reason TEXT --json
"$PRIFLY_BIN" --project "$authority_root" run drive "$RUN" --json
```

Release снимает exact stop generation. Resume и drive — отдельные действия; release не запускает скрытый driver.
Не используйте guessed epoch/version/generation. Новый stop после чтения может сделать прежнее действие stale.
Reopen и resolve применяются по [отдельной процедуре](recovery.md), а не как универсальное resume.

## Installation/project stops

```sh
"$PRIFLY_BIN" --project "$authority_root" control status --json
"$PRIFLY_BIN" --project "$authority_root" control stop --scope project --reason TEXT --command-id ID --json
"$PRIFLY_BIN" --project "$authority_root" control release --scope project \
  --expected-epoch N --stop ID:GENERATION --reason TEXT --json
```

Scope `installation|project` ограничивает новые admissions в выбранной области.
Снятие Run stop не снимает независимый authority stop.
Control status также сообщает enrolled session principal и доступ к объектам.
Не редактируйте SQLite, locks или epochs вручную.

## Потерянный ответ команды

Stable command identity отличает retry доставки той же команды от нового намерения.
При потерянном ответе сначала прочитайте receipt; не создавайте второй Run или второй внешний effect.

```sh
"$PRIFLY_BIN" --project "$authority_root" command receipt --id "$COMMAND_ID" --json
```

Там, где CLI принимает `--command-id`/`--command`, повтор используйте с тем же ID и теми же payload bytes.
Изменённые payload либо expected version — новое намерение, а не безопасный transport retry.
Некоторые read/start forms не имеют общего flag; сначала проверьте exact `help` выбранной операции.
Receipt подтверждает записанную команду, но не произвольное состояние внешнего сервиса.

## Approvals

```sh
"$PRIFLY_BIN" --project "$authority_root" approval policy --operation OP --quorum N --independence RULE --reason TEXT --json
"$PRIFLY_BIN" --project "$authority_root" approval request --operation OP --intent-digest DIGEST --reason TEXT --json
"$PRIFLY_BIN" --project "$authority_root" approval decide --id ID --decision approve --reason TEXT --json
"$PRIFLY_BIN" --project "$authority_root" approval revoke --id ID --reason TEXT --json
"$PRIFLY_BIN" --project "$authority_root" approval list --json
```

`approve|reject` относится к exact request и intent digest.
Approval проверяется при consume/dispatch с текущим authority state; изменение arguments не наследует прежнее разрешение.
Local actor provenance не доказывает независимость двух людей на одном OS account.
Не объявляйте обычный runtime decision техническим Approval.

## Grants

```sh
"$PRIFLY_BIN" --project "$authority_root" grant issue --subject ID --capability OP \
  --resource RESOURCE.json --max-operations N --lifetime-ms MS --reason TEXT --json
"$PRIFLY_BIN" --project "$authority_root" grant revoke --id ID --reason TEXT --json
"$PRIFLY_BIN" --project "$authority_root" grant list --json
```

Grant ограничивает subject, capability, optional resource, число операций и срок.
Выдача grant требует того же gate, что делегируемая операция. Grant не расширяет declared effects шага.
`max-operations` и `lifetime-ms` берите из явного разрешения владельца, не из удобного большого числа.

## Action protocol и waivers

```sh
"$PRIFLY_BIN" --project "$authority_root" action propose --file COMMAND.json --json
"$PRIFLY_BIN" --project "$authority_root" action admit --file COMMAND.json --json
"$PRIFLY_BIN" --project "$authority_root" session action --file COMMAND.json --json
```

Intent proposal, admission, delivery и observed effect — отдельные факты.
`action_delivery_prepared` означает подготовленную границу, а не реализованную внешнюю доставку.
Не обещайте выполнение HTTP/deploy только по успешно admitted intent.
Точные request fields получайте через `schema ActionIntent`, `schema ProposeActionCommand` и `schema AdmitActionCommand`.

```sh
"$PRIFLY_BIN" --project "$authority_root" run waive "$RUN" --step STEP \
  --check-id ID --check-version X.Y.Z --check-digest DIGEST --reason TEXT --json
"$PRIFLY_BIN" --project "$authority_root" run waivers "$RUN" --json
```

Waiver допустим только по policy для exact check ref. Он не является pass: итог сохраняет `completed_with_waivers`.
Открытое неизвестное effect obligation закрывается owner resolve после inspection; waiver его не устраняет.

См. также [восстановление](recovery.md), [решения](decisions.md), [нормативное управление](source-unavailable.md).

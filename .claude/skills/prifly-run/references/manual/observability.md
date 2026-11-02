[Индекс](index.md) · [Управление](control.md) · [Обслуживание](operations.md)

# Наблюдение, вывод и измерения

## Прочитайте минимальный нужный view

```sh
"$PRIFLY_BIN" --project "$authority_root" run list --limit 20 --json
"$PRIFLY_BIN" --project "$authority_root" run status "$RUN" --json
"$PRIFLY_BIN" --project "$authority_root" run next "$RUN" --json
"$PRIFLY_BIN" --project "$authority_root" run explain "$RUN" --json
"$PRIFLY_BIN" --project "$authority_root" run timing "$RUN" --json
```

`status` показывает состояние, outcome, failure, attempts, effects и recorded questions.
`next` выбирает текущий следующий ход; это основной host routing view.
`explain` на terminal Run называет finish stage, workflow invocation и edge/verdict, приведшие к исходу.
Accepted fail не становится technical failure только потому, что outcome неудобен.
Named `failure` объясняет технический разрыв. Unknown outcome не заменяется нулём или succeeded.

## Дочитайте events

```sh
"$PRIFLY_BIN" --project "$authority_root" run events "$RUN" --limit 100 --json
"$PRIFLY_BIN" --project "$authority_root" run events "$RUN" --limit 100 --after SEQ --json
```

Events находятся в `.view.events`, а не в `.events`.
`--limit` имеет диапазон 1..1000. Если view сообщает partial/next_after, продолжите с точным `next_after`.
Не читайте authority database для обхода pagination.
Общий список Run также не является источником полной истории конкретного Run.

## Stdout/stderr программы

```sh
"$PRIFLY_BIN" --project "$authority_root" run output "$RUN" --attempt "$ATTEMPT" --json
"$PRIFLY_BIN" --project "$authority_root" run progress "$RUN" --json
```

Output доступен только для executor с declared `live_output`.
View показывает newest 64 KiB stdout и stderr; offsets описывают пропущенный prefix.
`output_limit_bytes` в execution bindings ограничивает каждый поток отдельно и не является размером возвращаемого tail.
Предел output до 16 MiB; default 64 KiB. Превышение даёт `stdout_limit` или `stderr_limit`.
Не включайте live output задним числом в sealed Run и не раскрывайте секреты ради диагностики.

## Progress protocol

Программа может писать строки `program-progress/1` в descriptor из `PRIFLY_PROGRESS_FD`, обычно fd 4.
Phase, message и current/total описывают самосообщённый прогресс. Result channel fd 3 остаётся отдельным.
`reported`, `stale`, `not_reported` и `unavailable` имеют разный смысл.
Отсутствие progress не доказывает зависание, а `current:100` не означает accepted pass.
[Точный progress contract](https://github.com/StenHigh/prifly/blob/v0.13.87/schemas/core/program-progress.schema.json) задаёт форму строки.

## Timing и cost

Timing nodes различают Run, workflow invocation, stage activation, step instance, attempt и check execution.
`metrics` содержит числа; одноимённые `intervals` называют границы from_ref/to_ref.
Boundary `kind` имеет отдельный vocabulary и не равен node kind.
Host pickup, host work, decision wait и idle вычисляются по записанным границам, а не догадке о работе модели.
Reported costs — самоотчёт host. Provider tokens/usage не появляются автоматически.
Не складывайте метрики разных источников и не выдавайте отсутствующее значение за ноль.

```sh
"$PRIFLY_BIN" --project "$authority_root" telemetry catalog --json
"$PRIFLY_BIN" --project "$authority_root" telemetry query --file QUERY.json --json
```

Query использует declared telemetry contract; catalog показывает доступные показатели.
Telemetry records/aggregate не заменяют outcome, check evidence или подтверждение внешнего effect.

## Full monitor

```sh
"$PRIFLY_BIN" monitor --addr 127.0.0.1:7777
# Только для явного discovery вне registry:
"$PRIFLY_BIN" monitor --addr 127.0.0.1:7781 --scan-root '/projects:/history'
```

Monitor автозапускается при создании первого Run и не открывает браузер автоматически.
По умолчанию он читает зарегистрированные authority текущего OS пользователя, без обхода диска.
`--scan-root` явно включает обход; список путей использует OS separator.
Registry содержит пути, а не копию Run. Недоступный source не скрывает остальные и не восстанавливает удалённые данные.
Loopback и Host guards сохраняются. Remote access требует отдельного authenticated proxy с корректным upstream Host.

Run screen показывает sealed graph, фактические исполнения, inputs/outputs, решения и журнал.
Доска Run использует реальные states/ожидания; issued task без take не равен начатой работе host.
Фильтры и страницы не превращают counts текущей страницы в полный итог истории.
Переписка модели и её tool calls не подключены автоматически.

## Light

```sh
"$PRIFLY_BIN" monitor --ui light --addr 127.0.0.1:7782 \
  --full-monitor-url http://127.0.0.1:7777/
# При необходимости: --base-path /light/ --light-projection /absolute/path/mapping.json
```

Light — отдельная read-only доска native tasks и review results, не замена authority.
`full-monitor-url` за proxy задаётся явно. Mapping не угадывает phase/review по именам файлов или стадий.
При отсутствии mappings показываются реальные stage/reason и «нет данных» для неизвестного review.
Continue может сохранять native task lineage; независимый Run получает fallback.
Успех Run сам по себе не завершает продуктовую задачу, merge или deploy.
Статус released/production qualification проверяйте по установленной поставке; guide не доказывает rollout.
[Полный Light guide](../examples/authoring/light-guide.md) описывает поля, API и ограничения.

См. также [симптомы](troubleshooting.md), [storage cleanup](operations.md), [host protocol](hosting.md).

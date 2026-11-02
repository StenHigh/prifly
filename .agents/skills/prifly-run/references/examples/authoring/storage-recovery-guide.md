# Восстановление Run после исчерпания storage budget

Возможность `storage_budget_management` работает на authority, а не в YAML
сценария. Новая запись setting требует storage version **10**; новый CLI умеет
читать legacy storage и готовить review без миграции. WorkflowRevision и
контракт шага не меняются. `capabilities` показывает storage version и capability.

Лимит относится к allocated logical SQLite pages (`page_count × page_size`),
включая committed WAL view. `free_pages` — свободные страницы внутри allocation,
они всё ещё учитываются. Это не физический размер WAL, свободный диск, aggregate
ownership scan или размер отдельного snapshot. Stock default остаётся 512 МиБ.

Сначала прочитайте статус выбранной authority:

```sh
prifly --project /path/to/authority storage budget show --json
prifly --project /path/to/authority doctor --storage --json
```

Оба entry points возвращают `storage-budget-status/1`: `authority_id`, `epoch`,
`storage_version`, `cut`, `measurement`, `reason`, `usage` и `control`.
`usage.allocated_bytes`, `page_count`, `page_size`, `free_pages` и
`soft_limit_bytes` описывают один read cut. `control.limit_bytes`, `source` и
`generation` показывают effective ceiling и его происхождение. Generation 0 —
bootstrap, `source=owner` — явное изменение; unknown observation не становится
нулём. `doctor --claim ID` — отдельный, взаимоисключающий view ownership из #8.

Выберите достаточный **конечный** target сами. Ниже 1 ГиБ — пример явно выбранной
величины, не default и не автоматически предложенное значение:

```sh
prifly --project /path/to/authority storage budget set \
  --bytes 1073741824 --reason 'Сохранить принятые результаты Run' --prepare --json
```

`storage-budget-review/1` содержит `status`, `target_bytes`, `reason`,
`storage_from_version`, `storage_to_version` и `review_digest`. Проверьте authority,
epoch, current limit/generation, target/reason и upgrade plan. На storage 9
prepare ничего не пишет; применение новым writer выполнит показанный upgrade
9→10. Старые binary после него несовместимы, включая уже открытые старые writers.
Миграция сохраняет default и не повышает allowance сама по себе. Служебный
upgrade может уже сохраниться, даже если последующий set отказал; status
показывает фактическую version, отказ set не означает downgrade базы.

После проверки примените **тот же** request и digest, задав стабильный command ID:

```sh
prifly --project /path/to/authority storage budget set \
  --bytes 1073741824 --reason 'Сохранить принятые результаты Run' \
  --expected-review-digest sha256:REPLACE_WITH_REVIEW_DIGEST \
  --command command:raise-storage-001 --json
```

Результат `storage-budget-result/1` содержит `command_id`, `cut` и сохранённый
`control`: before (`previous_limit_bytes`), after (`limit_bytes`), generation,
actor/reason и command evidence. Exact retry возвращает тот же recorded result,
даже если позже лимит повышался ещё раз. Не меняйте payload под прежним command ID.
При потерянном ответе повторите exact command для reconciliation; не считайте
unknown успешной записью и не придумывайте новый request вслепую.

Set не запускает работу и не принимает pending outputs. Затем отдельно проверьте
состояние **существующего** Run и продолжите его обычным driver:

```sh
prifly --project /path/to/authority run status RUN_ID --json
prifly --project /path/to/authority run drive RUN_ID --json
```

Run ID, принятые artifacts/provenance и claim generation сохраняются; dirty и
untracked файлы остаются в его Workspace. Pending checker должен ещё пройти свою
проверку. Stops, unknown effects и обычные permissions продолжают действовать.
Уже открытый новый driver увидит committed ceiling на следующей транзакции;
перезапуск нужен для обновления старого binary, а не для чтения setting.

Первый срез разрешает только повышение: 64 КиБ…64 ГиБ, exact integer bytes,
непустой UTF-8 reason до 1024 байт и положительный headroom после записи самого
control/receipt. Zero/unlimited, уменьшение и same-value новая mutation
отказываются. `storage_budget_review_conflict` требует нового review;
`storage_budget_target_exhausted` — нового достаточного finite target.
`storage_budget_configuration_conflict` означает попытку использовать внутренний
bootstrap option как override уже сохранённого setting.

Повышение не обещает физического места, auto-grow, GC, retention или отсутствия
будущего overflow. `storage_budget_exhausted` по-прежнему возможен на новом
ceiling. Disk-full и corrupt/incompatible storage требуют своей диагностики.
Не удаляйте историю/Run и не подменяйте authority ради обхода guard. Downgrade и
ручное понижение storage version не являются recovery; migrated authority
нужен совместимый binary. Release qualification выполняется отдельно.

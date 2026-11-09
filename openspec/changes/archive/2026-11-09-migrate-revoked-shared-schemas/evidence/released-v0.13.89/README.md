# Задача 6.2: released acceptance v0.13.89

**PASS — 2026-11-09.** Все три generic public-CLI сценария прошли на опубликованном Linux binary.
Задача 6.2 закрыта после проверки evidence. Private authority acceptance не выполнялась.
Рабочая authority владельца не изменялась. Baseline версии 0.13.88 сохранён отдельно.

```text
qualified SHA -> signed GitHub Release -> downloaded binary -> generic acceptance
                                                                   |
                                                      verified temporary cleanup
```

## Идентичность поставки

- Release: [v0.13.89](https://github.com/StenHigh/prifly/releases/tag/v0.13.89).
- Fixed SHA: `30c3635081f8f012aef3953072dcd45d0807d618`.
- Platform: `linux/amd64`, Go `1.27.3`, reported version `0.13.89`.
- Archive SHA256: `1bf00adc2a1258ee6ab7c4781e4931e860a36d551bcc44febc2611219f8f3a28`.
- Binary SHA256: `90e3e37c79a88ae0279a2542e40e34d75afa524b2b3cf7188900830ea7fe2985`.
- Manifest SHA256: `5c5cd35f83ecb473cfec86071977e37ac36d218e8911c7ac191433c5623b875f`.

OpenSSL подтвердил обе Ed25519 подписи: RFC 8785 canonical signature и legacy signature.
Public key получен из repository variable `PRIFLY_RELEASE_PUBLIC_KEY`.
Digest архива совпал с подписанным manifest. Binary извлечён только из этого архива.
Значения archive digest и binary digest различаются и сохранены раздельно.
Файлы manifest, обе подписи и происхождение поставки находятся в `assets/` и `provenance.json`.

## Exact-SHA gates и публикация

| Проверка | Результат | Evidence |
|---|---|---|
| verify | PASS, direct full batch, attempt 1 | [GitHub Actions](https://github.com/StenHigh/prifly/actions/runs/42686688249) |
| qualify: race и полный e2e | PASS, direct full batch, attempt 1 | [GitHub Actions](https://github.com/StenHigh/prifly/actions/runs/42686698890) |
| Release: Linux и native Apple Silicon builds, подпись, публикация | PASS | [GitHub Actions](https://github.com/StenHigh/prifly/actions/runs/42690718528) |

`release-gates.json` закрепляет exact SHA, tag, время наблюдения и результаты gate selection.
`release-metadata.json` сохраняет опубликованный Release и identities его шести assets.
Владелец явно поручил выпуск. Environment approval относится только к этому release workflow и SHA.

## Generic acceptance

| Сценарий | Результат |
|---|---|
| `baseline-clean` | PASS: два workers без migration binding |
| `baseline-remove` | PASS: ordinary remove, новый supplier, два workers без migration |
| `scoped-migration` | PASS: две schemas, по два revoked legacy suppliers, reviewed apply, explicit Start и два workers |

Сохранён исходный отказ `package_not_resolvable`. Prepare показал полный scope и не изменил authority.
Stale plan отказал с `schema_migration_plan_stale`. Reviewed apply сохранил atomic receipt и decision.
Новый Run сохранил binding в `core-read/45`. Два workers завершились через durable wait и новые CLI processes.
Старый Run сохранил отказ `package_revoked`. Последующий revoke остановил второй worker с `schema_migration_decision_invalid`.
Точный retry вернул сохранённый receipt. Изменённый payload с прежним command ID дал `command_conflict`.
Restart означает новый CLI process между workers. Проверка не моделирует power loss.

Runner выполнил 70 CLI-команд. `evidence.json` содержит argv, exit codes и SHA256 каждого stdout/stderr.
Все 140 transcript files проверены по записанным digest и размеру перед сохранением.
Временные authority удалены. Все три собственные monitor registrations удалены и проверены по точным путям.
Fixtures, worker и transcript сохранены. Runtime state временных authority не сохраняется.

## Сохранённые материалы и границы

- `evidence.json`, `transcript/`, `fixtures/`, `worker.py` — неизменённый результат released runner.
- `harness/` — точный runner, collector, gate policy и исходное происхождение пакета приёмки.
- `verify-assets.py` — использованная проверка подписей и digest поставки.
- `SHA256SUMS` — контрольные суммы всех сохранённых материалов этого каталога.

Поле `task_closed: false` в raw evidence отражает границу runner: он не редактирует OpenSpec ledger.
После review этого результата ledger `tasks.md` закрывает задачу 6.2.
Этот результат подтверждает synthetic fixture. Он не подтверждает ремонт private authority или закрытие более широкого продуктового gate.
Owner handoff для отдельной private acceptance остаётся в `../owner-handoff.md`.
Archive и sync спецификаций выполняются отдельным этапом.

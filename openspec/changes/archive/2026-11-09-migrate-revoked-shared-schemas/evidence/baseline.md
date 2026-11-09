# Baseline до реализации

Дата: 2026-11-09. Это результат проверки issue, а не квалификация будущей миграции.

Binary: установленный Pri-Fly 0.13.88, linux/amd64, Go 1.27.3.
SHA-256: `43bb756b7c44fe4c858496678d67b9eac48fb420bc99920f7a59a9209f7a3e85`.

Воспроизводитель: `StenHigh/prifly-aif-workflows@8809214aa08ba2c4ed3efef666042dd9ad1526db`, `tests/probes/shared_schema_revocation.py`.

```sh
python3 -B tests/probes/shared_schema_revocation.py \
  --binary /home/dev/.local/bin/prifly \
  --output /tmp/prifly-issue21-reproduction-0.13.88.json
```

Команда выполнена из temporary checkout закреплённого consumer repository.
Она завершилась с кодом 0 и вывела три строки `reproduced`.
Выход 0 означает воспроизведение ожидаемого отказа revoke-case.

| Fixture | Inspect / prepare | Start | Runs |
|---|---|---|---|
| clean | PASS | succeeded | 1 |
| remove | PASS | succeeded | 1 |
| revoke | PASS | package_not_resolvable | 0 |

Fixture проверяет только `aif:schema/feature-enabled@1.0.0` с digest
`sha256:29bce348352ac8e11f65737dc77e990bc8f7687ad896e8757802ab1cd52557ee`.
Она не покрывает несколько schemas, apply или новые admissions после миграции.

Все три временные authority и три собственные monitor registrations удалены.
Fixture проверил отсутствие оставшихся active claims. Рабочая authority не использовалась.

Пять существующих runtime tests прошли на Core checkout `ade4d982`:
`TestRevocationIsTerminalAndNotUndoneByReimport`,
`TestRevokedPackageBlocksAdmissionWithoutReopeningEngine`,
`TestAComponentNeedsOneResolvableSupplierNotEveryPast`,
`TestQuarantineStopsResolutionAndIsReversible`,
`TestWithdrawalRespectsTheDependencyClosure`.
Результат: `ok github.com/stenhigh/prifly/internal/runtime 0.318s`, exit 0.

Полный JSON фактически выполненного воспроизводителя сохранён в
`baseline-0.13.88.json`. Результаты 0.13.86 из issue повторно не измерялись.

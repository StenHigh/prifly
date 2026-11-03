# Приёмка опубликованного Core для issue №19

Приёмка выполнена на опубликованном `v0.13.88`.
Release commit: `e1f714157e0b9932e0657100b66bd0a4d7282cd8`.
Полный результат: [released-core-v0.13.88.json](released-core-v0.13.88.json).

`acceptance.md`, `verification.md` и source JSON сохраняют состояние до публикации.
Их открытый released gate относится к тому source checkpoint.
Этот документ фиксирует последующую приёмку и закрывает task 5.4.

```text
verify + qualify -> release tag -> signed assets -> released probes -> acceptance
```

| Критерий | Результат |
|---|---|
| Prepare/start с отдельными naming inputs | Generic и official journey создали `.prifly/work/claims/demo-tracker-issue-19`. |
| Missing/invalid/stale до effects | Generic probe подтвердил ожидаемые отказы; ID не извлекается из prose. |
| Consumer update и upstream integrity | Official `aif-classic v1.57.0` сохранил consumer bytes и upstream inventory. |
| Продолжение по исходным metadata | Generic resume сохранил snapshot после удаления policy, schema и values. Source regressions покрывают handover, recovery и legacy source. |
| Native probe на released Core | Опубликованный Linux AMD64 binary прошёл 30 generic и 11 official CLI-команд без LLM и приложения. |

[Release](https://github.com/StenHigh/prifly/releases/tag/v0.13.88),
[verify](https://github.com/StenHigh/prifly/actions/runs/41613077967),
[qualify](https://github.com/StenHigh/prifly/actions/runs/41613148340),
[сборка и публикация](https://github.com/StenHigh/prifly/actions/runs/41615945726) завершились успешно на одном SHA.

Обе подписи manifest проверены через Ed25519 repository public key.
SHA-256 обоих архивов совпали с подписанным manifest.
Linux binary: `sha256:43bb756b7c44fe4c858496678d67b9eac48fb420bc99920f7a59a9209f7a3e85`.
Darwin asset прошёл native build, minOS и doctor в release workflow.
Generic probe локально выполнялся на Linux AMD64.

Raw [native-probe-released-v0.13.88.json](native-probe-released-v0.13.88.json) сохранён без изменения.
Его `released_acceptance:false` означает, что сам probe не классифицирует происхождение executable.
Отдельный binding `released-core-v0.13.88.json` связывает raw evidence с проверенным опубликованным asset и release SHA.

Первый candidate `84b4bd61` остановился на устаревшем e2e inventory.
Его race был отменён; успех ему не присваивается.
Commit `e1f71415` обновил inventory и modeline; новый полный `verify/qualify` прошёл.
Исторические source evidence и прежние schemas не переписаны.

# Приёмка reuse-unchanged-verification-results

Implementation candidate: `17b3986ef4caa5bfc7704c2b1f3dd77bca46bd2b`.
Base: `e4487d7e89391241e9bef8d690a41df1ac7c8ad0`.

## Локальные проверки

Полный состав candidate перед commit выполнен через `ci-check-once`:
fingerprint `40efd38aa9cebb38079c5cae1b1a512ec80506d7b4555e85e5f95d93c2442092`.
HEAD этого pre-commit evidence — base, рабочие bytes — implementation candidate.
После commit смена HEAD инвалидирует этот receipt; он не переносится на новый SHA.

Ранее на одном неизменном implementation снимке наблюдались executed и reused
с fingerprint `dc0dd7a968ef0e0cec726ceebf0afe33c7c9aaf6ba79e888d42459c3d853d682`:
второе обращение выполнило только свежий vulnerability scan и mod verification.
Затем исправление forced-result visibility создало новый snapshot и потребовало
нового полного candidate batch, указанного выше.

Helper/selector/guard/tag tests входят в `scripts-check`; failure, interruption,
corrupt logs, изменённые документы/bytes/modes/HEAD/toolchain/context, partial
staging, expiry, force и concurrency проверены counted fixtures. Force/error/
new-SHA CI ветви проверены fixtures, без дополнительных дорогих живых suites.
OpenSpec strict validation: 30/30; diff check прошёл. Итоговый снимок с этой
приёмкой отдельно проходит gate в текущем Run; его fingerprint и executed/reused
результаты сохраняются в runtime отчёте verify, не в самоссылочном receipt-файле.

## Живой CI на candidate SHA

| Workflow | Run | Наблюдение |
|---|---|---|
| verify full | [38297856861](https://github.com/StenHigh/prifly/actions/runs/38297856861) | PASS: Static gates, три Engine shard, Every package but the engine выполнены |
| verify duplicate | [38298448433](https://github.com/StenHigh/prifly/actions/runs/38298448433) | REUSED: preflight direct source 38297856861; тяжёлые jobs skipped |
| qualify full | [38298280873](https://github.com/StenHigh/prifly/actions/runs/38298280873) | PASS: Race detector suite и End-to-end checks выполнены |
| qualify duplicate | [38299353861](https://github.com/StenHigh/prifly/actions/runs/38299353861) | REUSED: preflight direct source 38298280873; race/e2e skipped |

Повторное обращение ensure-release-checks во время qualify вернуло wait с тем же
run ID, exit 3; второго dispatch не было. Observation timeout также остался
pending, а не success. После завершения full batches helper вернул success для обоих источников,
без dispatch. После обоих duplicates tag-release --dry-run принял candidate
17b3986 и назвал исходные полные run IDs, а не skipped wrappers. Непроверенный
SHA e4487d7 dry-run отклонил. Теги, push тегов и release не выполнялись.
Direct source проверяется по exact SHA, workflow identity,
latest attempt и реально исполненным required jobs/steps.

## Сохранённые границы

Protected paths относительно base не изменены: archived changes, schemas,
protocol.schema.json, historical release evidence и прежний release.yml.
Imported upstream aif-classic files и origin digest не изменены; standing
composition меняется только через project extend.yaml. Public prepare проверен
в disposable project; новый Run в нём не запускался.

Claude UI acceptance остаётся отдельным открытым change
`verify-claude-launch-choice-ui`, без отметок PASS. Эта приёмка не создаёт tag,
release, product roadmap qualification или перенос CI evidence между commits.
После фиксации отчёта документации новый commit потребует собственных CI gates
перед будущим релизом.

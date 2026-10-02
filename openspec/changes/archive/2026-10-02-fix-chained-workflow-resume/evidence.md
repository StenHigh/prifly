# Поставка и приёмка fix-chained-workflow-resume

Дата: 2026-10-02. Завершены инженерный фикс, поставка и живой пилот workflow.
Это не формальная приёмка P1/P2 или продуктовая QA-приёмка SMSPlace.

## Причина и постоянная защита

Прежний planner читал только собственные activations непосредственного source Run:
при повторном resume inherited prefix отсутствовал в них. Фикс проверяет sealed
ancestry и объединяет inherited/local evidence без фиктивных executions;
сохраняет root outputs/bindings и дерево текущего владельца claim.
`TestCLIResumeChain` до фикса воспроизвёл resume_prefix_empty; nested regression
терял две ранние записи (5 вместо 7). После фикса обе проверки проходят.
CLI/runtime regressions покрывают recover→recover→resume, inherited from-stage,
missing/cyclic/unsupported ancestry, changed contracts, read-only prepare,
dirty tracked/untracked work и concurrent transfer. Старые public schemas и
historical runner pins сохранены.

## Проверенная поставка

Engineering commit: `651c5c2145060bf9f45dfaacf068d8f10bfd0b6e`.
Local check-fast, ci-check и e2e PASS; strict OpenSpec 29/29, diff-check PASS.
[Verify](https://github.com/StenHigh/prifly/actions/runs/37001474494) и
[qualify](https://github.com/StenHigh/prifly/actions/runs/37001474709) PASS
на exact commit (race/e2e включены). `scripts/tag-release.py v0.13.75`
проверил gates; native linux/amd64 и darwin/arm64 builds и защищённая
[публикация](https://github.com/StenHigh/prifly/actions/runs/37002574275) PASS.
[0.13.75](https://github.com/StenHigh/prifly/releases/tag/v0.13.75) опубликован
2026-10-02T11:45:43Z и установлен штатным signed `prifly update`.
Installed-release prepare B PASS: verify/execute, 13 Reused, 4 root outputs,
claim generation2; source projections и dirty bytes неизменны.
Прежний development Run a18fa5be… с rejected outcome не переписан: полный
план включал поставку/пилот, выполненные позднее по отдельному поручению владельца.

## Живая цепочка

Authority: `/Users/sh/PhpstormProjects/SMSPlace/.prifly-sandbox-authority`.
Repository: `/Users/sh/PhpstormProjects/SMSPlace/backend`.
Claim: `df98f72e4297c34fa926562ddc831d998e8bb4de58c2234b37e18747e961a62e`.
Каждый start использовал fresh prepare, верхний review digest, текущего owner
и действующее разрешение владельца; ни raw fork, ни reset/release claim,
ни ручная правка authority для обхода guard не выполнялись.

| Run | ID | Итог | Generation |
|---|---|---|---|
| B | `80e270db507eab9ba8faad23205852d2be95ade93e16f41be94819c6e6c20d1e` | partial v36 | 2 |
| C | `daf00caa9fd061a4c6f53067e996aad9a4dbe288ded8d0351f20fb531220ae03` | partial v107, лимит review | 3 |
| D | `42c666adcc4267dbbb1f135d04147d1f2b4edce13d7b05961459b11a6cab2907` | partial v71, лимит review | 4 |
| E | `42a5ab20c9bb55e3c8daf3a75a30084108fef06d77d6b41c1d7fd8072763ee63` | succeeded v49 | 5 |

B→C сохранил HEAD793df111… и 11 dirty файлов, binary diff SHA-256
`325741a53a69a596a890fabdda4bfa8960a1509ad0f8d8ca610c3a603719f5fa`.
Позднейшие согласованные QA fixes сохранялись в том же дереве; C→D→E
не повторяли warmup/plan/improve/implement/accepted verify, prepare показывал
25 Reused. Перед D→E binary diff SHA-256
`5b08f9e4cd3caa01a8585c0998d1a1159f08944c501762e6854baa2186a44b8e`
совпал до/после; HEAD793df111… и 15 dirty файлов сохранены. Commit stage
сохранил tree принятого tests proof, затем штатный publisher доставил ветку.

## Точный финальный checkpoint агента SMSPlace

## Финальный итог пилота: E succeeded, доставка в draft MR (2026-10-02)

Владелец ответил «да» на все три действия предыдущего checkpoint. Финальные 15 dirty files закоммичены; первый локальный commit получил только обязательные metadata trailers, остальные commits переиграны без изменения source tree. Итоговая цепочка от base `d8f872e8cd0020d8b449cb3ef7b658b76406210a`:

- `e3e12c75dea192b1f3d85b3f7aef301a763f683f` — `feat(qa): добавить засев двух рынков для fallback anchor`; `Задача: #163`, `Changelog: added` и QA-инструкция для Ценовой лаборатории.
- `efafd7aa65d552b2e110e5ba44033a9927003289` — `test(qa): уточнить bounded pricing browser replay`.
- `9fe6e2a1720ba4fd9cbc4f1fd504e3ffec7837a9` — `fix(qa): защитить ownership и границы pricing-сценариев`.

До metadata-only переписывания сохранены локальные refs `backup/smsplace-163-before-metadata-E-20261002` и `backup/smsplace-163-final-before-metadata-E-20261002`; они не отправлялись. Полный dirty patch `dirty-after-D3-fix.patch` вне claim сохранён. HEAD tree после commit/replay в точности равен принятому tests measured_tree `917393f049bea1c00d70ebfea80c6b6a641a750d`; код не менялся после проверок. Claim и первичный checkout чистые; первичный development остаётся на maintenance commit `c4f92544254794cf6e420f144a950676b62e1305`.

Commit Attempt `ecac86ce0e6ee5dfb55a2d810fbc9bf3` принят: artifact `a4ad89bb2118fc1bdf19ed94801c35da91a38e8b91e480a52a60567f66367552`, digest `34ebb2c139482606d97085a8c1ab1bddb251541cfd2f782a1c2976d549fb69fd1a`. Implementation output пересчитан по фактическому новому HEAD: 25 paths, 19 PHP. На commit stage push не выполнялся.

Штатная sealed merge-request program Attempt `766e5fdaa007f043a020f6b4befe0672` отправила ветку `prifly/df98f72e4297c34fa926562ddc831d998e8bb4de58c2234b37e18747e961a62e` обычным push, без force, и создала [draft MR !1246](https://gitlab.com/smsplace/backend/-/merge_requests/1246) в `development`. Publication artifact `a2a93da02ed9bc243f456b21b31d0451cb7e3e692db77d969c3c7cb86f53e97d`, digest `588bd60c3db9ac6fdbea81150e006636f7cefc99e85a535e09fc92eac4718642`. Remote HEAD подтверждён как `9fe6e2a1720ba4fd9cbc4f1fd504e3ffec7837a9`; описание MR содержит product-tests proof с тем же tree. Maintenance advance primary не подмешивался в task commits.

Run E `run:42a5ab20c9bb55e3c8daf3a75a30084108fef06d77d6b41c1d7fd8072763ee63` завершён: v49, terminal done, outcome **succeeded**, driver_live false, continuations пусты. После terminal, по отдельному разрешению владельца и bridge-инструкции, host исправил только MR title через `glab mr update`. Свежий server view подтвердил `Draft: #163 · QA-засев двух рынков для fallback anchor`, opened/draft, target development и exact HEAD. Исторический output publisher с исходным title не переписывался.

Итог проверок: свежий E review — Pint/Rector/PHPStan PASS, pins1064/28723 и admin242/14846 PASS, новых failures0. Штатный tests — product7150/60078 PASS и extra121/2438 PASS в приватном контейнере app74f1a8a62360 с проверенным mount claim; environment освобождён. Остались 10 PHPUnit notices, не failures. `jev_shadow` skipped_no_api_key, не выполненная network-проверка. Browser evidence унаследован от принятого verify: нового browser replay после backend fixes не заявляем. Профиль fast/autonomous, plan_tests=true, improve=all, gate_warnings=ignore сохранены; security skip — объявленный выбор workflow, не обход host. CI pipeline в server view ещё отсутствовал; CI PASS не заявляем.

Задача Pri-Fly 4.4 закрыта фактическим evidence chained resume B→C→D→E, сохранения dirty tree и доставки после полных оставшихся Attempts. Это завершение пилота workflow, не merge, релиз либо окончательная продуктовая QA-приёмка SMSPlace #163. Issue не закрывалась, MR остаётся draft, merge/release не выполнялись. Archive/sync Pri-Fly change и commit этих двух документальных updates не выполнялись. Предыдущие partial checkpoints сохранены как история, не подменены итогом E.

## Проверка при архивировании

Публичный `run status` E независимо подтвердил completed/succeeded v49,
driver_live=false. Более позднее read-only чтение GitLab подтвердило MR !1246
уже **merged**, draft=false, target development, exact head
`9fe6e2a1720ba4fd9cbc4f1fd504e3ffec7837a9`; это факт после исторического
checkpoint draft выше. Product QA, deploy и CI success этой проверкой не
устанавливались. Merge здесь не выполнялся.

Временный корневой incident report удалён после переноса этого evidence.
Generic инструкции находятся в generated runner и справочниках Pri-Fly;
SMSPlace-specific continuation/private-environment правила добавлены в оба
`prifly-run/PROJECT.md` в primary SMSPlace checkout, без разовых Run IDs и
разрешения менять baseline тесты. Эти два изменения ещё не закоммичены.

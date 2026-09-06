# Выпуск Pri-Fly 0.10.0

Версия явно утверждена владельцем 2026-09-06 после предложения перейти
с public stable 0.9.1 на 0.10.0. Исходная ветка —
`codex/workflow-neutral-plan`, базовый commit подготовки — `99b7a0c`.
Нормативный порядок выпуска остаётся в `release-distribution/spec.md`;
эта запись фиксирует конкретный выпуск, не новый release contract.

## Scope

В выпуск входят уже реализованные нейтральный Project profile `/3`,
детерминированные сборки вариантов workflow, запуск программ без Git и ИИ,
общая анкета и просмотр запуска, смешанные шаги и явный timed contract
`prifly-step/2`. Ожидание объявленного вопроса не расходует остаток рабочего
времени; ответ не заменяет повторный допуск. Рабочая папка закреплена за Run.
Прежние profiles, packages и сохранённые Runs не переписываются.

Поддержанные release assets: `linux/amd64`, `darwin/arm64`. Обе сборки
создаёт существующий GitHub workflow; macOS собирается нативно. Signing key
остаётся в protected environment `release`, настройки защиты не меняются.

Что в этот срез не входит: новая квалификация UI Codex/Claude, внешнего AIF
package/pilot, формальные P1/P2 gates, auto-wakeup, долгий managed process,
обновление установленной программы или пользовательского полигона.
Существующие 17/24 выполненных задач parent change не превращаются в 24/24
из-за отдельного выпуска; внешняя часть task 4.4 остаётся незавершённой.

Известное ограничение: при отказе допуска после подготовки input tree
созданные подготовкой файлы могут остаться в рабочей папке. Старый дефект
cleanup с закрытым `os.Root` записан в roadmap как
`workspace-tree-preparation-rollback`; в этом выпуске его исправление не
заявляется. Обхода новой claim exclusivity либо потери пользовательских
файлов проведённый осмотр не показал.

## Подготовка

Последний branch CI на `f1c9ebd` остановился на устаревшем отрицательном
fixture `core-state/26` в `TestRepeatModelVersionedWire`. В текущем candidate
также устарело ожидание current `/26` в
`TestContextFieldsNeverExtendOlderStateContracts`. Исправление касается
только проверок уже реализованных editions; runtime и frozen schemas ради
зелёного результата не меняются.

Полные `make check` и `make e2e` выполняются локально один раз на candidate.
Tag workflow затем только собирает и публикует подписанные assets: второй
product test batch в CI не запускается. При обнаружении отказа сохраняется
его причина; результаты не подменяются проверками предыдущего commit.

## Checklist

- [x] Исправление устаревших ожиданий проверено адресно.
- [x] Полные проверки candidate завершены; точные итоги записаны ниже.
- [ ] Candidate commit опубликован, tag `v0.10.0` указывает на него.
- [ ] Native сборки и protected публикация завершены.
- [ ] Оба скачанных assets, manifest и подписи проверены; latest — 0.10.0.

## Verification

Адресные Go tests: 4 top-level и 59 subtests PASS, 0 failed, 0.691 s.
Проверки читают существующий `versionContracts`, сохраняют отказ неизвестному
и flat state и проверяют, что capability manifest не теряет прежние editions.
Production source и старые schemas для исправления этих тестов не менялись.

`make check GOCACHE=/private/tmp/prifly-neutral-go-build`: PASS, exit 0,
986.72 s wall time. Выполнены все восемь Go packages: пять с tests прошли
обычный и race прогон, три не содержат tests. Обычный runtime — 227.072 s,
race runtime — 699.537 s; обычный CLI — 36.628 s, race CLI — 177.027 s.
Оба `go vet` (обычный и `CGO_ENABLED=0`), formatting, refusal guard,
46 schema profiles и release CI contract — PASS. Старые bundles не
перегенерировались. Toolchain: Go 1.27.0, native darwin/arm64.

`openspec validate --all --strict --no-interactive`: 20 passed, 0 failed.
`git diff --check` и проверка сохранности historical archive — PASS.

Первый `make e2e` (29.98 s) успешно выполнил build, install verification и
6 Python example tests (0.387 s), затем остановился на старом catalog fixture:
neutral init больше не подключает codex-cli автоматически. Fixture теперь
проверяет отсутствие AI folders после init и явно подключает нужный host
через public `project runners add`. Повторён только authoring gate: PASS,
7 authoring cases, 1 workspace launch case и workflow catalog. Продолжение
с оставшихся Makefile-команд дало:

- `verify-cli.py`: 7 cases PASS, export bytes и telemetry проверены, 4.39 s.
- `verify-core.py`: 169 commands PASS, 16.44 s.
- `verify-context.py`: 75 commands, 10 cases PASS, 9.97 s.

Таким образом исполнены все e2e targets, но не утверждается, что первоначальный
`make e2e` вернул 0: его отказ и исправление описаны выше. E2E binary SHA-256:
`68b0a14fccd9778162152d4c67b039a68768b47913497d8991f670892f486c51`.
Логи текущей подготовки находятся в
`/private/tmp/prifly-release-0.10.0.xb3Jmo/`; эта временная копия не заменяет
проверки в репозитории и настоящую запись результатов.

Tag будет создан на candidate commit без skip-инструкций и опубликован
отдельно от branch push, чтобы штатный release workflow не оказался пропущен.
Итоговая docs-only запись будет отдельным branch commit с `[skip ci]`.
Это использует существующие trigger filters, не меняет protections и не
переопределяет required gates. Семантика skip-инструкций сверена с
[документацией GitHub](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/skip-workflow-runs).

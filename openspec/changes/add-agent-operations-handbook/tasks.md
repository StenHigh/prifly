## 1. Зафиксировать проверяемую поверхность

- [x] 1.1 Сверить `coverage.md` с текущими CLI dispatch/help, parsed flags, schemas и `prifly capabilities`; записать каждый пробел и версию baseline.
- [x] 1.2 Зафиксировать protected paths и digest прежних runner templates; проверить неизменность archive, release evidence и прежних schema bundles относительно baseline.
- [x] 1.3 Подготовить fixture-проект без Core checkout для H01–H16; подтвердить доступность установленного CLI, host inputs и наблюдаемых результатов.
- [x] 1.4 Записать baseline чтения со старым skill для одинакового набора задач; проверить наличие bytes/calls и явно указать доступность фактических token metrics.

## 2. Написать эксплуатационный справочник

- [x] 2.1 Создать `examples/operations/index.md`: задача, симптом, команда и capability ведут на нужную страницу; проверить все переходы.
- [x] 2.2 Написать `installation.md` со всеми вариантами установки, workflows и runners; сверить flags/defaults и выполнить поддержанные операции в fixture.
- [x] 2.3 Написать `launch.md` с typed inputs, standing answers, профилями и review; проверить H01/H02 и отказ stale digest.
- [x] 2.4 Написать `hosting.md` с task/take/template/submit, slots, effects, model и questions; выполнить H03 и проверить принятый StepResult.
- [x] 2.5 Написать `decisions.md` с preflight/runtime/yield и native questions; проверить H04 и отсутствие принятого ответа при timeout/dismissal.
- [x] 2.6 Написать `recovery.md` с continue/resume/reopen/recover/resolve; проверить H05–H07 по фактическому prefix, claim и outcome.
- [x] 2.7 Написать `workspace.md` с worktree/checkout/read-only/effects/claims; проверить сохранение дерева и отказ недопустимой записи.
- [x] 2.8 Написать `authoring.md` как маршрут ко всем каноническим references; проверить H10 и минимальные ревизии полей.
- [x] 2.9 Дополнить пробелы в существующих `examples/authoring/`; сохранить независимый `examples/README.md` capability index и выполнить `TestEveryDeclaredCapabilityIsInTheAuthorIndex`.
- [x] 2.10 Написать `artifacts.md` с refs/schema/source/task/packages/trust/publications; проверить canonical/raw bytes и реальные ответы команд в fixture.
- [x] 2.11 Написать `control.md` с stops/receipts/approvals/grants/actions; проверить lost-response путь и различие admission/delivery/effect.
- [x] 2.12 Написать `observability.md` с monitor/Light/events/timing/output/progress/telemetry; проверить H09 и состояния unknown/stale/unavailable.
- [x] 2.13 Написать `operations.md` с queue/capacity/storage/cleanup; проверить H08/H14, reviewed mutation и отсутствие повторного dispatch.
- [x] 2.14 Связать новый `troubleshooting.md` с существующим указателем симптомов; проверить Problem codes и безопасные next actions для каждой описанной ветви.

## 3. Доставлять справочник в каждый host

- [x] 3.1 Включить canonical Markdown/reference assets в CLI через стандартный embed; проверить build и наличие только явно выбранных текстовых assets.
- [x] 3.2 Добавить согласованную установку skill/manual/metadata в host roots; проверить ссылки в изолированной установленной структуре без Core checkout.
- [x] 3.3 Дополнить read-only `runners update --check` проверкой manual; доказать отсутствие записи при missing page, outdated page и конфликте.
- [x] 3.4 Реализовать обновление известных assets с сохранением `PROJECT.md` и пользовательских файлов; проверить H11 для всех трёх hosts.
- [x] 3.5 Проверить failure/concurrency/conflict сценарии доставки; подтвердить отсутствие ложного успеха при смешанных assets и сохранение пользовательских bytes.
- [x] 3.6 Проверить работу `runners add` и `project init` с новым manual; existing missing hosts не добавляются через `update` без отдельного выбора.

## 4. Обновить канонический runner

- [x] 4.1 Заморозить прежний текущий runner до изменения recognition chain; выполнить `TestProjectRunnerUpdateReplacesEveryReleasedRunner` и `TestProjectFrozenRunnerTextIsPinned`.
- [x] 4.2 Сократить основной skill и добавить чтение индекса/нужной страницы; проверить сохранение обязательных host invariants и H03–H07.
- [x] 4.3 Добавить обязательный update binary, workflows, runner до новой подготовки; проверить порядок H01 и отсутствие рекурсивного preflight.
- [x] 4.4 Добавить остановку на failed update/source build/custom runner/workflow drift; проверить H12 и отсутствие нового Run/claim/import.
- [x] 4.5 Описать и проверить fixed refs, catalog release change и local package; выполнить H13 без молчаливой смены ref либо выдуманного upstream.
- [x] 4.6 Проверить active Attempt и existing Run без сети; H16 не вызывает preflight, не меняет package и следует pinned instructions.
- [x] 4.7 Обновить tracked Codex/Claude copies и current text pins; проверить совпадение с canonical template и сохранность PROJECT overlays.

## 5. Закрепить полноту и актуальность

- [x] 5.1 Добавить независимое сопоставление public command/flag/capability inventory с operations index; сначала показать отказ на пропущенном элементе, затем зелёную проверку.
- [x] 5.2 Проверить локальные ссылки и доступность всех assets в canonical и installed layouts; намеренно missing target должен давать отказ проверки.
- [x] 5.3 Обновить ссылки `README.md`, `examples/README.md` и карту документации `AGENTS.md`; проверить вход в нужную процедуру без поиска кода.
- [x] 5.4 Уточнить `openspec/SOURCE-OF-TRUTH.md` для active deltas без переноса ownership; проверить соответствие proposal и обеим delta specs.

## 6. Проверить результат и границы

- [x] 6.1 Выполнить H01–H16 в fixture без Core sources; записать команды, результаты, gaps и фактический доступ к файлам.
- [x] 6.2 Сравнить baseline и новый skill на тех же задачах и настройках; принять экономию только вместе с корректным результатом, без выдуманных token metrics.
- [x] 6.3 Выполнить `make test-changed` и `git diff --check` в target worktree; исправить обнаруженные регрессии, новые безопасные Go tests пометить `t.Parallel()`.
- [x] 6.4 Выполнить `openspec validate add-agent-operations-handbook --strict`; проверить требования, сценарии и соответствие tasks/design/coverage.
- [x] 6.5 Перед коммитом выполнить `make ci-check-once CHECK_CONTEXT=<actual-profile> CHECK_FOR_COMMIT=1`; зафиксировать реальный контекст, staged bytes и результат.
- [x] 6.6 Сверить protected historical paths с baseline; подтвердить неизменность frozen templates, public schema bundles и release evidence.
- [x] 6.7 Подготовить отчёт с coverage gaps, размером поставки, практическими результатами и измерениями; не объявлять release/roadmap gates закрытыми по документным проверкам.

## 1. Queue authority и contracts

- [ ] 1.1 Ввести QueueTask/Claim/LaunchIntent/Binding и closed queue schemas; обновить словарь и source map. Проверить TestGlossaryBindings, shape/refusal tests и отсутствие изменения старых schemas/архивов.
- [ ] 1.2 Реализовать add/read/update с exact request revision history и deterministic priority ordering; проверить roundtrip bytes, stale revision, project isolation, no-queue legacy и отсутствие mutation у read.
- [ ] 1.3 Реализовать атомарный claim-next и owner/generation CAS через durable commands; проверить concurrent claim-next, exact replay, same-ID payload conflict и поздний replay после release.
- [ ] 1.4 Расширить local coupled transaction для pure дополнительных authority mutations и Run pins при authority mutation; проверить all-or-nothing rollback, per-task CAS при чужом queue update, receipt-only replay и internal/purity guard.

## 2. Intent и Run lifecycle

- [ ] 2.1 Реализовать durable intent/reserved/dispatching и replay lookup до executable effects; проверить crash до/после claim и intent, неизвестный dispatch без повторного исполнения, сохранение exact reviewed selection.
- [ ] 2.2 Связать admission/association и immutable QueueTaskBinding атомарно с Run и Workspace/control changes; проверить crash/rollback вокруг admission, exact existing Run recovery и отсутствие orphan/double start.
- [ ] 2.3 Добавить opt-in core-state/44, core-read/44 и новые bundles/capability declarations; проверить реально запечатанный queue Run/Read против новых schemas, old-version rejection и unchanged legacy writer/read.
- [ ] 2.4 Реализовать явный reconcile с dispatcher evidence, owner resolution и запретом позднего admission; проверить reserved/unknown/admitted случаи, live dispatcher refusal, отсутствие повторного preflight и Run.
- [ ] 2.5 Реализовать безопасные release/complete и queue guard reopen; проверить live/uncertain/decision refusals, Run-version race, stale owner/generation, explicit local done и неизменность истории.
- [ ] 2.6 Сохранить Task identity для continue/recover/fork; проверить partial → continue → cancel → continue, повторный continue, отдельные outcomes/bindings и отказ потери/подмены Task identity.

## 3. CLI и generated runner

- [ ] 3.1 Добавить project queue команды и versioned JSON outputs/refusal codes; проверить CLI integration для всех операций, conflict-no-mutation и readonly команды без инициализации queue.
- [ ] 3.2 Провести queue selection через questionnaire/prepare/reviewed start и /4 summary/result; проверить stale revision refusal до preflight/workspace/dispatch, exact digest, typed input + original request и неизменные inputs прежнего Run после update.
- [ ] 3.3 Обновить generated runner/templates/recognition для queue intake и фактического ожидания выбора/ответов; проверить host-specific templates и isolated CLI сценарий без start при claim или missing answer, без перезаписи owner runners.
- [ ] 3.4 Показать capability/selection fields и минимальную contract revision в examples/README.md, нужном examples/authoring/ и README; проверить author index test и воспроизводимость примеров.

## 4. Инженерная приёмка и подготовка квалификации

- [ ] 4.1 Добавить reusable CLI concurrency/crash/recovery replay, пригодный для exact released binary; выполнить на development candidate с отдельной маркировкой E2-A1–A9 и проверить no private payload/test backdoor.
- [ ] 4.2 Подготовить воспроизводимый live generated-runner сценарий для codex-app/codex-cli/claude-code и evidence matrix E2-A1–A10; проверить, что только фактический released/live PASS закрывает A10, а неподтверждённые строки остаются pending.
- [ ] 4.3 Выполнить targeted tests/make check-fast, openspec validate add-project-task-queue --strict --no-interactive, protected-history diff audit и полные staged ворота make ci-check-once CHECK_CONTEXT=codex-app-authorized CHECK_FOR_COMMIT=1; зафиксировать инженерную готовность к canonical verify/security/review/commit.

Галочки выше относятся к инженерной реализации и подготовке воспроизводимой квалификации. Canonical Run заканчивается commit, после него claim-ветка вливается в main. Они **не закрывают E2-A10**: до sync/archive и закрытия #5 обязательны actual release verify/qualify на фиксированном SHA, fixtures на реально released binary и live generated runner на всех трёх поддерживаемых hosts по design.md/Migration Plan. Evidence обязан различать engineering PASS и эту последующую поставку; push и release tags остаются действиями владельца. AIF plan удаляется только в том же коммите, что archive после завершения всех критериев.

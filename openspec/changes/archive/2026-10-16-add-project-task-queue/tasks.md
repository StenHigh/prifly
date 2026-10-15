## 1. Queue authority и contracts

- [x] 1.1 Ввести QueueTask/Claim/LaunchIntent/Binding и closed queue schemas; обновить словарь и source map. Проверить TestGlossaryBindings, shape/refusal tests и отсутствие изменения старых schemas/архивов.
- [x] 1.2 Реализовать add/read/update с exact request revision history и deterministic priority ordering; проверить roundtrip bytes, stale revision, project isolation, no-queue legacy и отсутствие mutation у read.
- [x] 1.3 Реализовать атомарный claim-next и owner/generation CAS через durable commands; проверить concurrent claim-next, exact replay, same-ID payload conflict и поздний replay после release.
- [x] 1.4 Расширить local coupled transaction для pure read-only authority guards, Run pins и fence original Run command identity при authority mutation; проверить rollback Run/primary control при отказе guard, per-task CAS при чужом queue update, receipt-only replay и internal/purity guard без изменения storage v9.

## 2. Intent и Run lifecycle

- [x] 2.1 Реализовать durable intent/reserved/dispatching и replay lookup до executable effects; проверить crash до/после claim и intent, неизвестный dispatch без повторного исполнения, сохранение exact reviewed selection.
- [x] 2.2 Связать immutable QueueTaskBinding атомарно с Run и Workspace/control changes при current queue guard; добавить recoverable финализацию queue index по intent. Проверить crash/rollback до/после admission/index, exact existing Run recovery и отсутствие orphan/double start.
- [x] 2.3 Добавить opt-in core-state/44, core-read/44 и новые bundles/capability declarations; проверить реально запечатанный queue Run/Read против новых schemas, old-version rejection и unchanged legacy writer/read.
- [x] 2.4 Реализовать явный reconcile с dispatcher evidence, owner resolution и запретом позднего admission; проверить reserved/unknown/admitted случаи, live dispatcher refusal, отсутствие повторного preflight и Run.
- [x] 2.5 Реализовать безопасные release/complete и queue guard reopen; проверить live/uncertain/decision refusals, Run-version race, stale owner/generation, explicit local done и неизменность истории.
- [x] 2.6 Сохранить Task identity для continue/recover/fork; проверить partial → continue → cancel → continue, повторный continue, отдельные outcomes/bindings и отказ потери/подмены Task identity.

## 3. CLI и generated runner

- [x] 3.1 Добавить project queue команды и versioned JSON outputs/refusal codes; проверить CLI integration для всех операций, conflict-no-mutation и readonly команды без инициализации queue.
- [x] 3.2 Провести queue selection через questionnaire/prepare/reviewed start и /4 summary/result; проверить stale revision refusal до preflight/workspace/dispatch, exact digest, typed input + original request и неизменные inputs прежнего Run после update.
- [x] 3.3 Обновить generated runner/templates/recognition для queue intake и фактического ожидания выбора/ответов; проверить host-specific templates и isolated CLI сценарий без start при claim или missing answer, без перезаписи owner runners.
- [x] 3.4 Показать capability/selection fields и минимальную contract revision в examples/README.md, нужном examples/authoring/ и README; проверить author index test и воспроизводимость примеров.

## 4. Инженерная приёмка и подготовка квалификации

- [x] 4.1 Добавить reusable CLI concurrency/crash/recovery replay, пригодный для exact released binary; выполнить на development candidate с отдельной маркировкой E2-A1–A9 и проверить no private payload/test backdoor.
- [x] 4.2 Подготовить воспроизводимый live generated-runner сценарий для codex-app/codex-cli/claude-code и evidence matrix E2-A1–A10; проверить, что только фактический released/live PASS закрывает A10, а неподтверждённые строки остаются pending.
- [x] 4.3 Выполнить targeted tests/make check-fast, openspec validate add-project-task-queue --strict --no-interactive, protected-history diff audit и полные staged ворота make ci-check-once CHECK_CONTEXT=codex-app-authorized CHECK_FOR_COMMIT=1; зафиксировать инженерную готовность к canonical verify/security/review/commit.

Галочки выше относятся к инженерной реализации и подготовке воспроизводимой квалификации. Canonical Run заканчивается commit, после него claim-ветка вливается в main. Они сами по себе **не закрывают E2-A10**. Поставка ниже проверена отдельно; для v0.13.77 владелец явно исключил недоступный Claude из приёмки, оставив его qualification `pending`, и разрешил агенту push/выпуск. Критерии следующих выпусков не изменены. AIF plan удаляется только в том же коммите, что archive после завершения согласованных критериев.

## 5. Поставка v0.13.77 и согласованная приёмка

- [x] 5.1 Пройти GitHub verify/qualify на exact SHA `16c3dfdd1e8ed4c72ea986f9f27dee45ae57e326`, выпустить v0.13.77 и проверить происхождение/хеш фактического опубликованного binary.
- [x] 5.2 Выполнить released CLI concurrency/crash/recovery replay без подмены development binary.
- [x] 5.3 Провести живые generated runners codex-app и codex-cli с фактическими ответами владельца, нулём Runs при ожидании, exact review/start и одним Run/association после точного повтора.
- [x] 5.4 Записать явное исключение владельца для недоступного Claude только в приёмке v0.13.77; сохранить его qualification pending и приложить actual evidence перед sync/archive.

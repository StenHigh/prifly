# Условный совет и нативный выбор project launch

Ветка: `prifly/cf737abd795a1f016717cb3d991881b62ed88015f0599df97e8e74631fe5d750`
Дата: 2026-10-12
Change: `openspec/changes/add-project-launch-advice`

## Original Request

Implement openspec/changes/add-project-launch-advice: all open task ids; requirements in specs/ and design.md. Совет по title/description и PROJECT.md остаётся условным и отключаемым, без новых schema fields. Нативный выбор использует доступный применимый инструмент Codex/Claude и ждёт явный ответ. Реальные host observations записывать честно; непроверенные случаи не закрывать. Дополнительно по запросу владельца обновить Go до стабильной 1.27.2, go.mod и точные совместимые pins staticcheck/govulncheck в Makefile по design; выполнить пункты 5.1 и 5.2.

## Settings

- Testing: yes — проверяются bytes/upgrade и действующий CLI protocol; реальные host observations ведутся отдельно.
- Logging: minimal — новые runtime logs не нужны; диагностические результаты остаются в test output и observation evidence.
- Docs: no — дополнительного aif-docs checkpoint нет; обязательные README/examples правки входят в change.
- Profile: full; policy: attended; branch creation: false; gate_warnings: ignore.
- Sealed gate composition берётся из decision_context; Go для worktree: `/opt/homebrew/Cellar/go/1.27.2/libexec/bin/go`.
- Использовать только выданный claim worktree; standalone launcher, engine selector и новые schema fields не создаются.

## Roadmap Linkage

Milestone: "none"
Rationale: change не объявляет milestone; проектный overlay устанавливает none в этом случае. Локальная реализация не закрывает formal P1/P2 acceptance.

## Requirements Reconciliation

Authority: delta и design названного change → current OpenSpec specs → карты проекта; пользовательский AGENTS.md старше нижестоящих правил.

| Поддержанная комбинация | Источник | Проверка |
|---|---|---|
| Exact ID / неизвестный ID | cli-protocol delta, условный совет | Нет повторного выбора / неизвестный не заменён похожим |
| Несколько launch + задача + достаточные descriptions | cli-protocol delta, условный совет | Один обоснованный совет и все варианты, затем явный выбор |
| Совет пропущен/отключён, один launch, file-only вход | cli-protocol delta, условность и входы | Нет обязательного подбора/фиктивного brief; ручной выбор сохранён |
| Codex blocking/async, Claude native, нет применимого tool | cli-protocol delta, native выбор | Реальный tool call/ответ либо явный текстовый fallback |
| Асинхронный receipt без ответа | control-security-ux delta | Нет зависимого questionnaire/start и authority mutations |
| Source/typed input изменился после prepare | workflow-and-context и действующие guards | Existing typed/digest отказ остаётся до claim/Run |
| Старый generated/current/custom runner + overlay | cli-protocol delta, upgrade | Старые hashes, overlay bytes и атомарный custom refusal |
| Go 1.27.2 + анализаторы | design, Toolchain update | Исправленный stable Go, exact compatible pins, все прежние gates |

Совет уточняет запрет автоматического выбора в main specs; он не нарушает определения launch из glossary. Old active neutral delta пересекается с host entry points: последующий sync сохраняет новые уточнения, не переписывая исторический evidence.
AGENTS.md ограничивает full e2e/race релизом; sealed gate_checks этого Run уже исключает e2e из standing AIF composition.
Реальные UI observations не подтверждены заранее и не заменяются string assertions.

## Tasks

Прогресс отмечается только в [change/tasks.md](../../openspec/changes/add-project-launch-advice/tasks.md). Таблица — карта исполнения, без второго чеклиста или новых task ids. Каждая строка сохраняет проверку исходного пункта; команды ниже проверены на этом checkout.

| Пункт change | Работа и файлы | Зависимость / проверка |
|---|---|---|
| 1.1 | Сохранить current renderer как previous variant в `cmd/prifly/project.go`, добавить прежние current pins к frozen arrays | Не править общий старый template; runner/frozen/upgrade tests |
| 1.2 | В том же renderer заменить launch chooser условным советом | После 1.1; все hosts, exact/unknown/one/zero/skip/PROJECT cases |
| 1.3 | Native finite question guidance непосредственно перед chooser и questionnaire | После 1.1; host tools, async wait, pages, свободный ввод, approval limits |
| 1.4 | Сохранить task text/ссылки по выбранному declared port | После 1.2; generic JSON/text ports и existing file-only/integer fixtures |
| 2.1 | Parameterized upgrade/overlay test и сохранность custom bytes | После 1.1–1.4; existing release variants и current pin tests |
| 2.2 | Existing refusal/digest/authorization guards; не дублировать уже имеющееся покрытие | После 2.1; make test-changed по исходному tasks.md |
| 2.3 | README, examples index/profile comments; без новых runtime capability names | После 1.2–1.4; YAML/reference checks по tasks.md |
| 3.1 | Generic corpus и воспроизводимый fixture setup в test/fixtures/project-launch-advice | Разные ID/ports, overlay и pagination; setup/self-check |
| 3.2 | Реальные Codex app observations | После 3.1 и candidate runner; actual async choice и отсутствие ранних commands |
| 3.3 | Реальные Codex CLI observations и проверенный fallback | После 3.1; actual tool/mode/response evidence |
| 3.4 | Зафиксировать actual Claude launch/overlay observations и разрешённый владельцем DEFERRED follow-up | Остальные finite/limit/dismissal не блокируют текущий commit и не считаются PASS |
| 3.5 | Доступные Codex edge cases и заданная native navigation | Непроверенная реакция на Claude tool limit остаётся в follow-up 3.4 |
| 4.1 | Сверить delta/main/active neutral overlap, защиту истории | Strict OpenSpec validation; ownership неизменна |
| 4.2 | Итоговые объявленные gates и защищённые пути | После 5.1, один ci-check перед commit; конкретные результаты без qualification claims |
| 5.1 | go.mod 1.27.2, Makefile compatible exact analyzer pins | Предшествует 4.2; установленный Go и .tools/go уже 1.27.2 |
| 5.2 | Полные gates на patched toolchain | Вместе с 4.2; vuln/staticcheck остаются включены |

Логирование для всех пунктов: новый production logging отсутствует; tests выводят failure details, fixture setup — пути/metadata, observations — фактические результаты.

## Commit Plan

Проверяемые checkpoints: после 1.1–1.4 (renderer), после 2.1–2.3 (tests/docs), после 3.1–3.5 (host evidence), после 4.x/5.x (gates/toolchain). Git commit делает объявленный commit stage этого Run после его gates; отдельные commits/этапы плана не добавляются.
Предлагаемый итоговый subject: `feat(project): advise launch choice without selecting it`; Go/pins описываются в body. Непроверенные UI случаи остаются явно незакрытыми.

## Проверенные команды

На этом claim checkout до сохранения плана:
- `env GOTOOLCHAIN=local GOTELEMETRY=off GOCACHE=/Users/sh/PhpstormProjects/Pri-Fly/github/.cache/go-build GOMODCACHE=/Users/sh/PhpstormProjects/Pri-Fly/github/.cache/go-mod /opt/homebrew/Cellar/go/1.27.2/libexec/bin/go test ./cmd/prifly -run '^TestProject(RunnerUpdateReplacesEveryReleasedRunner|RunnerTextIsPinned|FrozenRunnerTextIsPinned|CurrentRunnerIsWorkflowNeutral)$'` — exit 0.
- `/Applications/ServBay/package/node/current/bin/node /Applications/ServBay/package/node/current/bin/openspec validate add-project-launch-advice --strict --no-interactive` — exit 0.
- `git diff --check` — exit 0.

Дальнейшие проверки из tasks.md выполняются после изменения соответствующих файлов; этот раздел не объявляет их уже пройденными. Diff scope каждый раз выводится из текущего tree, а не фиксируется устаревающими числами.

## Owner scope update — 2026-10-12

Владелец разрешил вынести оставшуюся Claude UI-приёмку в явный DEFERRED follow-up и завершить текущие verify/review/commit. Tasks 3.4/3.5 и design уточнены в этом объёме; live Claude finite/limit/dismissal не выдаются за PASS. Original Request выше сохранён как исходный запрос; эта более поздняя директива определяет текущую приёмку.

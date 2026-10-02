# План реализации: цепочка возобновлений

Branch: prifly/4d58103d5bf8fe7c5c843dded89714d6b05a92d1344bd86bc4688d8bec878b96
Created: 2026-10-02

## Original Request
Implement openspec/changes/fix-chained-workflow-resume: all open task ids; requirements in specs/ and design.md. Preserve inherited prefix, bindings, current claim and dirty work through repeated resumption; do not modify SMSPlace during engine implementation. Release and pilot tasks require their stated owner authority and are not completed by source tests.

## Settings

- Testing: yes — regression до фикса и проверки guards.
- Logging: minimal — typed Fault, JSON plan; новых stdout/stderr логов нет.
- Docs: no — без aif-docs; справочники и report меняются только в scope change.

## Roadmap Linkage

Milestone: "none"
Rationale: Отдельный bug-fix change не закрывает formal product milestone; roadmap linkage skip запечатан в Run.

## Requirements Reconciliation

Authority: delta specs/design change > current capability specs > AI context; AGENTS.md обязателен.

| Правило / сочетание | Источник | Проверка |
|---|---|---|
| inherited + local prefix; same/later frontier; цепочка до D | specs/domain-execution/spec.md change | CLI и runtime ResumeChain fixtures |
| root inherited from-stage; nested имя не root | тот же delta | explicit from-stage negative/positive cases |
| ancestry origin, локальные sequences, effective contracts | design.md Decisions 1–2 | missing/cyclic ancestor и changed-contract cases |
| current claim, dirty tracked/untracked, stale/dedup | domain-execution delta; existing start.go | реальные transfer cases до write Attempt |
| публичный origin, старые editions, индекс не admission | cli-protocol delta | read/runner compatibility cases |
| release/pilot отдельно от инженерного фикса | delivery-roadmap delta; AGENTS/RULES | незакрытые owner-dependent tasks до фактического evidence |

## Tasks

Источник прогресса: `openspec/changes/fix-chained-workflow-resume/tasks.md`; здесь нет второго списка галочек.

- 1.1 Добавить CLI regression реальных программных Runs A(partial) → resume B(partial) → prepare C, используя существующий blocked-example fixture; сначала получить `resume_prefix_empty` при непустом `Recovery.Reused`, сохранить точный red result. Проверка: `.tools/go/bin/go test ./cmd/prifly -run 'Test.*ResumeChain' -count=1 -v` (Unix socket должен быть разрешён; `local_socket_unavailable` не является воспроизведением дефекта).
- 1.2 Добавить fixture nested verify → fix → verify и tree transfer с tracked dirty diff и untracked bytes; зафиксировать hashes до prepare/transfer, отсутствие ранних повторных Attempts и версии/события source. Проверка: targeted `Test.*ResumeChain` сначала падает на втором prepare и не меняет дерево от просмотра.
- 2.1 Разрешать inherited provenance по sealed ancestor evidence, проверять ancestry/root/nested identities и effective compatibility, соединять inherited и local accepted prefix без сравнения EventSequence разных Runs. Проверка: runtime tests цепочки, nested call/repeat, missing/cyclic/unsupported ancestor и изменённый inherited contract; `.tools/go/bin/go test ./internal/runtime -run 'Test.*(ResumeChain|Recover)' -count=1`.
- 2.2 Сохранить root outputs и bindings через A → B → C → D, в том числе при остановке B позднее прежнего frontier; не создавать фиктивные executions. Проверка: CLI fixture принимает inputs ранних outputs после третьего переноса, счётчик реальных исполнений и provenance соответствуют исходным Runs.
- 2.3 Поддержать `--from-stage` из inherited root prefix с отсечением выбранной стадии/её subtree/хвоста; сохранить отказы для nested fix, unknown stage и действительно пустого entry-prefix. Проверка: targeted runtime/CLI cases с повторяющимися nested именами и изменённой ранней root-стадией.
- 2.4 Проверить использование общего planner через `project recover` от ранее восстановленного technically failed source; сохранить eligibility, candidate validation и source history. Проверка: цепочка recover → recover и смешанная recover → resume fixture в `.tools/go/bin/go test ./internal/runtime ./cmd/prifly -run 'Test.*(ResumeChain|Recover)' -count=1`.
- 3.1 Сохранить read-only prepare, transfer только claim непосредственного source, atomic create/transfer, command dedup и stale guards. Проверка: repeat prepare имеет тот же digest; active child/unresolved effect, released/missing claim, changed generation/source version и concurrent start дают точные отказы; tracked/untracked hashes не меняются от prepare и transfer до новой write Attempt.
- 3.2 Проверить публичную навигацию origin и поддержку прежних `recovery/1`/`recovery/2`; использовать имеющиеся поля, при необходимом расширении выпустить новую edition без изменения frozen bundles. Проверка: targeted compatibility tests, `make schemas-check` при изменении схем, `TestGlossaryBindings` при изменении карты терминов; отказ при неполном historical evidence явно называет причину.
- 3.3 Исправить CLI refusal explanations и runner guidance: индекс `run next` не admission, `resume_prefix_empty` не совет потерять dirty work, concrete prepare требует правильный repository. Показать repeated resume и inherited `--from-stage` в существующем справочнике authoring/troubleshooting с минимальной исправленной release revision; сверить строку существующей capability в `examples/README.md`. Проверка: runner/CLI tests и `TestEveryDeclaredCapabilityIsInTheAuthorIndex`, review соответствия всех инструкций.
- 3.4 Согласовать delta с действующим source set и соседним recovery change, добавить актуальную запись в единую очередь delivery-roadmap при sync, не дублировать tasks и не закрывать P1/P2. Проверка: `openspec validate fix-chained-workflow-resume --strict`, review ownership и связей roadmap.
- 4.1 Выполнить затронутые проверки (`make test-changed`/`make check-fast`), `git diff --check`, затем `make ci-check` один раз перед commit. Подтвердить, что source Run/state/event bytes, `openspec/changes/archive/**`, прежние published bundles и release records не переписаны; показать точные результаты и diff затронутых путей.
- 4.2 Провести стандартную release qualification на коммите фикса: зелёные verify/qualify, `gh workflow run qualify.yml --ref main` и `python3 scripts/tag-release.py vX.Y.Z` по правилам поставки и полномочиям владельца. Записать фактический build/version и постоянное regression evidence; не объявлять поставку по одной локальной сборке.
- 4.3 После обновления installed binary выполнить read-only prepare SMSPlace B по разделу «Инструкция агенту SMSPlace после поставки» в отчёте; проверить frontier verify, inherited prefix, exact claim и source answers, сохранить digest и неизменность дерева/authority projections. При новом отказе сохранить evidence и остановить зависимые действия без обхода.
- 4.4 При действующем разрешении владельца выполнить reviewed start, записать C и передачу B → C, продолжить разрешённую правку и оставшиеся stages по Attempts. Сохранить фактический outcome, результаты gates и состояние доставки/MR в том же отчёте; при повторном partial проверить новый prepare от текущего владельца. Если пилот не завершён, задача остаётся открытой, а инженерный фикс и бизнес-outcome описываются отдельно.


Реализация начинается с existing blocked-example CLI fixture, затем общий recovery.go planner; start.go/CLI меняются только по установленной необходимости. Проверки source evidence идут до write transform. Сначала проверить сохранённую lineage и её root outputs; не переносить graph-stage identity по одному имени.

## Commit Plan

- Один инженерный commit после инженерной части 1.1–3.4 и 4.1 (в 3.3 минимальная release revision зависит от 4.2): `fix: preserve accepted evidence across resumed runs`; промежуточный красный test не коммитить. Зависимые задачи поставки/пилота остаются явными, без ложного completion.
- Release/pilot evidence — отдельный срез после действий владельца. Push/merge/tag не выполняются этим Run.

## Проверенные команды и границы

`openspec validate fix-chained-workflow-resume --strict` и `git diff --check` выполнены на planning tree: успешно. Команды новых regression tests в задачах — требуемая проверка после добавления tests, не заявление о текущем coverage. Новые результаты записывать в tasks по факту. Standing gate_checks этого Run содержит ci-check, e2e, OpenSpec validation и diff-check; не добавлять race.

## Checkpoint 2026-10-02

Инженерная реализация и все локальные standing gates завершены; source tests, ci-check, e2e и strict OpenSpec validation прошли. В tasks отмечены 10/14 задач; 3.3 ждёт фактической минимальной release revision, 4.2–4.4 ждут owner-dependent поставки и пилота. Не возвращать pass для полного плана: незавершённые зависимости сохраняются, checkpoint commit доступен владельцу.

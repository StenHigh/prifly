## Context

Мотивация — [proposal.md](proposal.md); нормативный контракт — delta `domain-execution`. Read-only prepare установленного 0.13.76 воспроизвёл `recover_frontier_unsupported` и без `--from-stage`, и с `--from-stage plan`. Evidence содержит одну failed leaf activation с diagnostic `workspace_tree_input_drift` в preparation без Attempt и четыре propagated `child_failed` у родителей. Source Run settled, активных Attempt нет; dirty/untracked план и действующий claim сохранены.

`brokenStage()` считает все failed activations независимыми; recovery дополнительно требует root step и recorded Attempt. Та же функция обслуживает прежний `reopen`, поэтому расширение её ответа сразу изменило бы объявляемый маршрут reopen. Prevalidation и history также требуют сначала доказать естественную точку остановки.

## Goals / Non-Goals

**Goals:** восстановление доказуемого технического сбоя по существующим `project recover` / `--from-stage`; один root point, законный prefix, новый допуск и полный takeover текущего дерева.

**Non-Goals:** ослабление drift/effect checks, автоматическое принятие post-submit edits, угадывание результата невыданной Attempt, поддержка неоднозначных независимых отказов, изменение reopen, новый recovery wire contract, SMSPlace-specific ветка, release или запуск внешнего recovery.

## Decisions

### Recovery получает собственный selector

Выбор natural frontier выполняется в recovery policy отдельно от `brokenStage()` для reopen. Проверка использует сохранённые activations, invocations, CallerActivationID/ParentInvocationID и, где применимо, recorded call/repeat outcome/diagnostics; не совпадение stage names или текст ошибки. Все relevant failed activations должны объясняться одной исходной failure chain. Случай нескольких независимых causes, отсутствующего caller, цикла, незавершённого родителя либо несогласованной recorded связи остаётся отказом.

Корневый `step` с выданной Attempt сохраняет прежние settlement/uncertainty проверки. Settled preparation failure до admission допускается только при доказанном отсутствии Attempt и recorded отказе подготовки. Корневые `call/repeat` запускаются целиком и не получают leaf Attempt как собственного результата. `source_frontier_attempt_id` остаётся отсутствующим, `frontier_action` — execute, без candidate/revalidate.

Альтернатива — просто считать root failures или игнорировать `child_failed` по строке — скрывает независимые причины и не доказывает связь. Расширение `brokenStage()` без разделения policy объявило бы reopen для control stage, которую этот маршрут не умеет перезапускать.

### Явная стадия не отменяет eligibility

Сначала проверяются settled status, stops/cancellation, active/unresolved obligations и однозначная natural frontier. Затем существующий выбор ранее принятой root стадии и inherited accepted history ограничивают cutoff. Prevalidation/history используют тот же recovery selector; ранний `--from-stage` не обходится мимо безопасности. Binding/route/inputs/decisions/model profile compatibility и непустой prefix продолжают проверяться прежним planner.

### Дерево и история остаются авторитетными

Передаётся текущий source claim с dirty/untracked files, а не checkout старого commit или восстановленная копия плана. В disposable integration fixture recovery start проверяет takeover, новое generation и отсутствие изменений source history; реальный внешний source используется только для двух prepare. Ни SQLite/state, ни source artifacts не редактируются вручную. Drift исходного плана не принимается как result; оператор выбирает более ранний accepted `plan`, если нужно его новое исполнение.

### Предыдущая работа сохраняется

Готовый claim verification reuse `prifly/993e5ca3bcf37e7e70b21db03d90e8c88e8eddc19ba0ba7740e9cafab141c19f` с HEAD `b98064d` не освобождается и не удаляется. Для нового Run планируется отдельная authority, созданная публичным CLI, с явным `--project`; старые authority/profile и claim не меняются. Если public CLI требует другое безопасное устройство запуска, оно показывается владельцу до start. Никакой скрытой подмены machine profile.

Новый Run базируется на текущем main и этом planning commit, без неразрешённого merge предыдущей ветки. Недоступные на main reuse helpers не предполагаются установленными. Для нового Run можно явно закрепить в prepare ранее проверенный `ensure-ci-check.py` из immutable commit `17b3986`: материализовать exact bytes в scratch, проверять их SHA256 перед каждым вызовом и включить этот digest в CHECK_CONTEXT рядом с реальным permission profile. Это reviewed локальный tool input, не новая runtime возможность и не правка прежнего claim. Подготовленный gate_checks показывает полный путь, digest и весь состав до start. Helper выполняет raw `ci-check` в новом claim для итогового снимка либо проверяет evidence этого же снимка; staged bytes и свежие vuln/mod checks сохраняются. Targeted проверки при repair обязательны. Это не перенос evidence прежнего claim или другого commit.

### Локальный CLI для приёмки

После targeted regressions зафиксировать implementation checkpoint и собрать CLI из его exact HEAD с явной development version/commit, записать SHA256 и путь. Проверить им обе внешние `recover --prepare`, сравнив source version/history/claim/plan hashes до и после. Итоговый полный gate и canonical verify дополнительно проверяют снимок, содержащий записанную приёмку; финальный documentation commit не меняет code checkpoint этой сборки. Исходному хосту передаётся отдельный проверенный executable с точной командой и зарегистрированной версией; установленный release CLI 0.13.76 сохраняется. Замена machine executable, если её отдельно выберет владелец, не означает выпуск или подпись release. При несовместимости prefix prepare честно сообщает следующий отказ, который разбирается в пределах заявленного контракта.

## Risks / Trade-offs

- Схлопывание независимых failures → проверять единственную причинную цепочку и отрицательные cases, не только root count.
- Candidate reuse для невыданной Attempt → отдельная execute-only ветка без candidate refs.
- Выбор inherited стадии обходит отказ → natural eligibility проверяется до обоих выборов.
- Отдельный селектор отличается от views → не объявлять новый reopen; проверить прежние безопасные действия и refusals.
- Подготовка прошла, но новое исполнение теряет файл → disposable start fixture проверяет takeover и source immutability; внешний START остаётся у исходного хоста.
- Новая authority очищает прежний claim → публичный read-only prepare и просмотр exact effects до start; исходный claim сохраняется.

## Migration Plan

Новые данные или миграция не требуются: существующие recorded invocation/activation/diagnostic факты читаются новой policy. Сначала failing regression, затем runtime/CLI fix, targeted checks и один полный local gate. После accepted verify/review/commit — локальная сборка и read-only внешняя приёмка. Rollback возвращает executable 0.13.76; source Run/claim/план не требуют отката, потому что в реальной authority recovery не стартовал.

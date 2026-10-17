## 1. Profile и разрешение значений

- [x] 1.1 Добавить explicit profile `/4`, наследующий `/3`, и closed parser `worktree_naming`, не менять legacy `/2`/`/3` или shared профиль репозитория; проверить parser fixtures для валидного `/4`, отказа нового блока в старых профилях, unknown/extra полей и command-only режима без Git/host.
- [x] 1.2 Реализовать template grammar, exact placeholders/bindings и bounded rendering по design §2; сначала воспроизвести отрицательные случаи, затем проверить первое имя, повтор placeholder, missing/extra binding, malformed braces, длины, uppercase/Unicode, traversal, shell/ref metacharacters и reserved basename. Все unsafe cases отказывают до Git effects.
- [x] 1.3 Разрешать `{value}` или JSON Pointer effective declared input из файла/ArtifactRef вне transform; проверить nested/escaped/empty pointer, required/optional source, null/non-string/non-JSON, неизвестный port и одинаковые результаты обоих input путей; исходная schema input остаётся пользовательской.

## 2. Prepare и reviewed запуск

- [x] 2.1 Добавить reviewed naming policy и отдельный advisory preview в новую launch summary; проверить повторяемость digest, отсутствие изменений authority/registry/claims/Git/files и `reserved=false`, в том числе когда все кандидаты заняты.
- [x] 2.2 Включить declaration/bindings/resolved values в existing freshness checks до claim, исключить только availability preview; проверить template/binding/value change → `project_start_stale_launch`, а конкурентное занятие candidate → следующий suffix без изменения reviewed policy.
- [x] 2.3 Подключить resolution только к new worktree ветви start; проверить checkout, command-only и handover с отсутствующими naming inputs, queue-bound launch с явным task JSON input и отсутствие неявного извлечения данных из queue/brief.

## 3. Claim reservation и создание дерева

- [x] 3.1 Ввести validated runtime naming request, pure bounded allocator и stable semantic payload отдельно от availability/выбранного suffix; проверить последовательность `base`, `-02`, `-03`, границы `-99`/`-100`/`-9999`, exhaustion и неизменность claim ID при allocator retries. Прямой/batch claim без настройки проходит прежние fixtures.
- [x] 3.2 Добавить durable named preparing reservation и новую authority contract boundary; проверить выбор на актуальном snapshot claims при stale внешнем inventory, конфликт общего ClaimRoot между разными repositories, generation/capacity и поддерживаемое чтение legacy records без naming metadata.
- [x] 3.3 Выполнять no-overwrite Git/filesystem effects вне transform, с per-claim guard и записываемым ownership evidence; проверить внешнюю гонку ref/path, ancestor/descendant refs, symlink target/root, занятый каталог без ref и ref без каталога. Retry меняет только тот же claim после доказанного cleanup, ни один чужой объект не удаляется.
- [x] 3.4 Обработать duplicate/pending/interrupted allocation без нового имени; проверить одновременный повтор одной команды, changed payload conflict, прерывания до/после reservation, ref creation, worktree creation и activation. Unknown effect даёт `claim_preparation_incomplete` с существующим claim; allocator не оставляет скрытых собственных refs после безопасного retry.

## 4. Lifecycle и ownership

- [x] 4.1 Проверить/исправить admission, Run binding, heartbeat, doctor и assisted handoff так, чтобы они читали сохранённые path/branch; native test с двумя активными named worktrees одного repository выдаёт каждому Run его дерево на нескольких шагах, без repo-wide exclusion.
- [x] 4.2 Сохранить path/branch при continue/recover handover и generation change; проверить изменение/удаление naming declaration, отсутствие naming input у target launch, повтор продолжения, legacy source claim, новый claim после released source для continue и прежний recover refusal.
- [x] 4.3 Подключить named allocation к release и start rollback с проверкой физической identity/branch ownership; проверить штатное удаление, stale generation, подменённый каталог/ref, failed cleanup, чужой duplicate/handed-over claim и неизвестный RunStart outcome. Исходная ошибка и оставшийся claim видны в Problem.

## 5. Сквозная приёмка issue #10

- [x] 5.1 Добавить process-level CLI fixture с объявленным typed task input: prepare → start создаёт `smsplace-trucker-default-123`, независимый повтор — `-02`, каталог/ref collisions — следующий suffix; concurrent starts с barrier получают разные имена и не меняют source checkout. Проверить exhaustion на bounded inventory без создания 9999 полноценных worktrees.
- [x] 5.2 В CLI fixture доказать потерянный ответ/duplicate start с тем же command ID → те же claim/path/branch/Run и отсутствие расхода suffix; добавить stale naming digest, parallel duplicate и accepted continuation/recovery со старым именем после смены policy.
- [x] 5.3 В fixture проверить success cleanup/rollback, pruned/missing tree, leftover branch, interruption и безопасное reuse полностью освобождённого basename. Читать фактические claim records, Run и Git topology, не только формат возвращённой строки.
- [x] 5.4 Прогнать regression matrix: `/2`, `/3`, `/4` без настройки; historical claims `/1`..`/3`; command-only без Git/host; checkout; queue-bound launch; native call extensions. Все прежние state/read/session shapes и independent worktree behavior сохраняются. Новые Go tests получают `t.Parallel()`, кроме конкретных исключений AGENTS.md; проверить `internal/testguard` и `internal/purity`.

## 6. Contracts, документация и итоговые ворота

- [x] 6.1 Выпустить canonical definitions/generation и новые closed schemas для profile `/4`, summary/start `/5`, authority claims `/4`, claims view `/3`; проверить реально полученные named и naming+queue outputs, persisted claims, отрицание их старыми closed schemas и отсутствие новых полей на legacy outputs. Аудировать questionnaire/project init/read selectors на profile `/4`; новую форму выпускать отдельно, если существующая schema закрывает допустимые profile versions. Проверить `make schemas-check`, не регенерируя исторические bundles.
- [x] 6.2 Добавить typed refusal codes и capability `project_worktree_naming` с минимальной ревизией; обновить ровно соответствующую строку `examples/README.md`, полноценный authoring пример профиля/declared input/prepare/start, troubleshooting и фактический runner/read handoff. Проверить `TestEveryDeclaredCapabilityIsInTheAuthorIndex`, `make refusal-check` и CLI fixture по опубликованному примеру; review подтверждает, что показаны поля и версии, не только ссылка.
- [x] 6.3 Уточнить existing glossary entries по delta `specification-governance`, устранить устаревшую фразу о единственном claim на repository в соответствии с текущим worktree/checkout conflict contract, добавить Go/JSON bindings только реально реализованных полей. Выполнить `TestGlossaryBindings`; проверить `openspec validate configure-project-worktree-names --strict`, source-set consistency и отсутствие новых самостоятельных источников нормы.
- [x] 6.4 Во время реализации использовать `make test-changed`/`make check-fast`; для итогового снимка перед коммитом один раз выполнить `make ci-check-once CHECK_CONTEXT=<фактический-профиль>`, с `CHECK_FOR_COMMIT=1` для staged bytes. Проверить `git diff --check` и diff/untracked inventory: `openspec/changes/archive/**`, прошлые release evidence и уже опубликованные schema bytes неизменны. В evidence назвать выполненные команды и границы проверки; race/full e2e не запускать до release, qualification/roadmap gate не объявлять закрытыми по document validation.

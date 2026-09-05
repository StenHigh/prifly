## 1. Контракты и воспроизводимый дефект

- [ ] 1.1 Сверить версии StepDefinition, authoring, assisted session, state/read и затронутые readers с опубликованным inventory; сохранить точную карту в design и добавить regression нынешнего позднего ответа: ответ сохранён, результат отклонён по deadline. Проверка: адресный тест воспроизводит отказ без настоящего ожидания.
- [ ] 1.2 Добавить новый authoring marker и versioned `session_limits` с конечным active default и бессрочным wait default; закреплять настройки в definition, не менять legacy compilation. Проверка: compiler tests для defaults, индивидуальных значений, новой revision, неверного executor, нуля, отрицательных чисел и переполнения; прежние fixture digests неизменны.
- [ ] 1.3 Ввести необходимые новые session/state/read editions с сохраняемыми остатком, фазой ожидания, observations и поколением доставки; связать initial envelope, answer context и effective timing проверяемыми bytes. Проверка: round-trip/reopen, отказ stale или подменённой delivery и чтение old editions без переписывания frozen schemas.

## 2. Время и повторный допуск

- [ ] 2.1 Реализовать закрытие активного интервала при допустимом вопросе и сохранение остатка; не допускать пополнения при ответе, automatic/preanswer, повторной команде, restart или rollback. Проверка: детерминированные переходы с управляемыми Observation, включая active expiry до вопроса и точную границу finite wait.
- [ ] 2.2 При безопасной передаче управления парковать ту же Attempt: fence старую delivery, освободить execution slot и исключить её из executing parallelism, сохранив pins/outputs/claims. Проверка: capacity 1 позволяет другому Run работать; неизвестный in-flight effect даёт отказ до освобождения, второй singleton question не портит свою delivery.
- [ ] 2.3 Разделить сохранение ответа и повторный admission через существующую очередь; выдавать новую delivery той же Attempt только после capacity, ControlPin, revocation и resource checks, с прежними inputs и остатком. Проверка: занятый slot не теряет ответ и не тратит бюджет; Pause/Stop и истёкшие права не обходятся, claim не продлевается и не освобождается автоматически.
- [ ] 2.4 Убрать блокирование независимой работы и управляющих команд открытым вопросом. Проверка: sibling может завершиться, join ждёт припаркованную ветвь, cancellation/recovery доступны раньше idle/waiting_decision, новый вопрос не требуется для продвижения уже отвеченной Attempt.

## 3. Истечение, отмена и понятный интерфейс

- [ ] 3.1 Обрабатывать active/wait expiry и cancel открытого вопроса с сохранением причины закрытия; отвергать late answer/result без hidden retry и не применять managed-only false ProcessOutcome к переданной host работе. Проверка: cancel/answer race, effects:none без обязательств завершается отменой, unknown effect остаётся в recovery; legacy deadline не получает продление.
- [ ] 3.2 Показать в launch questionnaire/summary оба срока, defaults и контекст вопроса; различать «ответ сохранён» и «работа может продолжиться», объяснять capacity/control/claim recovery допустимыми командами. Проверка: CLI snapshots и runner instruction tests, включая legacy Run, не повторяют известный ответ и не обещают auto-wakeup или физическую остановку host.

## 4. Сквозная проверка и согласование документации

- [ ] 4.1 Расширить существующий mixed regression: 10 минут работы, вопрос, две недели управляемого времени, reopen, ответ, допустимое продолжение с 50 минутами и точные bytes итогового отчёта. Проверка: тот же Run/Attempt, реальные admitted transitions и outputs; без sleep, подмены snapshot или нового полного бюджета. Дополнить case отказа при неподтверждённом Git claim, не выдавая no-Git успех за его квалификацию.
- [ ] 4.2 Выполнить один короткий public CLI mixed Run на named source binary с человеческим ответом и итоговой программой; сохранить версии, digest, точный итог и наблюдённые границы в этой записи. Проверка: повторное чтение completed/succeeded и экспорт ожидаемого отчёта; native UI обоих hosts и недели реального времени не объявлять проверенными.
- [ ] 4.3 Выполнить адресные Go tests затронутых packages, race-проверки переходов, `make schemas-check`, `git diff --check` и `openspec validate --all --strict --no-interactive`; записать точные counts и результаты. Полные `make check` и `make e2e` выполнить один раз на общем candidate при завершении `make-project-launch-workflow-neutral`, не повторять на каждом срезе. Проверка защиты: `git diff f1c9ebd85a971ab0394ea19f49e089a666dc5864 -- openspec/changes/archive` пуст; old public bundles проверены в 1.3.
- [ ] 4.4 Синхронизировать реализованные delta specs, glossary bindings, полный YAML reference, examples и roadmap; запустить `TestGlossaryBindings` при изменении карты и editor/reference checks для новых YAML полей. Проверка: документация разделяет готовую возможность, сохранённые legacy сроки и незакрытые UI/AIF gates; абзац «Что в этот срез не входит» явно оставляет managed process >1 часа, auto-wakeup, автоматическое обновление claims, Release и обновление полигона вне этой работы. Коммит с объяснением причин и push без переписывания истории.

## Planning verification — 2026-09-06

Подготовлены proposal, пять delta specs, design и 13 задач реализации.
`openspec validate --all --strict --no-interactive`: 20 passed, 0 failed.
`git diff --check`: pass; historical archive относительно `f1c9ebd` не изменён.
Реализовано 0/13: план и проверка документов не означают исправление runtime.

Что в этот срез не входит: код, новые бинарники, миграция старых Runs,
Release, полигон и product qualification. Дорогие code/CI gates для этого
docs-only planning среза не запускаются; результаты прежнего короткого
живого теста и его ограничения сохранены в design.

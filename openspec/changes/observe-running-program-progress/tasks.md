## 1. Контракт необязательного прогресса

- [ ] 1.1 Задать versioned `program-progress/1` и обновить словарь/опубликованный контракт без изменения StepResult; проверить закрытую schema и старые bundles через `make schemas-check` и `TestGlossaryBindings`.
- [ ] 1.2 Добавить необязательный fd 4, ограничение размера/частоты и parser с отказом от неверных счётчиков; проверить старую программу без fd 4, частые/вредоносные сообщения и неизменный verdict через `go test ./internal/local -run 'Test.*Progress' -count=1`.
- [ ] 1.3 Добавить новую редакцию StepDefinition/YAML authoring с явным opt-in live-вывода только для program step и конечным process-output budget; отсутствие полей и старые editions сохраняют прежний режим и 64 КиБ. Проверить compile/reseal, отказ несовместимого executor и чтение сохранённых bundles.

## 2. Authority и read projection

- [ ] 2.1 Хранить только последнюю запись конкретной running Attempt и loss counter с проверкой владельца без изменения Run version/deadline; проверить restart, stale owner и dedup через `go test ./internal/runtime ./internal/local -run 'Test.*Progress' -count=1`.
- [ ] 2.2 Добавить versioned read-only CLI projection с `reported|not_reported|stale|unavailable`, временем и известными counters; проверить старые JSON readers, отсутствие side effects чтения и доступ чужого principal через `go test ./cmd/prifly -run 'Test.*Progress' -count=1`.
- [ ] 2.3 Использовать существующее считывание stdout/stderr для bounded tail только opt-in Attempt; хранить порядок, offsets/gaps, stream identity и свежесть отдельно от Run events. Проверить шумную программу, overflow tail без смены verdict, конечный process budget, restart и закрытый доступ без opt-in.
- [ ] 2.4 Добавить versioned read-only CLI projection фрагментов с теми же правами, что Run; проверить cursor/gap, отсутствие содержимого для других steps и чужого principal, неизменность старых ответов.

## 3. Монитор и реальный tests-worker

- [ ] 3.1 Показать фазу, давность и неполноту в выбранной program Attempt, не теряя selection/scroll; проверить HTML escaping, отсутствие ложного `pass` и polling в `node cmd/prifly/monitor_ui_test.cjs`.
- [ ] 3.2 Показать отдельную read-only панель stdout/stderr лишь для opt-in Attempt: пропуски, усечение, давность, предел процесса и финальный статус. Проверить HTML/ANSI/control escaping, сохранение scroll/selection и отсутствие terminal input/actions в `node cmd/prifly/monitor_ui_test.cjs`.
- [ ] 3.3 Обновить SMSPlace tests-worker для редких `product`, `extra`, `baseline`, `result` сообщений и коротких stdout summaries; проверить fixture без полного product-suite, не выводя полный PHPUnit log.
- [ ] 3.4 На нейтральной программе и длительном тестовом Run проверить live-фрагменты во втором read-only клиенте, смену фаз, overflow/gap, terminal/failure без подмены результата и неизменность bytes исторического Run/evidence; выполнить `openspec validate observe-running-program-progress --strict`, `git diff --check` и затронутые Go tests.

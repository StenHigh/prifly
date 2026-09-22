## 1. Понятная колонка исполнения

- [ ] 1.1 Переименовать заголовок колонки и добавить короткое видимое, доступное с клавиатуры пояснение StepInstance, Attempt и Settlement без hover-only подсказки; проверить текст и узкий экран в `node cmd/prifly/monitor_ui_test.cjs`.
- [ ] 1.2 Заменить `N / M` отдельными подписанными счётчиками из существующего RunSummary, показать учтённые/неучтённые Attempt и известное ожидание хоста, а недоступные/несогласованные значения обозначить неизвестными; проверить `node cmd/prifly/monitor_ui_test.cjs`.

## 2. Регрессия и границы

- [ ] 2.1 Добавить UI fixtures для `4` шагов/`4` попыток/`3` учтённых/`1` ожидания, повторной Attempt, невыбранной ветви и `rejected` outcome; проверить, что числа не выглядят прогрессом или успехом, а обновление сохраняет фильтр и позицию через `node cmd/prifly/monitor_ui_test.cjs`.
- [ ] 2.2 Проверить старые сохранённые Run в read-only мониторе и неизменность их state/event/evidence bytes; убедиться, что runtime, JSON API и schema не менялись, затем выполнить `openspec validate clarify-run-list-execution-counts --strict` и `git diff --check`.

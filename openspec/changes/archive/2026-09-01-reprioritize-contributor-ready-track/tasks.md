## 1. Обновление очереди

- [x] 1.1 Синхронизировать изменённую requirement `delivery-roadmap` в
  постоянную specification и проверить `openspec validate --specs --strict`.
- [x] 1.2 Убедиться, что current queue последовательно называет GitLab CI,
  YAML-only cleanup, independent corpus и editor contract, а P1/P2 status не
  меняется; проверить diff вручную.

## 2. Защита истории и плана

- [x] 2.1 Проверить `git diff --check` и что change не изменяет historical
  evidence или archived OpenSpec changes.
- [x] 2.2 Создать следующий отдельный OpenSpec change для native GitLab CI и
  проверить, что его proposal ограничивает CI командами `make check` и
  `make e2e`.

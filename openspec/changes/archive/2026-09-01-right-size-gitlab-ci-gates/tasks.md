## 1. Экономный automatic gate

- [x] 1.1 Добавить `make ci-check` без race и проверить, что он выполняет
  normal tests, vet, formatting и schemas.
- [x] 1.2 Переключить automatic GitLab job на `ci-check` и e2e, добавить
  manual `make race` job с теми же relevant-change rules и проверить CI lint.

## 2. Документация и приёмка

- [x] 2.1 Синхронизировать delivery roadmap с разделением fast и release
  gates, не меняя historical evidence.
- [x] 2.2 Выполнить `make ci-check`, `make e2e`, OpenSpec strict validation и
  `git diff --check`; следующий готовый remote batch использует новый policy.

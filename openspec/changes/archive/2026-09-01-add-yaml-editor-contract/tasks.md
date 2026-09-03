## 1. Published editor contract

- [x] 1.1 Добавить local versioned JSON Schema и manifest для шести YAML document kinds; проверить JSON parsing и stable IDs stdlib test.
- [x] 1.2 Добавить portable modelines в полные workflow и step references и краткое руководство с local mappings; проверить, что existing Go lowering tests принимают references.

## 2. Verification and publication

- [x] 2.1 Включить static editor-contract check в `make e2e`, обновить user/test pointers и проверить targeted test, `make ci-check`, `make e2e`, strict OpenSpec validation, `git diff --check` и неизменность historical archived changes.

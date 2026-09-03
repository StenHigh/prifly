## 1. Единственный authoring route

- [x] 1.1 Ограничить Project execution profile profile v2 и folder workflow
  launches; удалить v1/task-recipe/direct-machine branches и проверить CLI
  rejection tests.
- [x] 1.2 Принимать package source только как workflow folder, сохранив internal
  folder compiler, и проверить folder compile/import plus flat-file rejection.

## 2. Current source set

- [x] 2.1 Обновить generated runner skill, glossary, workflow specification и
  authoring examples; проверить, что current tree не называет legacy source
  supported.

## 3. Verification

- [x] 3.1 Выполнить targeted `cmd/prifly` tests и race, `make ci-check`,
  `make e2e`, OpenSpec strict validation и `git diff --check`; проверить
  неизменность historical evidence и archived changes. Full `make race`
  остаётся ручным RC/release gate по delivery policy.

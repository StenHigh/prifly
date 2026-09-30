## 1. Версионированный контракт

- [x] 1.1 Добавить StepDefinition v13 и Go field без расширения v11/v12; проверить `TestAssistedReadOnlyRepositoryWorkspaceLowersToV13` и `TestEveryStepContractThisBuildLowersToIsNamed`.
- [x] 1.2 Добавить YAML lowering и обе editor schemas, отказ старому marker/pin; проверить `TestSessionLimitsEditorSchemaMatchesAuthoring`.

## 2. Claim и handoff

- [x] 2.1 Вывести Git requirement из декларации шага, отказать Start без claim и передать read-only claim только объявившему шагу; проверить `TestCLIProjectStepDeclaresReadOnlyClaim` и `TestExternalWriterMayReadAnExplicitlyClaimedRepository`.
- [x] 2.2 Проверить отказ `effect_not_permitted` после изменения дерева read-only шагом; прогнать focused runtime test.

## 3. Публикация и регрессия

- [x] 3.1 Обновить author index, справочник и glossary; проверить `TestEveryDeclaredCapabilityIsInTheAuthorIndex` и `TestGlossaryBindings`.
- [x] 3.2 Выполнить `make ci-check`, `make e2e`, `openspec validate declare-read-only-repository-workspace --strict` и `git diff --check`; проверить `git diff -- openspec/changes/archive schemas/core` на отсутствие правок protected historical evidence.

## 1. Перенос действующих правил

- [x] 1.1 Перенести текущие glossary entries, anchors и Go/JSON bindings в
  `openspec/specs/specification-governance/terms.md`; сверить число entries и
  marker pair с legacy glossary до изменения test path.
- [x] 1.2 Дополнить permanent governance spec правилами contributor process и
  обновить entry pointers; проверить, что current OpenSpec source не ссылается
  на удаляемый `docs/` source как на норму.
- [x] 1.3 Создать archived coverage crosswalk для четырёх legacy files и
  перенести autonomous decision journal как неизменённый historical material;
  проверить byte identity перенесённого журнала.

## 2. Cutover и защита совместимости

- [x] 2.1 Перенаправить `TestGlossaryBindings` на OpenSpec glossary без
  изменения проверяемой Go/JSON карты; выполнить его точечный Go test.
- [x] 2.2 Отметить `specification-governance` перенесённой в карте источников
  и проверить, что legacy source set, evidence и оба manifests не изменены.
- [x] 2.3 Выполнить `openspec validate --specs --strict`, strict validation
  change, `git diff --check` и relevant Go test; архивировать completed change
  только после успешных результатов.

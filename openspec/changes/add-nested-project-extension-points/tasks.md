## 1. Граница и контракт

- [x] 1.1 Воспроизвести exact v0.13.82 baseline и проверить существующую композицию; evidence: success/repair PASS, root measurement=1 при reader=2, nested project_extension_unknown_workflow; сохранить summary в change.
- [x] 1.2 Оформить opt-in package points и project outputs contract; проверить OpenSpec change строгой validation до реализации.

## 2. Компилятор и регрессии

- [x] 2.1 Реализовать closed sidecar/extension parsing, per-child dependency order, entry/direct-edge splicing и explicit consumer bindings; проверить целевые CLI tests для step/call и nested repeat.
- [x] 2.2 Проверить named refusals для unknown/ambiguous/conflicting anchors, bindings/components, missing outputs, mismatch schema, unavailable producer и cycles; проверить legacy identity, disabled/absent passthrough, Sources/build/review freshness и effects.

## 3. Native example и поставка

- [x] 3.1 Добавить generic runnable example и bounded native driver: success, repair/repeat, blocked/partial, fail/no_work, reviewed continue, stale/missing outputs, report/log refs после удаления producer directory; synthetic граница должна быть явной.
- [x] 3.2 Добавить closed editor schemas, capability, author index, reference и glossary; проверить schema checks, TestGlossaryBindings и capability index.

## 4. Итоговый снимок

- [x] 4.1 Выполнить Run verify/security/review и локальный commit; проверки задаёт gate_checks: ci-check-once с actual CHECK_CONTEXT, strict OpenSpec, diff --check. Проверить staged bytes и отсутствие изменений protected historical evidence через git diff --name-only относительно a13146c.
- [ ] 4.2 После owner release выполнить generic example на released binary и передать capability consumer issue #9; local candidate tests не закрывают этот release gate.

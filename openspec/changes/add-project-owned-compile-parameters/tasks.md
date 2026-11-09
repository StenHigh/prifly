## 1. Контракт и resolver

- [x] 1.1 Добавить regression checks неизвестного параметра и type/range/binding отказов; подтвердить исходное падение через targeted Go tests.
- [x] 1.2 Реализовать scoped sidecar reader и existing scalar substitution до sealing; проверить default/owner values и profile /2 refusal.
- [x] 1.3 Проверить composition/import ownership и неизменность baseline без sidecars через targeted compilation tests.

## 2. Приёмка issue #22

- [x] 2.1 Проверить C1–C3: длины 12000/12001/16557/32000/32001, Unicode и одинаковую диагностику prepare/start.
- [x] 2.2 Проверить C4 и C8: root/child/worker, project-owned consumer и несовместимый binding через execution handoffs.
- [x] 2.3 Проверить C5–C7: stale review, два проекта в authority, update preservation, pinned Run и continue либо явный pre-effect refusal.

## 3. Публикация контрактов и проверка

- [x] 3.1 Добавить closed authoring schemas, capability, authoring reference, operations guide и термин; выполнить schema/capability/glossary checks.
- [x] 3.2 Выполнить `make test-changed`, `openspec validate add-project-owned-compile-parameters --strict` и `git diff --check`; проверить отсутствие изменений historical evidence и old bundles.
- [ ] 3.3 Выполнить `make ci-check-once CHECK_CONTEXT=codex-app-authorized`; проверить итоговый diff и сохранить готовый commit.
- [ ] 3.4 Сохранить evidence C1–C8 и закрыть issue #22 с доступной ссылкой на готовое решение; downstream issue #3 остаётся самостоятельной приёмкой.

## 1. Архитектурный contract и основания

- [x] 1.1 Сверить 17 architecture boundaries с replacement requirements; проверить 17 blocks, scenarios и strict validation без legacy IDs.
- [x] 1.2 Перенести первые 7 decision records в descriptive OpenSpec paths и index; проверить context, decision, consequences и reconsideration trigger каждого.
- [x] 1.3 Перенести следующие 8 decision records в descriptive OpenSpec paths и index; проверить внутренние links и отсутствие ordinal filenames.
- [x] 1.4 Перенести последние 7 decision records, включая два legacy duplicates, в unique descriptive paths; проверить total 22 records.

## 2. Трассировка и переключение

- [x] 2.1 Создать individual archived crosswalk для 17 rules, 22 decision files и exact acceptance links; проверить отсутствие ARCH/ADR IDs в permanent architecture docs.
- [x] 2.2 Синхронизировать `architecture-decisions`, выполнить `openspec validate --specs --strict` и переключить только её source-map row при неизменных legacy-байтах.

## 3. Сохранность и архив

- [x] 3.1 Проверить через `git diff` неизменность кода, schemas, evidence и manifests; архивировать change и выполнить `openspec validate --all --strict --archived`.

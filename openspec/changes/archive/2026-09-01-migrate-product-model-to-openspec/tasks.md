## 1. Проверка replacement

- [x] 1.1 Сверить 20 requirements `docs/spec/01-product.md` с 20
  descriptive requirements candidate spec и отметить все 20 строк crosswalk
  как `verified`; проверить равные counts через `rg -c '^### PROD-'` и
  `rg -c '^\| `docs/spec/01-product.md#'`.
- [x] 1.2 Сверить acceptance cases каждой строки crosswalk с
  `docs/roadmap/requirements-map.csv`; проверить, что permanent spec не
  содержит legacy `PROD-*` или `AC-*` identifiers.

## 2. Cutover ownership

- [x] 2.1 Применить `product-model` в `openspec/specs/` штатным OpenSpec
  archive workflow и проверить `openspec validate --specs --strict`.
- [x] 2.2 Переключить ровно строку `Модель продукта` в
  `openspec/SOURCE-OF-TRUTH.md` на `openspec/specs/product-model/spec.md` со
  статусом `Перенесено`; проверить, что legacy глава и CSV-карты не изменены.

## 3. Проверка и история

- [x] 3.1 Выполнить `openspec validate migrate-product-model-to-openspec
  --strict` до archive и проверить, что archive сохраняет crosswalk в
  `openspec/changes/archive/`.
- [x] 3.2 Выполнить `git diff --check` и review diff; подтвердить отсутствие
  runtime, wire-contract, `docs/evidence/**` и manifest changes.

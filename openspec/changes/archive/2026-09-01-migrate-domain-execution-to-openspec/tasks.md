## 1. Проверка покрытия legacy source set

- [x] 1.1 Сверить 26 headings execution chapter, TaskInput/1 и 27 строк crosswalk с replacement requirements; проверить счётчиками и ручным content review.
- [x] 1.2 Сверить каждый указанный acceptance link с `docs/roadmap/requirements-map.csv`; зафиксировать `verified` только после проверки каждого link.
- [x] 1.3 Проверить, что delta spec не содержит legacy `DOM-*` или `AC-*` identifiers, а detailed TaskInput contract не становится второй independently editable schema в product-model.

## 2. Cutover нормативного источника

- [x] 2.1 После review перенести validated `domain-execution` из change в `openspec/specs/` и проверить `openspec validate --specs --strict`.
- [x] 2.2 Переключить только строку `Исполнение и вход задачи` в `openspec/SOURCE-OF-TRUTH.md` на `openspec/specs/domain-execution/spec.md`; проверить, что legacy source set остался byte-for-byte неизменным.

## 3. Сохранность и архив

- [x] 3.1 Проверить `git diff --check`, `openspec validate migrate-domain-execution-to-openspec --strict` и protected evidence/manifests/runtime diff; документальная проверка не закрывает product gates.
- [x] 3.2 Архивировать change после завершения всех задач и проверить `openspec validate --all --strict --archived` вместе с отсутствием незакрытых task checkboxes.

## 1. Проверка покрытия source set

- [x] 1.1 Развернуть ranges crosswalk в 51 numbered package/workflow/context records и YAML contract sections; сверить каждый с replacement requirement и source content.
- [x] 1.2 Сверить каждый acceptance link из `docs/roadmap/requirements-map.csv` и chapter acceptance table; отметить crosswalk `verified` только после content review.
- [x] 1.3 Проверить отсутствие legacy IDs в delta spec и отсутствие второй YAML/runtime semantics.

## 2. Cutover capability

- [x] 2.1 Синхронизировать reviewed `workflow-and-context` в `openspec/specs/` и проверить `openspec validate --specs --strict`.
- [x] 2.2 Переключить только соответствующую строку `openspec/SOURCE-OF-TRUTH.md`; проверить byte-for-byte неизменность обоих legacy sources.

## 3. Сохранность и архив

- [x] 3.1 Проверить strict OpenSpec validation, `git diff --check` и отсутствие diff в runtime, schemas, evidence и historical manifests.
- [x] 3.2 Архивировать change после всех задач и проверить `openspec validate --all --strict --archived` без незакрытых checkboxes.

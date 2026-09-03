## Why

После завершения OpenSpec cutover текущая очередь всё ещё требует поддержки
дорелизных вариантов YAML authoring, хотя Pri-Fly ещё не имеет публичного
релиза. Это создаёт лишний объём до появления реальных пользователей и
мешает определить единственный проверяемый путь для contributor.

## What Changes

- Переставить contributor-ready работы в явную последовательность: native
  GitLab CI на актуальных воротах, один YAML authoring route без дорелизной
  совместимости, independent YAML validator corpus, затем editor contract.
- Зафиксировать, что CI не запускает удалённые custom document checks: его
  единственные ворота — актуальные `make check` и `make e2e`.
- **BREAKING** До первого публичного релиза удалить альтернативные source
  forms: project profile v1, Python task recipes и плоский package source.
  Единственным authoring input остаётся YAML-каталог workflow.
- Не менять порядок P1/P2 milestones и не объявлять contributor-ready работу
  закрытием product qualification.

## Capabilities

### New Capabilities

<!-- Нет: change меняет приоритет и границу уже существующей capability. -->

### Modified Capabilities

- `delivery-roadmap`: текущая post-RC очередь и её порядок заменяются на
  YAML-only contributor-ready track.

## Impact

Меняется план разработки и будущие границы project authoring. Runtime, sealed
packages, JSON wire contracts и уже сохранённые Runs в этом planning change не
изменяются. Current source ownership не меняется: `delivery-roadmap` остаётся
в `openspec/specs/delivery-roadmap/spec.md`.

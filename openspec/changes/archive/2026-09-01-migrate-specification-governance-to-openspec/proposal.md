## Why

Последний неперенесённый нормативный source set смешивает правила изменения
спецификации, словарь, инструкции участникам и журнал рабочих решений. Пока он
живёт в `docs/`, финальная очистка оставит Pri-Fly без единого места, где
понятно, как называть и менять его понятия.

## What Changes

- Перенести каноническую терминологию и правила её изменения в capability
  `specification-governance`; OpenSpec станет единственной текущей точкой для
  этих правил после cutover.
- Перенести полезные contributor-инструкции: честность утверждений, границы
  проверок, сохранение версионированных контрактов и порядок OpenSpec change.
- Сохранить автономные решения как исторический журнал в archive migration,
  отделив их от действующих требований и ADR.
- Обновить карту источников и стартовые pointers. **BREAKING**: при финальной
  очистке legacy `docs/glossary.md`, `docs/development.md`,
  `docs/agent-brief.md` и `docs/autonomous-decisions.md` будут удалены из
  release tree, но их Git-история и archive migration сохранятся.

## Capabilities

### New Capabilities

<!-- Нет. -->

### Modified Capabilities

- `specification-governance`: capability получает полный действующий контракт
  терминологии, contributor process и правила хранения исторических решений.

## Impact

Меняется только нормативная документация и development process. Go runtime,
CLI, YAML authoring, package format, public JSON contracts и сохранённые Runs
не меняются.

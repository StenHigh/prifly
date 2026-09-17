# Project Rules

> Short, actionable rules and conventions for this project. Loaded automatically by /aif-implement.

## Rules

- Обязательные правила репозитория — `AGENTS.md`; они старше этого файла и правил ниже.
- Нормативное изменение (spec, словарь, контракт, roadmap) начинается с OpenSpec change (`/openspec-propose` → apply → archive с sync); без change правятся только код, тесты и справочники.
- Не создавать `docs/` и не запускать `/aif-docs`: документация — `README.md`, `examples/` и `openspec/specs/` (spec `release-documentation-layout`).
- Roadmap — только `openspec/specs/delivery-roadmap/spec.md`; `/aif-roadmap` и `/aif-archive` его не правят.
- Не переписывать historical evidence: `openspec/changes/archive/**`, `release-*.md`, старые public bundles в `schemas/`.
- Коммиты — на английском в conventional style (`feat:`, `fix:`, `docs:`), без amend; push и теги релиза — только владелец.
- Отчёты владельцу — по-русски, результат первым: что сделано, что намеренно нет, точный результат ворот.

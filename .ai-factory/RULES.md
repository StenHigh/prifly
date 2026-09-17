# Project Rules

> Short, actionable rules and conventions for this project. Loaded automatically by /aif-implement.

## Rules

- Обязательные правила репозитория — `AGENTS.md`; они старше этого файла и правил ниже.
- Старшинство источников требований: активный OpenSpec change (его `specs/**` и `design.md`) > `openspec/specs/<capability>/spec.md` > карты `.ai-factory/*`, roadmap и `CONTEXT_STATE.md` (ориентиры, не контракт).
- Порядок работы над задачей: OpenSpec change (`/openspec-propose` → подтверждение → commit) → Run `aif-classic` с задачей, ссылающейся на change → merge claim-ветки в `main` владельцем → `/openspec-archive-change` с sync. Задача без нормативного следа (баг в рамках контракта, рефакторинг, инструменты) идёт в Run без change.
- Прогресс change — галочки в его `tasks.md`; план AI Factory их не дублирует, а ссылается на пункты по id.
- Одна история: план Run (`.ai-factory/plans/*`) удаляется в том же коммите, что архивирует его change; `/aif-archive` не используется. Run идёт в `worktree` (standing choice в `project.yaml`), claim-ветку `prifly/<claim>` владелец вливает в `main` до следующего `project start`.
- `/aif-docs`, `/aif-roadmap`, `/aif-architecture` и `/aif-archive --roadmap` здесь не запускаются: их артефакты принадлежат OpenSpec (overlay каждого навыка в `.ai-factory/skill-context/` говорит это ему сам).
- Нормативное изменение (spec, словарь, контракт, roadmap) начинается с OpenSpec change (`/openspec-propose` → apply → archive с sync); без change правятся только код, тесты и справочники.
- Не создавать `docs/` и не запускать `/aif-docs`: документация — `README.md`, `examples/` и `openspec/specs/` (spec `release-documentation-layout`).
- Roadmap — только `openspec/specs/delivery-roadmap/spec.md`; `/aif-roadmap` и `/aif-archive` его не правят.
- Не переписывать historical evidence: `openspec/changes/archive/**`, `release-*.md`, старые public bundles в `schemas/`.
- Коммиты — на английском в conventional style (`feat:`, `fix:`, `docs:`), без amend; push и теги релиза — только владелец.
- Отчёты владельцу — по-русски, результат первым: что сделано, что намеренно нет, точный результат ворот.

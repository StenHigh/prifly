# Pri-Fly — карта нормативных источников

Эта карта отвечает на один вопрос: **где сегодня менять правило**. Она не
повторяет требования и не заменяет их. «Source set» — один явно названный
комплект взаимосвязанных файлов; у capability не может быть второго,
независимо редактируемого описания с тем же смыслом.

## Текущая ownership-карта

Строка ниже называет **один текущий source set** и его конечный OpenSpec path.
Состояние «Частично перенесено» означает: в OpenSpec уже есть baseline, но
указанные legacy-файлы всё ещё содержат правила, которых там нет. До смены
строки на «Перенесено» менять нужно весь названный current source set, а не
создавать второй пересказ.

| Capability | Единственный текущий source set | Состояние миграции | Конечный OpenSpec путь |
|---|---|---|---|
| Управление спецификацией и общая терминология | `openspec/specs/specification-governance/spec.md`, `openspec/specs/specification-governance/terms.md`, `openspec/specs/specification-governance/agent-brief.md` | Перенесено | `specification-governance` |
| Release documentation layout | `openspec/specs/release-documentation-layout/spec.md` | Перенесено | `release-documentation-layout` |
| Public release distribution | `openspec/specs/release-distribution/spec.md` | Перенесено | `release-distribution` |
| Модель продукта | `openspec/specs/product-model/spec.md` | Перенесено | `product-model` |
| Исполнение и вход задачи | `openspec/specs/domain-execution/spec.md` | Перенесено | `domain-execution` |
| Сценарии, пакеты, контекст и YAML authoring | `openspec/specs/workflow-and-context/spec.md` | Перенесено | `workflow-and-context` |
| Каталог и жизненный цикл решений Run | `openspec/specs/run-decisions/spec.md` | Перенесено | `run-decisions` |
| Runtime, ресурсы и хранение | `openspec/specs/runtime-resources/spec.md` | Перенесено | `runtime-resources` |
| Управление, безопасность и интерфейс | `openspec/specs/control-security-ux/spec.md` | Перенесено | `control-security-ux` |
| CLI и публичный протокол | `openspec/specs/cli-protocol/spec.md` | Перенесено | `cli-protocol` |
| Качество и приёмка | `openspec/specs/quality-and-acceptance/` | Перенесено | `quality-and-acceptance` |
| Архитектурные решения | `openspec/specs/architecture-decisions/` | Перенесено | `architecture-decisions` |
| Foundation profile | `openspec/specs/foundation-profile/` | Перенесено | `foundation-profile` |
| Наблюдаемость, публикации и реакции | `openspec/specs/observability-publication-reactions/` | Перенесено | `observability-publication-reactions` |
| Delivery, статус и будущая очередь | `openspec/specs/delivery-roadmap/spec.md` | Перенесено | `delivery-roadmap` |
| Опубликованные контракты | `openspec/specs/published-contracts/spec.md` | Перенесено | `published-contracts` |

Historical documentation, evidence, manifests и custom document tooling были
удалены из release tree после полного cutover. Их exact history сохранена в
Git и archived OpenSpec changes; они не являются current source set и не
восстанавливаются как самостоятельные документы. Versioned JSON Schema и Go
definitions остаются product artifacts рядом с кодом, а не Markdown-копиями в
OpenSpec.

## Как вести изменение

1. Найди capability в таблице и прочитай её current source set.
2. Создай change: `openspec new change <kebab-case-name>`; для Codex можно
   попросить `$openspec-propose`.
3. В proposal назови затронутые capabilities. Пока строка не отмечена как
   перенесённая, меняй указанный source set, а OpenSpec change используй как
   проверяемый plan/delta.
4. Отдельный migration change переносит одну capability: создаёт проверяемый
   legacy coverage crosswalk, сохраняет acceptance meaning, затем меняет эту
   строку и входные ссылки. Постоянный OpenSpec spec использует понятные
   headings, а старые внутренние IDs остаются только в archived crosswalk.
   Только после этого `openspec/specs/...` становится источником правды.

Для локальной работы требуется только development tool, не часть Pri-Fly
binary: `npm install -g @fission-ai/openspec@1.11.0`. После установки
`openspec status` показывает активные changes. Архивированный change сохраняет
историю планирования в `openspec/changes/archive/`.

## Границы

OpenSpec не меняет Go runtime, user-facing YAML, sealed JSON package или
сохранённые Runs. Historical release evidence и manifests сохранены в Git и
archive changes; текущая документация не переписывает эту историю.

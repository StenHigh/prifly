## Context

`docs/spec/01-product.md` содержит 20 legacy requirements о назначении и
границах Pri-Fly. Их связь с acceptance cases хранится в
`docs/roadmap/requirements-map.csv`. По `openspec/SOURCE-OF-TRUTH.md` эти
файлы остаются единственным current source set до завершения этого change;
новый `product-model` становится source of truth только при cutover.

## Goals / Non-Goals

**Goals:**

- Перенести каждый смысловой rule главы в один читабельный OpenSpec
  requirement и scenario.
- Оставить проверяемую связь старых requirements и acceptance cases в archive
  change, но не в постоянном product spec.
- Сделать cutover обратимым до final cleanup: legacy source остаётся в дереве
  без изменения bytes.

**Non-Goals:**

- Не менять runtime, CLI, YAML, schemas, acceptance evidence или roadmap
  status.
- Не удалять `docs/spec/01-product.md`, CSV-карты или historical manifests.
- Не превращать OpenSpec в новый ID-based document system.

## Decisions

### Сохранять одну смысловую границу на прежнее требование

Постоянный spec содержит 20 descriptive requirements, по одному на каждую
границу legacy-главы. Это сохраняет reviewable granularity без старых номеров.
Объединение нескольких правил в широкий абзац отвергнуто: оно сделало бы
невозможной проверку полноты и связей с acceptance cases.

### Legacy IDs живут только в этом crosswalk

Таблица ниже — единственное место этого change с `PROD-*` и `AC-*`. После
archive она остаётся проверяемой историей перехода; `openspec/specs/product-model`
использует только понятные headings. Альтернатива — оставить IDs в постоянном
spec — отвергнута как сохранение прежнего custom tracking format.

### Cutover происходит после content review и OpenSpec validation

Apply сначала сверяет каждую строку crosswalk с permanent spec и source
chapter, затем обновляет source map. Не удаляет и не редактирует legacy source.
Так rollback до final cleanup — это возвращение одной map row, а не попытка
восстановить historical bytes.

### Legacy coverage crosswalk

| Legacy source | Legacy record | Acceptance cases | Replacement requirement | Review |
|---|---|---|---|---|
| `docs/spec/01-product.md#что-делает-pri-fly` | `PROD-001` | `AC-003` | «Pri-Fly управляет проверяемым выполнением работы» | verified |
| `docs/spec/01-product.md#универсальность` | `PROD-002` | `AC-010`, `AC-029` | «Новый сценарий выражается существующими контрактами» | verified |
| `docs/spec/01-product.md#пустая-установка` | `PROD-003` | `AC-001` | «Пустое ядро остаётся допустимой установкой» | verified |
| `docs/spec/01-product.md#границы-ответственности` | `PROD-004` | `AC-012`, `AC-147` | «Ответственность частей продукта разделена» | verified |
| `docs/spec/01-product.md#что-не-входит-в-базовый-продукт` | `PROD-005` | `AC-012`, `AC-148` | «Базовый продукт не подменяет внешние системы» | verified |
| `docs/spec/01-product.md#пользовательские-роли` | `PROD-006` | `AC-006`, `AC-093`, `AC-145` | «Роли и полномочия не выводятся из текста» | verified |
| `docs/spec/01-product.md#матрица-применений` | `PROD-007` | `AC-011`, `AC-139` | «Одна модель поддерживает разные предметные сценарии» | verified |
| `docs/spec/01-product.md#цель-до-выполнения` | `PROD-008` | `AC-003`, `AC-132` | «Предмет и границы закреплены до выполнения» | verified |
| `docs/spec/01-product.md#изменение-намерения-пользователем` | `PROD-009` | `AC-004` | «Изменение намерения не продолжает прежний объём» | verified |
| `docs/spec/01-product.md#сценарий-и-план` | `PROD-010` | `AC-004`, `AC-005` | «Сценарий отделён от предметного плана» | verified |
| `docs/spec/01-product.md#режимы-взаимодействия-и-исполнения` | `PROD-011` | `AC-007`, `AC-009` | «Взаимодействие с человеком отделено от исполнения» | verified |
| `docs/spec/01-product.md#группы-и-параллельность` | `PROD-012` | `AC-042` | «Параллельная работа ограничена зависимостями и ресурсами» | verified |
| `docs/spec/01-product.md#успех-и-доказательства` | `PROD-013` | `AC-137` | «Результат и evidence имеют разные уровни достоверности» | verified |
| `docs/spec/01-product.md#проверки-соразмерны-задаче` | `PROD-014` | `AC-105` | «Проверки выбираются явно и не обходят integrity guards» | verified |
| `docs/spec/01-product.md#остановка-продолжение-и-восстановление` | `PROD-015` | `AC-100` | «Остановка и восстановление сохраняют границы работы» | verified |
| `docs/spec/01-product.md#выбор-пакетов` | `PROD-016` | `AC-025`, `AC-031` | «Packages выбираются явно и проверяются до запуска» | verified |
| `docs/spec/01-product.md#контроль-стоимости-и-качества-процесса` | `PROD-017` | `AC-122`, `AC-127` | «Стоимость и качество процесса наблюдаются честно» | verified |
| `docs/spec/01-product.md#предел-доказуемости` | `PROD-018` | `AC-016`, `AC-137`, `AC-144` | «Заявленная capability не равна qualification» | verified |
| `docs/spec/01-product.md#экспорт-и-независимость-владельца` | `PROD-019` | `AC-026`, `AC-129` | «Owner сохраняет переносимый доступ к работе» | verified |
| `docs/spec/01-product.md#критерии-целостности-продукта` | `PROD-020` | `AC-010`, `AC-028`, `AC-142` | «Независимость продукта проверяется обязательными конфигурациями» | verified |

## Risks / Trade-offs

- [Перефразирование изменит границу requirement] → reviewer сравнивает каждую
  строку crosswalk с source heading и назначенными acceptance cases.
- [Временная параллель двух документов станет второй правдой] → source map
  остаётся на legacy до завершённой проверки, а replacement spec не меняется
  отдельно от этого change.
- [Документальная проверка будет выдана за release evidence] → tasks явно
  разделяют OpenSpec validation и существующие product gates.

## Migration Plan

1. Проверить 20 legacy headings, их 20 crosswalk rows и acceptance case links.
2. Проверить strict OpenSpec validation для change и будущего main spec.
3. После content review зафиксировать `verified` для строк crosswalk и
   переключить ровно одну строку source map на `openspec/specs/product-model`.
4. Архивировать change; legacy глава и CSV-карты оставить неизменёнными до
   final release cleanup.

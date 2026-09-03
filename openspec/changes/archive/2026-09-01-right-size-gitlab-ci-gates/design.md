## Context

См. `proposal.md`. Native pipeline уже запускает только product gates, но
`make check` включает полный race по всему runtime и один такой job занял
28:24 на GitLab-hosted runner. Это делает автоматический gate непригодным для
Free quota.

## Goals / Non-Goals

**Goals:**

- Сохранить быстрый automatic feedback на изменениях продукта.
- Сохранить race как доступную, воспроизводимую и явно выбранную release
  проверку.
- Не запускать runner для README и прочей обычной документации.

**Non-Goals:**

- Не менять состав `make check`, runtime или public contract.
- Не вводить schedule, внешний runner, платный GitLab plan или новую test
  framework.

## Decisions

### Отдельный быстрый Make target

`ci-check` запускает уже существующие normal test, vet, format и schema
checks, но не race. CI вызывает этот target вместе с `make e2e`; его имя
делает отличие от полного `make check` видимым в trace.

Альтернатива — изменить `make check` — отклонена: локальный и выпускной
contract не должен молча потерять race coverage.

### Race — manual CI job

Job `race` доступен для тех же relevant changes и запускает ровно `make race`.
Он manual и optional для обычной ветки; RC/release owner явно запускает его и
сохраняет successful job как evidence полного gate.

Альтернатива — запускать race только на default branch — отклонена: это всё
равно тратит почти 30 минут на каждый merge и переносит позднюю ошибку после
merge.

## Risks / Trade-offs

- [Race bug дойдёт до обычного branch pipeline] → выпуск не принимается без
  отдельно запущенного `make race` и full `make check`.
- [Contributor примет manual job за автоматический gate] → разные имена jobs
  и явное правило в delivery roadmap отделяют их.

## Migration Plan

1. Добавить `ci-check` без изменения `check` или `race`.
2. Переключить automatic job и добавить manual `race` job с теми же rules.
3. Проверить Make target, e2e, CI lint и отсутствие изменений historical
   evidence; следующий готовый remote batch подтвердит native execution.

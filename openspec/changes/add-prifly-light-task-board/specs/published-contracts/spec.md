## ADDED Requirements

### Requirement: Light публикует отдельные read и mapping contracts

Light board/task/reviews/report responses и declarative projection mapping SHALL иметь новые closed versioned schema identities, canonical definitions, фактически emitted fixtures и воспроизводимые schema checks. Они SHALL явно различать source routing identity и authority identity, task/Run identities, coverage/freshness, cut/generation, candidate/round basis, gate/report scope и null unknowns. Cursor SHALL быть связан с query/source generations и доступом, не давать полномочий сам по себе. Invalid/unsupported mapping SHALL отказывать до применения; unsupported producer SHALL давать видимый unknown summary, а не исполняемый adapter.

`local-run-board/1`, queue contracts, прежние full-monitor APIs, Run state/read, StepResult и historical bundles MUST сохранять прежние bytes/meaning. Light observer config MUST NOT становиться новым обязательным полем workflow/project authoring или требовать Run/storage migration. Новые Light schema exports, readers, capability declaration, glossary и author index/guide SHALL поставляться вместе, с минимальной фактической Core release/contract revision. Structural validation SHALL быть отдельной от runtime/browser/released qualification.

Current source set — `openspec/specs/published-contracts/spec.md` и canonical JSON/Go artifacts; ownership не переносится. Новый Light contract не расширяет закрытую историческую форму.

#### Scenario: Реальный Light output и legacy reader
- **WHEN** новый Light reader выдаёт task/review page, а regression читает прежний saved Run и public bundle
- **THEN** Light output принимается собственным новым closed contract, а legacy bytes и emitted version/meaning сохраняются

#### Scenario: Нет данных и новая mapping version
- **WHEN** нет current candidate/round либо operator задаёт неподдерживаемую mapping version
- **THEN** первый случай выдаёт null/unknown с причиной в поддержанной read форме, второй отказывает при config validation без мутации или молчаливого расширения schema

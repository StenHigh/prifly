## ADDED Requirements

### Requirement: Репозиторий Pri-Fly не поставляет product workflows
`examples/` репозитория Pri-Fly MUST содержать только authoring references и
нейтральные примеры контракта; product workflow packages, включая AI Factory,
MUST жить во внешних Workflow repositories и попадать в проект через Workflow
catalog или `project workflows add`. Engine-гарантии — declared Workspace tree
bindings, package profiles, decision catalog, `exclude` и `settings` — MUST
проверяться нейтральными fixtures Pri-Fly, а контракт конкретного product
package — проверками его собственного repository против latest stable release
Pri-Fly. Изменение YAML authoring contract Pri-Fly MUST NOT считаться
совместимым с внешним package молча: его repository проверяет совместимость
сам и поднимает свою версию.

#### Scenario: Проекту нужен product workflow
- **WHEN** разработчик хочет сценарий AI Factory
- **THEN** он устанавливает его из каталога или repository, а в `examples/`
  Pri-Fly такой папки нет

#### Scenario: Engine feature меняется
- **WHEN** Pri-Fly меняет compile, profiles или decision catalog
- **THEN** регрессия ловится нейтральным fixture Pri-Fly, а внешний repository
  проверяет свой package против released Pri-Fly отдельно

## REMOVED Requirements

### Requirement: AI Factory examples separate classic and fanout workflows
**Reason**: Product packages AI Factory больше не входят в репозиторий Pri-Fly;
их состав и поведение — контракт собственного repository.
**Migration**: `https://github.com/StenHigh/prifly-aif-workflows` (папки
`aif-classic` и `aif-fanout`, tag `v1.0.0`) и записи `aif-classic`,
`aif-fanout` каталога `StenHigh/prifly-workflows`.

### Requirement: AI Factory classic preserves native Fast, Full and Ultra plans
**Reason**: Native plan layouts, adapter-before-upstream-skill и mapping
`commit_grouping` — свойства package `aif-classic`, а не движка; generic
контракт declared Workspace tree binding и runtime decisions остаётся в
требованиях «YAML authoring явно объявляет Workspace artifact tree transform»
и capability `run-decisions`.
**Migration**: те же папки в `https://github.com/StenHigh/prifly-aif-workflows`;
их compile-проверки (`tests/verify.py`) выполняются там против released
Pri-Fly.

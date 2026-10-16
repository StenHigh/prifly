## ADDED Requirements

### Requirement: Authority storage budget отделён от Run policy и других лимитов
`terms.md` SHALL определить Authority storage budget как конечный soft ceiling
allocated logical SQLite pages одной local authority, с owner-controlled
durable configuration и shared enforcement для всех её Run. Определение SHALL
отделять его от Run limits, ReadCut/aggregate scan bounds, максимального
snapshot, размера WAL и физического диска. Default observation и явный owner
update MUST различаться по provenance; setting не является Grant или
подтверждением достаточности среды.

Новые Go/JSON bindings SHALL добавляться вместе с implementation, schemas,
operator guide и проверками. Существующие canonical names/поля SHALL
сохраняться; `TestGlossaryBindings` MUST проверять новые объявленные mappings.
Единственный source set остаётся `specification-governance` по карте источников.

#### Scenario: Владелец меняет allowance authority
- **WHEN** reader смотрит словарь и storage status
- **THEN** он отличает настройку shared storage от полномочий и sealed Run policy, а actual usage не выдаётся за лимит или свободный диск

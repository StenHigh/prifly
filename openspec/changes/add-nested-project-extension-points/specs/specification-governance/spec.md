## ADDED Requirements

### Requirement: Workflow extension point has one canonical meaning
Glossary SHALL определять Workflow extension point как package-owned authoring declaration разрешённого места подключения и consumer ports. Оно MUST отличаться от runtime stage/operator, ArtifactRef transport и произвольного patch. Existing Workflow extension SHALL явно различать root legacy branch и opt-in point branch.

#### Scenario: Author chooses a nested hook
- **WHEN** author читает point declaration и marked project entry
- **THEN** fixed package authority, project implementation choice и explicit output provenance имеют однозначные термины

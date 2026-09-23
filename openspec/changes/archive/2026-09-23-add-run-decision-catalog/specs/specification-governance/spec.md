## ADDED Requirements

### Requirement: Словарь отделяет решения Run от authority primitives
Specification governance MUST define `DecisionCatalog`, `DecisionRequest`,
`DecisionAnswer`, `DecisionRecord`, attended Run, autonomous Run and the
restricted meaning of unattended Run. Definitions MUST state that none of
these is an Approval, Grant, ActionIntent or DecisionArtifact and MUST link to
the authoritative execution, runtime and UX requirements.

#### Scenario: Contributor добавляет automatic recommendation
- **WHEN** он обновляет contract или example с automatic decision
- **THEN** glossary prevents calling recommendation an approval or treating
  it as permission for an external effect


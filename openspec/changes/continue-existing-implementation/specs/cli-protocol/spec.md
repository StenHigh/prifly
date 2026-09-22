## ADDED Requirements

### Requirement: Project CLI готовит continuation без ручного чтения state
Project CLI MUST предоставить explicit continuation entry point с source Run ID
и declared launch ID. До mutation он MUST прочитать exact source artifacts,
проверить актуальную committed Implementation и вернуть reviewable launch
summary с source provenance, workspace, package, inputs и ожидаемыми stages.
Host MUST не извлекать ArtifactRef из `run status` JSON вручную и MUST не
использовать raw `run fork` как замену continuation. CLI MAY использовать
runtime fork internally only after validating the declared source artifacts.

#### Scenario: Подготовка continuation успешна

- **WHEN** пользователь называет eligible source Run и чистый commit в
  выбранном workspace
- **THEN** prepare возвращает exact source refs и review digest, а start с
  этим digest создаёт новый continuation Run

#### Scenario: Source Run не подходит

- **WHEN** source Run не terminal, принадлежит другой authority либо не
  содержит declared continuation artifacts
- **THEN** CLI отказывает до package import, claim и Run creation и называет
  безопасное следующее действие

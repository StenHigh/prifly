## ADDED Requirements

### Requirement: Project YAML authoring имеет независимый corpus
Repository MUST хранить author-visible positive и negative `.prifly` YAML
fixtures вне Go unit tests. Independent verifier MUST копировать каждый case,
инициализировать только local authority когда это необходимо, и проверять
единственный public Project route через `project workflows` или `project
compile`. Corpus MUST подтверждать accepted workflow folder и отказ profile v1,
`task_recipe`, flat package file и unmarked direct workflow. Проверка MUST не
требовать сеть, AI Factory, credentials, package import или execution worker.

#### Scenario: Допустимая YAML folder проверяется вне unit tests
- **WHEN** external verifier получает accepted fixture
- **THEN** `project workflows` объявляет declared inputs, а `project compile`
  создаёт sealed package без authority mutation

#### Scenario: Legacy source попадает в corpus
- **WHEN** external verifier получает каждую legacy fixture
- **THEN** public CLI отказывает с понятной diagnostic до output package или
  Run

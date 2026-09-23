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

#### Scenario: Реализация сохранена только в незамерженной ветке

- **WHEN** владелец передаёт полный commit ID реализации через
  `--implementation-head`, доступный в Git repository, а этот commit содержит
  source Implementation и выбирается `worktree` mode
- **THEN** prepare проверяет ancestry и включает выбранный HEAD в review
  digest, а start создаёт чистый отдельный claim-worktree от того же commit;
  текущую ветку repository и source Run он не меняет
- **AND** неизвестный commit, сокращённый SHA или `checkout` mode отказывает
  до создания claim и Run

#### Scenario: Уже есть активное продолжение того же Run

- **WHEN** от source Run уже запущен continuation, который ещё не завершён
- **THEN** prepare и start отказывают с кодом `project_continue_active_child`,
  показывая ID и статус дочернего Run без создания нового claim или Run
- **AND** явный `--allow-duplicate-continuation` на prepare и start разрешает
  независимый повторный запуск; этот выбор входит в review digest
- **AND** continuation от другого source Run не препятствует запуску

#### Scenario: Source Run не подходит

- **WHEN** source Run не terminal, принадлежит другой authority либо не
  содержит declared continuation artifacts
- **THEN** CLI отказывает до package import, claim и Run creation и называет
  безопасное следующее действие

## ADDED Requirements

### Requirement: Project CLI готовит continuation без ручного чтения state
Project CLI MUST предоставить explicit continuation entry point с source Run
ID и declared launch ID, чей workflow объявил продолжение. До mutation он MUST
прочитать объявленные source artifacts и вернуть reviewable launch summary с
source provenance, источником каждого перенесённого входа, передаваемым
деревом source Run, package и inputs. Host MUST не извлекать ArtifactRef из
`run status` JSON вручную и MUST не использовать raw `run fork` как замену
continuation. CLI MUST NOT вычислять входы из Git за workflow.

#### Scenario: Подготовка continuation успешна
- **WHEN** пользователь называет eligible source Run и launch с объявленным
  продолжением
- **THEN** prepare возвращает exact source refs и review digest, а start с
  этим digest создаёт новый continuation Run

#### Scenario: Работа сохранена вне дерева source Run
- **WHEN** владелец передаёт полный commit ID через `--workspace-commit`,
  доступный в Git repository, и выбирает `worktree` mode
- **THEN** prepare включает выбранный commit в review digest, а start создаёт
  отдельный claim-worktree от него; дерево source Run остаётся за ним
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
- **WHEN** source Run не terminal, принадлежит другой authority, его workflow
  или исход не объявлены продолжением либо он не содержит объявленных
  artifacts
- **THEN** CLI отказывает до package import, claim и Run creation и называет
  безопасное следующее действие

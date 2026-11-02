## ADDED Requirements

### Requirement: Project owns typed worktree naming inputs outside the workflow

Project profile `/5` SHALL разрешать `worktree_naming.inputs`: именованные JSON inputs с `schema` — относительным путём к локальной JSON Schema проекта. Схемы MUST находиться внутри `.prifly/`, вне upstream-owned файлов. Reader MUST отвергать symlinks, выход из repository и сетевые schema references. В первой редакции допускаются самодостаточные JSON Schema 2020-12 с локальными fragment references. Profile SHALL допускать не более 16 naming inputs; каждый input MUST использоваться хотя бы одним binding. Values и schemas SHALL соблюдать опубликованные ограничения размера JSON input.

Binding SHALL содержать ровно один вариант: `value`, `input` с `pointer` либо `naming_input` с `pointer`. `input` SHALL сохранять значение входа workflow. `naming_input` SHALL адресовать только именованный input из `worktree_naming.inputs`. Одинаковые имена в этих пространствах SHALL оставаться различимыми.

Для нового worktree каждый объявленный naming input MUST иметь явно переданное JSON value. Defaults, tracker lookup и извлечение ID из текста MUST NOT применяться. Значение MUST пройти объявленную schema до JSON Pointer и существующей проверки безопасного имени. Неизвестные inputs, лишние поля declarations и смешанные binding forms MUST отказываться до claim.

Naming inputs MUST NOT добавляться в WorkflowRevision inputs, task schema, step inputs, compiled package или native graph. Установка и обновление upstream package MUST сохранять project profile и собственные naming schemas потребителя. Compile без naming values SHALL оставаться возможным.

Source set остаётся `openspec/specs/workflow-and-context/spec.md`; ownership не переносится. Profiles `/2`–`/4`, их bindings и compiled identities сохраняют прежнее значение.

#### Scenario: Official workflow retains its closed task schema
- **WHEN** проект запускает неизменённый официальный `aif-classic` с прежним task и отдельным naming input `{source: tracker, id: issue-19}`
- **THEN** Core вычисляет имя по project binding; upstream files, task schema и graph остаются неизменными

#### Scenario: Workflow and naming inputs have the same name
- **WHEN** workflow input и naming input называются `identity`, а binding явно выбирает `naming_input: identity`
- **THEN** Core читает отдельное naming value и не подменяет workflow input

#### Scenario: Invalid metadata is rejected before a claim
- **WHEN** naming value отсутствует, нарушает schema, даёт неверный pointer либо недопустимый компонент имени
- **THEN** prepare/start возвращает typed refusal до создания claim, worktree, Run или dispatch

#### Scenario: Package update retains consumer configuration
- **WHEN** `project workflows update` обновляет upstream package, а naming declarations и schemas находятся в project-owned файлах
- **THEN** update сохраняет consumer bytes и проверяет upstream inventory обычным способом

#### Scenario: Naming does not apply to the selected launch
- **WHEN** запуск использует checkout либо вообще не требует Git Workspace
- **THEN** Core не требует naming values; структурная проверка profile сохраняется, явно переданные неиспользуемые naming values отказываются

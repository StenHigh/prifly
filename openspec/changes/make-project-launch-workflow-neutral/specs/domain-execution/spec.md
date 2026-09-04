## MODIFIED Requirements

### Requirement: Области исполнения имеют независимую идентичность
Installation, Project, Workspace, Authority и Principal MUST оставаться
разными сущностями. Их roots MUST задаваться независимо, а physical resource
MUST не получать нескольких authority owners из-за разных отображаемых имён.
Remote identity MUST включать provider, account, namespace и resource.

Versioned Project profile MUST храниться в папке проекта отдельно от authority.
Для `/3` эта папка MUST NOT требовать Git. `project init` MUST создавать
profile, отдельную local authority и local configuration с exact Pri-Fly path;
`prifly-run` создаётся только для выбранных hosts. Если skill используется,
он MUST выбирать launch до чтения его inputs. Local state, receipts, artifacts
и claimed worktree MUST оставаться вне папки проекта, а local configuration
MUST быть исключена из Git при его использовании. Raw `--project` MUST означать
authority root, а не каталог profile. `/2` сохраняет прежний Git/host layout.

#### Scenario: Один каталог назван двумя проектами
- **WHEN** два profiles указывают на один physical workspace
- **THEN** система распознаёт общий resource identity без независимых owners

#### Scenario: Project не использует Git
- **WHEN** `/3` profile находится в обычной папке
- **THEN** authority и history остаются вне неё без создания Git или AI scaffolding

### Requirement: Run закрепляет полный состав исполнения
До исполнения Run MUST lock-ить declared input snapshots, workflow closure,
steps, tools, adapters, schemas, instructions, contexts, configuration, checks
и policy revisions с exact refs. Task-driven Run MUST также закрепить RunBrief;
новый neutral contract MUST сохранять его отсутствие, когда он не объявлен,
без фиктивного `brief_ref`. Старые state/read versions сохраняют обязательный
brief и прежние bytes. Mutable alias разрешается до lock; поздняя правка
package/config/settings MUST не менять активный Run. Недоступные pinned bytes
MUST давать `pinned_resource_unavailable`, не latest version.

Required configuration MUST быть valid до RunStart; optional absence остаётся
отсутствием без hidden default. Repeat MUST lock-ить body closure и независимо
применять initial/next bindings. Emergency deny и revocation проверяются по
текущему состоянию перед эффектом.

#### Scenario: Автор меняет workflow после запуска
- **WHEN** installed workflow изменяется во время активного Run
- **THEN** он продолжает использовать прежние pinned bytes

#### Scenario: Новый Run без brief прочитан после restart
- **WHEN** neutral contract не объявлял RunBrief
- **THEN** compatible reader сохраняет отсутствие brief, а старый reader
  отказывает unsupported version, не реконструирует вымышленный документ

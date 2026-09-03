## ADDED Requirements

### Requirement: CLI управляет Project workflow folders из репозиториев явными командами
`prifly project workflows` без аргументов MUST по-прежнему перечислять
declared launches. Дополнительно CLI MUST предоставлять
`search [QUERY] [--category ID] [--catalog URL]`,
`add SOURCE [--ref REF] [--path DIR] [--name NAME] [--catalog URL]`,
`update NAME [--ref REF]` и `remove NAME`; `add`, `update` и `remove`
принимают общий `--repository DIR`, `search` не требует repository, и ни одна
из них не требует `--project` и не открывает authority. `SOURCE` MUST толковаться
механически: имя без `/` и `:` — запись каталога; `owner/repo` — GitHub HTTPS
repository; иначе Git URL или абсолютный локальный путь. Опущенный
`--catalog` MUST использовать встроенный официальный каталог
`https://github.com/StenHigh/prifly-workflows.git`; явный `--catalog URL`
переопределяет его для одной команды и MUST проходить те же проверки, что и
`SOURCE`. Сеть MAY выполняться только во время `search`, `add` и
`update`; `init`, `workflows`, `questionnaire`, `compile` и `start` MUST NOT
её использовать. Результаты MUST быть typed JSON с `schema_version`
`project-workflow-catalog/1`, `project-workflow-add/1`,
`project-workflow-update/1` и `project-workflow-remove/1`. Неверные
аргументы MUST давать `invalid_usage` до сети; отказы MUST использовать
stable codes, среди них `project_workflow_source_invalid`,
`project_workflow_repository_unreachable`,
`project_workflow_repository_empty`,
`project_workflow_repository_ambiguous`, `project_workflow_exists`,
`project_workflow_package_conflict`, `project_workflow_origin_missing`,
`project_workflow_modified`, `project_workflow_commit_mismatch`,
`project_workflow_catalog_invalid` и `project_workflow_catalog_entry_unknown`.

#### Scenario: Сценарий установлен по имени каталога
- **WHEN** пользователь вызывает `add NAME --catalog URL`
- **THEN** ответ `project-workflow-add/1` называет package identity, origin и
  launch, а authority не открывается

#### Scenario: Repository неоднозначен
- **WHEN** repository содержит несколько сценариев и `--path` не задан
- **THEN** CLI возвращает `project_workflow_repository_ambiguous` с перечнем
  путей и не меняет `.prifly/`

#### Scenario: Неверный SOURCE
- **WHEN** пользователь передаёт относительный путь, URL с credentials или
  аргумент с ведущим `-`
- **THEN** CLI возвращает stable diagnostic до любой сетевой операции

### Requirement: Host runner предлагает поиск и установку сценария одним вопросом
Runner `prifly-run` MUST содержать инструкции: по явной просьбе разработчика
найти или установить сценарий выполнить `project workflows search --json`,
показать категории и записи одним native finite вопросом, после выбора
вызвать `project workflows add NAME` и предложить разработчику проверить и
закоммитить изменения `.prifly`. Runner MUST NOT устанавливать сценарий без
явного выбора и MUST NOT начинать Run как часть установки.
`project runners update` MUST распознавать прежний exact runner и заменять
его; кастомизированный runner по-прежнему отказывается.

#### Scenario: Разработчик просит установить сценарий
- **WHEN** host получает просьбу найти или установить workflow
- **THEN** он показывает список из `search --json` одним вопросом и вызывает
  `add` только для выбранной записи

#### Scenario: Runner обновлён после изменения
- **WHEN** repository содержит exact runner предыдущей версии
- **THEN** `project runners update` заменяет его новым, не трогая
  кастомизированные файлы

## ADDED Requirements

### Requirement: Project YAML объявляет решения без скрытого control flow
Project workflow authoring source MUST поддерживать один декларативный каталог
решений и readable tree для его крупных записей. Каждая запись MUST иметь
stable ID и полную машинно валидируемую форму; optional compact YAML form MAY
опускать только безопасные presentation defaults. Решение может supply только
явно объявленный launch/configuration/step input и MUST NOT добавлять stage,
route, capability, effect или правомочие.

#### Scenario: Большой package раскладывает решения по файлам
- **WHEN** author помещает допустимые YAML decision declarations в разрешённое
  дерево package
- **THEN** compiler читает их как один catalog, а пути улучшают навигацию, но
  не меняют graph или semantic meaning

#### Scenario: Решение пытается включить optional feature
- **WHEN** catalog answer прямо изменяет feature, route или capability, не
  будучи declared configuration input с обычной validation
- **THEN** compiler отказывает до sealing

### Requirement: Условный вопрос зависит только от sealed предшественника
Project YAML MAY ограничить видимость решения выбранным package profile и
exact typed answer previously declared preflight decision. Compiler MUST
reject unknown, forward or cyclic predecessor references before sealing. Such
a predicate MUST NOT add a route, stage, effect, capability or permission.

#### Scenario: Roadmap milestone появляется после linkage
- **WHEN** `roadmap_milestone` declares exact predecessor answer
  `roadmap_linkage = "link"`
- **THEN** host presents it only after that answer and records both values in
  the same immutable Decision Sheet

### Requirement: Project default не маскирует выбор запуска
Project configuration MUST различать reviewed package default и explicit
per-Run launch value. Default MAY заполнить omitted optional launch choice
только по rule sealed package, но host/CLI selection для одного Run MUST иметь
приоритет без записи обратно в tracked authoring files.

#### Scenario: Interactive host выбирает другой profile
- **WHEN** host получает omitted package profile и пользователь выбирает
  допустимый не-default profile
- **THEN** новый Run использует выбор пользователя, а `extend.yaml` остаётся
  byte-for-byte прежним

### Requirement: Package adapter явно ограничивает AIF runtime bridge
`aif-classic` MUST place its package-owned adapter before the exact pinned
upstream skill as primary and supporting instruction contexts. Adapter MAY map
only an inventoried upstream question to one declared runtime decision; all
other native dialogs MUST remain ordinary attended interaction. Its Commit Plan
grouping mapping MUST use declared `commit_grouping`; generated commit text and
push MUST NOT be presented as generic Pri-Fly decisions.

#### Scenario: Commit Plan grouping ждёт тот же Attempt
- **WHEN** pinned `aif-commit` reaches its Commit Plan grouping question
- **THEN** adapter emits the universal declared request, and its typed answer
  reaches only the redelivered same Attempt

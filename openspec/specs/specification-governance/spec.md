# specification-governance Specification

## Purpose
Определяет единый, поэтапный и проверяемый способ развивать нормативную
спецификацию Pri-Fly с помощью OpenSpec, не создавая конкурирующих источников
правды и не меняя историю принятых доказательств.

## Requirements

### Requirement: Новые изменения спецификации ведутся через OpenSpec
Репозиторий SHALL хранить OpenSpec configuration, capability specifications и
историю изменений в version control. Новое изменение нормативной семантики
Pri-Fly MUST иметь OpenSpec change с proposal, requirements, design и tasks до
его применения. OpenSpec SHALL применяться только к документации и процессу
разработки Pri-Fly, а не к runtime, authoring YAML или формату package.

#### Scenario: Агент начинает новое изменение спецификации
- **WHEN** разработчик или ИИ-агент предлагает изменить нормативное поведение
- **THEN** он создаёт или продолжает именованный OpenSpec change до изменения
  соответствующего источника спецификации

### Requirement: У capability ровно один текущий источник правды
Для каждой capability карта миграции MUST указывать единственный текущий
нормативный source set. Source set может состоять из явно перечисленных
взаимосвязанных файлов, но не допускает второго независимо редактируемого
описания того же смысла. До явного переноса capability существующий source set
остаётся источником правды; OpenSpec change не SHALL дублировать его полное
содержание. После переноса OpenSpec specification становится источником
правды, а старый читательский документ MUST ссылаться на неё или собираться из
неё.

#### Scenario: Capability ещё не перенесена
- **WHEN** изменение касается capability без записи о завершённом переносе
- **THEN** автор меняет её существующий нормативный source set и фиксирует
  будущий перенос отдельным OpenSpec change

#### Scenario: Capability перенесена
- **WHEN** карта миграции отмечает capability как перенесённую
- **THEN** автор меняет её OpenSpec specification, а не создаёт независимую
  вторую версию в старом документе

### Requirement: Исторические evidence сохраняют исходное содержание
OpenSpec migration MUST не переписывать historical release evidence и
manifests ради согласования с новой структурой. Пока legacy source set
существует, исторические файлы остаются проверяемой записью прошлого среза.
После полного replacement в OpenSpec отдельный final cleanup change MAY удалить
legacy files из release tree, сохранив их неизменённые bytes в Git history. Он
MUST сначала подтвердить, что карта источников указывает только migrated
OpenSpec sources и что relevant OpenSpec checks прошли.

#### Scenario: Перенос затрагивает материал с исторической ссылкой
- **WHEN** новый документ должен сослаться на материал, который присутствует в
  historical evidence до полного cutover
- **THEN** новая документация указывает актуальный путь без изменения
  исторического файла или его сохранённого отпечатка

#### Scenario: Финальный release cleanup удаляет historical evidence
- **WHEN** все replacement specifications существуют и final cleanup change
  готовится к первому релизу
- **THEN** он удаляет historical evidence без изменения его содержания в
  предыдущих commits и без изменения product semantics

### Requirement: Вход в спецификацию понятен новому участнику
README, brief для агентов и правила репозитория MUST объяснять назначение
OpenSpec, текущий источник правды до миграции capability и ссылку на roadmap.
Они MUST явно сообщать, что OpenSpec не является функцией поставляемого
Pri-Fly. После завершённого cutover entry-point documents MUST указывать
`openspec/SOURCE-OF-TRUTH.md`, а не удалённый legacy `docs/` путь.

#### Scenario: Новый участник открывает репозиторий
- **WHEN** участник читает стартовые документы
- **THEN** он видит, где начать OpenSpec change и какие действующие документы
  читать до завершения поэтапного переноса

#### Scenario: Новый участник открывает репозиторий после cutover
- **WHEN** участник читает README или repository instructions
- **THEN** он видит, где найти карту источников, выбрать capability и начать
  OpenSpec change без ссылки на legacy documentation source

### Requirement: Каноническая терминология имеет один текущий словарь
Capability `specification-governance` MUST хранить действующий словарь в
`openspec/specs/specification-governance/terms.md`. Словарь определяет
канонические понятия Pri-Fly, их границы и перечисленные Go/JSON соответствия;
он не меняет wire-схему, сохранённые данные или фактический статус roadmap.
Словарь и `spec.md` составляют один явно названный source set этой capability,
а не независимо редактируемые конкурирующие документы. Описание Project
execution profile MUST называть host-specific skills roots нейтральной
настройкой проекта, а не выдавать directory одного agent host за обязательный
путь Pri-Fly. Новые WorkspaceTreeManifest и declared Workspace tree binding
MUST получить канонические определения, границу относительно ArtifactRef и
Workspace и Go/JSON соответствия только вместе с реализацией. Новые Workflow
repository, Workflow catalog и Workflow folder origin MUST получить
канонические определения, границы относительно Registry, SealedPackage,
TrustDecision и SourceSnapshot и Go/JSON соответствия только вместе с
реализацией; слово «catalog» пишется как в `DecisionCatalog`.

#### Scenario: Участник ищет значение понятия
- **WHEN** участник встречает термин Pri-Fly или хочет добавить сущность
- **THEN** он находит единственное каноническое определение в `terms.md` и
  уточняет предметную семантику в указанной capability specification

### Requirement: Изменение термина сохраняет совместимость и проверяемость
Изменение значения или имени понятия MUST в одном OpenSpec change обновить
словарь, затронутую capability specification, код, schemas, примеры и tests.
Сохранённые JSON fields и смысл уже закреплённых Runs MUST не меняться только
ради стилистики. `TestGlossaryBindings` MUST продолжать сверять явно
перечисленные Go/JSON соответствия со словарём; его зелёный результат не
заменяет смысловое review и не требует создавать тип ради заполнения карты.

#### Scenario: Переименование затрагивает сохранённый контракт
- **WHEN** автор предлагает изменить термин, связанный с Go или JSON именем
- **THEN** change содержит решение о совместимости и обновляет все
затронутые текущие источники, либо оставляет сохранённое wire-имя явным
соответствием без ложного переименования

### Requirement: Утверждения о продукте опираются на проверенный факт
Contributor и agent MUST не выдавать непроверенную capability, qualification,
значение или результат gate за подтверждённый. Неизвестный исход MUST
оставаться неизвестным, а название test MUST соответствовать тому, что он
действительно воспроизводит. Пройденная document validation сама по себе не
закрывает roadmap phase или product gate.

#### Scenario: Проверка не воспроизводит обещанное поведение
- **WHEN** author или reviewer обнаруживает, что test не доказывает своё
  заявленное утверждение
- **THEN** он исправляет test или его название и явно называет непокрытую
  границу вместо объявления capability принятой

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

### Requirement: Рабочие решения и архитектурные решения имеют разный статус
Исторический журнал автономных рабочих развилок MUST храниться только как
архив migration, а не как второй действующий нормативный источник.
Долговременное архитектурное решение MUST жить в capability
`architecture-decisions` и изменяться отдельным OpenSpec change. Новая
рабочая развилка, не меняющая архитектурный контракт, MUST фиксировать
контекст, выбранный путь, отвергнутую альтернативу и условие пересмотра в
соответствующем active change до реализации.

#### Scenario: Решение меняет архитектурный контракт
- **WHEN** команда принимает решение, которое меняет устойчивую архитектурную
  границу Pri-Fly
- **THEN** оно оформляется в `architecture-decisions`, а не добавляется в
  рабочий журнал как действующее правило

### Requirement: Authority storage budget отделён от Run policy и других лимитов
`terms.md` SHALL определить Authority storage budget как конечный soft ceiling
allocated logical SQLite pages одной local authority, с owner-controlled
durable configuration и shared enforcement для всех её Run. Определение SHALL
отделять его от Run limits, ReadCut/aggregate scan bounds, максимального
snapshot, размера WAL и физического диска. Default observation и явный owner
update MUST различаться по provenance; setting не является Grant или
подтверждением достаточности среды.

Новые Go/JSON bindings SHALL добавляться вместе с implementation, schemas,
operator guide и проверками. Существующие canonical names/поля SHALL
сохраняться; `TestGlossaryBindings` MUST проверять новые объявленные mappings.
Единственный source set остаётся `specification-governance` по карте источников.

#### Scenario: Владелец меняет allowance authority
- **WHEN** reader смотрит словарь и storage status
- **THEN** он отличает настройку shared storage от полномочий и sealed Run policy, а actual usage не выдаётся за лимит или свободный диск

### Requirement: Workflow extension охватывает step и native Call без смешения identities
Source set `specification-governance` SHALL сохранять единственное определение
Workflow extension в `terms.md` и ссылаться на `workflow-and-context` для точного
authoring contract. Определение SHALL описывать единичную compile-time вставку
объявленного Project step либо native Call в exact root route; legacy step
binding limit MUST отличаться от multiple bindings новой call ветви.

Call target SHALL называться WorkflowRevision, его runtime включение —
WorkflowInvocation внутри того же Run. Extension ID SHALL отличаться от
callee identity; прежний step stage ID MUST сохраняться. Compiler MUST NOT
вводить новую предметную сущность или Go тип только ради названия ветви.
Изменение определения SHALL сопровождаться затронутыми contract artifacts,
примерами и tests; опубликованные JSON fields и meaning pinned Runs MUST
сохраняться. Существующие Go/JSON bindings SHALL проверяться
`TestGlossaryBindings`; source ownership не переносится.

#### Scenario: Автор читает словарь перед новой вставкой
- **WHEN** автор выбирает новую call ветвь Workflow extension
- **THEN** словарь отличает authoring запись, exact target и runtime invocation и ведёт к единственному актуальному контракту без обещания независимого Run или рекурсивного patch

#### Scenario: Legacy step читается новым бинарником
- **WHEN** прежняя step extension и сохранённый Run читаются после расширения определения
- **THEN** имена и смысл существующих fields/identities остаются прежними, а новая call возможность не переинтерпретирует старую запись

### Requirement: Читаемое имя Workspace не подменяет идентичность claim

Словарь SHALL уточнять существующие Project execution profile, Project questionnaire / Launch summary и WorktreeClaim / Workspace mode: профиль объявляет optional naming policy, prepare показывает незарезервированный кандидат, claim хранит фактические path/branch. Claim ID SHALL оставаться служебной идентичностью для ownership, generation, идемпотентности и привязки Run; basename MUST NOT использоваться как новый task ID или Run identity.

Уточнение SHALL сохранять значения и JSON bindings исторических claims/Runs. Ограничение независимых claims SHALL соответствовать текущей physical conflict relation worktree/checkout, без возврата к одному claim на repository. Новые Go/JSON bindings SHALL добавляться только после реализации; смысловое ревью и `TestGlossaryBindings` SHALL сопровождать изменение карты.

Текущий authoritative source set — `openspec/specs/specification-governance/spec.md`, `terms.md`, `agent-brief.md`; ownership не переносится. Эти уточнения выполняются вместе с реализацией, без переписывания historical evidence.

#### Scenario: Имя совпадает с названием задачи
- **WHEN** два независимых claim имеют основы для одной задачи и разные suffix
- **THEN** у каждого собственный claim ID и привязка Run, а общее читаемое имя не объединяет ownership или идемпотентность

### Requirement: Unified source composition retains canonical terms
Glossary SHALL уточнять Workflow composition, Workflow extension, Workflow extension point, Component export, Workflow layer и Workflow contribution для `/5`. Числа markers MUST описываться как версии конкретных документов, а не как линейная замена всех capabilities.

Compiled workflow variant SHALL означать immutable compiler result одного source workflow с определённой point policy и settings. Он MUST отличаться от source author identity, runtime WorkflowInvocation и отдельного Run. Реальные Go/JSON bindings SHALL добавляться только вместе с соответствующими types и проверками.

#### Scenario: One source produces two compiled variants
- **WHEN** author сравнивает source identity и exact compiled refs в review
- **THEN** glossary однозначно объясняет разные versions без нового runtime объекта или переименования saved JSON fields

### Requirement: Unified change owns only its new versioned delta
Main source sets SHALL сохранять ownership по `SOURCE-OF-TRUTH.md`. Новый active delta SHALL определять `/5`, folder `/3`, b2 и новые DTO. Прежние active changes SHALL сохранять baseline `/3` и `/4` до своего archive/sync.

Реализация SHALL обновлять source-of-truth map, glossary, agent brief и author entry points в одном change. Historical release evidence, archived changes и старые schemas MUST NOT переписываться как доказательство новой поддержки. Документная проверка MUST NOT закрывать runtime или продуктовый gate.

#### Scenario: Unified support is documented before implementation
- **WHEN** proposal и schemas design прошли OpenSpec validation
- **THEN** capability остаётся проектируемой; `/4` published qualification и будущий `/5` release evidence не смешиваются

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

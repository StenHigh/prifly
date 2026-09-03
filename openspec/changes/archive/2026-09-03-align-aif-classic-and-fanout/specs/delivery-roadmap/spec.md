## MODIFIED Requirements

### Requirement: Release candidate имеет явные scope и exclusions
Каждый RC SHALL называть target profile, обязательные gates, known gaps,
deferred work и rollback boundary. Qualified narrow RC MUST NOT становиться
заявлением о whole-product release, managed execution или external-provider
qualification.

Текущий RC profile — `core-local` AI Factory с человеком в контуре. Его
проверенный путь: из чистого проекта установить trusted package, подготовить
readable YAML workflow, запустить planning cycle и сохранить result. В
`aif-classic` improve читает исправленный plan в следующей iteration, review
следует только за выполненной работой, а blocking verify/security/review
предлагает `/aif-fix` без автоматического исправления. Отдельный `aif-fanout`
показывает declared parallel profiles, но не квалифицирует выбор provider,
model или reasoning level.

На 2026-09-01 закрыты physical suspend/resume gate и финальный reproducible
gate этого exact candidate. Это квалифицирует только local owner-host путь.
В RC намеренно отсутствуют daemon wakeup, automatic external action/retry/
reconcile, throughput promise, retention/GC, backup/restore RPO/RTO, managed
isolation, remote trust boundary, provider cost/usage qualification и
assisted-host model-profile selection.

#### Scenario: Core-local RC представлен пользователю
- **WHEN** reader открывает current RC boundary
- **THEN** он видит supported local scope, qualification boundary и отдельно
  перечисленные unsupported integrations и future gates

## ADDED Requirements

### Requirement: Выбор model profile остаётся отдельной high-priority задачей
После разделения `aif-classic` и `aif-fanout` delivery roadmap SHALL хранить
отдельную high-priority задачу `assisted-model-profile-protocol`. Она должна
изменить versioned contract assisted host до того, как Pri-Fly заявит реальный
выбор provider, model или reasoning level. `aif-fanout` является её будущим
acceptance fixture, но его успешная compilation не является evidence такого
выбора.

#### Scenario: Веер компилируется до нового host protocol
- **WHEN** `aif-fanout` успешно проходит authoring checks
- **THEN** roadmap и report называют это проверкой graph/profile data, а не
  доказательством фактического model selection

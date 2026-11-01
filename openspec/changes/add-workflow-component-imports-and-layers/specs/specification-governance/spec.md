## ADDED Requirements

### Requirement: Понятия композиции описывают authoring и не создают runtime-наследование
Словарь SHALL определить export, import компонента, базовый workflow, layer и contribution расширения вместе с реализацией. Export SHALL означать объявленную ссылку на PackageComponent. Import SHALL связывать scoped имя с exact зависимостью. Layer SHALL означать упорядоченный authoring-документ.

Base SHALL означать общий закреплённый источник root graph. Parent SHALL означать непосредственно выбранный производный workflow. Contribution SHALL означать добавление с source provenance, входами/исходами и точкой подключения. Ромб наследования SHALL означать достижение общего ancestor через нескольких родителей; он MUST NOT означать повторное выполнение общего маршрута.

Новые понятия MUST NOT становиться синонимами WorkflowInvocation, Run, Registry, TrustDecision или runtime operator. Уточнение Workflow extension SHALL сохранять legacy `/2` scope и явно описывать `/3`. Новые Go/JSON bindings SHALL проверяться `TestGlossaryBindings`.

#### Scenario: Читатель ищет смысл слоя
- **WHEN** читатель открывает glossary по ссылке authoring reference
- **THEN** он видит порядок применения, compile-time границу и отличие от runtime исполнения

### Requirement: Core composition и downstream UI/UX имеют отдельную приёмку
Current source sets SHALL оставаться указанными в `openspec/SOURCE-OF-TRUTH.md`; этот change не переносит ownership. Его deltas SHALL задавать opt-in дополнения только после применения. Historical evidence и прежние release manifests MUST сохранять bytes.

Core acceptance SHALL доказывать generic imports/layers и native control. Изменение пакета SHALL иметь отдельный release после Core. Подключение smsplace SHALL иметь отдельный reviewed pilot после package release. Завершение любого раннего этапа MUST NOT закрывать последующие этапы.

#### Scenario: Core fixture выполнил required и skip routes
- **WHEN** generic synthetic fixture прошла на released Core
- **THEN** evidence не объявляет опубликованным UI/UX export или успешным пилот smsplace

## Purpose

Позволяет начать новый Run, который продолжает работу завершённого Run так, как
объявил workflow продолжения, не переписывая завершённый Run и не повторяя
стадии, которые уже выполнили работу.

## ADDED Requirements

### Requirement: Continuation создаёт новый Run из объявленных источников
Project continuation MUST создать новый Run, связанный с exact source Run, и
MUST не изменять source Run, его outcome, evidence или sealed graph. Каждый
вход, который workflow продолжения объявил заполняемым из source Run, MUST
приниматься только как exact sealed ArtifactRevision из объявленного места:
вход source Run, принятый выход стадии его корневого вызова или последний
принятый checkpoint. Недоступный source artifact или отсутствующий объявленный
выход MUST отказать до создания claim и Run.

#### Scenario: Работа доделана после terminal partial
- **WHEN** source Run завершён `partial`, объявленные выходы доступны, а
  workflow продолжения объявил продолжение от этого workflow и исхода
- **THEN** continuation создаёт новый Run с отдельной identity и provenance
  source Run, получает объявленные входы и не меняет source Run

#### Scenario: Ранее удалённая версия пакета снова объявлена проектом
- **WHEN** Project launcher восстанавливает exact версию пакета из `removed`
- **THEN** тот же runtime process видит восстановленную версию при Start;
  отказ восстановления не маскируется последующим `missing_ref`

### Requirement: Continuation начинает с entry своего workflow
Continuation MUST начинать с entry workflow продолжения и MUST не выдавать
Attempt стадий, которых этот workflow не объявил. Проверку того, что рабочее
дерево соответствует checkpoint или иному объявленному условию, workflow
продолжения выражает своими шагами; Core не выполняет её за workflow.

#### Scenario: Работа уже выполнена
- **WHEN** continuation получила объявленные входы
- **THEN** первым выдаваемым рабочим шагом является entry stage workflow
  продолжения, а стадии исходного workflow не выдаются

#### Scenario: Gate находит исправимое препятствие
- **WHEN** gate continuation возвращает вердикт, для которого автор объявил
  маршрут исправления
- **THEN** этот маршрут исполняется в новом Run в пределах объявленных лимитов

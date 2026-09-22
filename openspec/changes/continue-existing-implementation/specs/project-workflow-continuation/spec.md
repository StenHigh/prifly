## Purpose

Позволяет начать новый проверочный Run от точной существующей реализации, не
переписывая завершённый Run и не повторяя стадии, которые уже выполнили работу.

## ADDED Requirements

### Requirement: Continuation создаёт новый Run с проверяемой реализацией
Project continuation MUST создать новый Run, связанный с exact source Run,
и MUST не изменять source Run, его outcome, evidence или sealed graph.
Continuation MUST принять source task, handoff и plan только как exact sealed
ArtifactRevision, а Implementation MUST описывать чистый claimed workspace:
base commit является предком head commit, head существует в workspace, а
changed files совпадают с Git diff между ними. Несовпадение, незакоммиченная
рабочая копия или недоступный source artifact MUST отказать до первого gate.

#### Scenario: Исправление сделано после terminal partial

- **WHEN** source Run завершён `partial`, его plan/handoff доступны, а
  разработчик перенёс исправления в чистый commit claimed workspace
- **THEN** continuation создаёт новый Run с отдельной identity и provenance
  source Run, принимает актуальную Implementation и не меняет source Run

#### Scenario: Рабочая копия не доказывает заявленную Implementation

- **WHEN** continuation получает незакоммиченные изменения, head не содержит
  source base или changed files не совпадают с Git
- **THEN** она отказывает до создания Attempt и не считает workspace
  продолжением source Run

### Requirement: Continuation выполняет только declared quality tail
Continuation workflow MUST не выдавать warmup, plan, improve или implement
Attempt. После принятия Implementation он MUST пройти только declared tail
качества и его собственные bounded repair rounds; `no_work` отсутствующего
implement не может привести continuation к `abandoned`.

#### Scenario: Код уже реализован

- **WHEN** continuation получила пригодную Implementation
- **THEN** первым выдаваемым рабочим шагом является declared verify gate, а
  Run достигает review/commit либо своего declared quality outcome без
  вызова implement

#### Scenario: Gate находит исправимый blocker

- **WHEN** verify или review continuation возвращает пригодный blocking gate
- **THEN** declared fix round запускается в новом Run, затем тот же gate
  проверяет обновлённую Implementation в пределах своего limit

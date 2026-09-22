## ADDED Requirements

### Requirement: Предмет проверки определяет пригодность старого результата

Reuse принятого результата MUST проверять declared semantic subject, а не один факт `pass`: exact inputs и bindings, Git tree либо иной code subject, effective StepDefinition, executor/tool, context, checks, decision values и требуемую freshness. Невозможность доказать хотя бы одно значимое совпадение MUST быть cache miss/refusal, не молчаливый перенос. Reuse не является новым execution и не может выдать новые права.

#### Scenario: Изменился только output schema отказавшего tests
- **WHEN** префикс workflow и его предмет проверки совпадают, а новая версия пакета меняет лишь схему результата технически failed `tests`
- **THEN** прежние принятые `verify` и `review` остаются пригодным evidence, а tests candidate оценивается отдельно по новой схеме

#### Scenario: Изменился проверяемый код
- **WHEN** новый Git tree включает правку после принятого gate, а gate проверял всё дерево
- **THEN** gate не считается покрывающим новую правку даже при неизменном имени стадии

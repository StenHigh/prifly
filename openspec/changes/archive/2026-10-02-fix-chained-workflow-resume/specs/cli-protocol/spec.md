## ADDED Requirements

### Requirement: План цепочки возобновления объясняет перенос и безопасный следующий шаг

`project continue --prepare` того же workflow и `project recover --prepare` SHALL показывать весь доказанно переносимый префикс, различая исходное исполнение и перенос через непосредственный source Run, точку возобновления и текущий передаваемый claim. Оператор MUST иметь возможность проверить происхождение унаследованных результатов по публичному плану и чтению связанных Runs без извлечения входных ArtifactRefs вручную. Prepare MUST NOT создавать Run, импортировать package, менять claim или dirty tree и запускать программы.

`resume_prefix_empty` SHALL означать только отсутствие собственного и доказанного унаследованного префикса до выбранной точки. Отказы на недоступное или несовместимое evidence MUST называть это основание, а не советовать новый полный Run как эквивалент сохранённой цепочки. Runner guidance SHALL объяснять, что `run next` перечисляет объявленные возможности installed workflow, а окончательная применимость конкретного launch проверяется prepare. При отказе prepare агент MUST сохранять текущего владельца и дерево; обход через raw fork, ручную правку authority или самовольный сброс работы не является следующим шагом.

Повторный start SHALL использовать проверенные repository, authority, launch, host и review digest prepare; input/answers source не подменяются. Он MUST выдавать новый Run с публичной связью на source и без фиктивных Attempts для перенесённых стадий. Изменившийся review MUST требовать нового prepare, а не снятия guards.

Current source set остаётся `openspec/specs/cli-protocol/spec.md` с delta этого change до sync. CLI-флаги и сохранённые JSON-поля не переименовываются; изменение формы публичного ответа, если необходимо, SHALL иметь новую edition рядом с прежней.

#### Scenario: Агент продолжает B после решения владельца
- **WHEN** B завершился объявленным resumable outcome и хранит унаследованный префикс и claim
- **THEN** prepare показывает перенос ранних стадий, current source B и его claim; после проверки оператор запускает новый Run с этим digest и продолжает по выданным Attempts

#### Scenario: Индекс объявлений не равен admission
- **WHEN** `run next` называет workflow в continuations, но конкретный launch или его package несовместим с source
- **THEN** prepare сообщает конкретный отказ, а guidance направляет агента сохранить состояние и сообщить причину без перебора обходных запусков

#### Scenario: Настоящий пустой префикс при выборе entry
- **WHEN** оператор выбрал начальную стадию и до неё нет ни собственного, ни унаследованного результата
- **THEN** CLI возвращает `resume_prefix_empty`, объясняет отсутствие переноса и не освобождает уже существующий claim

#### Scenario: Prepare повторяется без изменений
- **WHEN** агент повторяет prepare при неизменных source, target и authority evidence
- **THEN** перенос и review digest детерминированы, а Run versions, claims, Registry и рабочее дерево остаются неизменными

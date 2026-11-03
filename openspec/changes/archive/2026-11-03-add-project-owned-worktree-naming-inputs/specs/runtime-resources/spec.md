## ADDED Requirements

### Requirement: Claim preserves the original project naming snapshot

Новый claim с project-owned naming inputs SHALL сохранять immutable snapshot declaration, JSON schemas, canonical values и digests вместе с naming allocation evidence. Snapshot SHALL поступать проверенным payload до durable reservation. Его общий canonical UTF-8 JSON размер MUST NOT превышать 128 KiB. Existing command/result ceilings MUST сохраняться; serialized carriers с повторным snapshot MUST проверяться до effects. Новый claims record SHALL иметь bounded strict reader с existing state ceiling 16 MiB. Prospective record MUST проверяться до reservation, в том числе повторно внутри pure transform до mutation. Transform MUST NOT читать naming files, schemas или blobs внутри write-транзакции.

Idempotency SHALL учитывать полный naming snapshot. Повтор принятой команды SHALL возвращать исходный результат; изменённый snapshot под тем же command ID MUST давать idempotency conflict. Existing allocation, ownership, generation, cleanup и collision semantics SHALL сохраняться.

Continue/resume/recover с передачей существующего claim SHALL сохранять snapshot, claim identity, path и branch. Штатное повышение generation SHALL сохраняться. Изменение либо удаление project policy, schema или файлов значений MUST NOT требовать повторного ввода naming values и MUST NOT переименовывать дерево.

Если существующий continuation contract требует новый claim, Core SHALL использовать сохранённый snapshot источника с прежней основой и обычным collision policy. Он MUST NOT подставлять текущие defaults. Переданный continuation override naming inputs MUST отказываться явно. Legacy Runs без такого snapshot SHALL сохранять прежний continuation path без синтеза metadata.

Source set остаётся `openspec/specs/runtime-resources/spec.md`; ownership не переносится. Snapshot является metadata claim и MUST NOT менять WorkflowRevision, Run task inputs или смысл принятых результатов.

#### Scenario: Continue ignores changed project naming files
- **WHEN** source claim содержит snapshot, а проект изменил template, schema и value file
- **THEN** продолжение передаёт прежний claim и snapshot с штатной новой generation без чтения новых naming values

#### Scenario: A continuation needs a new worktree
- **WHEN** существующий контракт продолжения требует новый claim от выбранного commit, а source claim содержит naming snapshot
- **THEN** Core использует прежнюю основу и первый свободный suffix; новый snapshot из текущих project files не создаётся

#### Scenario: Restart preserves original input evidence
- **WHEN** authority закрыта и повторно открыта после принятого start
- **THEN** read/continue возвращают исходные naming values и digests без обращения к исходным файлам

#### Scenario: A legacy source has no naming input snapshot
- **WHEN** caller продолжает Run, созданный до project-owned naming inputs
- **THEN** Core сохраняет прежнее поведение и не требует новых metadata или profile migration

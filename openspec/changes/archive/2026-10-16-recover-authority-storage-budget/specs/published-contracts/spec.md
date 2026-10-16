## ADDED Requirements

### Requirement: Durable storage budget имеет явную writer compatibility boundary
Persisted configurable storage ceiling SHALL поставляться в новой storage
version после baseline storage 9 с атомарной supported migration. Migration
MUST сохранять прежний effective default, authority identity, Run/event bytes,
pins, claims и receipts; read-only show/prepare MUST NOT мигрировать storage.
Устаревший writer, открытый до migration, MUST отказываться до записи после
изменения storage header; обновлённый writer SHALL читать current persisted
setting при каждом budget decision. Downgrade binary MUST NOT обещать
read/write совместимость с новой storage version.

Новые status/review/result и authority budget control SHALL иметь отдельные
closed versioned schemas и реально emitted fixtures. Existing Run state/read,
Problem и plain doctor contracts MUST NOT получать новые fields/meaning.
Historic schemas/bundles/evidence SHALL сохранять bytes. Storage-only migration
MUST NOT трактоваться как новая WorkflowRevision или пересчёт принятых Run.

#### Scenario: Старый process продолжает запись после migration
- **WHEN** старый writer держал соединение до перехода storage на новую version
- **THEN** его следующая write transaction отказывается как incompatible до mutation, без обхода нового ceiling

#### Scenario: Reader выполняет prepare на storage 9
- **WHEN** новый CLI читает budget или готовит повышение на прежней authority
- **THEN** storage header и история остаются прежними, review явно показывает требуемый upgrade, а setting не повышается автоматически

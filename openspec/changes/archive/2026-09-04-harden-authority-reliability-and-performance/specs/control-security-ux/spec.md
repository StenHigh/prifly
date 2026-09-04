Authoritative source set: `openspec/specs/control-security-ux/spec.md`
(перенесено). Compatibility path: существующие stop/release/resume semantics
сохраняются; резолюция — новая отдельная операция.

## ADDED Requirements

### Requirement: Read-only monitor проверяет origin и не раскрывает внутренности

Loopback monitor MUST принимать запросы только с `Host`, равным его
собственному loopback адресу, MUST отвечать `X-Content-Type-Options: nosniff`,
MUST выдавать ошибки через тот же safe Problem contract, что и CLI, без
внутренних путей и сообщений, и MUST ограничивать отдаваемое содержимое
артефакта объявленным пределом без чтения всего blob в память. Monitor
остаётся окном без команд.

#### Scenario: Запрос с чужим Host
- **WHEN** страница с внешнего домена, разрешённого в loopback, обращается к
  `/api/*`
- **THEN** monitor отказывает и не отдаёт записанные данные

## MODIFIED Requirements

### Requirement: Resume не обходит uncertainty или stop

Resume MUST revalidate current restrictions, resources, outstanding intents,
approval/Grant срок и budget. Он не снимает stop, не переиспользует consumed
approval и не допускает ordinary work при conflicting unknown effect. Backup
recovery не обнуляет generations или used admissions. Резолюция uncertain
obligation — отдельная аудируемая control operation с собственным permission:
resume, cancel, release и drive MUST NOT выполнять её неявно, а резолюция MUST
NOT объявлять success или снимать stop.

#### Scenario: Новый stop появился между release и resume
- **WHEN** operator последовательно выпускает release и resume
- **THEN** resume отказывает из-за нового применимого stop

#### Scenario: Resume после uncertain attempt
- **WHEN** Run содержит uncertain attempt без резолюции
- **THEN** resume отказывает с `recovery_required`, а safe next action
  называет резолюцию, не retry

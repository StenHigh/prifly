## Context

Current quality source set contains 16 quality rules, 148 detailed product
acceptance cases, their CSV inventory and two legacy traceability tables.
The catalog currently describes future checks: its statuses and historical
evidence MUST NOT be reinterpreted during a documentation move.

## Goals / Non-Goals

**Goals:**

- Move every quality boundary and all 148 product cases into the
  `quality-and-acceptance` capability without legacy authoring IDs.
- Retain priority, verification kind, declared status, requirement coverage and
  evidence boundary as reviewable information.
- Preserve exact legacy identifiers and all 337 legacy traceability entries in
  the archived crosswalk.

**Non-Goals:**

- Do not run product acceptance, update status/evidence, change roadmap phase,
  Go code, tests, schemas, manifests or release qualification.
- Do not duplicate foundation, package or other capability requirements merely
  because the legacy traceability table links to them.

## Decisions

### Permanent spec keeps behavior; archive keeps old identities

The permanent spec will have one descriptive requirement for each quality
boundary and 148 individually named acceptance scenarios grouped by domain.
Scenario prose carries its Given/When/Then, priority, verification kind,
declared status and evidence boundary. Former `QUAL`/`AC` names and old
requirement links remain only in the archived crosswalk.

### Traceability remains explicit after the ID cutover

The permanent catalog will link a scenario to readable OpenSpec requirement
headings and capability paths, rather than inventing a new ID system. The
archived matrix will preserve the exact old identifier mappings while other
capabilities still migrate. This keeps the current map searchable without
making legacy IDs the future editing interface.

### Derived legacy tables are recorded, not promoted to runtime evidence

`requirements-map.csv` covers all 148 product cases; `acceptance-map.csv`
contains 337 rows, including 189 supplemental entries owned by foundation,
package, workflow and other source areas. Apply records both tables in the
archived crosswalk. The product corpus moves to this capability; supplemental
case semantics remain with their owning future capability and are not copied
as quality requirements.

### Cutover stays one source-map row

After coverage and strict validation, apply creates the main spec and switches
only the quality-and-acceptance source-map row. Legacy source bytes remain
unchanged until final cleanup, making rollback a one-row restore.

## Risks / Trade-offs

- [A detailed case is lost while reducing IDs] → count all 148 scenario blocks
  and compare every legacy case to an archived crosswalk row.
- [Traceability is mistaken for execution] → retain the declared status and
  evidence boundary in each scenario; preserve historical evidence unchanged.
- [Supplemental cases are duplicated or orphaned] → partition 337 legacy map
  rows by ownership before cutover and review the partition.

## Migration Plan

1. Expand the candidate with all 16 quality boundaries and 148 individual
   product scenarios; build an archived individual crosswalk for quality rules,
   cases and both traceability tables.
2. Verify counts, links and statuses; sync `quality-and-acceptance`, validate
   strictly and switch the one source-map row without modifying legacy bytes.
3. Verify code/schema/evidence/manifest preservation, archive the change and
   run strict archived validation.

## Archived crosswalk

### Quality rules

| Historical rule | Permanent requirement |
|---|---|
| QUAL-001 | Качество разделяет specification, conformance и qualification |
| QUAL-002 | Заявленная самостоятельность проходит применимые конфигурации |
| QUAL-003 | Технические проверки соответствуют границе свойства |
| QUAL-004 | Model invariants проверяются на допустимых interleavings |
| QUAL-005 | Fault injection проверяет состояние мира |
| QUAL-006 | Доказательство соразмерно механизму |
| QUAL-007 | Negative и adversarial cases проверяют enforcement |
| QUAL-008 | Performance report сохраняет гарантии |
| QUAL-009 | Recovery qualification учитывает внешний unknown effect |
| QUAL-010 | Управление доступно и честно показывает состояние |
| QUAL-011 | Документация является проверяемой частью контракта |
| QUAL-012 | Qualified release проходит definition of done |
| QUAL-013 | Изменение контракта создаёт regression и revised assessment |
| QUAL-014 | Универсальность проверяется сквозными сценариями |
| QUAL-015 | Acceptance catalog сохраняет проверяемую трассировку |
| QUAL-016 | Неподтверждённая гарантия остаётся ограничением |

### Product acceptance corpus

| Historical case | Permanent scenario | Exact historical requirement links | Permanent subject capabilities |
|---|---|---|---|
| AC-001 | Пустая независимая установка | `PROD-003;RUN-001;PKG-001;QUAL-002;ARCH-001` | product-model, runtime-resources, workflow-and-context, quality-and-acceptance, architecture-decisions |
| AC-002 | Controller не обращается к ИИ | `DOM-010;WF-002;CTRL-001;RUN-001;QUAL-003` | domain-execution, workflow-and-context, control-security-ux, runtime-resources, quality-and-acceptance |
| AC-003 | Проверка предмета до массовой работы | `PROD-001;PROD-008;UX-002` | product-model, control-security-ux |
| AC-004 | Изменение цели не продолжает старые intents | `PROD-009;CTRL-002;CTX-013;PROD-010` | product-model, control-security-ux, workflow-and-context |
| AC-005 | План модели остаётся предложением | `PROD-010;DOM-024;WF-022;API-024` | product-model, domain-execution, workflow-and-context, cli-protocol |
| AC-006 | Роль берётся из удостоверенной области | `PROD-006;DOM-001;CTRL-004;ARCH-011` | product-model, domain-execution, control-security-ux, architecture-decisions |
| AC-007 | Assisted без host не обещает wakeup | `PROD-011;RUN-002;CTRL-013;API-022` | product-model, runtime-resources, control-security-ux, cli-protocol |
| AC-008 | Managed isolated требует реального runner | `ARCH-006;QUAL-002;RUN-002;SEC-007` | architecture-decisions, quality-and-acceptance, runtime-resources, control-security-ux |
| AC-009 | Отказ и молчание человека не равны согласию | `DOM-017;WF-016;CTX-013;PROD-011` | domain-execution, workflow-and-context, product-model |
| AC-010 | Свой маленький workflow без чужого lifecycle | `PROD-002;PROD-020;PKG-011;ARCH-008;QUAL-014` | product-model, workflow-and-context, architecture-decisions, quality-and-acceptance |
| AC-011 | Модель выражает разные классы сценариев | `PROD-007;QUAL-014;ARCH-001` | product-model, quality-and-acceptance, architecture-decisions |
| AC-012 | Логические границы не превращены в обязательные сервисы | `PROD-004;PROD-005;RUN-003;ARCH-002;ARCH-007` | product-model, runtime-resources, architecture-decisions |
| AC-013 | SourceSnapshot не привязывает core к tracker | `WF-007;WF-019;DATA-002;DOM-001` | workflow-and-context, domain-execution |
| AC-014 | Импорт artifact до создания Run | `DOM-002;DOM-007;DATA-003;API-005;CTRL-014` | domain-execution, cli-protocol, control-security-ux |
| AC-015 | Durable receipt входящего batch не равен применению | `DATA-002;DOM-025;API-014` | domain-execution, cli-protocol |
| AC-016 | Unsupported capability не наследует чужую qualification | `PROD-018;PKG-013;API-025;ARCH-006` | product-model, workflow-and-context, cli-protocol, architecture-decisions |
| AC-017 | Identity пакета не допускает замены bytes | `PKG-003;DOM-003;API-004` | workflow-and-context, domain-execution, cli-protocol |
| AC-018 | Архив пакета не выходит за свой root | `PKG-004;SEC-007;QUAL-007` | workflow-and-context, control-security-ux, quality-and-acceptance |
| AC-019 | Lock включает транзитивные инструкции и schemas | `PKG-005;DOM-004;DATA-001` | workflow-and-context, domain-execution |
| AC-020 | Отсутствующие pinned bytes не заменяются похожей версией | `DATA-001;PKG-009;DOM-004` | domain-execution, workflow-and-context |
| AC-021 | Установка атомарна и не запускает hooks | `PKG-006;SEC-006;API-021` | workflow-and-context, control-security-ux, cli-protocol |
| AC-022 | Offline доверие не назначается ключом внутри пакета | `PKG-007;ARCH-015;PKG-006` | workflow-and-context, architecture-decisions |
| AC-023 | Capabilities пакета не являются Grant | `PKG-008;SEC-004;CTRL-007` | workflow-and-context, control-security-ux |
| AC-024 | Config precedence не перезаписывает ограничения | `PKG-012;CTRL-002` | workflow-and-context, control-security-ux |
| AC-025 | Обновление установлено рядом с активным lock | `PKG-009;PROD-016;ARCH-012` | workflow-and-context, product-model, architecture-decisions |
| AC-026 | Uninstall сохраняет pinned execution и evidence | `PKG-010;PROD-019;API-021` | workflow-and-context, product-model, cli-protocol |
| AC-027 | Quarantine отменяет trusted reuse cached pass | `PKG-010;DOM-009;SEC-006;QUAL-007` | workflow-and-context, domain-execution, control-security-ux, quality-and-acceptance |
| AC-028 | Минимальный свой step имеет public contract | `PKG-011;PROD-020;ARCH-008` | workflow-and-context, product-model, architecture-decisions |
| AC-029 | Удаление AI Factory не ломает независимый сценарий | `PKG-014;PROD-002;ARCH-016` | workflow-and-context, product-model, architecture-decisions |
| AC-030 | Qualification относится к конкретной операции | `PKG-013;RUN-011;ARCH-009` | workflow-and-context, runtime-resources, architecture-decisions |
| AC-031 | Каталог пакета не считает самоаттестацию испытанием | `PKG-015;PROD-016;QUAL-012` | workflow-and-context, product-model, quality-and-acceptance |
| AC-032 | Command, worker и check сохраняют разные контракты | `WF-001;WF-008;DOM-016;API-006` | workflow-and-context, domain-execution, cli-protocol |
| AC-033 | Ports требуют точной schema либо converter | `WF-004;DOM-006;API-007` | workflow-and-context, domain-execution, cli-protocol |
| AC-034 | Binding не выбирает последний похожий artifact | `WF-005;DOM-006;API-008;DATA-003` | workflow-and-context, domain-execution, cli-protocol |
| AC-035 | Обязательный input существует на каждом пути | `WF-006;WF-020;API-009` | workflow-and-context, cli-protocol |
| AC-036 | Skip и no_work не создают отсутствующие outputs | `DOM-017;CTRL-008;API-007;WF-006` | domain-execution, control-security-ux, cli-protocol, workflow-and-context |
| AC-037 | Трёхзначная логика отличает отсутствие и null | `WF-009;DOM-022;API-010` | workflow-and-context, domain-execution, cli-protocol |
| AC-038 | Exclusive choice не угадывает единственную ветвь | `WF-010;DOM-022;API-010` | workflow-and-context, domain-execution, cli-protocol |
| AC-039 | First_match использует pinned порядок | `WF-010;API-010;CTRL-017` | workflow-and-context, cli-protocol, control-security-ux |
| AC-040 | Condition не является исполняемым скриптом | `WF-009;API-002;DOM-022;QUAL-007` | workflow-and-context, cli-protocol, domain-execution, quality-and-acceptance |
| AC-041 | Структура definition не заменяет semantic validation | `WF-003;WF-020;API-003;API-009` | workflow-and-context, cli-protocol |
| AC-042 | Parallel соблюдает глобальные слоты и справедливость | `WF-011;RUN-006;PROD-012;API-011` | workflow-and-context, runtime-resources, product-model, cli-protocol |
| AC-043 | Quorum не публикует aggregate до settlement remainder | `WF-012;API-011;RUN-006;RUN-017` | workflow-and-context, cli-protocol, runtime-resources |
| AC-044 | Satisfied join не равен успешному verdict | `WF-012;WF-021;API-011;CTRL-019` | workflow-and-context, cli-protocol, control-security-ux |
| AC-045 | Map проверяет всю коллекцию до первого child | `WF-023;API-012;DOM-023;RUN-006` | workflow-and-context, cli-protocol, domain-execution, runtime-resources |
| AC-046 | Изменение source collection не расширяет активный map | `WF-023;API-012;DATA-003;RUN-006` | workflow-and-context, cli-protocol, domain-execution, runtime-resources |
| AC-047 | Пустой map имеет явный исход | `WF-023;API-012;WF-021` | workflow-and-context, cli-protocol |
| AC-048 | Общий budget ограничивает пустые и вложенные expansions | `WF-015;DOM-023;RUN-006;RUN-024;API-013` | workflow-and-context, domain-execution, runtime-resources, cli-protocol |
| AC-049 | Repeat переносит явное состояние и сохраняет причины rework | `WF-013;API-013;RUN-005` | workflow-and-context, cli-protocol, runtime-resources |
| AC-050 | Call graph ацикличен и экспортирует объявленные outcomes | `WF-014;API-009;DOM-023;WF-020;DOM-003` | workflow-and-context, cli-protocol, domain-execution |
| AC-051 | Event и timeout разрешают wait только один раз | `WF-016;API-014;DOM-025;RUN-022` | workflow-and-context, cli-protocol, domain-execution, runtime-resources |
| AC-052 | Callback раньше wait сохраняется через WaitRegistration | `API-014;WF-016;DATA-002;DOM-025` | cli-protocol, workflow-and-context, domain-execution |
| AC-053 | Невыбранная и истёкшая wait registration не запускает работу | `API-014;DATA-002;WF-016` | cli-protocol, domain-execution, workflow-and-context |
| AC-054 | Schedule slot не дублируется при DST и restart | `DOM-025;API-014;QUAL-014` | domain-execution, cli-protocol, quality-and-acceptance |
| AC-055 | Control stage может быть producer без фиктивного worker | `DOM-007;API-008;WF-005;CTRL-014` | domain-execution, cli-protocol, workflow-and-context, control-security-ux |
| AC-056 | Framing отвергает неоднозначный и чрезмерный JSON | `DOM-005;API-002;API-003;QUAL-007` | domain-execution, cli-protocol, quality-and-acceptance |
| AC-057 | ContextManifest фиксирует точный переданный набор | `CTX-001;DOM-012;RUN-008` | workflow-and-context, domain-execution, runtime-resources |
| AC-058 | Renderer не добавляет скрытое поручение | `CTX-003;RUN-009;DOM-012` | workflow-and-context, runtime-resources, domain-execution |
| AC-059 | Prompt injection не повышает доверие через summary | `CTX-002;SEC-008;QUAL-007` | workflow-and-context, control-security-ux, quality-and-acceptance |
| AC-060 | Новый worker ID не доказывает fresh context | `CTX-004;SEC-008;RUN-009` | workflow-and-context, control-security-ux, runtime-resources |
| AC-061 | Данные command передаются без shell expansion | `CTX-005;RUN-016;SEC-005` | workflow-and-context, runtime-resources, control-security-ux |
| AC-062 | Context reference не выдаёт право публикации | `CTX-006;SEC-004;CTRL-005` | workflow-and-context, control-security-ux |
| AC-063 | Подмена live файла между preview и dispatch | `CTX-007;DATA-003;RUN-016` | workflow-and-context, domain-execution, runtime-resources |
| AC-064 | Дополнительный контекст получается явной операцией | `CTX-008;CTX-001;CTRL-007` | workflow-and-context, control-security-ux |
| AC-065 | Переполнение контекста не удаляет обязательный запрет | `CTX-009;RUN-023;API-002` | workflow-and-context, runtime-resources, cli-protocol |
| AC-066 | Credentials и запрещённые данные не отправляются модели | `CTX-010;SEC-009;SEC-010` | workflow-and-context, control-security-ux |
| AC-067 | Валидный descriptor не доказывает содержимое PDF | `CTX-011;DOM-006;CTRL-015` | workflow-and-context, domain-execution, control-security-ux |
| AC-068 | Evidence различает исполнение, форму и смысл | `CTX-012;DOM-008;CTRL-014;QUAL-006` | workflow-and-context, domain-execution, control-security-ux, quality-and-acceptance |
| AC-069 | Reuse реагирует на значимые изменения, не bookkeeping | `DOM-009;CTRL-015;DATA-003` | domain-execution, control-security-ux |
| AC-070 | Declared новые данные не требуют fork, замена исходных требует | `CTX-013;CTRL-002;WF-019` | workflow-and-context, control-security-ux |
| AC-071 | Read-only router не выдаёт admission | `DOM-011;RUN-007;ARCH-003` | domain-execution, runtime-resources, architecture-decisions |
| AC-072 | Конкурентный CAS не оставляет частичный допуск | `DOM-013;DATA-005;DATA-004;CTRL-003;QUAL-004` | domain-execution, control-security-ux, quality-and-acceptance |
| AC-073 | Потерянный ответ command не вызывает повторный эффект | `DATA-005;API-018;CTRL-003` | domain-execution, cli-protocol, control-security-ux |
| AC-074 | Receipt replay проверяет текущее право чтения | `SEC-002;DATA-005;API-018` | control-security-ux, domain-execution, cli-protocol |
| AC-075 | Crash windows blob sealing и reference commit | `DATA-006;DOM-007;QUAL-005;ARCH-004` | domain-execution, quality-and-acceptance, architecture-decisions |
| AC-076 | Потерянный referenced blob не восстанавливается заглушкой | `DATA-006;ARCH-004;CTRL-015` | domain-execution, architecture-decisions, control-security-ux |
| AC-077 | Независимый результат переживает чужой heartbeat | `DOM-013;RUN-008;API-020` | domain-execution, runtime-resources, cli-protocol |
| AC-078 | Terminal result immutable и различается с progress | `RUN-008;API-020;DOM-016` | runtime-resources, cli-protocol, domain-execution |
| AC-079 | ExecutionAdmission не разрешает произвольные tools | `CTRL-005;RUN-007;ARCH-005;API-016` | control-security-ux, runtime-resources, architecture-decisions, cli-protocol |
| AC-080 | Transport retry создаёт Delivery, не Step Attempt | `RUN-012;DOM-016;API-016;CTRL-009` | runtime-resources, domain-execution, cli-protocol, control-security-ux |
| AC-081 | Whole-worker restart не повторяет первый успешный tool | `RUN-012;RUN-020;ARCH-005;QUAL-005` | runtime-resources, architecture-decisions, quality-and-acceptance |
| AC-082 | HTTP202 остаётся pending outcome | `RUN-010;DOM-020;API-017` | runtime-resources, domain-execution, cli-protocol |
| AC-083 | Потеря ответа после effect вводит uncertainty barrier | `RUN-010;DOM-020;RUN-004;QUAL-005` | runtime-resources, domain-execution, quality-and-acceptance |
| AC-084 | Qualified recovery resend без отдельного GET | `RUN-011;RUN-012;CTRL-009;API-017;WF-017` | runtime-resources, control-security-ux, cli-protocol, workflow-and-context |
| AC-085 | Известный partial отличается от неизвестного остатка | `DOM-020;RUN-010;API-017` | domain-execution, runtime-resources, cli-protocol |
| AC-086 | Compensation имеет новый допуск и правильный контекст | `RUN-013;WF-018;API-015;DOM-020` | runtime-resources, workflow-and-context, cli-protocol, domain-execution |
| AC-087 | Отказ компенсации оставляет residual obligations | `RUN-013;CTRL-018;WF-018` | runtime-resources, control-security-ux, workflow-and-context |
| AC-088 | Terminal outcome отражает объявленные обязательства | `DOM-015;RUN-004;WF-021;CTRL-019` | domain-execution, runtime-resources, workflow-and-context, control-security-ux |
| AC-089 | Семантическая доработка отличается от технического retry | `DOM-016;RUN-005;RUN-020` | domain-execution, runtime-resources |
| AC-090 | Claims учитывают physical aliases и весь conflict set | `RUN-014;DOM-001;DATA-004` | runtime-resources, domain-execution |
| AC-091 | Истёкший lease не освобождает живого старого writer | `RUN-015;CTRL-011;RUN-014;QUAL-004` | runtime-resources, control-security-ux, quality-and-acceptance |
| AC-092 | Cleanup не удаляет ресурс нового поколения | `RUN-017;RUN-016;SEC-007` | runtime-resources, control-security-ux |
| AC-093 | Локальная и remote identity имеют честную границу | `SEC-003;PROD-006;DOM-001` | control-security-ux, product-model, domain-execution |
| AC-094 | IDOR, forged callbacks и чужие exports отвергаются | `SEC-002;SEC-011;QUAL-007` | control-security-ux, quality-and-acceptance |
| AC-095 | Quorum approval учитывает действительных участников | `CTRL-004;CTRL-006;DOM-019` | control-security-ux, domain-execution |
| AC-096 | Intent pin-ит существующий subject и контракт будущего output | `DOM-018;CTRL-005;DOM-019` | domain-execution, control-security-ux |
| AC-097 | Consume конкурирует с expiry/revoke атомарно | `CTRL-006;DOM-019;RUN-007;QUAL-005` | control-security-ux, domain-execution, runtime-resources, quality-and-acceptance |
| AC-098 | Revoke после consume действует на ещё не начатый dispatch | `CTRL-006;RUN-018;API-019` | control-security-ux, runtime-resources, cli-protocol |
| AC-099 | Глобальный release использует ControlIntent без фиктивного Run | `DOM-002;API-019;CTRL-005;CTRL-010` | domain-execution, cli-protocol, control-security-ux |
| AC-100 | Pure resume не снимает stop | `API-018;CTRL-011;RUN-020;PROD-015` | cli-protocol, control-security-ux, runtime-resources, product-model |
| AC-101 | Stale UI не мешает durable stop | `DATA-005;CTRL-010;RUN-018` | domain-execution, control-security-ux, runtime-resources |
| AC-102 | Release не снимает чужие и более новые stops | `CTRL-010;API-018;ARCH-003` | control-security-ux, cli-protocol, architecture-decisions |
| AC-103 | Recovery после stop не является bypass | `CTRL-010;CTRL-018;RUN-018` | control-security-ux, runtime-resources |
| AC-104 | Grant ограничен сроком, областью и общим бюджетом | `CTRL-007;DOM-019;API-019` | control-security-ux, domain-execution, cli-protocol |
| AC-105 | Waiver не отменяет integrity и authorization | `CTRL-008;PROD-014;DOM-017` | control-security-ux, product-model, domain-execution |
| AC-106 | Waiver виден потребителям и в итоговом outcome | `CTRL-008;CTRL-019;UX-004` | control-security-ux |
| AC-107 | Новый запрет пользователя выше pinned package instructions | `SEC-004;CTRL-002;RUN-009` | control-security-ux, runtime-resources |
| AC-108 | Target проверяется на реальном соединении | `SEC-005;CTX-005;QUAL-007` | control-security-ux, workflow-and-context, quality-and-acceptance |
| AC-109 | Изоляция проверяется действиями, а не названием контейнера | `SEC-001;SEC-007;RUN-016` | control-security-ux, runtime-resources |
| AC-110 | Credential rotation сохраняет или меняет authority явно | `SEC-009;RUN-011` | control-security-ux, runtime-resources |
| AC-111 | Hash и worker prose не подделывают trusted receipt | `SEC-011;CTRL-014;DOM-008` | control-security-ux, domain-execution |
| AC-112 | Preview и ошибки не исполняют непроверенный контент | `SEC-010;API-023` | control-security-ux, cli-protocol |
| AC-113 | Data policy действует на все выходные каналы | `SEC-010;DATA-008;CTX-010` | control-security-ux, domain-execution, workflow-and-context |
| AC-114 | Ручной результат и break-glass не являются force success | `DOM-021;CTRL-018;CTRL-003` | domain-execution, control-security-ux |
| AC-115 | Crash boundaries сохраняют точное состояние эффекта | `RUN-019;QUAL-005;QUAL-009;ARCH-004` | runtime-resources, quality-and-acceptance, architecture-decisions |
| AC-116 | Checkpoint и текущая проекция не образуют вторую историю | `DATA-007;DOM-026;CTRL-016` | domain-execution, control-security-ux |
| AC-117 | Restore старого snapshot не воскрешает consumed права | `RUN-021;RUN-025;QUAL-009;ARCH-013` | runtime-resources, quality-and-acceptance, architecture-decisions |
| AC-118 | Erasure переживает более старое восстановление | `DATA-008;RUN-021;ARCH-013` | domain-execution, runtime-resources, architecture-decisions |
| AC-119 | Clone backup и shared filesystem не создают HA | `RUN-003;RUN-021;ARCH-006;QUAL-009` | runtime-resources, architecture-decisions, quality-and-acceptance |
| AC-120 | Clock rollback не продлевает approval и deadline | `RUN-022;DOM-014;CTRL-016` | runtime-resources, domain-execution, control-security-ux |
| AC-121 | Durable timer не порождает повторный transition | `RUN-022;WF-016;DOM-025` | runtime-resources, workflow-and-context, domain-execution |
| AC-122 | Общий budget не размножается в tools и children | `RUN-023;CTRL-012;PROD-017;ARCH-014` | runtime-resources, control-security-ux, product-model, architecture-decisions |
| AC-123 | Disk full и queue pressure останавливают новый effect | `RUN-023;DATA-006;QUAL-005` | runtime-resources, domain-execution, quality-and-acceptance |
| AC-124 | Local-1 измеряется с полной durability и ошибками | `RUN-024;QUAL-008;ARCH-014` | runtime-resources, quality-and-acceptance, architecture-decisions |
| AC-125 | RPO/RTO разделены по failure model и effect recovery | `RUN-025;RUN-026;QUAL-009` | runtime-resources, quality-and-acceptance |
| AC-126 | Реальная SQLite configuration соответствует durable profile | `DATA-004;DATA-006;RUN-003` | domain-execution, runtime-resources |
| AC-127 | Telemetry и отчёт восстанавливаются из journal | `DATA-009;PROD-017` | domain-execution, product-model |
| AC-128 | Security update находит затронутые runs без переписи прошлого | `ARCH-012;QUAL-013;DOM-026` | architecture-decisions, quality-and-acceptance, domain-execution |
| AC-129 | Retention не стирает безопасную identity операций | `DATA-008;PROD-019;CTRL-016` | domain-execution, product-model, control-security-ux |
| AC-130 | Storage upgrade и export не запускают работу | `DATA-007;ARCH-013;API-025` | domain-execution, architecture-decisions, cli-protocol |
| AC-131 | CLI, library и UI не имеют отдельных переходов состояния | `UX-001;API-001;API-023` | control-security-ux, cli-protocol |
| AC-132 | Preview показывает цель и не является разрешением | `API-024;UX-002;PROD-008` | cli-protocol, control-security-ux, product-model |
| AC-133 | Прогресс показывает реальное ожидание и исполнителя | `UX-003;DATA-009;CTRL-013` | control-security-ux, domain-execution |
| AC-134 | Approval UI не скрывает scope за коротким названием | `UX-004;CTRL-005;CTRL-006` | control-security-ux |
| AC-135 | Ошибка объясняет безопасный следующий шаг | `UX-005;API-023` | control-security-ux, cli-protocol |
| AC-136 | Управление доступно без цвета и чтения полного лога | `UX-006;QUAL-010` | control-security-ux, quality-and-acceptance |
| AC-137 | Смысловая оценка не подменяет проверку исполнения | `QUAL-006;PROD-013;PROD-018` | quality-and-acceptance, product-model |
| AC-138 | Explain и replay не повторяют внешнюю публикацию | `CTRL-017;DOM-009;API-024` | control-security-ux, domain-execution, cli-protocol |
| AC-139 | Разные предметные сценарии выражаются одной моделью | `UX-007;QUAL-014;PROD-007` | control-security-ux, quality-and-acceptance, product-model |
| AC-140 | Acceptance traceability отделена от выполненных тестов | `QUAL-015;QUAL-001;API-026;UX-008` | quality-and-acceptance, cli-protocol, control-security-ux |
| AC-141 | Quick start проверен на реально поставленном build | `QUAL-011;ARCH-010;API-026` | quality-and-acceptance, architecture-decisions, cli-protocol |
| AC-142 | Универсальный release работает в трёх обязательных конфигурациях | `PROD-020;QUAL-002;ARCH-010;QUAL-012` | product-model, quality-and-acceptance, architecture-decisions |
| AC-143 | Qualification не наследует невозможные гарантии | `QUAL-016;ARCH-006;ARCH-009;RUN-026` | quality-and-acceptance, architecture-decisions, runtime-resources |
| AC-144 | Handover связывает результаты с конкретной поставкой | `QUAL-001;QUAL-012;ARCH-017;PROD-018` | quality-and-acceptance, architecture-decisions, product-model |
| AC-145 | Owner defaults не блокируют пустой core и не выдают лишних прав | `ARCH-011;PROD-006;ARCH-009` | architecture-decisions, product-model |
| AC-146 | Лицензия, происхождение и runtime trust проверяются отдельно | `ARCH-015;PKG-007;ARCH-012` | architecture-decisions, workflow-and-context |
| AC-147 | Границы модулей не требуют лишних сервисов или бесконечного SDK | `ARCH-002;ARCH-007;ARCH-008;PROD-004;PKG-002` | architecture-decisions, product-model, workflow-and-context |
| AC-148 | Пилот даёт принципы, а не обязательные stage names | `ARCH-016;PROD-005;PKG-014;QUAL-011` | architecture-decisions, product-model, workflow-and-context, quality-and-acceptance |

The table preserves identifiers and direct legacy requirement links only for
archive traceability. Permanent scenarios retain their title, metadata and
readable subject paths without these legacy authoring IDs.

### Full legacy acceptance-map partition

`archive-acceptance-map.csv` is a byte-identical archival copy of the 337-row
legacy map. Its ownership partition is:

| Owner prefix | Rows | Destination |
|---|---:|---|
| `AC` | 148 | This product acceptance corpus |
| `CTX` | 8 | workflow-and-context |
| `FND` | 24 | foundation-profile |
| `OBS` | 45 | Future observability capability |
| `PKG` | 7 | workflow-and-context |
| `PUB` | 14 | Future publication capability |
| `REA` | 29 | Future recovery/action capability |
| `RT` | 30 | runtime-resources |
| `UX` | 17 | control-security-ux |
| `WF` | 15 | workflow-and-context |

The `AC` set has 148 rows; the remaining owner sets total 189. Supplemental
rows retain their exact evidence in this archive but are not copied into the
quality-and-acceptance requirement corpus.

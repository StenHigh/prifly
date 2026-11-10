Текущий source set — `openspec/specs/control-security-ux/spec.md`. Ownership не меняется.
Старые trust decisions, lifecycle generations и Run bindings сохраняют смысл.

## ADDED Requirements

### Requirement: Владелец управляет сохранённым выбором атомарно
Установка, замена и удаление выбора SHALL требовать текущего owner права управления package trust.
Команда MUST сохранять actor, reason и command identity.
Замена и удаление MUST проверять ожидаемую generation выбора, включая отсутствие записи.
Установка MUST проверять текущее решение, точную сборку и authority до commit.
Read-only inspect и prepare MUST NOT создавать запись.
Выбор MUST NOT создавать SchemaMigrationDecision либо менять PackageStatus.
Transform MUST использовать только snapshot и подготовленный payload без чтения bytes или запуска процессов.

#### Scenario: Сессия пытается сохранить выбор
- **WHEN** session principal без owner права отправляет команду установки
- **THEN** authority возвращает object_access_denied без изменения выбора, trust или Run

#### Scenario: Два владельца заменяют одну запись
- **WHEN** две команды используют одну ожидаемую generation
- **THEN** только первая принятая команда меняет запись; вторая получает stale refusal

#### Scenario: Потерян ответ команды
- **WHEN** клиент повторяет принятую command identity с тем же payload
- **THEN** authority возвращает прежний receipt; другой payload той же команды получает command conflict

#### Scenario: Источник отозван повторно
- **WHEN** новый revoke инвалидирует решение, на которое указывает сохранённый выбор
- **THEN** prepare и start отказывают с причиной; запись остаётся доступной для inspect, замены и удаления

### Requirement: Start сохраняет проверенный выбор при конкурирующих командах
Start через сохранённый выбор SHALL проверять его generation и текущие migration preconditions при принятии нового Run.
Изменение выбора после prepare MUST инвалидировать проверенный запуск.
Удаление и повторная установка того же ID MUST NOT восстанавливать действительность прежнего review.
Изменение выбора MUST NOT менять binding уже принятого Run.

#### Scenario: Выбор удалён после prepare
- **WHEN** owner удаляет запись до start с прежним launch digest
- **THEN** start отказывает до создания Run и постоянного claim

#### Scenario: Запись изменилась после последнего чтения start
- **WHEN** другая команда успевает заменить выбор перед принятием Run
- **THEN** authority отказывает атомарно; start не запускает работу с прежней записью

#### Scenario: Запись удалена после успешного start
- **WHEN** owner удаляет выбор после принятия Run
- **THEN** Run сохраняет прежний binding и обычные проверки trust на следующих admissions

## MODIFIED Requirements

### Requirement: Package trust ограничивает executable content

Manifest, schemas and dependencies MUST validate before package code loading;
list, doctor and preview do not run install hooks. Pinned revision covers
executable content and dependencies. Revocation blocks new admissions and
trusted acceptance/reuse for affected revisions while preserving history and
physical observations.

Отдельное решение владельца SHALL разрешать только проверенные schema refs
для точного нового consumer и выбранных доверенных поставщиков. Оно MUST
сохранять terminal revoke и запрет для code, старых pins и непокрытых отзывов.
Reason сам по себе MUST NOT считаться доказательством безопасности или отменой отзыва.

#### Scenario: Checker revision отозвана

- **WHEN** cached result зависит от quarantined checker
- **THEN** new trusted reuse is blocked without deleting old evidence

#### Scenario: Повторный import выглядит как обычное обновление
- **WHEN** пакет или alias снова предоставляет ранее отозванный schema Ref без отдельного решения владельца
- **THEN** security revocation продолжает блокировать admission

## ADDED Requirements

### Requirement: Решение миграции требует точного owner consent
Применение миграции SHALL требовать действующего права менять package trust,
точного digest плана, явной причины и command identity. Право владельца MUST
проверяться при commit. Документ плана MUST NOT сам предоставлять полномочия.
Apply MUST сохранять actor, причину, одобренный scope и исходные lifecycle facts.
Отсутствующий источник отзыва, неизвестное происхождение bytes или неполный
closure MUST сохранять запрет. Миграция MUST NOT переписывать старые trust decisions.

#### Scenario: Сессия потеряла право менять trust
- **WHEN** право действовало при prepare, но отсутствует при apply
- **THEN** authority возвращает именованный отказ без записи решения миграции

#### Scenario: В плане изменили один Ref
- **WHEN** apply получает изменённый план или digest другого плана
- **THEN** authority отказывает и не допускает часть schemas

### Requirement: Apply миграции проверяет свежесть атомарно
Apply SHALL повторно проверять bytes и closure до write-транзакции.
Authority SHALL атомарно проверять package state, control policy, decision state
и влияющие Runs относительно prepare. Изменение проверенных inputs MUST давать
именованный stale либо integrity отказ без частичного применения.
Read-only prepare MUST NOT создавать approvals, менять trust или запускать работу.
Повтор той же принятой command identity и payload MUST возвращать прежний receipt.
Повтор command identity с другим payload MUST давать command conflict.

#### Scenario: Run изменился без изменения общего количества Runs
- **WHEN** влияющий Run изменил состояние после prepare
- **THEN** apply отказывает атомарно; сравнения только количества Runs недостаточно

#### Scenario: Bytes поставщика изменены после prepare
- **WHEN** apply обнаруживает отличие manifest или schema bytes от одобренных digest
- **THEN** authority возвращает именованный integrity отказ и не записывает разрешение

#### Scenario: Ответ apply потерян
- **WHEN** клиент повторяет принятую команду с теми же identity и payload
- **THEN** authority возвращает сохранённый результат без второго решения и без повторной миграции

### Requirement: Новое lifecycle решение сильнее старой миграции
Решение миграции SHALL покрывать конкретные состояния отзыва и доверия, а не
пожизненно разрешать package Ref. Новый revoke покрытого источника, выбранного
поставщика или необходимой зависимости MUST блокировать последующее применение
исключения. Quarantine, remove и утрата разрешаемости выбранного поставщика MUST
также запрещать новые admissions через него. Уже наблюдённые effects MUST сохраняться.
Не связанный с миграцией security revoke MUST сохранять действие.

#### Scenario: После apply источник отозван повторно
- **WHEN** новая принятая lifecycle команда снова отзывает тот же exact package Ref
- **THEN** прежнее решение не покрывает новый отзыв и следующий admission блокируется

#### Scenario: Поставщик отозван после первого шага
- **WHEN** новый Run ожидает следующий admission, а выбранный поставщик либо его dependency теряет доверие
- **THEN** admission блокируется; движок не подбирает другого поставщика и не переписывает effects первого шага

Источник — `openspec/specs/control-security-ux/spec.md`; ownership не меняется.
Существующие Stops, permissions, grants и общие trust checks сохраняют свои границы.

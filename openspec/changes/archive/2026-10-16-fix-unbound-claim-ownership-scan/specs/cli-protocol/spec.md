## ADDED Requirements

### Requirement: Ошибка запуска показывает незавершённый cleanup без потери исходной причины
При подтверждённой ошибке до RunStart Project launch SHALL пытаться освободить
только claim, созданный именно этой попыткой. Existing, handed-over или
принадлежащий admitted Run claim MUST NOT освобождаться как rollback нового
запуска. Неизвестный исход RunStart SHALL сначала требовать reconciliation
по command/receipt и записанному Run; ошибка ответа MUST NOT доказывать, что
Run не создан.

CLI SHALL сохранять code и объяснение исходного отказа. Если cleanup не
завершён, сообщение SHALL явно называть оставшийся claim ID, generation,
наблюдаемый status, причину cleanup failure и применимые inspect/retry действия.
Unknown cleanup state MUST оставаться unknown. Команда MUST NOT сообщать
«ничего не создано» или советовать слепой новый start при оставшемся claim.
Один Problem-конверт MUST сохранять исходную schema и не маскировать ошибку
новыми полями старого closed contract. Успешный rollback SHALL отличаться от
незавершённого.

#### Scenario: Start отказал, cleanup выполнился
- **WHEN** confirmed pre-Run failure следует после создания своего свободного claim, в том числе на authority >64 МиБ
- **THEN** новый Run отсутствует, claim штатно released, а CLI возвращает исходный отказ и достоверный cleanup result

#### Scenario: Release не завершился
- **WHEN** rollback не смог доказать ownership, встретил changed generation либо filesystem failure
- **THEN** original refusal code сохраняется, сообщение называет конкретный оставшийся claim/cleanup причину, новый start не предлагается как обход

#### Scenario: Ответ RunStart потерян
- **WHEN** ошибка транспорта/хранилища не доказывает отсутствие committed Run
- **THEN** launch сверяет recorded command/Run либо сохраняет неопределённость; admitted claim не удаляется

### Requirement: Scan refusal и doctor различают ownership scan и logical-page budget
Ошибка aggregate bounded scan SHALL доходить до CLI через typed named отказ
`authority_scan_budget_exhausted` с observed count/bytes, ограничением и видом
лимита. Она MUST NOT превращаться в `invalid_input` и MUST NOT считаться
исчерпанием SQLite logical-page budget. Наблюдаемая нижняя граница MUST NOT
выдаваться за точный total population, если чтение оборвано.

Read-only `doctor --claim ID` SHALL диагностировать claim без RunID: exact
claim identity/generation/status, проверенный ReadCut, полноту ownership
проверки, observed scan metrics, blocker либо причину неполноты и отдельно
allocated logical-page usage / effective limit. При непрочитанной/несовместимой
части SHALL сохраняться unknown, без raw snapshots, task bodies или secrets.
Diagnostic observation MUST NOT быть разрешением на release.

Новый view SHALL публиковаться как отдельный `claim-ownership-diagnostics/1`
contract; plain doctor и прежний Problem MUST сохранять свои forms. Если
полезная диагностика возможна при отказе полной проверки, view SHALL явно
указывать её неполноту, а не фабриковать успешную проверку.

#### Scenario: Bounded read достиг лимита
- **WHEN** bounded read API превышает bytes/count allowance
- **THEN** Problem содержит named scan reason и наблюдённые threshold values; next actions ведут к применимой диагностике, а не к Run status несуществующего Run

#### Scenario: Orphan claim исследуется без записи
- **WHEN** оператор вызывает `doctor --claim ID` для unbound claim большой authority
- **THEN** он получает полное либо явно incomplete ownership наблюдение отдельно от storage budget, без изменения claim, Run history, files или registry

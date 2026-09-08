## ADDED Requirements

### Requirement: Локальная очистка сохраняет удерживаемые данные
Локальное обслуживание SHALL удалять только окончательно settled terminal Run без живых attempts/checks, pending operations, unresolved effects и неосвобождённых claims. Сохраняющиеся зависимые Runs SHALL удерживать нужную исходную историю. Проверка и удаление SHALL выполняться при эксклюзивном доступе к authority; занятый процесс не останавливается ради очистки.

#### Scenario: Завершённый статус не доказывает settlement
- **WHEN** у Run остаются pending attempts, effects, claims или операции
- **THEN** удаление этого Run отклоняется независимо от текста terminal status

#### Scenario: Другой процесс использует authority
- **WHEN** exclusive доступ для обслуживания недоступен
- **THEN** операция возвращает отказ без удаления и без прерывания процесса

### Requirement: Сборка мусора защищает ссылки и audit
Удаление Run и его журнала SHALL сохранять command receipts/dedup, authority controls, grants/approval audit и требуемый audit решений/действий. GC SHALL сохранять независимые import artifacts и транзитивно удерживаемые артефакты/provenance/manifests. Неизвестный либо повреждённый граф ссылок MUST NOT трактоваться как пустой. Удаление служебного workspace SHALL ограничиваться каталогом, записанным для удаляемой attempt/check, не затрагивая внешние рабочие папки и цели symlink. План и результат retention SHALL журналироваться; сбой после history commit SHALL оставлять видимую выполненную часть и допускаемые к повторной очистке остатки. Это не erasure из backup.

#### Scenario: Общий артефакт остаётся нужен
- **WHEN** удаляемый и сохраняющийся Run используют одинаковый артефакт либо его provenance
- **THEN** нужные metadata и bytes остаются доступными сохраняющемуся Run

#### Scenario: Рабочий каталог содержит внешнюю ссылку
- **WHEN** служебный workspace удаляемого Run содержит symlink вне authority
- **THEN** очистка не меняет внешнюю цель ссылки

#### Scenario: Повтор старой команды
- **WHEN** приходит retry команды удалённого Run
- **THEN** сохранённая receipt не позволяет считать этот command ID новым разрешением на effect

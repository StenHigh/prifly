## ADDED Requirements

### Requirement: Assisted шаг объявляет время работы отдельно от ожидания решения
Новый versioned authoring и StepDefinition MUST позволять автору задать
конечный положительный срок активной assisted доставки и отдельно срок одного
объявленного ожидания человека. В новом контракте отсутствие настройки
ожидания MUST означать отсутствие календарного срока; активный срок MUST
иметь явный документированный конечный default. Предпросмотр MUST показывать
оба значения и их источник до запуска. Настройки MUST закрепляться вместе
с definition; изменение YAML не меняет уже созданный Run.

Compiler MUST отвергать нулевой/отрицательный рабочий срок, переполнение,
неподдержанный contract и применение assisted настроек к несовместимому
executor. Legacy authoring MUST сохранять прежнюю compilation и сроки;
переход на новый contract MUST быть явным и видимым в новой revision.
Текущий source set остаётся `openspec/specs/workflow-and-context/` до sync.

#### Scenario: Автор не ограничил ожидание человека
- **WHEN** шаг объявлен в новом контракте без настройки срока ожидания
- **THEN** compilation закрепляет бессрочное ожидание отдельно от конечного
  рабочего срока, а preview объясняет эту разницу

#### Scenario: Автор задаёт другой рабочий срок
- **WHEN** два шага нового контракта имеют разные допустимые active limits
- **THEN** каждый использует свой закреплённый предел без общего скрытого часа

#### Scenario: После запуска изменены настройки
- **WHEN** YAML получает новые значения сроков
- **THEN** только новая revision использует их, а ранее созданный Run не меняется

## MODIFIED Requirements

### Requirement: YAML является lossless authoring frontend
`prifly-workflow/1`, `prifly-step/1` и `prifly-step/2` MUST детерминированно
опускаться в canonical JSON definitions до schema, compiler, digest и Run.
Runtime MUST не интерпретировать YAML shortcuts. Full JSON и full YAML without
marker MUST сохранять машинный contract; every rare field MUST иметь полную
форму. Новый step marker MUST явно выбирать StepDefinition v6; старый marker
MUST не получать его timing semantics автоматически.

#### Scenario: Автор использует полное машинное поле
- **WHEN** compact frontend не сокращает нужную настройку
- **THEN** compiler принимает её как unchanged canonical definition field

### Requirement: YAML defaults ограничены безопасной структурой
Authoring YAML MUST требовать identity, entry/stages, policy и execution
ceilings. Он MAY default title, empty ports/bindings, outcome set и безопасные
structural values, но MUST не угадывать routes, permissions, max iterations,
join, configuration scope или security semantics. Срок MUST иметь явное
значение либо документированный default явно выбранной contract edition;
`prifly-step/2` закрепляет active default 3600000 ms и бессрочное human wait.
Это MUST NOT изменять deadlines legacy definitions или defaults операторов.

#### Scenario: Автор опускает join rule
- **WHEN** parallel stage требует join semantics
- **THEN** compiler отклоняет definition вместо скрытого выбора default

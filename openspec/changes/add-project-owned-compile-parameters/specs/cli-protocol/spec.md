## ADDED Requirements

### Requirement: Project entry points используют один контракт параметров сборки
Standalone compile, read-only prepare и reviewed start SHALL читать одинаковые declarations и owner values. Effective value MUST входить в build identity и launch review digest. Изменение owner value MUST инвалидировать прежний review; start MUST отказать до создания Run при старом digest. Prepare MUST сохранять read-only semantics. Ошибка длины MUST показывать точный JSON pointer, effective limit и actual length в Unicode code points в prepare и start. Source set остаётся `cli-protocol`; существующие CLI flags и формы ответов сохраняются.

#### Scenario: Значение изменилось после prepare
- **WHEN** owner меняет 32000 на 40000 после проверки summary
- **THEN** start с прежним review digest возвращает stale-launch refusal до Run

#### Scenario: Unicode превышает effective limit
- **WHEN** description содержит 32001 Unicode code points при пределе 32000
- **THEN** prepare и start возвращают schema_invalid с pointer /description и числами 32000 и 32001

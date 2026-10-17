## ADDED Requirements

### Requirement: E6 Light имеет самостоятельные инженерные и production gates

Delivery backlog SHALL учитывать `add-prifly-light-task-board` / issue #11 как самостоятельный E6/P1 Core change: отдельная task/review доска поверх принятых E1 board и E2 native queue. E1/#4 и E2/#5 SHALL сохранять принятую runtime qualification; UI gap MUST NOT переоткрывать их gates. W1/W2/W5 SHALL быть consumer qualification fixtures; W6 recipes MUST NOT блокировать basic Light.

Gate SHALL включать E6-A1–A10: отдельный UI и сохранённый full monitor, task/continuation identity, mapped/generic phases, typed review без false PASS, repair/parallel scope, actual browser UX/navigation, measured ≥1000-task/Run performance и coherent reads, byte-compatible consumer/core regression, released local run/proxy/health/rollback и отдельную production acceptance. Инженерная реализация и released replay SHALL фиксироваться отдельно от server rollout.

Production acceptance SHALL после выпуска Core отдельно подтвердить, что ожидаемый deployment `https://prify-light.stenhigh.net/` показывает Light, а `https://prifly.stenhigh.net/` сохраняет полный monitor, существующая authentication действует и реальные owner tasks/review outcomes читаются верно. Эти адреса SHALL оставаться deployment configuration, не Core constants. Evidence SHALL включать exact binary/source SHA и browser screenshots/наблюдения; до этого issue и production gate MUST оставаться открытыми.

Current source set — `openspec/specs/delivery-roadmap/spec.md`; ownership не переносится. Historical releases и broader F1/F2 gates не меняются этим срезом. Прогресс SHALL храниться в tasks E6 change без дублирующего AI Factory плана.

#### Scenario: Код проверен до rollout
- **WHEN** локальные/browser/инженерные gates прошли, но released binary или production URLs ещё не проверены
- **THEN** статус называет подтверждённый этап и открытые A9/A10, не объявляет E6 развёрнутым либо полностью принятым

#### Scenario: Production принята владельцем
- **WHEN** released binary развёрнут, оба интерфейса проверены на реальных данных за прежним authenticated proxy и exact-SHA evidence сохранено
- **THEN** E6-A10 допускается закрыть в проверенном scope без изменения принятой E1/E2 qualification или consumer workflow packages

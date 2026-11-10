## ADDED Requirements

### Requirement: Параметры сборки имеют самостоятельные authoring contracts
Новые sidecars SHALL использовать закрытые `prifly-compile-parameters/1` и `prifly-compile-values/1` с опубликованными editor schemas. Feature SHALL требовать Project profile /3 либо более новую поддержанную версию с compiled variants. Profile /2 MUST явно отказать до sealing. Existing published bundles, workflow markers, Run wire forms и historical evidence MUST сохранять bytes и meaning. Source set остаётся `published-contracts` и canonical Go/JSON artifacts; ownership не переносится.

#### Scenario: Старый profile пытается применить параметр
- **WHEN** Project profile /2 читает sidecar нового контракта
- **THEN** compiler явно отказывает вместо замены схем под прежней identity

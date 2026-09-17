## ADDED Requirements

### Requirement: Read view терминального Run называет причину остановки
Versioned read view Run MUST у Run со status `failed` или `cancelled` нести
одно поле `failure` с code остановившей диагностики, её id и, если они есть,
attempt и step, в которых она возникла; у Run со status `completed` и у
незавершённого Run поле MUST отсутствовать. Значение MUST выводиться из
recorded diagnostics при чтении и MUST NOT менять saved Run state или прежние
read versions: reader прежней read version получает прежний shape без этого
поля.

#### Scenario: Программа шага без права записи изменила дерево
- **WHEN** Run завершился `failed`, потому что submission шага без
  `workspace_write` отказан `effect_not_permitted`
- **THEN** `run status --json` новой read version несёт `failure.code =
  effect_not_permitted` с attempt и step этой попытки, а `diagnostics[]`
  остаётся полным списком, как прежде

#### Scenario: Прежний reader читает тот же Run
- **WHEN** тот же Run читается reader-ом read version без поля `failure`
- **THEN** он получает прежний shape: status, diagnostics и `outcome` без
  нового поля, и saved state Run не изменён

#### Scenario: Run завершён успешно
- **WHEN** Run завершился `completed` с известным outcome
- **THEN** поле `failure` отсутствует, а `outcome` назван как прежде

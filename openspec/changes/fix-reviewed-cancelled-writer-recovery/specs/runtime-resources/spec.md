## ADDED Requirements

### Requirement: Recovery после резолюции сохраняет native workspace guards

Reviewed recovery отменённого writer SHALL передавать единственный existing claim обычным способом с повышением generation, не меняя path/branch/base. Prepare MUST NOT писать source, claim или рабочие bytes. Native workspace tree materialization/capture SHALL сохранять existing guards: занятый exact producer output даёт `workspace_tree_output_exists` с точным путём и действием владельца; несовпадение с sealed manifest даёт `workspace_tree_input_drift`. Эти проверки SHALL выполняться при native preparation Attempt, включая replay вложенной корневой стадии.

Partial unsealed PLAN MUST NOT автоматически удаляться, перезаписываться или становиться manifest. После осмотра owner SHALL явно сохранить его вне занятого capture path и освободить путь, затем повторить reviewed recovery от source, который сейчас владеет claim. Consumer materialization MAY использовать уже существующие точные bytes sealed manifest; `not_applied` MUST NOT удалять их как actor output.

Current source set — `openspec/specs/runtime-resources/spec.md`; ownership, saved claim и Workspace tree contracts сохраняются.

#### Scenario: Занятый producer output
- **WHEN** повтор producer достигает native admission при существующем partial PLAN
- **THEN** Attempt не выдаётся, named refusal сохраняет bytes, а владелец видит требуемое освобождение capture path

#### Scenario: Owner освободил capture path
- **WHEN** owner сохранил partial bytes отдельно и явно освободил занятый путь
- **THEN** новый reviewed recovery от текущего владельца claim может выдать producer без подмены original inputs или принятых результатов

#### Scenario: Consumer использует ранее материализованный input
- **WHEN** отменённый consumer разрешён `not_applied`, а existing input совпадает с sealed manifest
- **THEN** повтор использует exact bytes и сохраняет остальные tracked/untracked owner files

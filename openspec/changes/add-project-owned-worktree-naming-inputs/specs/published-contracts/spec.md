## ADDED Requirements

### Requirement: Project-owned naming inputs have additive closed versions

Новые declarations SHALL публиковаться в `prifly-project-profile/5`, questionnaire fields — в `project-questionnaire/5`, review/result — в `project-launch-summary/8` и `project-start/8`. Persisted naming snapshot SHALL использовать `authority-claims/5` и новую closed claims read форму. Его версия SHALL закреплять общий canonical UTF-8 JSON limit 128 KiB; новые author/CLI references MUST публиковать предел и typed overflow refusal. Это MUST NOT повышать существующие command/result/storage ceilings. Изменения nested claim views, continuation/recovery outputs и DTO MUST сопровождаться новыми closed versions во всех местах, где появляются новые поля.

Новые формы SHALL поддерживать совместное использование queue association и unified composition. Старые schemas, profiles `/2`–`/4`, Project DTO и claim contracts MUST сохранять bytes и meaning. Fresh init MUST NOT автоматически переводить existing profile на `/5`. Пути без новых данных SHALL сохранять прежний применимый wire version; source claim со snapshot SHALL выбирать новую форму даже после удаления policy.

Reader MUST поддерживать старые claims без snapshot. Reader, не поддерживающий `authority-claims/5`, MUST отвергать новый contract до ownership effects. Новая metadata MUST NOT добавляться в saved Run/state/read либо native definitions только ради naming. Если конкретный output включает новый nested claim, его новая версия SHALL публиковаться отдельно.

Canonical schema/Go artifacts SHALL проверять реальные emitted и persisted documents. Старые closed schemas MUST отвергать новые поля. Source sets остаются `openspec/specs/published-contracts/spec.md` и canonical JSON/Go artifacts; ownership не переносится. Historical bundles и evidence MUST оставаться неизменными.

#### Scenario: New profile data is submitted as profile four
- **WHEN** `worktree_naming.inputs` либо `naming_input` поданы с marker `/4`
- **THEN** старый contract отказывает; binary не включает новую семантику молча

#### Scenario: Named inputs coexist with queue and composition
- **WHEN** новый naming snapshot используется с queue selection и unified composition
- **THEN** новые closed DTO принимают все реальные поля, сохраняя queue identity и composition evidence

#### Scenario: Handover remains readable after policy removal
- **WHEN** profile больше не содержит naming policy, но передаваемый claim хранит новый snapshot
- **THEN** output использует новую closed форму; old schema не получает новых полей

#### Scenario: An old binary opens the new claim record
- **WHEN** reader поддерживает только `authority-claims/4`, а record использует `/5`
- **THEN** он отказывает до rename, release, cleanup или claim mutation

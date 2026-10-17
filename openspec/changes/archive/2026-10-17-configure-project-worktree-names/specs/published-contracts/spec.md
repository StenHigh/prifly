## ADDED Requirements

### Requirement: Project worktree naming имеет opt-in version boundary

Новые naming declaration, allocation metadata и review/result fields SHALL публиковаться как новые closed versioned authoring и CLI/authority формы. Профили `/2`, `/3`, старые launch summary/start/claim views и historical bundles MUST сохранять bytes и meaning. Явный переход shared profile на `/4` SHALL быть достаточен для включения naming declaration; init/start/runner upgrade MUST NOT автоматически менять существующий shared profile.

Новый reader SHALL поддерживать legacy hash claims без naming metadata. Старый reader MUST отвергать новый authority claim contract до любых ownership effects; отсутствие понимания named allocation MUST NOT превращаться в удаление/переименование дерева. Saved Run contracts, WorkflowRevision и StepDefinition MUST NOT получать новые версии только из-за внешнего имени рабочего дерева; при отсутствии нового Run field прежние state/read contracts SHALL сохраняться.

Новые review/result формы SHALL одновременно поддерживать naming и существующий queue-bound launch; queue identity, intent и idempotency semantics MUST сохраняться. Structural проверки SHALL валидировать фактические outputs и persisted claim bytes, проверять отказ старой closed schema для новых полей и отсутствие новых полей в legacy outputs.

Выбор wire version SHALL учитывать metadata передаваемого claim даже после удаления policy из shared profile: named handover MUST NOT расширять старую closed форму. Отсутствие настройки SHALL сохранять старые output versions для обычного hash пути без inherited naming metadata.

Текущий source set — `openspec/specs/published-contracts/spec.md` и canonical JSON/Go artifacts; ownership не переносится. Historical evidence и archived changes MUST оставаться неизменными.

#### Scenario: Named launch вместе с queue selection
- **WHEN** профиль `/4` создаёт named worktree для queue-bound launch
- **THEN** новые output schemas принимают реальные naming и queue данные, Run сохраняет штатную queue association и прежнюю границу state/read для queue

#### Scenario: Новый claim прочитан старым reader
- **WHEN** authority содержит новый allocation contract, который reader не поддерживает
- **THEN** reader возвращает unsupported contract до Git/filesystem/claim mutation

#### Scenario: Профиль не включает настройку
- **WHEN** `/2`, `/3` или `/4` без `worktree_naming` запускает прежний workflow без передачи named claim
- **THEN** naming fields отсутствуют, имена и ранее применимые output/state versions сохраняются

## Why

Issue [#14](https://github.com/StenHigh/prifly/issues/14) требует проектное измерение внутри child repeat и явный новый carrier для следующего reader. Root extension и статический native Call не позволяют выбрать вложенное место подключения без изменения исходников пакета.

## What Changes

- Добавить package-owned `extension-points.yaml` с marker `prifly-extension-points/1`.
- Объявить exact local workflow, entry либо прямой edge, фиксированные inputs, routes и разрешённые consumer ports.
- Добавить opt-in `prifly-extension/4`: project выбирает point, свой step или Call и явные output bindings; `enabled: false` сохраняет passthrough.
- Применять изменение до sealing child; включать manifest, программы и supporting files в build identity и review freshness.
- Сохранить native step/call/repeat, typed validation, ArtifactRef, effects, recovery и sealed Runs. Новый runtime operator не нужен.
- Поставить bounded native tests, generic example и authoring reference. Новый release и consumer issue #9 остаются отдельными действиями владельца.

## Capabilities

### New Capabilities

Нет. Новая возможность расширяет существующий authoring source set.

### Modified Capabilities

- `workflow-and-context`: закрытые package-declared extension points во вложенных local workflows и явные outputs потребителей.
- `cli-protocol`: deterministic compile, read paths, именованные отказы и prepare/start/continue freshness.
- `published-contracts`: отдельные closed editor schemas и capability inventory без изменения saved runtime forms.
- `specification-governance`: понятие Workflow extension point и связь с существующим Workflow extension.

## Impact

Изменение затрагивает authoring compiler, а не новые runtime primitives. Области: `cmd/prifly/project*.go`, authoring schemas, capability inventory, glossary и `examples/`. Ownership нормативных source sets не переносится. Чужой незакоммиченный change `add-workflow-component-imports-and-layers` не входит; `/3` extension и folder `/2` зарезервированы им.

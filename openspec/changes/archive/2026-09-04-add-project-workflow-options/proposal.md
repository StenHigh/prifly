## Why

Команда может использовать общий сценарий, но не обязана выполнять каждую его
необязательную часть. Сейчас `extend.yaml` умеет только вставить простой шаг:
чтобы пропустить improve, verify или review либо выбрать другие project values,
автору приходится копировать и вручную менять основной graph.

## What Changes

- Добавить в `extend.yaml` два ограниченных authoring-механизма: `settings` для
  значений уже объявленных project configuration inputs и `exclude` для
  именованных необязательных частей, которые явно объявил автор workflow.
- Сохранить `extensions` для существующей простой вставки шага между exact
  соседними stages; не вводить произвольные JSON-patch, stage override или
  удаление технических stages.
- Требовать, чтобы отключаемая часть имела заранее описанный в YAML безопасный
  маршрут. Compiler обязан отвергать неизвестную настройку, неизвестное
  исключение и несовместимый набор выбора до sealing.
- Применять выбранные values и исключения при compile, затем валидировать
  получившийся graph и выпускать новую WorkflowRevision. Уже начатый Run не
  меняется.
- Обновить local JSON Schema, authoring reference, AIF-cycle example и
  независимый YAML corpus.

## Capabilities

### New Capabilities

_Нет._

### Modified Capabilities

- `workflow-and-context`: Project workflow folder получает безопасный,
  декларативный выбор declared settings и optional частей сценария.

## Impact

Изменяются Project authoring parser/compiler, schema `extension-v1`,
authoring references, AIF-cycle YAML и проверки corpus. Это изменение
authoring surface и sealed package content; Go runtime, новый CLI command,
authority state, внешние зависимости и ранее созданные Runs не меняются.

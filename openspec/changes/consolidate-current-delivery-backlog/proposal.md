## Why

После переноса документации в OpenSpec формальный план P1/P2 сохранён, но
будущая очередь видна только частично. Владельцу и контрибьюторам приходится
сверять current roadmap с архивной crosswalk и историей Git, чтобы понять весь
объём оставшейся работы.

## What Changes

- Собрать в `delivery-roadmap` единый читаемый backlog: текущая работа,
  последовательные product milestones и post-P2 catalogue.
- Явно отделить будущую работу от завершённых инженерных срезов и immutable
  release evidence.
- Зафиксировать правило обновления backlog: новая работа появляется через
  OpenSpec change, а закрытая — получает ссылку на change/release evidence.
- Не восстанавливать удалённые legacy roadmap, progress или evidence-файлы как
  второй редактируемый источник.

## Capabilities

### New Capabilities

_Нет._

### Modified Capabilities

- `delivery-roadmap`: план поставки получает полный единый backlog и правила
  его актуализации.

## Impact

Изменяется только процесс и нормативная документация. Runtime, публичные API,
YAML authoring contract, сохранённые Runs и исторические evidence не меняются.

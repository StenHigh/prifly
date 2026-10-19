## Why

Issue [#13](https://github.com/StenHigh/prifly/issues/13) показывает: после отмены writer и inspected owner `run resolve` Run остаётся failed с `cancel_requested`, а reviewed continue отказывает даже при отсутствии неизвестных эффектов. Нужен поддержанный повтор после явной резолюции без ослабления запрета на blind retry.

## What Changes

- Разрешить reviewed recovery нового Run после отмены и доказанной owner resolution writer; сохранить исходный Run, cancellation facts, original inputs/base, decisions и claim.
- Проверять единственную доказанную failure chain, включая вложенные call/repeat, и journal резолюции; unknown effects, прочие Stops и активная работа сохраняют отказ.
- Исполнять writer и все последующие quality stages заново. `applied` не превращает partial output в принятый результат.
- Сохранить native запрет занятого output path и materialization mismatch; описать owner действие для partial PLAN и повторную подготовку после него.
- Добавить native runtime/CLI регрессии и обновить опубликованную семантику, authoring index, справочник и runner instructions.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `domain-execution`: eligibility и точка reviewed recovery отменённого writer после резолюции.
- `runtime-resources`: сохранение дерева и native output/materialization guards при повторе.
- `cli-protocol`: `project continue/recover --prepare`, named refusals и owner маршрут.
- `published-contracts`: совместимость существующих cancellation/resolution facts и recovery plan/provenance без переписывания схем.

## Impact

Изменение product runtime, tests и пользовательской документации; OpenSpec остаётся процессом разработки. Затронуты `internal/runtime/recovery*.go`, Project CLI tests и инструкции runner. Новых dependencies и переименований сохранённых JSON нет. Current source sets — соответствующие `openspec/specs/<capability>/spec.md`; нормативная ownership не меняется ни у одной capability. Historical evidence, archived changes и старые public bundles защищены. Canonical classic/profiled и внешние consumer packages не меняются; release qualification и закрытие E3 требуют отдельной проверки выпущенного binary.

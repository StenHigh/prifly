# Ownership и cycles у nested extension points

**Дата:** 2026-11-01
**Файлы:** `cmd/prifly/project_extension_points.go`, `project_extension_points_test.go`
**Серьёзность:** medium

## Проблема

Valid package folder `.prifly/workflows/project` ошибочно получал
`project_extension_point_invalid`. Отдельные cycle guards пропускали замкнутую
dependency цепь через legacy root Call и nested point; compile выдавал
`project_compile_unresolved_values` вместо `call_cycle`.

## Первопричина

Ownership проверялся по подстроке `/project/`, без привязки к package folder.
Каждый cycle guard читал только собственные insertion edges. Объединение двух
acyclic subgraphs может содержать cycle.

## Решение

Compiler вычисляет exact project subtree prefix из root document source.
Общий point dependency graph включает legacy root step/Call edges.
Отказы остаются typed; дополнительный stdout logging не вводится, согласно
project minimal logging contract.

## Проверка

`TestProjectExtensionPointsOwnershipIsRelativeToPackageFolder` и
`TestProjectExtensionPointsCycleIncludesLegacyRootCall` сначала падали.
Оба прошли после исправления. Related point/legacy Call tests также прошли.
Full verify batch с дефектом прерван; успешное evidence для него не заявлено.

## Предотвращение

- Проверять ownership относительно известного root, а не по имени сегмента.
- Проверять cycle на объединении всех поддержанных композиционных edges.
- Включать valid package names, совпадающие со служебными именами, в regression.

## Теги

`#authoring` `#ownership` `#cycles` `#extension-points`

Дополнительная regression: JSON decoder принимал null как отсутствие optional
scalar и скрывал двойной selector. Embedded closed schemas теперь проверяют
новые authoring forms на intake. Пять null/double-selector cases сначала
падали, затем прошли. Старые extension-v1/v2 проходят прежний parser.

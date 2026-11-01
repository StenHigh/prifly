# Идентичность унаследованной группы сохраняется при arrange

**Date:** 2026-11-01
**Files:** cmd/prifly/project_composition_group.go, cmd/prifly/project_composition_inherited_groups_test.go, test/e2e/verify-workflow-composition.py
**Severity:** critical

## Problem

Повторное группирование могло удалить mandatory checks. Native regression на прежнем candidate дошёл до synthetic commit без applicability. Повторный group ID также скрывал группу. Общий origin внутри разных групп мог исполняться повторно.

## Root Cause

Emission сравнивал selector всей группы с flattened leaves. Повторные IDs схлопывались через emitted map. Проверка dedup видела только top-level contributions и не учитывала origins внутри групп.

## Solution

Matching проверяет named container перед рекурсией. Compiler проверяет уникальность group IDs до emission и сохранение каждой новой группы после него. Recursive validation запрещает пересечения origins и ограничивает effective depth, включая inherited groups. Неизменённая общая группа по-прежнему deduplicates через общий ancestor.

## Prevention

Проверять путь вызовов от compiled root, а не наличие компонентов в inventory. Native assertions должны требовать выбранный mandatory audit. Отдельно проверять только inherited groups, несколько групп, unnamed wrappers, duplicate IDs и shared origins. Нельзя удалять внутренний member ради dedup.

## Tags

`#composition` `#mandatory-checks` `#group-identity` `#regression-first`

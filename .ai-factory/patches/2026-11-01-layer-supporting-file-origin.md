# Supporting files слоя сохраняют каталог объявления

**Date:** 2026-11-01
**Files:** cmd/prifly/project_composition.go, cmd/prifly/project_composition_files_test.go
**Severity:** high

## Problem

Layer binding читал supporting files относительно workflow folder. При совпадении имён compiler закреплял другие bytes. Без одноимённого файла compilation отказывала.

## Root Cause

Resolver сохранил origin package для программы, хотя layer объявляет собственную базу относительных путей. Старые fixtures проверяли только programs provider и extend рядом с workflow.

## Solution

Program сохраняет каталог фактического source документа. Прежний confined reader продолжает проверять путь. Regression test сначала воспроизвёл missing-file и decoy-file случаи. Проверка также сравнивает sealed bytes, provenance, build identity и отказ на symlink.

## Prevention

Для любого вынесенного authoring документа проверять две разные папки с одинаковыми именами supporting files. Ревью должно прослеживать origin до последнего чтения файла, включая freshness и provenance.

## Tags

`#composition` `#source-origin` `#files` `#regression-first`

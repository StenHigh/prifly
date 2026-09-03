## Why

Единый YAML authoring route нельзя считать устойчивым, если его подтверждают
только Go unit tests рядом с parser. Отдельный corpus с файлами, которые видит
автор, должен запускать собранный CLI и ловить расхождение между документами,
fixtures и внешним contract.

## What Changes

- Добавить самостоятельные positive и negative `.prifly` YAML fixtures.
- Добавить black-box verifier, который копирует fixture, создаёт local core
  authority и вызывает только public `project workflows` и `project compile`.
- Включить corpus в существующий e2e gate и описать его границу в `test/`.

## Capabilities

### New Capabilities

_Нет._

### Modified Capabilities

- `workflow-and-context`: YAML authoring surface получает независимый
  проверочный corpus наряду с unit tests.

## Impact

Изменяются test fixtures, один developer-only Python verifier, `Makefile` e2e
sequence и документация тестов. Runtime, Project source format, package bytes,
authority semantics, external systems и зависимости не меняются.

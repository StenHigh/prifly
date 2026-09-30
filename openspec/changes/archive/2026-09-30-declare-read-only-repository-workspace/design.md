## Context

См. proposal.md. StepDefinition v11 уже несёт независимую границу `external_write`, а assisted handoff уже умеет передавать claim ID, mode и `repository_workspace`. Project launch сейчас выводит потребность в Git только из `workspace_write` и `workspace_trees`; runtime даёт путь read-only шагу только при materialize-only binding.

## Goals / Non-Goals

**Goals:** объявление чтения принадлежит самому шагу, проверяется до Run и не меняет effects. Старые definition bytes и сценарии без Git сохраняются.

**Non-Goals:** запуск команд за агента, общий файловый sandbox, новая версия state/session DTO или интерпретация внешних систем.

## Decisions

1. `repository_workspace: read_only` живёт в StepDefinition v13, который наследует assisted line v11; v12 остаётся отдельной программной веткой. Это сохраняет закрытые v11/v12 и не привязывает чтение к `external_write`. Альтернатива — вывести чтение из effect либо launch-wide flag — либо принудительно потребовала бы Git для задач с API, либо раскрыла бы claim шагам без декларации.
2. Project compiler выводит Git requirement из поля шага и применяет существующий `workspace` launch/flag. Start требует связанный claim до dispatch. Handoff использует существующие claim fields; новые DTO не нужны, поскольку это ранее объявленная форма. Право записи по-прежнему следует только из `effects.class`, а существующая Git mark проверяет read-only отчёт.
3. `read_only` вместе с `workspace_write` отказывается как противоречивая декларация. Программная ветка и старые authoring markers поля не получают.

## Risks / Trade-offs

- [Новый Run с v13 не читается старым бинарником] → обычная versioned compatibility boundary: выпускать и обновлять binary до запуска такого package, не обещать downgrade.
- [Claim создан, но шаг не получил путь] → end-to-end test проходит compile, prepare, Start, `session task` и проверяет точный claim и отсутствие write permission.
- [Host меняет дерево при внешней операции] → существующая mark/refusal проверяется тестом для v13; `external_write` не снимает этот запрет.

## Migration Plan

Сначала тесты и `make ci-check`, затем выпуск совместимого бинарника; только после установки подключать launch с v13. Старые packages не переписываются. Для отката не запускать новые v13 Runs на старом бинарнике; сохранённые Runs остаются под их pinned contracts.

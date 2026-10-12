# Входы локальных ворот

Исходный HEAD реализации: `e4487d7e89391241e9bef8d690a41df1ac7c8ad0`.
Protected paths: archived OpenSpec changes, schemas, protocol.schema.json и historical release evidence. Ownership source sets не меняются.

| Targets / inputs | Правило | Негативная проверка |
|---|---|---|
| test, vet, staticcheck, fmt/refusal | Все tracked bytes/modes, удаления и добавления, Go compiler/stdlib, compiler resources, SDK headers, native linker, actual tools, flags/environment | dirty bytes при том же status, смена Go/context, mode/deletion |
| schemas-check | Все Go inputs, schemas, generator/script bytes | изменённая schema/script |
| release-ci-check | Makefile, workflows, installer и Python guard | workflow/Makefile bytes |
| scripts-check, shard-check | Scripts/tests, Go test inventory, fixtures | новые test/fixture/untracked |
| README, examples, glossary, OpenSpec | Все tracked/untracked файлы включены без фильтра по расширению | изменение документа инвалидирует успех |
| Ignored/external inputs | Ignored файлы вне build output включены; selected Go и .tools хешируются отдельно | ignored source и local tool edits |
| Build/module caches, node_modules | Исключённые generated artifacts; package dependencies закреплены go.sum; mod integrity проверяется свежим go mod verify при reuse | отсутствие/повреждение evidence log не даёт hit |
| Permission context | Caller явно называет CHECK_CONTEXT; неизвестный context — execute-only. Context label не выдаёт права; sandbox/escalated вызовы должны иметь разные labels | разные/пустые labels |
| Environment / tools | Salted hashes полного environment без volatile shell/make bookkeeping; tool paths/bytes, platform и uid; generated Go debug-prefix workdir нормализован, остальные compiler flags сохранены | неизвестная env переменная, новый tool; external/test override отключает reuse |
| Индекс commit | При --for-commit changed staged files обязаны совпадать с рабочими bytes/modes | partial staging mismatch |
| vuln-check | Fresh при каждом обращении; срок исходного batch не продлевается | новый failure отменяет cached pass |

Helper — cooperative evidence в одном worktree; не proof аппаратной изоляции и не release qualification. Не переносить success.json между worktrees. Неизвестные symlink/non-file inputs отключают reuse.

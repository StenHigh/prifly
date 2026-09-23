## 1. Устойчивый routing gate

- [x] 1.1 Добавить минимальный regression test compiled `aif-classic`: verify/review с `needs_revision` и repairable `gate` выдаёт `$aif-fix`, а owner-only gate завершает round; подтверждено до правки: оба route вели в `inconclusive`.
- [x] 1.2 Направить `needs_revision` verify/review с required gate через existing decision, сохранить terminal routes для owner-only/unresolved outcomes и поднять versions affected package components; regression test проходит после правки.

## 2. Контекст host и поставка package

- [x] 2.1 Уточнить verify/review bridges и README: host возвращает gate и завершает лишь свой Attempt, Pri-Fly выдаёт `$aif-fix` и повторный gate; текстовый assertion больше не находит terminal repair bypass.
- [x] 2.2 Поднять authoring versions affected package components и выполнить `project compile` для обоих declared hosts; `origin` сохранён как provenance upstream base, а compile сформировал новые build keys без изменения historical Run/evidence.

## 3. Control loop host

- [x] 3.1 Добавить в `prifly-run` явную таблицу `run next` → действие, включая обязательный цикл после session submit и запрет ручного перехода между skills; текстовый scenario покрывает control, program, assisted_session, waiting и terminal.
- [x] 3.2 Описать separate-session capability как optional и честный fallback в host session с unavailable provenance; regression assertion подтверждает, что runner не обещает subagent без механизма.

## 4. Проверка

- [x] 4.1 Выполнить regression fixture, соответствующие Go tests, `openspec validate harden-aif-classic-gate-routing --strict` и `git diff --check`; `TestAIFClassic*`, runner upgrade checks, build и compile обоих hosts прошли, старый Run не менялся.

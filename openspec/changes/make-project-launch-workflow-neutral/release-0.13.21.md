# Выпуск Pri-Fly 0.13.21

Решение владельца: правила проекта для host runner'а живут в отдельном файле,
и при противоречии он **сильнее** сгенерированного текста.

## Что было

Runner пилота — своя редакция сгенерированного `SKILL.md` (258 строк от
эталона, «свести хост с нормой»). `project runners update` верно отвечал
`project_runner_conflict`, и следствие — каждый выпуск с новым текстом
раннера (0.13.19 сменил все три хоста) они переносили руками. Пакетчик назвал
это пробелом, не отказом: обход отсутствующего механизма.

## Что стало

Рядом с сгенерированным `prifly-run/SKILL.md` может лежать `PROJECT.md` —
надстройка проекта. Сгенерированный текст называет его в первых строках,
условно и адресно:

```
If PROJECT.md exists beside this file, read it right after this text: it is
this project's addition to the runner, and prifly project runners update
replaces this file only, never PROJECT.md. PROJECT.md wins where the two
conflict; it cannot widen what the engine enforces -- declared effects,
claimed workspaces and output slots are measured by the engine, not read from
text.
```

`update` заменяет только `SKILL.md`; `PROJECT.md` остаётся байт-в-байт. Хост
загружает только `SKILL.md` (progressive disclosure из спецификации навыков —
подтверждено обеими сессиями), соседний файл читает исполнитель по этому
указанию; у пилота такая форма держится месяцами (overlay warmup, роутеры
навыков) при условиях, которые текст соблюдает: первые строки, императив,
точный путь, «если существует», явное старшинство.

Тест — на два свойства, потому что каждое поодиночке зелёное ничего не читая:
после `update` `PROJECT.md` не тронут **и** сгенерированный текст его называет
до первого шага протокола. Разрезы: указание на другой файл — красный на
втором свойстве; текст без абзаца — `updated_hosts: []`.

Текст раннера 0.13.19–0.13.20 заморожен вариантом 9 (его digest'ы — прежние:
codex-cli `sha256:9a3be36e…`, codex-app `sha256:28347f95…`, claude-code
`sha256:9796f270…`); `update` берёт его. Digest'ы **текста 0.13.21**:
codex-cli `sha256:d8b32fac…`, codex-app `sha256:aff816b5…`, claude-code
`sha256:9cc0bf9b…`.

## Миграция кастомного runner'а

`diff` своей редакции против эталона (снять `project init` во временном
профиле) и есть содержимое `PROJECT.md`; после переноса `SKILL.md`
возвращается к эталону, и `update` снова принимает его. Разово.

## Ворота

`make check` (race), `make e2e`, CI `verify`; текст раннера — digest-pinned.

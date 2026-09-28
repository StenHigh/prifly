## 1. Объявление

- [ ] 1.1 `resumable` в WorkflowRevision 8: схема `workflow-revision-v8`, авторинг и схемы редактора, лестница выводит 8, отказ пустого объявления и явной ревизии 7 с полем; ревизии 1–7 и опубликованные схемы прежние. Возможность `workflow_resume` в capabilities и строка в `examples/README.md`.

## 2. Возобновление

- [ ] 2.1 Сначала тесты: источник `partial` без объявления — отказ; с объявлением — план с точкой на стадии finish-ребра; отменённый — точка на отменённой стадии; `--from-stage` раньше точки — перенос обрезан; `--from-stage` не раньше или не завершённая — отказ; пустой префикс — отказ; неразрешённая execution — отказ.
- [ ] 2.2 Обобщить `recoveryFrontier`/`recoveryTrace`: источник по `resumable`, точка `step`/`call`/`repeat`, перенос по журналу до точки, принятые программные шаги переносятся; `fork.reason` `resume_stopped_run`.
- [ ] 2.3 Настоящий Run: workflow с `call`-стадией, завершившийся `partial` через `blocked` на ней, возобновлён тем же launch; перенесённые стадии не исполняются, `call` исполняется заново, дерево передано, исход нового Run по маршруту.

## 3. CLI и run next

- [ ] 3.1 `project continue` с launch того же workflow идёт путём возобновления, `--from-stage`; `project recover` принимает `--from-stage`; prepare показывает точку, перенос и дерево; справка и инструкция раннера (прежний текст раннера заморожен).
- [ ] 3.2 `continuations` в `run next` включает собственный `resumable` workflow; тесты.

## 4. Документация и пример

- [ ] 4.1 Руководство: раздел «Возобновить тем же workflow» в `continuation-guide.md` и путь в `blocked-guide.md`; справочник полей ревизии 8; `examples/troubleshooting.md` — отказы.
- [ ] 4.2 Пример `blocked-condition`: `resumable` в исходном workflow и шаг README «продолжить тем же launch»; приёмочный тест прогоняет его.

## 5. Проверка и выпуск

- [ ] 5.1 `make check`, `make race`, `make e2e`, `git diff --check`, `openspec validate --strict`; verify и race на GitHub; релиз.
- [ ] 5.2 Сообщить сессии пакета форму объявления, команду и номер релиза.

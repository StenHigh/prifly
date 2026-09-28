## 1. Объявление

- [x] 1.1 `resumable` в WorkflowRevision 8: схема `workflow-revision-v8`, авторинг и схемы редактора, лестница выводит 8, отказ пустого объявления и явной ревизии 7 с полем; ревизии 1–7 и опубликованные схемы прежние. Возможность `workflow_resume` в capabilities и строка в `examples/README.md`.

## 2. Возобновление

- [x] 2.1 Сначала тесты: источник `partial` без объявления — отказ; с объявлением — план с точкой на стадии finish-ребра; отменённый — точка на отменённой стадии; `--from-stage` раньше точки — перенос обрезан; `--from-stage` не раньше или не завершённая — отказ; пустой префикс — отказ; неразрешённая execution — отказ.
- [x] 2.2 Обобщить `recoveryFrontier`/`recoveryTrace`: источник по `resumable`, точка `step`/`call`/`repeat`, перенос по журналу до точки, принятые программные шаги переносятся; `fork.reason` `resume_stopped_run`.
- [x] 2.3 Настоящий Run: пример `blocked-condition` (стадия `prepare`, затем `repeat` вокруг вложенного вызова), завершившийся `partial` через `blocked`, возобновлён тем же launch; `prepare` перенесён и не исполняется, `repeat` с вложенным телом исполняется заново, исход нового Run по маршруту (`TestCLIBlockedExampleAcceptance`). `call`-точка идёт тем же путём старта (`startStage`), что и `repeat`; передача дерева — `TestCLIContinuationTakesOverTheSourceTree` и `TestRecoveryFindsTheSourceTree`.

## 3. CLI и run next

- [x] 3.1 `project continue` с launch того же workflow идёт путём возобновления, `--from-stage`; `project recover` принимает `--from-stage`; prepare показывает точку, перенос и дерево; справка и инструкция раннера (прежний текст раннера заморожен).
- [x] 3.2 `continuations` в `run next` включает собственный `resumable` workflow; тесты.

## 4. Документация и пример

- [x] 4.1 Руководство: раздел «Возобновить тем же workflow» в `continuation-guide.md` и путь в `blocked-guide.md`; справочник полей ревизии 8; `examples/troubleshooting.md` — отказы.
- [x] 4.2 Пример `blocked-condition`: `resumable` в исходном workflow и шаг README «продолжить тем же launch»; приёмочный тест прогоняет его.

## 5. Проверка и выпуск

- [x] 5.1 `make check`, `make race`, `make e2e`, `git diff --check`, `openspec validate --strict`; verify и race на GitHub; релиз.
- [ ] 5.2 Сообщить сессии пакета форму объявления, команду и номер релиза.

Локально 2026-09-28: `make check` (с `race`) — exit 0, `workflow-revision-v8` 18991 байт `sha256:99fa3a51…`; `make e2e` — exit 0; `git diff --check` и `openspec validate --strict` — чисто. GitHub 2026-09-28 на `6937373`: `verify` 36415827660 — первый прогон красный на `TestCLIProjectSessionLimitsPrepareShowsPinnedPolicies` (очистка TempDir: `directory not empty`, тест изменением не затронут), перезапуск зелёный; `race` 36415829237 — зелёный. Релиз `v0.13.61` — run 36417317953, окружение `release` одобрено, 6 ассетов.

0.13.62 (`de0601c`, 2026-09-28): сессия пакета нашла на 0.13.61, что workflow, меняющий дерево, не возобновляется с launch без `workspace:` (`project_start_workspace_required`, а `--workspace` — `resume_input_override`); у `project recover` та же ловушка. Режим дерева теперь берётся у исходного Run (`TestCLIResumeTakesTheSourceTreeModeWithoutAStandingWorkspace`, красный на 0.13.61). По решению владельца ответы анкеты без переданных ответов берутся из исходного Run (`TestCLIResumeTakesTheSourceRunsAnswers`, красный без изменения). `make check` — exit 0; GitHub `verify` 36420579909 и `race` 36420579906 зелёные с первого раза; релиз — run 36422174861, 6 ассетов.

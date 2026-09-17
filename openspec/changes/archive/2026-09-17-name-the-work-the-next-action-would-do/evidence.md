# Evidence — name-the-work-the-next-action-would-do

Ворота на финальном дереве, 2026-09-17 (Apple M1, go1.27.0):

- `make ci-check` exit 0: fmt-check 291 файл, refusal-check 160 файлов,
  staticcheck 9 пакетов — находок нет, vuln-check 9 пакетов — уязвимостей нет,
  schemas-check — все bundle'ы совпали, в том числе новый `stage-work`
  (233 792 байта, `895fbf72…`); `materialized-session` (/30) и остальные — байт
  в байт прежние.
- `make e2e` exit 0: 6 наборов `passed`.
- `make race` exit 0 (первый прогон этого изменения: `cmd/prifly` 266.2 с,
  `internal/flow` 48.8 с, `internal/runtime` 907.9 с, гонок нет).
- `openspec validate --all --strict`: 21 passed, 0 failed.

Тесты, которых не было:
- `TestNextNamesTheWorkAReadyStageHolds`: готовая ассистируемая стадия
  читается как `stage_work: assisted_session`; чтение не допускает работу
  (попыток 0 после `Next`); у выданного задания род не называется, а
  `safe_next_actions` несёт `session.task`; после submit следующая стадия
  снова названа.
- `TestStageWorkNamesControlAndStaysSilentWhenItCannotTell`: `finish` —
  `control`; неизвестная стадия и нечитаемый план — пустое значение, не догадка.

Побочная починка того же выпуска (не этот change, дефект внутри объявленного
обещания): `project workflows update` терял бит исполнения у файлов папки
`project/`. Тест в `TestCLIProjectWorkflowsUpdateAndRemove` красный без правки
(«lost its executable bit»), проверяет обе стороны — воркер остаётся
исполняемым, обычный файл исполняемым не становится.

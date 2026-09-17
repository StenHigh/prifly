## Context

См. proposal.md — Why. Сегодня binding дерева живёт только на `workspace_write`
шаге (`internal/flow/compile.go`, `checkWorkspaceTrees`: версии 5–7 и класс
эффекта), схема binding'а требует `output_port` и `capture`
(`internal/flow/schema.go`, `step-definition-v7`), а runtime materialize-ит
вход только внутри такого binding'а и берёт отпечаток рабочей копии read-only
шага до handoff (0.13.19). Все три места придётся расширить согласованно;
опубликованные bundle'ы v5–v7 заморожены по digest.

## Goals / Non-Goals

**Goals:**
- Read-only assisted шаг читает захваченное дерево по declared location в
  claim-worktree того же Run, без права записи и без захвата.
- Отказ `effect_not_permitted` для такого шага измеряет только то, что сделал
  host: материализация движка не считается его изменением.
- Старые контракты (v5–v7, guide/1, сохранённые Run) не меняют ни байта, ни
  смысла.

**Non-Goals:**
- Материализация в отдельное дерево шага (aif-verify ищет план от корня
  repository — отдельное дерево бесполезно).
- Дерево на вход для шага-программы (`operation: process`): у программы свой
  контекст, запрос не поступал.
- Проводка `plan` в verify/review пакета — сторона `aif-classic` (1.38.0 у
  пакетчика после выпуска).

## Decisions

1. **Форма — та же запись `workspace_trees[]` без `output_port`, не новое
   поле.** Альтернатива — отдельный список `workspace_inputs` — удвоила бы
   guide, проверку путей и текст runner'а ради одного бита; отсутствие
   `output_port` уже однозначно означает «захвата нет». Версия StepDefinition
   поднимается до v8, потому что schema v7 объявляет `output_port`
   обязательным, а опубликованная схема не расширяется молча
   (`published-contracts`).
2. **Только на `effects.class: none`.** На `workspace_write` шаге чтение без
   захвата оставило бы изменения дерева без manifest — compiler отказывает.
   Обратное (binding с `output_port` на read-only шаге) уже отказ.
3. **Отпечаток после материализации, снятие после settle.** Порядок в
   `driver`/`effects`: materialize → mark → handoff → submit (сверка с mark)
   → settle → cleanup. Альтернатива «исключить materialized пути из сверки»
   слабее: она пропустила бы правку materialized entry самим host'ом; сверка
   по отпечатку после материализации ловит и это. Cleanup — уже существующий
   `entryCleanup` binding'а.
4. **`repository_workspace` в SessionTask read-only шага.** Поле есть в
   `assisted-session/7` как optional; заполняется теперь и для read-only шага с
   materialize-only binding'ом (та же claim, тот же `workspace_mode`). Для
   read-only шагов без binding'а поведение не меняется — не расширять тихо.
5. **Guide `workspace-tree-guide/2`.** Запись без `output_port` — новая форма
   для host, поэтому версия guide поднимается; текст guide говорит: «порт не
   объявлять, дерево прочитать по location». Сам envelope
   `assisted-session/7` не меняет форму.
6. **State/read 30 (`core-state/30`, `core-read/30`) вместо «state не
   меняется».** Измерено при реализации: опубликованный bundle
   `effects-session` (/29) закрывает `runtime_WorkspaceTreeHandoff`
   (`additionalProperties: false`, `output_port` обязателен), а handoff'у
   нужно поле `materialized_entries` — список того, что положил движок, чтобы
   снять после settle ровно это, а не файл, который в дереве уже был (для
   `exact_file` файл после захвата освобождается движком, для bundle —
   остаётся). Значит новая граница; сохранённые Run /29 читаются как прежде,
   новые assisted Run пишутся под /30. Ту же границу использует change
   `report-run-failure-in-the-read-envelope` (`failure` у RunView).
7. **Отпечаток не видит содержимого untracked-файлов** (`git status` даёт
   только присутствие), поэтому при отчёте read-only шага байты materialized
   entries сверяются с pinned digest отдельно; правка — `effect_not_permitted`
   с путём.
8. **Один bundle `step-definition-v8.schema.json`**, сгенерированный
   `cmd/schema-gen` из тех же Go-типов с `output_port` как optional только для
   v8; `scripts/check-schema.py` получает новую запись; список digest-pinned
   bundle'ов v5–v7 не трогается.

## Risks / Trade-offs

- [Cleanup после settle падает (файл занят, права)] → попытка уже settled;
  отказ cleanup становится named diagnostic Run, а не молчанием; повторная
  материализация следующего шага проверяет bytes и откажет drift'ом, если
  файл остался другим.
- [Host правит materialized entry] → сверка отпечатка после материализации
  ловит это как `effect_not_permitted` с именем пути — то же поведение, что у
  любого файла.
- [Runner text старых хостов не знает формы без `output_port`] → guide/2
  объясняет её текстом рядом с манифестом; runner не меняется.
- [Пакет объявит materialize-only binding на v7] → отказ компиляции с
  версией в тексте; пакетчик поднимает шаги verify/review на v8 в 1.38.0.

## Migration Plan

Выпуск движка с v8 → пакетчик прогоняет ворота и стенды на кандидате →
`aif-classic` 1.38.0 объявляет `plan` входом verify/review. Откат: пакет на
1.37.0 компилируется прежним движком и новым без изменений; сохранённые Run
не затронуты.

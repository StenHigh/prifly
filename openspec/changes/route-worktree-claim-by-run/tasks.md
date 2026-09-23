## 1. Регрессия и исправление

- [x] 1.1 Воспроизвести `claim_ambiguous` при двух Run с отдельными worktree claims.
- [x] 1.2 Атомарно привязать claim при Project Start, выбирать claim текущего Run и узнавать legacy Project claim только по точным сохранённым identities.
- [x] 1.3 Проверить первую и повторную выдачу обеим сессиям, старый непривязанный Project Run и отказ чужому Run.

## 2. Приёмка

- [x] 2.1 Выполнить целевые Go tests, OpenSpec validation, `make refusal-check`, сборку CLI и `git diff --check`; не объявлять защиту общего тестового стенда.
- [ ] 2.2 На authority с Runs #142 и #155 проверить read-only состояния и claims; после установки исправленного бинарника продолжить оба Run через обычный `run next`/`run drive` без освобождения чужих claims и записать исход.

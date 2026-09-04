## 1. Инвентаризация незавершённой работы

- [x] 1.1 Сверить current `delivery-roadmap`, archived delivery crosswalk и legacy roadmap из Git; классифицировать каждую прежнюю запись как active work, formal milestone, future catalogue или historical evidence и проверить, что evidence не копируется в current plan.
- [x] 1.2 Сверить active OpenSpec changes и известные pilot gaps с active backlog; проверить, что у каждой записи есть статус, prerequisite и следующий шаг.

## 2. Единый OpenSpec backlog

- [x] 2.1 Обновить `openspec/specs/delivery-roadmap/spec.md`: заменить устаревшую короткую очередь на active backlog, полную P1/P2 последовательность и post-P2 catalogue; проверить, что новая запись не меняет runtime scope или formal acceptance status.
- [x] 2.2 Сохранить однонаправленную историческую границу: не восстанавливать `docs/roadmap`, `docs/f2-progress.md` или legacy evidence в рабочем дереве; проверить `git diff --name-only -- openspec/changes/archive` не выводит файлов.

## 3. Проверка документации

- [x] 3.1 Выполнить `openspec validate consolidate-current-delivery-backlog --strict` и `git diff --check`; проверить успешные exit codes.
- [x] 3.2 Проверить `test ! -e docs/roadmap/roadmap.md` и `test ! -e docs/f2-progress.md`; убедиться, что единственный current delivery source остаётся `openspec/specs/delivery-roadmap/spec.md`.

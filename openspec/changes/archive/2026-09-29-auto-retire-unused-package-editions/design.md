## Context

См. proposal.md и три delta specs. `RegistryBudget` считает local entries и компоненты всех resolvable trusted packages; каждая compiled edition continuation добавляет около 58 entries. `package remove` уже переводит edition в `Removed`, защищает незавершённые Runs и сохраняет package bytes, но проверка держателей сейчас выполняется отдельно от изменения package record. Обычный `project start` импортирует package до создания Run. Нормативные source sets: `workflow-and-context`, `runtime-resources`, `cli-protocol` в `openspec/SOURCE-OF-TRUTH.md`.

## Goals / Non-Goals

**Goals:** один воспроизводимый план кандидатов для preview и применения; отсутствие гонки с admission нового Run; возврат места без удаления доказательств.

**Non-Goals:** чистка файлов на диске, Runs, revoked/quarantined packages, чужих package IDs или произвольная глобальная LRU-политика. Явный ручной `package remove` не заменяется.

## Decisions

### Отбирать только старые редакции той же package identity

Считать итоговый бюджет с новой edition. Если лимит не достигнут, никого не выводить из доверия. Иначе выбрать минимальное число trusted editions того же ID по `Imported.UTC`, разрешая равенство exact ref; защищать новую и последнюю предыдущую edition, dependencies и все non-terminal holders. Это ограничивает неожиданное влияние на другие integrations. Альтернатива — глобальная LRU по всем packages — не может доказать, что редакция больше не нужна владельцу.

### Перепроверять план в одной authority-границе с установкой

Read-only planner возвращает candidate refs, reasons, count и digest, связанные с package record version, текущим набором Run holders и target package digest. Применение не использует ранее вычисленный список как полномочие: атомарно повторяет selection под сериализованным доступом к authority и фиксирует retirement вместе с trust новой edition. Start/admission и изменение trust должны конкурировать на одной проверяемой границе; нынешнего внешнего вызова `packageHolders` перед `ApplyAuthority` недостаточно. Если текущий store не умеет такую транзакцию, расширить его точечно; не читать файлы или blobs внутри transform. Альтернатива — сначала `package remove`, потом `package import` — оставляет промежуточное состояние и гонку.

### Сохранять bytes и фиксировать отдельный audit результата

Применение меняет только существующий lifecycle status, освобождает inventory pins и пишет command receipt с exact retired refs. Directory sealed package, manifest и Run snapshots не удаляются. При отказе в освобождении вернуть `dependency_limit` и защищённые причины. Новая versioned CLI projection показывает preview и итог, старые JSON-поля не переопределяются. Откат после установки — явный `package restore` с обычной проверкой коллизий/лимита, не автоматическое переписывание истории.

## Risks / Trade-offs

- [Старую edition планировали использовать завтра] → сохранить одну предыдущую; ручной restore остаётся доступен, а preview называет отставку.
- [Незавершённый Run появляется после preview] → транзакционная перепроверка holders/admission, stale-plan refusal.
- [Несколько package IDs вместе превышают лимит] → fail closed с бюджетом; не удалять чужой package ради одного запуска.
- [Сбой после изменения trust, до создания Run] → отдельный тест crash/replay и компенсация нового trust/retirements в пределах подтверждённой операции; не оставлять скрыто отставленные editions.

## Migration Plan

1. Ввести read-only план и новую versioned проекцию без автоматического применения; проверить старые bundles и JSON readers.
2. Добавить транзакционное применение, dedup и гонки с Run admission на fixture; сохранить все прежние lifecycle operations.
3. Включить для нового Project/package launch после проверки бюджета; на реальном authority проверить preview и повторное открытие исторического Run. Rollback — отключить автоматическое применение, сохранив записанные lifecycle transitions и возможность явного restore.

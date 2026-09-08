## Context

См. proposal.md. Store.StorageUsage считает логические SQLite pages, а не файловую систему. Blobs публикуются до SQL reference commit, поэтому сборка мусора требует защиты всего этого интервала.

## Goals / Non-Goals

Отдельный экран Run; фоновый учёт выделенных блоков; удаление только окончательно завершённых Run, неиспользуемых артефактов и их служебных workspace. Это локальный retention, не cryptographic erasure и не удаление из старых backup. Внешние репозитории и Git worktree не удаляются.

## Decisions

- Hash route переключает список/подробности без пересоздания фильтров. Возврат сохраняет страницу и scroll списка; polling не меняет экран.
- Независимый фоновый обход каждые 30 секунд считает stat blocks, дедуплицирует device/inode и не раскрывает symlink. Размеры приходят отдельным endpoint и не задерживают частый polling Run. Ошибки и время измерения видны; недоступное не становится нулём.
- Каждый Engine держит shared flock immutable installation inode, обслуживание — exclusive. При занятости возвращается отказ без остановки процесса. Это сохраняет атомарную для GC границу blob publication → reference commit; более мелкие command leases понадобятся, если очистка должна пересекаться с долгоживущим driver.
- Preview и delete открывают источник заново, сверяют его физическую/authority identity, владельца и object.read/run.resolve access. POST требует same-origin Origin, случайный session token, JSON и ограниченный payload. GET не изменяет данные.
- Preview проверяет terminal status, settlement всех attempts/checks, unresolved effects/operations и claims. Сохраняющийся fork удерживает исходный Run. Confirmation связывает список и версии Run с точными именами/bytes удаляемых файлов; изменившийся план требует нового подтверждения.
- Журналы и проекции выбранных Run удаляются одной SQL transaction вместе с записью retention audit. Command receipts, authority controls, grants/approval audit сохраняются; audit действий и решений удаляемого Run переносится в запись обслуживания. Shared pinned bytes удаляются только при отсутствии оставшихся packed references.
- GC маркирует digests из оставшихся snapshots/history, receipts, authority state, inventory/config, служебных metadata и независимых import artifacts; проходит provenance и JSON manifests транзитивно. Нечитаемые/повреждённые metadata прерывают preview. Удаляются только не удерживаемые metadata/blob имена внутри os.Root.
- Служебные workspace берутся только из выбранных attempts/checks и должны совпадать с известным путём authority. Файлы сверяются потоковым digest и inode, symlink удаляется как ссылка; каталоги удаляются снизу вверх без RemoveAll. Остальные рабочие папки сохраняются.
- После удаления файлов записывается результат GC, затем SQLite уплотняется. Ошибка этой стадии не отменяет уже удалённую историю: ответ показывает выполненную часть и предупреждения. Незавершённый GC оставляет только лишние файлы, последующая очистка может подобрать остатки.

## Risks / Trade-offs

- Долгоживущий Engine → очистка отказывает до его закрытия; active Run без открытого driver не блокирует удаление другого завершённого Run.
- Кооперативные locks → перед обслуживанием все клиенты authority должны использовать обновлённый binary; старые уже открытые процессы не участвуют в новом протоколе блокировки. Активный старый driver дополнительно проверяется через существующий driver lock. Смешанные старые writers не квалифицированы.
- APFS shared blocks/snapshots → stat blocks не выдаются за точную оценку освобождения; preview считает объём содержимого, после операции размер измеряется заново.
- Неизвестные/независимые файлы → сохраняются; не используется удаление всей папки authority.
- VACUUM требует дополнительного места → failure показан отдельно, receipt/history deletion не объявляется отменённым.
- Браузерная квалификация пока заблокирована административной проверкой Browser Use; серверные и source UI checks не заменяют её.

## Migration Plan

Новых зависимостей или изменения public Run/Problem JSON нет. Обновить CLI и monitor, закрыть старые процессы перед обслуживанием authority. При rollback прежний binary видит сохранившиеся Runs и audit rows; удалённую историю rollback не восстанавливает. Backups остаются ответственностью владельца; erasure/replay recovery ledger не заявляется.

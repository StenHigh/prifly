## 1. Навигация и объём

- [x] 1.1 Реализовать отдельный экран Run, возврат к сохранённому списку и hash history; проверить связность DOM/JS и отсутствие синтаксических ошибок.
- [x] 1.2 Измерять размеры authority фоном отдельно от Run polling; проверить hardlinks, symlink и общий итог в TestMonitorStorageCountsHardlinksAndSkipsSymlinks.
- [ ] 1.3 Вручную проверить длинный список, возврат/Back, размеры, preview/cancel/confirm в браузере. Browser Use отказал из-за недоступной проверки административной политики; source checks этого не заменяют.

## 2. Безопасное удаление

- [x] 2.1 Зафиксировать подтверждённый владельцем scope: массовое удаление только окончательно завершённых Run и полноценный GC; проверить согласованность трёх delta specs.
- [x] 2.2 Реализовать exclusive maintenance, атомарное удаление history/audit и GC с сохранением общих данных; проверить TestMonitorMaintenanceRefusesAndReclaims и Store.Verify.
- [x] 2.3 Добавить защищённый POST и полное подтверждение в UI; проверить отказ чужому Origin/неверному token/GET, stale plan и positive HTTP deletion.
- [x] 2.4 Проверить active/uncertain/pending states, открытый Engine, shared artifacts, imports, orphan uploads, повторы и восстановление служебного workspace после ошибки, не меняя symlink target.

## 3. Завершение

- [x] 3.1 Завершить race/vet/schema/OpenSpec проверки, записать evidence и убедиться, что historical release evidence не менялось.

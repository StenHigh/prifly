## 1. Проверка покрытия

- [x] 1.1 Сверить все 26 правил публичного протокола и их acceptance-связи с replacement-spec; проверить индивидуальную архивную crosswalk-таблицу.

## 2. Переключение источника

- [x] 2.1 Синхронизировать capability `cli-protocol` в постоянные specs, выполнить `openspec validate --specs --strict` и переключить только её строку в source-of-truth map при неизменных legacy-байтах.

## 3. Сохранность и архив

- [x] 3.1 Проверить неизменность кода, схем, evidence и манифестов через `git diff`, архивировать change и выполнить `openspec validate --all --strict --archived`.

## 1. Нормы качества и каталог

- [x] 1.1 Сверить и завершить 16 quality boundaries без legacy IDs; проверить 16 requirement blocks и strict validation.
- [x] 1.2 Перенести scenarios корпуса 001–016 с Given/When/Then и metadata; проверить 16 отдельных scenario blocks.
- [x] 1.3 Перенести scenarios корпуса 017–031 с Given/When/Then и metadata; проверить 15 отдельных scenario blocks.
- [x] 1.4 Перенести scenarios корпуса 032–056 с Given/When/Then и metadata; проверить 25 отдельных scenario blocks.
- [x] 1.5 Перенести scenarios корпуса 057–070 с Given/When/Then и metadata; проверить 14 отдельных scenario blocks.
- [x] 1.6 Перенести scenarios корпуса 071–092 с Given/When/Then и metadata; проверить 22 отдельных scenario blocks.
- [x] 1.7 Перенести scenarios корпуса 093–114 с Given/When/Then и metadata; проверить 22 отдельных scenario blocks.
- [x] 1.8 Перенести scenarios корпуса 115–130 с Given/When/Then и metadata; проверить 16 отдельных scenario blocks.
- [x] 1.9 Перенести scenarios корпуса 131–148 с Given/When/Then и metadata; проверить 18 отдельных scenario blocks и итоговые 148.

## 2. Трассировка и cutover

- [x] 2.1 Создать individual archived crosswalk для 16 quality rules, 148 product cases и их exact links; проверить отсутствие legacy IDs в permanent spec.
- [x] 2.2 Разобрать все 337 строк legacy acceptance map по ownership, сохранить exact archive evidence и проверить partition count 148/189.
- [x] 2.3 Синхронизировать `quality-and-acceptance` в постоянные specs, выполнить `openspec validate --specs --strict` и переключить только её source-map row при неизменных legacy-байтах.

## 3. Сохранность и архив

- [x] 3.1 Проверить через `git diff` неизменность runtime, тестов, схем, evidence, манифестов и статусов cases; архивировать change и выполнить `openspec validate --all --strict --archived`.

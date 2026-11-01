## 1. Локализация и воспроизводимая регрессия

- [x] 1.1 Найти исходную cause `invalid_input` в concurrent `project start`, используя pinned baseline и существующее evidence. Проверка: acceptance.md называет точную фазу, исходную ошибку, interleaving и причину потери/изменения наблюдения; гипотеза package state не выдана за факт.
- [x] 1.2 Закрепить выявленное пересечение операций управляемой регрессией в существующем CLI/runtime test fixture. Проверка: тест падает на baseline по конкретному invariant без sleeps; записаны команда, RED exit и диагностический факт; все временные Runs/claims/registrations очищаются.

## 2. Минимальное исправление

- [x] 2.1 После поиска всех callers исправить общий участок start/наблюдения состояния. Проверка: регрессия из 1.2 проходит; command ID, reviewed values и claim identity сохраняются; нет blanket retry `invalid_input`, увеличения retry counts или блокировки всего lifecycle Run; краткая блокировка команд реестра Git допускается по подтверждённой причине; production diff ограничен подтверждённой причиной.
- [x] 2.2 Проверить concurrent independent и duplicate команды вместе с cleanup. Проверка: разные IDs дают два пригодных Run с разными claims/paths/refs; duplicate даёт один Run без нового suffix; отказ одного caller не меняет refs/bytes/package/claim другого; unknown outcome сохраняется; changed payload остаётся conflict. Существующие tests переиспользованы, новое покрытие добавлено только для пробела.

## 3. Повторяемость и локальная интеграция

- [x] 3.1 Выполнить публичный multi-process тест фиксированным batch из 50 повторов: `TMPDIR=/private/tmp GOMAXPROCS=2 go test ./cmd/prifly -run '^TestNamedWorktreeConcurrentCLIProcessesFromPublishedExample$' -count=50`. Проверка: 50/50 без проигнорированных failures; Linux CI проверяет тот же сценарий отдельно; flakes не скрыты повтором batch.
- [x] 3.2 Выполнить `make test-changed GO=ACTUAL_GO` и применимые targeted runtime/local tests. Проверка: stdout/exit и точная Go version записаны в acceptance.md; typed refusals, pure transforms и testguard проходят; прежние naming/review/duplicate отказы сохранены.

## 4. Итоговые ворота и возврат к readability

- [x] 4.1 Проверить `openspec validate fix-concurrent-named-worktree-start --strict`, `git diff --check` и protected historical paths. Проверка: archived changes, historical release evidence/manifests, saved contracts и installed `.prifly` не изменены этим fix; новые предметные понятия и schema/state versions отсутствуют.
- [x] 4.2 Проверить staged snapshot командой `TMPDIR=/private/tmp GOMAXPROCS=2 make ci-check-once GO=ACTUAL_GO CHECK_CONTEXT=ACTUAL_PROFILE CHECK_FOR_COMMIT=1`. Проверка: полный batch зелёный, staged bytes равны проверенным; failure из baseline остаётся в evidence; race/e2e qualify и релиз не запускаются.
- [x] 4.3 После успешного fix отдельно продолжить `improve-workflow-authoring-readability`. Проверка: его 5.2 закрывается только на проверенном snapshot; человеческие 1.2/4.1/4.2 и human-часть 3.1 не закрыты этим fix; статус 5.1 отражает решение владельца по handoff. Не отправлять `pass` от текущего implement Attempt до выполнения его исходных требований.

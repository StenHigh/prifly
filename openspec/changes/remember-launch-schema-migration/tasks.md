## 1. Контракт и исходные отказы

- [x] 1.1 Добавить CLI regression: повторный запуск без ID после owner выбора. Сначала подтвердить отказ текущего binary и отсутствие Run/постоянного claim.
- [x] 1.2 Обновить glossary и agent brief для LaunchSchemaMigrationChoice. Проверка: `TestGlossaryBindings` и смысловое ревью различий Choice, Decision, Binding, Selection.
- [x] 1.3 Добавить closed schemas объекта выбора, inspection и command result. Проверка: unknown version/field отказывает, отсутствие объекта читается без записи.

## 2. Authority хранение и owner commands

- [x] 2.1 Реализовать scoped объект с generation, включая tombstone после clear. Проверка: reopen authority сохраняет запись; другой project/repository/launch её не получает.
- [x] 2.2 Реализовать read-only inspection записи и текущей сборки. Проверка: inspection сообщает причины, остаётся доступным при ошибке compile и не меняет state.
- [x] 2.3 Реализовать set/replace с exact build/consumer/workflow и current Decision checks. Проверка: stale build key, foreign Decision, scope mismatch и lifecycle changes не записывают выбор.
- [x] 2.4 Реализовать clear без compile и с owner/generation checks. Проверка: invalid choice и исчезнувший launch удаляются; старый generation получает отказ.
- [x] 2.5 Проверить command retry, command conflict, два конкурирующих set и потерю owner права. Проверка: один commit на generation, прежний receipt для принятого retry, отсутствие записи при отказе.
- [x] 2.6 Подключить precondition выбора к атомарному принятию Run. Проверка: замена между чтением CLI и commit блокирует Run; guards `internal/purity` проходят.

## 3. Public CLI и reviewed launch

- [x] 3.1 Добавить `project schema-migration inspect|set|clear` и flags из design. Проверка: public help, Problem codes, ожидаемый JSON и типизированные отказы совпадают.
- [x] 3.2 Подключить общий resolver в prepare/start, включая существующий путь continue/recover нового launch. Проверка: explicit выше saved; invalid explicit не вызывает fallback; пустое значение означает отсутствие explicit.
- [x] 3.3 Добавить source, scope и saved generation в новые summary/start DTO и review digest. Проверка: реальные JSON проходят новые closed schemas; старые bundles неизменны.
- [x] 3.4 Требовать launch digest для saved start и повторно проверять выбор. Проверка: replace, clear, clear/set того же ID, смена source и смена сборки инвалидируют review до Run/постоянного claim.
- [x] 3.5 Сохранить принятую start command при retry после изменения выбора. Проверка: повтор возвращает прежний Run; новый ID команды требует нового действующего review.

## 4. Сквозные проверки и документация

- [x] 4.1 Воспроизвести несколько последовательных Runs одной сборки без ID через native CLI в новой временной authority, включая reopen процесса. Проверка: каждый prepare показывает saved, каждый Run имеет правильный Binding.
- [x] 4.2 Проверить отрицательную матрицу: другие launch/project/authority; consumer/workflow/build drift; изменение compile values, host-selected bytes и profile; повторный revoke, remove/quarantine supplier, byte tampering, revoked code и старый Run. Проверка: точные причины, исходный package остаётся revoked, нет переноса допуска.
- [x] 4.3 Проверить совместимость: нет выбора, explicit путь, standalone run start, legacy profile и существующие continuation/recovery ограничения. Проверка: прежние версии ответов без выбора и прежние Run bindings сохраняют смысл.
- [x] 4.4 Обновить public help, `examples/operations/{index,commands,launch,schema-migration}.md`, `examples/authoring/schema-migration-guide.md` и embedded assets. Проверка: оператор видит set → prepare → start без копирования ID, inspect/replace/clear и минимальную версию Core.
- [x] 4.5 Обновить capability индексы при объявлении новой capability и ссылку в `SOURCE-OF-TRUTH.md` без смены ownership. Проверка: `TestEveryDeclaredCapabilityIsInTheAuthorIndex`, handbook freshness и authoring reference показывают supported интерфейс.

## 5. Итоговые ворота

- [x] 5.1 Во время реализации выполнять `make test-changed` либо `make check-fast`. Для новых безопасных Go tests использовать `t.Parallel()`; проверка: затронутые tests и testguard проходят.
- [x] 5.2 Проверить change командой `openspec validate remember-launch-schema-migration --strict`, затем `git diff --check`. Проверка: оба exit code равны 0.
- [x] 5.3 Перед коммитом итогового implementation snapshot выполнить `make ci-check-once CHECK_CONTEXT=<actual-profile>`. Проверка: полный batch либо допустимый reuse прошёл; контекст и результат записаны точно.
- [x] 5.4 Проверить protected history через `git diff --name-status <base> -- openspec/changes/archive schemas` и отдельный просмотр release evidence. Проверка: старые evidence и bundles не менялись; добавлены только необходимые новые contracts.
- [x] 5.5 Подготовить handoff с результатами source tests и границами acceptance. Проверка: отчёт не объявляет выпущенный Core, private authority acceptance или roadmap gate без отдельного evidence.

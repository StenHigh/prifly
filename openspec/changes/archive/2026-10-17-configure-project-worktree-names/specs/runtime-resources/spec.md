## ADDED Requirements

### Requirement: Читаемое имя выделяется вместе с durable claim reservation

Runtime SHALL сохранять служебный `claim:<64 hex>` независимо от читаемого имени. Новый независимый claim с настроенной основой SHALL получать первое свободное имя в последовательности `base`, `base-02`, `base-03`, …, ограниченной опубликованным пределом. Кандидат SHALL считаться занятым при конфликте с reserved/active/releasing claim, существующим объектом каталога (включая symlink) или Git branch ref и его несовместимым ancestor/descendant ref. Путь проверяется во всём authority ClaimRoot, ref — в common Git repository; учитываются также ветки без worktree.

Проверка доступности, reservation и создание дерева SHALL составлять согласованный протокол: параллельные Pri-Fly callers MUST NOT получить одинаковый путь/ref. Durable `preparing` reservation SHALL предшествовать worktree effects. Внешняя гонка создания каталога/ref SHALL приводить к безопасному выбору следующего кандидата в рамках той же claim identity, либо явному отказу при невозможности доказать отсутствие собственных незавершённых effects. Чужие объекты MUST NOT удаляться/перезаписываться ради освобождения имени. Исчерпание namespace SHALL давать named refusal без Run/dispatch.

Transform команды MUST оставаться чистой функцией snapshot и payload: чтение files/blobs, Git и операции создания/очистки SHALL выполняться вне write-транзакции. Snapshot reservation, generation и command receipt SHALL защищать от stale наблюдений, duplicate creation и удаления чужого ресурса. Имя SHALL выбираться детерминированно из проверенного состояния, без скрытого случайного suffix.

#### Scenario: Три независимых запуска одной задачи
- **WHEN** `base` и `base-02` заняты claims либо оставшимися Git refs, а третий запуск создаёт новое дерево
- **THEN** новый claim сохраняет `base-03` и `prifly/base-03`, не затрагивая первых владельцев

#### Scenario: Конкурентные старты и общий корень
- **WHEN** два разных command ID одновременно запрашивают одинаковую основу, в том числе из разных repositories одной authority
- **THEN** оба успешных claim получают разные фактические каталоги; для одного repository различаются и refs, чужие деревья остаются неизменными

#### Scenario: Внешняя ветка появилась после наблюдения
- **WHEN** другой Git actor занял выбранный ref после preflight availability read
- **THEN** Pri-Fly не перезаписывает ref, безопасно учитывает коллизию под той же claim identity либо оставляет диагностируемый preparing claim при unknown effects

#### Scenario: Процесс прервался при подготовке
- **WHEN** reservation записана, но создание дерева/подтверждение identity не закончено
- **THEN** остаётся обнаруживаемый preparing claim с записанным выбранным именем; повтор не выдаёт это имя другому claim, неизвестная собственность не трактуется как разрешение cleanup

### Requirement: Идемпотентность и lifecycle используют сохранённые имена

Повтор того же semantic command payload SHALL возвращать прежний claim/result либо его явно незавершённое состояние, без нового suffix, дерева или Run. Изменённые naming data под тем же command ID MUST давать обычный idempotency conflict. Receipt MUST связывать команду с claim identity независимо от allocator retries и меняющейся доступности имён.

Admission, handoff, diagnostics, continue/recover и release SHALL брать path/branch из claim. Передача существующего claim SHALL сохранять path/branch при штатном повышении generation. Изменение профиля или входов именования MUST NOT переименовывать дерево либо препятствовать допустимой передаче из-за повторного вычисления шаблона. Продолжение, которому по прежнему контракту нужен новый claim, SHALL выделять новое имя; recover с освобождённым claim SHALL сохранять существующий отказ.

Release SHALL удалять только доказанно принадлежащие claim дерево/ref с existing ownership/generation fences; ручной rename, перемещение и редактирование authority MUST NOT требоваться для штатного случая. Без настройки и для legacy claims SHALL сохраняться прежние identity, пути, branch и admission semantics. Настройка MUST NOT вводить repository-wide exclusion для независимых worktrees.

Текущий source set — `openspec/specs/runtime-resources/spec.md`; ownership не переносится, saved Run contracts не переинтерпретируются.

#### Scenario: Ответ start потерян
- **WHEN** caller повторяет команду, уже создавшую named claim и Run
- **THEN** он получает те же claim/path/branch/Run; свободный следующий suffix не расходуется

#### Scenario: Продолжение получает существующее дерево
- **WHEN** continue/recover передаёт claim исходного Run после изменения naming template
- **THEN** передаются прежние path/branch и файлы с новой generation, новое имя не выделяется

#### Scenario: Штатная очистка и повторное использование
- **WHEN** освобождается допустимый named claim с доказанной identity
- **THEN** cleanup работает по сохранённым path/branch; имя вновь доступно только если claim больше не резервирует его и каталог/ref действительно отсутствуют

#### Scenario: Исторический claim читается новым binary
- **WHEN** claim не содержит naming metadata и использует прежний hash path/branch
- **THEN** чтение, admission, передача и release сохраняют прежнее поведение без автоматического rename

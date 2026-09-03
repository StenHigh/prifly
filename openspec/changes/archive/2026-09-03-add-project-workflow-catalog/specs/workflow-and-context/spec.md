## ADDED Requirements

### Requirement: Workflow repository содержит discoverable Project workflow folders
Workflow repository — любой Git-репозиторий, доступный пользователю, который
содержит одну или несколько Project workflow folders. Pri-Fly MUST находить
такие папки только по root `workflow.yaml` с marker
`authoring: prifly-project-workflow/1`, не спускаясь внутрь найденной папки,
не следуя symlinks и с ограниченной глубиной обхода. Repository MUST NOT
требовать отдельный manifest или registration file: `examples/` самого
Pri-Fly является таким repository без изменений. Если найдена ровно одна
папка, установка MAY обойтись без явного пути; при нескольких папках без
явного `--path` и при отсутствии папок команда MUST отказать с перечнем
найденных путей и без записи в repository проекта. Явный `--path` MUST
указывать на папку с marker; иначе — отказ.

#### Scenario: Repository содержит один сценарий
- **WHEN** пользователь устанавливает repository, в котором найдена одна
  Project workflow folder
- **THEN** именно она копируется без дополнительного выбора

#### Scenario: Repository содержит несколько сценариев
- **WHEN** пользователь устанавливает repository с несколькими папками и не
  задал `--path`
- **THEN** Pri-Fly отказывает, перечисляет найденные пути и не меняет
  `.prifly/`

#### Scenario: Путь указывает на папку без marker
- **WHEN** `--path` называет каталог, чей `workflow.yaml` отсутствует или не
  имеет marker `prifly-project-workflow/1`
- **THEN** установка отказывает до копирования и регистрации

### Requirement: Установка workflow folder разделяет obtain, validate и register
`prifly project workflows add` MUST получить exact commit по запрошенному ref
(tag, branch или commit; по умолчанию remote HEAD), скопировать из выбранной
папки только regular files и структурно проверить копию тем же reader
Project workflow folder, что использует compile, прежде чем атомарно
поместить её в `.prifly/workflows/NAME/` и объявить `packages.NAME` и
`launches.NAME` в tracked `project.yaml`. Symlink, вложенный Git repository
или gitlink, превышение лимитов количества и размера файлов MUST быть
отказом без частичной папки. Имя берётся из `--name` или basename папки и
MUST удовлетворять правилам launch ID; занятое имя папки, package или launch
и другой declared package с тем же `package.id` MUST быть отказом. Установка
MUST NOT seal-ить, импортировать или доверять package, компилировать его
против host, создавать Run или Git commit; ни один полученный файл не
исполняется. Результат MUST называть identity package, declared external
references и записанный origin, а также оставшиеся шаги владельца: review,
commit `.prifly` и обычный `project compile`.

#### Scenario: Сценарий установлен из репозитория
- **WHEN** пользователь выполняет `add` с валидным repository и ref
- **THEN** появляется `.prifly/workflows/NAME/` с declared package и launch, а
  authority, sealed packages и Runs не меняются

#### Scenario: Имя уже занято
- **WHEN** `.prifly/workflows/NAME`, `packages.NAME` или `launches.NAME` уже
  существуют
- **THEN** установка отказывает и не перезаписывает существующую папку или
  запись

#### Scenario: Папка содержит symlink
- **WHEN** выбранная папка в repository содержит symlink или вложенный Git
  repository
- **THEN** установка отказывает до записи в `.prifly/`

### Requirement: Workflow folder origin закрепляет exact commit и digest
Для установленной папки tracked `project.yaml` MUST хранить
`packages.NAME.origin` со строгими полями: `repository` без userinfo, `path`
внутри repository, `ref`, `commit` из 40 hex, `digest` дерева папки без
корневого `extend.yaml`, необязательные `extend_digest` для upstream
`extend.yaml` и `catalog` для установки по имени каталога. Origin — заявление
установившей команды, проверяемое локальным digest; это не TrustDecision и не
sealed `PackageOrigin`. Profile без `origin` остаётся допустимым
`prifly-project-profile/2`; неизвестное поле или невалидное значение внутри
`origin` MUST быть отказом чтения профиля. Написанная вручную папка без
origin не подлежит `update`.

#### Scenario: Профиль без origin читается как прежде
- **WHEN** `project.yaml` объявляет package только с `source`
- **THEN** `project workflows`, `compile` и `start` работают без изменений

#### Scenario: Origin содержит неизвестное поле
- **WHEN** `packages.NAME.origin` содержит поле вне закрытого списка или
  `commit` не из 40 hex
- **THEN** чтение профиля отказывает с понятной diagnostic

### Requirement: Update сохраняет exact identity и правки команды
`prifly project workflows update NAME` MUST требовать записанный origin,
пересчитать digest текущей папки без `extend.yaml` и при расхождении
отказать с перечнем изменённых путей, ничего не перезаписывая. Если удалённый
commit для ref не изменился и digest совпадает, команда MUST завершиться
успешным read-only результатом. Иначе она MUST получить новую папку по тому
же `path`, проверить её как при установке, перенести локальный `extend.yaml`
byte-for-byte, атомарно заменить папку и обновить `origin`. Результат MUST
сообщать, изменился ли upstream `extend.yaml` и остался ли `package.version`
прежним. Sealed packages, locks, Runs и evidence в authority MUST NOT
меняться; конфликт exact identity при следующем запуске остаётся честным
отказом, а не тихой заменой.

#### Scenario: Папка изменена локально
- **WHEN** digest установленной папки без `extend.yaml` отличается от origin
- **THEN** `update` отказывает, перечисляет изменённые пути и не трогает файлы

#### Scenario: Удалённый commit не изменился
- **WHEN** ref указывает на тот же commit, а digest совпадает
- **THEN** `update` сообщает актуальность и ничего не записывает

#### Scenario: Upstream не поднял версию package
- **WHEN** новая папка отличается по bytes, но `package.version` тот же
- **THEN** `update` применяет папку и явно называет, что следующий запуск
  столкнётся с уже sealed package той же identity

### Requirement: Remove убирает folder из tracked profile, а не из authority
`prifly project workflows remove NAME` MUST удалить `.prifly/workflows/NAME`,
запись `packages.NAME` и каждый launch, чей `workflow` лежит в этой папке.
Команда MUST NOT изменять sealed packages, Runs, receipts или evidence в
authority: их история сохраняется по общим правилам uninstall.

#### Scenario: Установленный сценарий удалён
- **WHEN** пользователь выполняет `remove` для объявленного package
- **THEN** папка и её launches исчезают из tracked profile, а authority
  остаётся прежней

### Requirement: Workflow catalog служит только discovery
Workflow catalog — Git-репозиторий с root `catalog.yaml`
`prifly-workflow-catalog/1`: карта `categories` и карта `workflows`, где
каждая запись называет `title`, `description`, `category`, `repository`,
`path`, необязательные `ref`, `commit` и `tags`. Имена записей и категорий
MUST следовать правилам launch ID; неизвестное поле, неизвестная категория,
относительный `repository` или превышение лимитов MUST быть отказом.
`search` MUST возвращать детерминированный список с категориями и
фильтровать по подстроке и категории. `add NAME` MUST разрешить запись
каталога и далее следовать обычной установке из repository; если запись
объявляет `commit`, полученный commit MUST совпасть, иначе — отказ. Каталог
MUST NOT переносить bytes сценариев, ключи или trust; его чтение не делает
запись доверенной.

#### Scenario: Поиск по каталогу
- **WHEN** пользователь выполняет `search` с подстрокой или категорией
- **THEN** он получает отфильтрованный детерминированный список записей без
  изменения repository проекта

#### Scenario: Закреплённый commit не совпал
- **WHEN** запись каталога объявляет `commit`, а repository по `ref` даёт
  другой commit
- **THEN** установка отказывает до копирования

#### Scenario: Запись каталога неизвестна
- **WHEN** `add NAME` не находит запись в выбранном каталоге
- **THEN** команда отказывает и не пытается угадать repository

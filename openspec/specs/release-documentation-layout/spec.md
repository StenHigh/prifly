# release-documentation-layout Specification

## Purpose

Определяет состав repository перед первым релизом: OpenSpec хранит всю
нормативную документацию Pri-Fly, а короткий README остаётся единственной
внешней витриной и входом в эту спецификацию.

## Requirements

### Requirement: Нормативная спецификация перед релизом живёт только в OpenSpec
До первого release candidate все действующие нормативные capabilities Pri-Fly
MUST иметь source only в `openspec/specs/`. Для каждого source set карта
`openspec/SOURCE-OF-TRUTH.md` MUST указывать OpenSpec path и status
`Перенесено`. Release tree MUST не содержать `SPECIFICATION.md`, старые
`docs/spec/`, `docs/foundation/`, `docs/roadmap/`, `docs/decisions/`,
`docs/evidence/`, historical manifests или их независимые copies.

#### Scenario: Подготовлен первый release candidate
- **WHEN** repository готовится к первому release candidate
- **THEN** каждая нормативная capability читается из `openspec/specs/`, а
  legacy documentation source sets отсутствуют в release tree

### Requirement: Перенос сохраняет проверяемый смысл
Каждый migration change MUST перенести acceptance meaning, договорные версии и
ссылки на versioned JSON Schema в OpenSpec specifications. Постоянные OpenSpec
requirements используют понятные names и capability paths, а не legacy
documentation IDs. `design.md` migration change MUST содержать legacy coverage
crosswalk от прежнего правила к новому requirement/scenario; после archive он
остаётся частью OpenSpec history. Машинные JSON Schema остаются versioned
product artifacts рядом с кодом и не дублируются как Markdown.

#### Scenario: Переносится capability с требованиями и контрактом
- **WHEN** migration change переводит действующую capability
- **THEN** его archived design содержит проверенный crosswalk, OpenSpec spec
  содержит replacement requirements/scenarios и точные versioned contracts, а
  карта источников переключается только после проверки replacement

### Requirement: README остаётся product-входом, а не второй спецификацией
`README.md` MAY находиться вне OpenSpec как красиво оформленная GitHub-витрина:
назначение Pri-Fly, быстрый start, поддерживаемая установка и ссылки. Он MUST
не дублировать нормативные требования, roadmap, contract semantics или
evidence; для них README ведёт в OpenSpec.

#### Scenario: Новый пользователь открывает GitHub repository
- **WHEN** пользователь читает README
- **THEN** он быстро понимает назначение и запуск Pri-Fly и получает точную
  ссылку на OpenSpec без второй версии продуктовой спецификации

### Requirement: Custom documentation process удалён до релиза
После завершения migration release tree MUST не зависеть от custom document
builders, assembled-spec generators, manifests и document-only verification
reports. Структура и complete change history проверяются стандартными командами
OpenSpec; проверка Go runtime и JSON contracts остаётся соответствующей
проверкой продукта.

#### Scenario: Contributor меняет спецификацию после миграции
- **WHEN** contributor меняет нормативное правило
- **THEN** он использует OpenSpec change и standard OpenSpec validation, без
  запуска или обновления project-specific document builder

### Requirement: Legacy документация удаляется только после полного cutover
Удаление legacy documentation source set MUST происходить отдельным final
cleanup change только после successful `openspec validate --all --strict`,
проверки ссылок и подтверждения, что карта источников не содержит строки
`Не перенесено`. Удаление сохраняет историю через Git и MUST не сопровождаться
изменением product runtime или wire contracts.

#### Scenario: Выполняется финальная очистка перед релизом
- **WHEN** все capability перенесены и прошли их checks
- **THEN** final cleanup change удаляет legacy documents и custom document
  tooling без переписывания их исторического содержания

### Requirement: Эксплуатационный справочник покрывает публичную поверхность Pri-Fly

Репозиторий MUST содержать эксплуатационный справочник в `examples/operations/` с индексом задач, симптомов и команд.
Справочник MUST покрывать каждую публичную команду и поддержанный flag, каждый объявленный capability и каждый поставляемый host protocol.
Описание операции MUST показывать условия, форму вызова, входы, значимые поля ответа, эффекты, ограничения и следующий шаг.
Версионные варианты MUST называть минимальный контракт или capability и путь проверки установленного binary.
Каждый capability MUST вести к практическому описанию и существующему примеру авторинга, когда capability имеет поле YAML.
Возможность, объявленная без доступного публичного способа применения, MUST иметь явное описание этой границы.
Подробности авторинга MUST оставаться в существующих справочниках `examples/authoring/`, а нормативные правила — в OpenSpec.

#### Scenario: Агент знает задачу, но не знает имя поля

- **WHEN** агент ищет способ выполнить поддержанную операцию
- **THEN** индекс ведёт к процедуре с проверяемой командой и ожидаемым следующим действием без чтения исходников Core

#### Scenario: Добавлена публичная команда или capability

- **WHEN** изменение добавляет публичную операцию, flag или объявленную capability
- **THEN** то же изменение обновляет индекс и применимый справочник, а проверка покрытия обнаруживает пропущенную запись

### Requirement: Справочник доступен установленному runner без исходников Core

Установка и обновление runner MUST доставлять совместимый справочник всем поддержанным хостам рядом со skill.
Основной skill MUST ссылаться на короткий индекс и требовать чтения только применимых страниц.
Локальное чтение справочника MUST работать без сети и checkout репозитория Core.
Справочник MUST указывать версию своей поставки и различать установленную поддержку, unreleased реализацию и будущие возможности.
Установка MUST сохранять `PROJECT.md` и пользовательские файлы; конфликт изменённого поставляемого файла MUST завершаться диагностикой без перезаписи этого файла.
Read-only проверка обновления runner MUST учитывать недостающие и изменённые страницы, не меняя файлов.

#### Scenario: Пользователь работает в стороннем репозитории без сети

- **WHEN** установленный runner ищет процедуру исполнения уже существующего Run
- **THEN** он читает локальную страницу своей поставки, не скачивая исходники и не запуская проверку обновлений для нового Run

#### Scenario: Проект изменил свои правила

- **WHEN** обновляется известный runner со справочником и пользовательским `PROJECT.md`
- **THEN** обновление сохраняет bytes `PROJECT.md` и применяет существующее старшинство его правил

#### Scenario: Страница справочника отсутствует

- **WHEN** read-only проверка runner находит отсутствующую поставляемую страницу
- **THEN** она сообщает необходимость обновления, а следующий явный update восстанавливает страницу без замены пользовательских файлов

### Requirement: Справочник сохраняет единственный нормативный источник

Справочник MUST объяснять применение действующего поведения и ссылаться на его нормативный источник.
Он MUST NOT создавать вторую независимо редактируемую спецификацию, обещать реализацию roadmap или переопределять сохранённый Run.
Источниками остаются `release-documentation-layout`, `cli-protocol` и применимые main capabilities по `openspec/SOURCE-OF-TRUTH.md`.
Ownership остальных capabilities и historical evidence сохраняется.

#### Scenario: Справочник описывает новую authoring revision

- **WHEN** страница показывает поле новой ревизии
- **THEN** она называет минимальную поддержку, ссылается на versioned contract и не меняет смысл прежних Runs

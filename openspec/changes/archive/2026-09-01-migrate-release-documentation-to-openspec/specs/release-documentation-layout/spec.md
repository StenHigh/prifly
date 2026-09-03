## Purpose

Определяет состав repository перед первым релизом: OpenSpec хранит всю
нормативную документацию Pri-Fly, а короткий README остаётся единственной
внешней витриной и входом в эту спецификацию.

## ADDED Requirements

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

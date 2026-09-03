## Context

См. мотивацию в `proposal.md`. Сегодня спецификация размазана между девятью
главами, foundation, roadmap, ADR, CSV/JSON maps, assembled document и
historical evidence. OpenSpec уже установлен, но пока содержит только правила
governance; current product sources остаются legacy.

## Goals / Non-Goals

**Goals:**

- Довести repository до OpenSpec-only нормативной документации к первому
  релизу.
- Переносить смысл и traceability малыми capability changes, а не одним
  неотзывчивым копированием.
- Оставить README красивым и полезным, но не нормативным.

**Non-Goals:**

- Не менять runtime, пользовательский YAML, package format или wire contracts.
- Не создавать custom OpenSpec schema, второй document generator или новую
  documentation platform.
- Не удалять legacy files до того, как OpenSpec replacement проверен.

## Decisions

### Использовать только стандартные OpenSpec specifications

Нормативные source files живут в `openspec/specs/<capability>/spec.md` и
меняются штатными proposal/specs/design/tasks/archiving workflow. Новых
project-specific formats, Markdown assemblers или schema extensions нет.

Планируемые capability paths отражают нынешние source sets:

```text
product-model                 ← docs/spec/01-product.md
domain-execution              ← docs/spec/02-domain-execution.md
workflow-and-context          ← docs/spec/03-* + workflow-yaml.md
runtime-resources             ← docs/spec/04-runtime-resources.md
control-security-ux           ← docs/spec/05-control-security-ux.md
cli-protocol                  ← docs/spec/06-cli-protocol.md
quality-and-acceptance        ← docs/spec/07-* + 09-* + acceptance maps
architecture-decisions        ← docs/spec/08-* + docs/decisions/
foundation-profile            ← docs/foundation/
delivery-roadmap              ← docs/roadmap/ + f2/rc/release status
published-contracts           ← contract indexes; JSON Schema remains code artifact
```

Постоянные OpenSpec requirements получают понятные names: их identity —
capability path и heading, а не `PROD-001`-подобный legacy ID. В каждом
migration change `design.md` содержит legacy coverage crosswalk от старого
правила к новому requirement/scenario. После archive crosswalk остаётся
проверяемой историей OpenSpec, но не становится частью постоянного spec.
Реальные public identifiers — JSON Schema `$id`, command names и versioned
contract refs — сохраняются. OpenSpec standard validation заменяет управление
формой документа; relevant Go/schema tests всё ещё доказывают исполняемые
контракты.

### Выполнять migration независимыми capability changes

Этот change задаёт release layout и порядок. Каждая строка выше переносится
следующим отдельным OpenSpec change: source inventory → requirements/scenarios
→ links/contracts → source-map cutover → validation. Так review остаётся
понятным и прежний source можно безопасно сравнить с replacement.

Альтернатива — перенести весь `docs/` одним change — отвергнута: diff будет
слишком велик, а ошибка в одном ID лишит нас проверяемой миграции.

### Legacy coverage crosswalk остаётся в архиве migration change

Каждый focused migration change добавляет в собственный `design.md` таблицу
legacy coverage crosswalk до изменения source map. В ней ровно по одной строке
на legacy requirement, case либо иной проверяемый rule source:

| Legacy source | Legacy record | Связанные acceptance cases/contracts | Replacement в OpenSpec | Review |
|---|---|---|---|---|

`Legacy source` содержит exact path и heading/line range; `Legacy record`
содержит прежний ID только здесь; `Replacement` содержит final capability path
и понятный heading requirement/scenario, а не новый числовой ID. Для source,
у которого не было ID, `Legacy record` даёт устойчивый heading либо exact
contract identifier. `Review` становится `verified` только после того, как
reviewer подтвердил один-к-одному смысл, acceptance meaning и точные
versioned-contract links. До этого source map не переключается.

После archive эта таблица остаётся в истории OpenSpec change. В постоянный
`openspec/specs/` она не копируется: там остаются только понятные headings и
применимые requirements/scenarios. Так можно доказать, что прежнее правило не
потерялось, не сохраняя старую систему IDs как новый постоянный формат.

#### Проверка формата на главе `01-product`

До первого переноса `product-model` этот preflight inventory доказывает, что
формат вмещает все 20 требований главы и все их связанные acceptance cases.
Будущий change `migrate-product-model-to-openspec` заменит target headings
реальными requirements/scenarios и сменит `inventoried` на `verified`.

| Legacy source | Legacy record | Связанные acceptance cases | Planned replacement в OpenSpec | Review |
|---|---|---|---|---|
| `docs/spec/01-product.md#что-делает-pri-fly` | `PROD-001` | `AC-003` | `product-model` / «Pri-Fly управляет проверяемым выполнением» | inventoried |
| `docs/spec/01-product.md#универсальность` | `PROD-002` | `AC-010`, `AC-029` | `product-model` / «Новый сценарий не требует правки ядра» | inventoried |
| `docs/spec/01-product.md#пустая-установка` | `PROD-003` | `AC-001` | `product-model` / «Пустое ядро остаётся допустимой установкой» | inventoried |
| `docs/spec/01-product.md#границы-ответственности` | `PROD-004` | `AC-012`, `AC-147` | `product-model` / «Ответственность разделена между Core, package, adapter и owner» | inventoried |
| `docs/spec/01-product.md#что-не-входит-в-базовый-продукт` | `PROD-005` | `AC-012`, `AC-148` | `product-model` / «Базовый продукт не подменяет внешние системы» | inventoried |
| `docs/spec/01-product.md#пользовательские-роли` | `PROD-006` | `AC-006`, `AC-093`, `AC-145` | `product-model` / «Роли не выводятся из незащищённого текста» | inventoried |
| `docs/spec/01-product.md#матрица-применений` | `PROD-007` | `AC-011`, `AC-139` | `product-model` / «Одна модель выражает разные предметные сценарии» | inventoried |
| `docs/spec/01-product.md#цель-до-выполнения` | `PROD-008` | `AC-003`, `AC-132` | `product-model` / «Предмет и границы закреплены до выполнения» | inventoried |
| `docs/spec/01-product.md#изменение-намерения-пользователем` | `PROD-009` | `AC-004` | `product-model` / «Изменение намерения не продолжает прежний объём» | inventoried |
| `docs/spec/01-product.md#сценарий-и-план` | `PROD-010` | `AC-004`, `AC-005` | `product-model` / «План остаётся артефактом выбранного сценария» | inventoried |
| `docs/spec/01-product.md#режимы-взаимодействия-и-исполнения` | `PROD-011` | `AC-007`, `AC-009` | `product-model` / «Взаимодействие с человеком отделено от способа исполнения» | inventoried |
| `docs/spec/01-product.md#группы-и-параллельность` | `PROD-012` | `AC-042` | `product-model` / «Параллельная работа ограничена данными и ресурсами» | inventoried |
| `docs/spec/01-product.md#успех-и-доказательства` | `PROD-013` | `AC-137` | `product-model` / «Технический исход, артефакт, проверка и effect различаются» | inventoried |
| `docs/spec/01-product.md#проверки-соразмерны-задаче` | `PROD-014` | `AC-105` | `product-model` / «Проверки выбираются явно и не отменяют integrity guards» | inventoried |
| `docs/spec/01-product.md#остановка-продолжение-и-восстановление` | `PROD-015` | `AC-100` | `product-model` / «Остановка и восстановление не повторяют подтверждённые effects» | inventoried |
| `docs/spec/01-product.md#выбор-пакетов` | `PROD-016` | `AC-025`, `AC-031` | `product-model` / «Пакеты выбираются явно и проверяются до запуска» | inventoried |
| `docs/spec/01-product.md#контроль-стоимости-и-качества-процесса` | `PROD-017` | `AC-122`, `AC-127` | `product-model` / «Стоимость и качество процесса наблюдаемы честно» | inventoried |
| `docs/spec/01-product.md#предел-доказуемости` | `PROD-018` | `AC-016`, `AC-137`, `AC-144` | `product-model` / «Заявленная capability не равна qualification» | inventoried |
| `docs/spec/01-product.md#экспорт-и-независимость-владельца` | `PROD-019` | `AC-026`, `AC-129` | `product-model` / «Владелец сохраняет доступ к данным и obligations» | inventoried |
| `docs/spec/01-product.md#критерии-целостности-продукта` | `PROD-020` | `AC-010`, `AC-028`, `AC-142` | `product-model` / «Независимость продукта проверяется обязательными конфигурациями» | inventoried |

Проверка preflight не выдаёт перенос за завершённый: она подтверждает только
полноту inventory. Для неё reviewer сравнивает 20 legacy headings и 20 строк
таблицы, а затем сверяет case IDs с `docs/roadmap/requirements-map.csv`.

### Удалять legacy только одним финальным release cleanup

До полного cutover historical docs не переписываются. После всех completed
capability migrations final cleanup удаляет `SPECIFICATION.md`, legacy `docs/`,
`tools/docs/` и manifests одним reviewable change. История Git остаётся
неизменяемым архивом, но поставляемое дерево не содержит старую документацию.

### README — отдельный product home

README остаётся в root, потому что это ожидаемая точка входа GitHub. Его
содержание ограничено product story, quick start и precise links в OpenSpec;
требования, roadmap и compatibility rules не копируются. Оформление README
делается отдельным небольшим change после стабилизации путей OpenSpec.

## Risks / Trade-offs

- [Перенос потеряет правило или acceptance meaning] → каждый capability change
  получает legacy coverage crosswalk и focused traceability review до cutover.
- [README снова станет второй спецификацией] → его scope ограничен витриной и
  links; normative details живут только в `openspec/specs/`.
- [Раннее удаление разрушит ссылки] → final cleanup запускается только когда
  source map полностью migrated и link checks подтверждают отсутствие legacy
  references.
- [OpenSpec не заменяет продуктовые тесты] → Go и JSON Schema validation
  сохраняются; удаляется только custom process для хранения документации.

## Migration Plan

1. Принять этот release layout и обновить source map с planned capability paths.
2. Создать и архивировать focused OpenSpec changes для каждой capability group
   в указанном порядке; legacy source остаётся authoritative до её cutover.
3. После последнего cutover обновить README как GitHub product home.
4. Выполнить final release cleanup: удалить legacy docs/evidence/manifests и
   `tools/docs/`, затем проверить OpenSpec, links, Go tests и schema checks.
5. Выпустить первый release только с OpenSpec specifications и README pointers.

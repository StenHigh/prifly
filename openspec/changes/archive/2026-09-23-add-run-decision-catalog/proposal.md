## Why

Сейчас выбор состава `aif-classic` (Fast / Full / Ultra) записывается в
tracked `extend.yaml` во время compilation. Это делает разовый выбор
разработчика изменением общей конфигурации проекта и позволяет sealed Fast Run
получить от навыка ответ, рассчитанный на Full или Ultra. Вопросы навыков также
либо остаются в чате конкретного host, либо их приходится обходить неявным
default, поэтому пользователь не получает полного, проверяемого выбора для
Run и не может безопасно оставить Run без присмотра.

Нужен общий для любого package механизм объявленных решений Run: до запуска
владелец выбирает известные варианты, включая package profile, а неизвестное
решение сохраняет Run в понятном ожидании. Это сохраняет Core независимым от
AI Factory и не заставляет редактировать YAML ради каждой задачи.

## What Changes

- Добавить декларативный **каталог решений Run** в Project workflow package:
  package author перечисляет известные вопросы, допустимые ответы, область,
  recommendation/default и то, разрешён ли автоматический выбор. Компилятор
  проверяет и seal-ит только объявленные решения; он не извлекает вопросы из
  prose навыка и не создаёт stages или routes.
- Сделать выбор package profile значением конкретного запуска до compilation:
  `project start` принимает explicit profile, а интерактивный host запрашивает
  его, если он не передан. `extend.yaml` остаётся reviewed project default,
  а не способом менять Fast / Full / Ultra для одной задачи.
- Разрешить декларативную зависимость следующего вопроса от уже выбранного
  ответа (например, milestone появляется только после выбора roadmap
  linkage), сохраняя predicate в sealed catalog. Это не новый control-flow
  operator и не способ изменить graph.
- Ввести два честно разделённых режима: attended Run получает ответы владельца;
  autonomous Run использует только заранее разрешённые объявленные значения и
  сохраняет источник каждого выбора. Необъявленный, scope-changing или
  approval-like вопрос останавливает Run в ожидании, а не передаётся модели
  для догадки. Термин `unattended` будет употребляться только там, где ни один
  новый человеческий выбор не требуется.
- Добавить versioned typed `DecisionRequest` / `DecisionAnswer`, durable
  ожидание, журнал решений и универсальный мост решений между host и шагом.
  Он позволяет любому совместимому executor показать динамический вопрос,
  передать ответ ровно ожидающему шагу и продолжить Run после reconnect или
  restart без повторной опасной работы; это не отдельный adapter для AI
  Factory.
- Показать до старта сводку выбранных решений и в финальном отчёте — все
  ответы, их источник и автоматические рекомендации. Зафиксировать в
  `aif-classic` полную анкету известных вопросов AI Factory; адаптер
  сопоставляет её с upstream skills явно, не подменяя их произвольным prompt.
- Добавить active high-priority запись delivery roadmap и обновить словарь
  терминов. Изменение затрагивает runtime продукта, а не только процесс
  документации.

## Capabilities

### New Capabilities

- `run-decisions`: декларативные решения конкретного Run, их выбор,
  сохранение, ожидание и видимое происхождение ответа.

### Modified Capabilities

- `workflow-and-context`: Project YAML получает явное authoring-описание
  каталога решений и per-Run profile selection до sealing.
- `domain-execution`: запуск получает sealed решения и связывает ответ с
  ожидающим входом шага, не меняя scope неявно.
- `runtime-resources`: state и recovery сохраняют запрос, ответ и ожидание
  решения как durable состояние Run.
- `control-security-ux`: CLI и host показывают последствия решения, различают
  attended/autonomous поведение и не скрывают auto-choice.
- `cli-protocol`: `project start` и наблюдение Run получают typed выбор profile
  и команды/результаты для decision lifecycle.
- `specification-governance`: словарь закрепляет новые понятия без смешения их
  с Approval, Grant или DecisionArtifact.
- `delivery-roadmap`: единый backlog получает эту active high-priority работу
  и её реальные prerequisite.

## Impact

Нужно изменить Go CLI, compiler/sealer, persisted Run state и host handoff
protocol; добавить schema, fixtures и целевые recovery/UX проверки. Меняются
YAML authoring contract и `aif-classic` package, но Core не получает знания об
AI Factory, Claude или Codex. Все перечисленные capabilities уже имеют
перенесённый OpenSpec source set; ownership нормативных источников не меняется.

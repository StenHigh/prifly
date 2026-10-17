## ADDED Requirements

### Requirement: Project объявляет общий шаблон имени disposable worktree

Tracked Project execution profile `/4` SHALL поддерживать optional общий блок `worktree_naming` с `template` и `bindings`. Он применяется только при создании нового `worktree` claim. Профиль без блока SHALL сохранять прежние имена; `/2` и `/3` MUST NOT принимать новый блок или автоматически мигрировать. `/4` SHALL наследовать остальные возможности `/3`, включая command-only запуск без Git и host, queue и step/call extensions.

Template SHALL задавать один basename с placeholders `{name}`; каталог SHALL находиться непосредственно под authority ClaimRoot, ветка SHALL использовать фиксированный prefix `prifly/`. Отдельный root, произвольный ref template, shell expression, environment interpolation и трактовка текста задачи как программы MUST NOT поддерживаться.

Каждый использованный placeholder MUST иметь ровно одну явную binding: строковую `value` либо объявленный `input` выбранного launch с JSON `pointer`. Pointer SHALL использовать RFC 6901, включая пустую строку для всего JSON value. Источник MUST давать строку; missing, null, числа, массивы, объекты, неизвестные inputs и placeholders SHALL отклоняться до claim creation. Неиспользованные bindings и неизвестные поля SHALL отклоняться. Task source/ID SHALL передаваться объявленными входами, без неявного чтения queue metadata, tracker lookup или извлечения номера из brief.

Значения SHALL проходить строгую проверку безопасных компонентов без silent slugification или усечения. Итоговый basename и каждый suffix candidate MUST быть допустимыми компонентом пути и Git branch ref; нарушения SHALL давать предметную диагностику до worktree effects. Для `checkout`, запуска без Workspace и передачи существующего claim runtime MUST NOT требовать task naming inputs, отсутствующие у выбранного launch; структурная корректность профиля всё равно SHALL проверяться.

Текущий authoritative source set — `openspec/specs/workflow-and-context/spec.md`; ownership не переносится. Новый профиль и schema публикуются отдельно от существующих форм.

#### Scenario: Проект передаёт источник и номер задачи явно
- **WHEN** template равен `{project_name}-{task_source}-{task_id}`, bindings дают `smsplace`, `trucker`, `default-123` из константы и объявленного JSON input
- **THEN** основа каталога равна `smsplace-trucker-default-123`, основа ветки — `prifly/smsplace-trucker-default-123`

#### Scenario: Значение не подходит для имени
- **WHEN** binding отсутствует, даёт null или содержит `../other`, slash, backslash либо Git ref control syntax
- **THEN** prepare/start возвращает named refusal с binding/port/pointer без создания claim или дерева

#### Scenario: Launch не создаёт новое дерево
- **WHEN** выбран command-only launch, `checkout` или продолжение с передачей существующего claim
- **THEN** именование нового дерева не вычисляется, а shared блок не добавляет этому launch фиктивные inputs или Git requirement

#### Scenario: Старый профиль содержит новый блок
- **WHEN** профиль `/2` или `/3` объявляет `worktree_naming`
- **THEN** parser отказывает и объясняет необходимость явного перехода на `/4`, сохраняя прежний контракт

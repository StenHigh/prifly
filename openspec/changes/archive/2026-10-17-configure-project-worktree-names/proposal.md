## Why

[Issue #10](https://github.com/StenHigh/prifly/issues/10): имена `prifly/<64 hex>` и `.prifly/work/claims/<64 hex>` скрывают назначение рабочих копий. Проекту нужна общая настройка читаемых имён с явными данными задачи, сохранением служебного claim ID и безопасной параллельной работой.

## What Changes

- Добавить opt-in `worktree_naming` в tracked `.prifly/project.yaml` нового профиля `/4`: один шаблон basename для каталога и ветки, привязки placeholder к строковым константам либо JSON Pointer объявленных входов выбранного launch.
- До создания рабочей копии проверять значения и показывать в prepare разрешённую основу имени, текущий свободный кандидат и правило окончательного выделения. Prepare не резервирует имя; start может выбрать следующий свободный суффикс в том же проверенном пространстве имён.
- Выделять первое свободное имя `base`, `base-02`, `base-03`, … с учётом claims, файловой системы и Git refs; согласовать reservation, идемпотентность, прерывание и cleanup с durable созданием claim, сохраняя чистоту transform.
- Сохранять выбранные path/branch в claim. Передача существующего claim при continue/recover не меняет имя; самостоятельный новый claim использует текущую настройку.
- Выпустить новые closed schemas для новых authoring/review/claim форм; старые профили, claims, Runs и опубликованные bundles сохранить. Без настройки оставить прежнюю генерацию.
- Показать возможность в `prifly capabilities`, таблице `examples/README.md`, авторском справочнике и CLI диагностике.

## Capabilities

### New Capabilities

Нет: возможность расширяет существующие Project profile и WorktreeClaim.

### Modified Capabilities

- `workflow-and-context`: декларация шаблона и явных источников значений в общем профиле проекта.
- `runtime-resources`: конкурентное выделение имён, durable reservation, идемпотентность и lifecycle сохранённых path/branch.
- `cli-protocol`: read-only prepare, digest, start/continuation и предметные отказы.
- `published-contracts`: версионирование authoring, launch summary и claim records/views без изменения прошлых bundles.
- `specification-governance`: уточнение существующих понятий Project execution profile, Launch summary и WorktreeClaim; отдельная сущность задачи не вводится.

## Impact

Меняется продуктовый runtime и Project CLI; это не изменение только процесса разработки. Текущие source sets — одноимённые `openspec/specs/<capability>/spec.md` и словарь `specification-governance/terms.md`; ownership не переносится. Нормы готовятся здесь как deltas, main specs синхронизируются при завершении change.

Затрагиваются `cmd/prifly/project*.go`, `internal/runtime/worktrees.go` и читатели claims/continuation, `internal/projectwire`, schema generation и `schemas/authoring`, capability inventory, примеры и проверки. Прямой `claim` CLI без проектной настройки остаётся прежним. Новые зависимости, tracker adapters, изменение upstream workflows, автоматическое переименование, произвольные branch/root templates и распределённые claims не требуются.

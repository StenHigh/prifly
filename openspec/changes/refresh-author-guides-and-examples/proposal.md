## Why

Возможности Pri-Fly описаны в отдельных справочниках и change-отчётах, но автору
сценария трудно пройти путь от выбора возможности до работающего workflow.
Индекс уже отстаёт: continuation-guide назван руководством по ревизии 7,
хотя содержит `resumable` ревизии 8; готовые учебные пакеты ограничены CSV
и blocked-condition.

## What Changes

- Сверить таблицу возможностей, описания справочников и ссылки README с
  актуальными контрактами; указать минимальные версии там, где они нужны.
- Добавить практическое руководство по внедрению выбора launch в свой проект:
  `title`/`description`, необязательные критерии `PROJECT.md`, отключение совета,
  явный выбор и доступные нативные вопросы. Новых полей схемы нет.
- Показать прогресс и opt-in live output на существующем программном примере,
  добавить небольшой исполняемый пример возобновления тем же workflow.
- Объяснить выбор между продолжением, возобновлением и восстановлением на
  существующих командах и контрактах, включая границы безопасного повтора.
- Учесть готовые тексты из `reuse-unchanged-verification-results` и
  `fix-nested-preparation-recovery` после их интеграции владельцем, сохранив
  различие stable, development и отложенной приёмки.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

Нет. Это документация и учебные примеры существующего поведения;
`skip_specs: true`. Нормативная ownership не меняется. Источники требований:
`release-documentation-layout`, `workflow-and-context`, `domain-execution`,
`cli-protocol`, `control-security-ux` и `release-distribution` в
`openspec/specs/`; карта — `openspec/SOURCE-OF-TRUTH.md`.

## Impact

Затрагиваются `README.md`, `examples/README.md`, тематические материалы
`examples/authoring/`, учебные пакеты `examples/workflows/` и при необходимости
существующие проверки примеров. Runtime, CLI API, схемы, Go baseline и семантика
закреплённых Runs не меняются. Основной путь работает без ИИ и Git; инструкции
хоста и Git workspace нужны только применимым workflows.

Новая иерархия `docs/`, документационный сборщик и массовое переписывание
справочников не нужны. Historical release evidence и архивы не редактируются.
Claude UI остаётся DEFERRED в `verify-claude-launch-choice-ui`: этот change
не запускает ту приёмку и не объявляет её пройденной.

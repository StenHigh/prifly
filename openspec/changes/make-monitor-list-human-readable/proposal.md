## Why

В общей таблице монитор показывает технические `run:` и `project:` как главные подписи. У сохранённого Run уже может быть проверяемый заголовок входа задачи, а у источника — имя локального проекта; интерфейс должен вывести их на первый план, не выдавая догадки за записанные факты.

## What Changes

- В списке Run показывать сохранённый заголовок `task` как название задачи, когда отдельный RunBrief отсутствует.
- Добавить явный title в Project execution profile и закреплять его в новых Run; для старых или неполных Run показывать имя каталога authority как fallback. Технические identity, workflow и путь оставить вторичным текстом.
- Явно обозначать отсутствие сохранённого названия вместо генерации названия из произвольных данных.

## Capabilities

### New Capabilities

Нет.

### Modified Capabilities

- `local-run-monitor`: общий список Run получает проверяемые человекочитаемые названия задачи и проекта.

## Impact

- `internal/runtime/monitor.go`, `internal/runtime/model.go`, `internal/flow/protocol.schema.json`: новые Run закрепляют title Project execution profile без изменения старых контрактов.
- `cmd/prifly/project*.go`, `cmd/prifly/monitor*.go`: authoring title и его отображение с локальным fallback.
- Сохранённые Run и historical evidence не переписываются; опубликованные схемы версионируются совместимо.

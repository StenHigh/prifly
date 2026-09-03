## Why

Сегодня единственный способ получить готовый Project workflow folder — вручную
скопировать папку из чужого checkout в `.prifly/workflows/` и руками дописать
`project.yaml`. Команда не может указать Git-репозиторий с несколькими
сценариями, выбрать нужный по каталогу и позже обновить его до exact commit
с проверкой, что локальные правки не потеряются. Roadmap хранит эту идею как
post-P2 «Каталог workflow packages и внешние source adapters», хотя копирование
YAML-папок не требует action authority: ничего не исполняется, trust не
меняется, а `init` и запуск остаются offline.

## What Changes

- Добавить в `prifly project workflows` явные сетевые команды `search`, `add`,
  `update` и `remove`, работающие с Git-репозиториями сценариев через `git`
  binary: любой Git-хост, приватные репозитории через credentials пользователя,
  локальные bare-репозитории для offline-проверок.
- Определить **Workflow repository**: Git-репозиторий с одной или несколькими
  Project workflow folders, которые находятся по существующему marker
  `authoring: prifly-project-workflow/1`; отдельный manifest не вводится.
- Определить **Workflow catalog**: Git-репозиторий с одним
  `catalog.yaml` (`prifly-workflow-catalog/1`) — категории и указатели
  `repository + path + ref [+ commit]` на сценарии. Каталог служит только для
  discovery и не является trust root или источником bytes.
- Записывать **Workflow folder origin** в tracked `project.yaml`
  (`packages.NAME.origin`: repository, path, ref, exact commit, digest дерева,
  digest upstream `extend.yaml`). Поле необязательное, `prifly-project-profile/2`
  сохраняется.
- `update` отказывает при локальных правках установленной папки и всегда
  сохраняет командный `extend.yaml`; `remove` удаляет папку и её launches,
  не трогая authority.
- Дать host skill `prifly-run` раздел про поиск и установку сценария одним
  конечным вопросом; `project runners update` распознаёт прежний runner.
- Разделить запись roadmap: репозитории и каталог YAML workflow folders
  становятся active change; внешние source adapters задач остаются post-P2 с
  прежним prerequisite.

Изменение затрагивает product runtime только на стороне project-команд CLI и
authoring contract `project.yaml`; authority, sealed packages, Runs и trust
flow при `project start` не меняются.

## Capabilities

### New Capabilities

_Нет._

### Modified Capabilities

- `workflow-and-context`: Project workflow folder может быть получен из
  Workflow repository и каталога; tracked profile хранит origin; update и
  remove сохраняют exact identity и локальные настройки команды.
- `cli-protocol`: `project workflows` получает typed `search`, `add`, `update`,
  `remove` с сетью только во время явного вызова и stable diagnostics.
- `control-security-ux`: установка из репозитория ничего не исполняет, не
  меняет trust и не принимает credentials в tracked файлах; сеть только в
  явных командах.
- `specification-governance`: словарь получает канонические понятия Workflow
  repository, Workflow catalog и Workflow folder origin с Go-соответствиями.
- `delivery-roadmap`: единый backlog получает эту active работу и разделяет
  прежнюю post-P2 идею каталога.

## Impact

Изменяются `cmd/prifly` (новый файл project-команд, reader/writer
`project.yaml`, runner template, help), локальные authoring JSON Schema и
reference-примеры (`origin` в profile, новая `catalog.yaml` schema),
README и `examples/README.md`, e2e authoring corpus, словарь и OpenSpec.
Новых Go-зависимостей нет: используется уже обязательный для project-команд
`git` binary и `go.yaml.in/yaml/v3`. Sealed `PackageOrigin`, published JSON
contracts, authority state и ранее созданные Runs не меняются. Официальный
каталог — публичный репозиторий `https://github.com/StenHigh/prifly-workflows.git`;
его URL встраивается как default для `--catalog`.

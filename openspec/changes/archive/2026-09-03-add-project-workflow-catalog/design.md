## Context

См. `proposal.md` — Why. Уже существует всё, на что опирается change:

- Project workflow folder с marker `authoring: prifly-project-workflow/1` и
  строгий reader `readProjectWorkflowFolder` в `cmd/prifly/project_compile.go`;
  compile и seal происходят только при `project start`, а тот же `id@version`
  с другими bytes даёт `project_start_package_identity_conflict`.
- `project.yaml` (`prifly-project-profile/2`) со строгим reader
  (`cmd/prifly/project.go`) и JSON Schema для редактора; у package сейчас
  допускается только `source`.
- Образцы безопасной записи в repository: `project init` пишет через temp
  directory и `Mkdir` как commit point; `project runners update` заменяет
  только exact известные файлы и отказывает кастомизацию.
- `git` binary уже обязателен для project-команд (`git rev-parse
  --show-toplevel`, worktree claims) и вызывается через helper с
  sanitized env в `internal/runtime/worktrees.go`.
- Единственный сетевой код — `prifly update`: explicit command, ограниченный
  fetch, atomic replacement.
- В словаре занято слово **Registry** (`flow.Registry`), а **SourceSnapshot**
  и **Task source adapter** относятся к задачам, не к сценариям.

## Goals / Non-Goals

**Goals:**

- Один код-путь obtain → validate → register для установки и обновления
  папки из любого Git-репозитория, проверяемый offline через локальные
  bare-репозитории.
- Exact identity установленного: commit и digest дерева в tracked
  `project.yaml`, чтобы `update` отличал upstream-изменения от локальных
  правок.
- Каталог как чистый discovery-слой поверх того же транспорта.
- Ни одного нового исполняемого пути: install не seal-ит, не импортирует и
  не компилирует против host.

**Non-Goals:**

- Раздача sealed packages или archives через каталог; `package import
  --archive` остаётся отдельным маршрутом.
- Разрешение внешних refs и проверка host skills при установке — это делает
  compile.
- Auto-update, фоновая сеть, кэш каталога, TUI, MCP, подпись каталога,
  `--force` для перезаписи локальных правок.
- Изменение sealed `PackageOrigin` или published JSON contracts: связь
  sealed package с git commit — отдельный follow-up про evidence.

## Decisions

### Транспорт — `git` binary, а не GitHub API

`git init` во временном каталоге вне repository → `git fetch --depth 1
--no-tags -- URL REF` (tag, branch, commit или `HEAD`) → `git rev-parse
FETCH_HEAD^{commit}` → `git checkout --detach FETCH_HEAD`. Если ref не задан,
`git ls-remote --symref -- URL HEAD` даёт имя default branch для `origin.ref`.
Один helper по образцу `Engine.git`: argv без shell, `--` перед URL, env
только `PATH`, `HOME`, `GIT_TERMINAL_PROMPT=0`, `GIT_OPTIONAL_LOCKS=0`,
`LC_ALL=C`, `GIT_ALLOW_PROTOCOL=https:ssh:file`; timeout 60 s для ls-remote и
10 min для fetch. Hooks не клонируются, submodules не инициализируются,
filters/config из repository не действуют.

Альтернатива — GitHub tarball/Contents API через `net/http`: без git, но
только GitHub, rate limits без токена, отдельная логика для GitLab и
self-hosted, приватные репозитории требуют собственной auth, а тесты — mock
сервера. Git уже обязателен для project-команд, поэтому новой зависимости
нет, приватные репозитории работают через credential helper/SSH
пользователя, а e2e проверяются через `file://` bare-репозитории. Чтение
дерева через `ls-tree`/`cat-file` без checkout отклонено как более сложное
без выигрыша для v1.

### Discovery по marker, без manifest репозитория

Обход checkout без `.git`, symlinks не следуем, глубина ≤ 6, внутрь найденной
папки не спускаемся. Один результат — установка, несколько — отказ со
списком, чтобы host показал выбор. Каталог всегда задаёт `path`, поэтому
fixtures и другие папки того же репозитория не мешают. Альтернатива —
`prifly-workflows.yaml` в корне репозитория — отклонена: второй формат,
который дублирует то, что уже говорит marker.

### Установка = staged copy в `.prifly/workflows/NAME/`

Копия собирается в temp-root `.prifly/workflows/.staging-XXXX/.prifly/workflows/NAME/`
внутри repository: `workflow.yaml` может ссылаться на собственные файлы
repository-relative путями (`decision_catalog` в `aif-classic` называет
`.prifly/workflows/aif-classic/decisions/...`), поэтому структурная проверка
`readProjectWorkflowFolder` выполняется под финальным именем папки, а
`--name`, отличное от имени папки в репозитории, честно отказывает для таких
папок. Копируются только regular files по `Lstat`, права 0644/0755, лимиты —
константы package import (10 000 файлов, 16 MiB файл, 64 MiB всего). После структурной проверки —
`os.Rename` в финальную папку как commit point, затем правка `project.yaml`
temp + rename; при ошибке правки папка удаляется, чтобы не осталось
незадекларированного folder. Название — `--name` или basename папки; отказ
при занятом имени и при другом declared package с тем же `package.id`, иначе
два folder столкнутся при seal.

### Origin живёт в `project.yaml`, а не в отдельном lock-файле

`packages.NAME.origin` — необязательный объект с закрытым списком полей
(`repository`, `path`, `ref`, `commit`, `digest`, `extend_digest`,
`catalog`). `schema_version` остаётся `/2`: authoring forms до release не
несут compatibility obligation, а старый binary честно отказывает `package …
requires source only`. Digest — sha256 над строками
`relative/path\0sha256(bytes)\n` в порядке путей без корневого
`extend.yaml`, чтобы командные `settings`/`exclude`/`profile` не считались
drift. Альтернатива — `.prifly/workflows.lock.json` — отклонена: вторая
запись про тот же package и второй файл для синхронизации.

Правка `project.yaml` — через `yaml.v3` Node API: добавляются ключи в
`packages` и `launches`, комментарии и порядок остальных узлов сохраняются;
`packages: {}` из init-template перерендерится block-стилем. Полная
перезапись через marshal отклонена, потому что стирает комментарии команды.

### Update отказывает при drift и сохраняет `extend.yaml`

Порядок: origin обязателен → drift по digest → дешёвая проверка
`git ls-remote` (тот же commit и digest — read-only success) → fetch и
проверка как при установке → перенос локального `extend.yaml` byte-for-byte →
atomic swap (staging → rename старой папки в `.previous-NAME-XXXX` → rename
staging → удаление previous) → обновление origin. `extend_upstream_changed`
сравнивает upstream `extend.yaml` с `origin.extend_digest`;
`package_version_unchanged` предупреждает о конфликте exact identity при
следующем `project start`, выход — `package remove --id --version` или bump
версии у автора. `--force` отклонён: команда, правившая файлы, владеет
папкой и может `remove` + `add` под другим именем.

### Каталог — один `catalog.yaml`, запись = один сценарий

Тот же транспорт: shallow fetch репозитория каталога, чтение root
`catalog.yaml`. Запись содержит `repository + path + ref [+ commit] + tags`,
категории — отдельная карта; парсер строгий, как все authoring-парсеры,
лимиты 1 MiB и 1000 записей. Альтернатива «запись = репозиторий, Pri-Fly
сканирует его при search» отклонена: N клонов на каждый поиск. Локальная
JSON Schema `workflow-catalog-v1` и reference-пример добавляются в editor
contract, как у остальных документов. Default catalog — константа
`https://github.com/StenHigh/prifly-workflows.git` в binary (публичный
репозиторий владельца, уже содержит `catalog.yaml` с `aif-classic` и
`aif-fanout` на tag `v0.4.0`); явный `--catalog URL` переопределяет её для
одной команды.

### Команды под `project workflows`, выбор делает host

`prifly project workflows add|update|remove|search`; bare `project workflows`
остаётся списком. Top-level `prifly workflow` отклонён: команды работают с
tracked profile, а не с authority. Интерактивный выбор — раздел в runner
`prifly-run` (`search --json` → один native вопрос → `add NAME` → предложить
commit); это новая версия template с добавлением текущей в accepted-previous
для `project runners update`.

### Диагностика и безопасность

Коды `project_workflow_*` из cli-protocol delta; git stderr попадает в
сообщение с вырезанным userinfo. URL с credentials, относительный путь,
ведущий `-` и `ext::` отклоняются до сети. Установка ничего не исполняет и
не меняет trust: единственное trust decision остаётся в `project start`.

## Risks / Trade-offs

- [Upstream изменил bytes без bump `package.version`] → `update` явно
  сообщает `package_version_unchanged`; следующий запуск даёт существующий
  честный отказ, документация каталога требует bump в `workflow.yaml`.
- [`aif-classic` в `examples/` расходится с released AI Factory skills] →
  записи каталога указывают на tag, а не `main`; разрыв остаётся отдельной
  записью backlog.
- [`yaml.v3` меняет стиль при правке] → тест на init-template и на файле с
  комментариями; изменение стиля допустимо, потеря узлов — нет.
- [Shallow fetch по SHA требует `uploadpack.allowReachableSHA1InWant`] →
  GitHub и GitLab поддерживают; диагностика советует tag или branch.
- [Большой монорепозиторий клонируется целиком на глубину 1] → достаточно
  для v1; `--filter=blob:none` со sparse-checkout помечается `ponytail:`
  комментарием как upgrade path.
- [Два `add` одновременно] → `Rename` папки атомарен, `project.yaml`
  пишется temp + rename; окно между чтением и записью принимается для
  человеческой CLI и помечается `ponytail:` комментарием.
- [macOS `rename` не заменяет даже пустой каталог] → swap при `update`
  резервирует имя `.previous-NAME-XXXX` через MkdirTemp, удаляет
  placeholder и только затем переименовывает; параллельные `update` одной
  папки не поддерживаются.
- [Runner template растёт] → новый раздел добавляется в конец; прежний
  exact runner попадает в accepted-previous, как при decision bridge.

## Migration Plan

1. Contract: термины, `origin` в reader/schema/reference, `catalog.yaml`
   schema и reference, OpenSpec deltas.
2. Core: git helper, discovery, staged copy, digest, Node-правка, `add` из
   repository с тестами на bare-репозиториях.
3. Catalog: parser, `search`, `add NAME`, pin `commit`.
4. Lifecycle: `update`, `remove`.
5. Host UX и документация: runner template, `runners update`, README,
   `examples/README.md`, help.
6. Приёмка: e2e без сети, `make check`, `make e2e`,
   `openspec validate add-project-workflow-catalog --strict`.

Rollback: удалить новые команды и поле `origin`; установленные папки
остаются обычными Project workflow folders, а `project.yaml` без `origin`
читается прежним reader. Sealed packages и Runs не затрагиваются ни в одном
направлении.

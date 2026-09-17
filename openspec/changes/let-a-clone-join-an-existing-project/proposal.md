## Why

Первый заход на новой машине упирается в тупик, из которого инструмент не
выводит. Замер (сессия без памяти о Pri-Fly, Ubuntu, холодная установка,
воспроизведено здесь дословно): на клоне репозитория, чей профиль объявляет
три хоста, а раннеры двух из них в Git не лежат (их держат не все),
`project init` отказывает `project_runner_missing: existing project profile
requires .codex/skills/prifly-run/SKILL.md` — и `project runners add --host
codex-cli`, **та самая команда, которая этот файл создаёт**, отказывает тем
же сообщением. `safe_next_actions` называет только `help`. Выход, найденный
за двадцать пять минут перебора, — временно сузить tracked `project.yaml`,
выполнить `init` без `--host` и откатить правку; документированного пути нет.
`prifly init` для этого не годится: authority для project-launch обязана быть
создана `project init` (`authority_configuration_incompatible`).

Рядом два места того же захода, где инструмент знает ответ и не называет его:
рабочая копия захода ищется наугад (в ответе `project start`
`workspace.path` относителен authority, а читается как относительный
репозиторию: хост пошёл в `<repo>/.prifly/work/claims/…` и получил «No such
file or directory», путь нашёл через `git worktree list`), и README обещает,
что environment в итог не печатается, тогда как `project questionnaire
--prepare` печатает `execution[].environment` целиком — из-за этого секрет,
переданный `project local set --env`, уехал бы в терминал и журнал.

Изменение затрагивает product runtime и опубликованный read contract
(`project-launch-summary`), а также README; ownership источников не меняется.

## What Changes

- `project init` на клоне требует раннер только того хоста, который назван
  `--host`, и хостов, чьи раннеры уже лежат; отсутствие раннера другого
  объявленного хоста перестаёт быть отказом и называется в ответе, как это
  уже делает `project runners update` (`missing_hosts`).
- `project runners add --host NAME` перестаёт отказывать из-за отсутствия
  раннера **другого** хоста: он для того и зовётся, чтобы недостающий
  создать. Конфликт существующего файла остаётся отказом.
- Отказ, который всё же случается (например, раннер назван и присутствует, но
  изменён), называет выход в `safe_next_actions`, а не только `help`.
- Ответ `project start` и `session task` называют рабочую копию абсолютным
  путём (или помечают, относительно чего путь), чтобы хост не искал её
  перебором.
- Расхождение README и вывода закрывается: либо `--prepare` перестаёт
  печатать значения environment (оставляя имена), либо README перестаёт
  обещать обратное. Выбор — в design.

## Capabilities

### New Capabilities

<!-- нет -->

### Modified Capabilities

- `cli-protocol`: подключение клона к существующему профилю не требует чужих
  раннеров, отказ называет выход, а рабочая копия называется однозначно.
- `control-security-ux`: итог запуска не раскрывает значения environment
  программ (или документация перестаёт это обещать).

## Impact

- `cmd/prifly/project.go` (`checkExistingProjectRunners`, `existingProjectProfile`,
  `projectInit`, `projectAddRunners`), `cmd/prifly/project_start.go`
  (`workspace`, launch summary), `internal/runtime/sessions.go` (путь рабочей
  копии в задаче), `README.md`, `examples/troubleshooting.md`.
- Опубликованный `project-launch-summary` при изменении формы пути.
